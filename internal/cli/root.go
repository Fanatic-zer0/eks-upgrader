// Package cli implements the eks-upgrader Cobra command surface. Each command is a
// thin adapter over internal/app and internal/upgrade; no orchestration logic lives
// here.
package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var (
	flagLogLevel  string
	flagLogFormat string
	flagTimeout   time.Duration
)

// ExitError carries the buildplan's stable CLI exit-code contract:
// 0 success/ready, 1 warnings, 2 blocking, 3 internal/unexpected error.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

func warningExit(err error) error  { return &ExitError{Code: 1, Err: err} }
func blockingExit(err error) error { return &ExitError{Code: 2, Err: err} }
func internalExit(err error) error {
	if err == nil {
		return nil
	}
	return &ExitError{Code: 3, Err: err}
}

// ExitCode extracts the process exit code for err per the stable CLI contract.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *ExitError
	if as(err, &ee) {
		return ee.Code
	}
	return 3
}

func as(err error, target **ExitError) bool {
	for err != nil {
		if ee, ok := err.(*ExitError); ok {
			*target = ee
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

var rootCmd = &cobra.Command{
	Use:           "eks-upgrader",
	Short:         "Safely orchestrate EKS cluster upgrades via eksctl",
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagLogLevel, "log-level", "info", "log level: debug|info|warn|error")
	rootCmd.PersistentFlags().StringVar(&flagLogFormat, "log-format", "json", "log format: json|text")
	rootCmd.PersistentFlags().DurationVar(&flagTimeout, "timeout", 0, "overall command timeout (e.g. 90m); 0 means no timeout")

	rootCmd.AddCommand(
		newValidateCmd(),
		newStateCmd(),
		newReadinessCmd(),
		newUpgradeCmd(),
		newResumeCmd(),
		newStatusCmd(),
		newAbortCmd(),
		newPauseCmd(),
		newApproveCmd(),
		newLockCmd(),
		newReportCmd(),
		newVersionCmd(),
	)
}

// Execute runs the CLI; the returned error's ExitCode() should be used as the process
// exit status.
func Execute() error {
	return rootCmd.Execute()
}

// commandContext builds a context that is cancelled on SIGINT/SIGTERM (graceful
// interruption per the buildplan) and additionally bounded by --timeout when set.
func commandContext() (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	if flagTimeout <= 0 {
		return ctx, stop
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, flagTimeout)
	return timeoutCtx, func() { cancel(); stop() }
}
