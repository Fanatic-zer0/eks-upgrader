// Package eksctl wraps eksctl as a subprocess. eksctl is the only tool permitted to
// mutate EKS and CloudFormation resources; this package captures stdout/stderr, exit
// codes, durations, and enforces context timeouts for every invocation.
package eksctl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// CommandResult captures full provenance of a single eksctl invocation.
type CommandResult struct {
	Args      []string
	ExitCode  int
	Stdout    string
	Stderr    string
	Duration  time.Duration
	StartedAt time.Time
}

// Executor runs eksctl subprocesses. It is the sole seam through which this
// application mutates EKS/CloudFormation, making it the primary mock point for tests.
type Executor interface {
	Run(ctx context.Context, args []string) (*CommandResult, error)
	Version(ctx context.Context) (string, error)
}

// SubprocessExecutor shells out to a real eksctl binary.
type SubprocessExecutor struct {
	// BinaryPath defaults to "eksctl" (resolved via $PATH) when empty.
	BinaryPath string
}

func (e *SubprocessExecutor) binary() string {
	if e.BinaryPath == "" {
		return "eksctl"
	}
	return e.BinaryPath
}

func (e *SubprocessExecutor) Run(ctx context.Context, args []string) (*CommandResult, error) {
	bin := e.binary()
	start := time.Now()

	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	duration := time.Since(start)

	result := &CommandResult{
		Args:      append([]string{bin}, args...),
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		Duration:  duration,
		StartedAt: start,
	}

	if ctx.Err() == context.DeadlineExceeded {
		result.ExitCode = -1
		return result, fmt.Errorf("eksctl command timed out after %s: %s %s", duration, bin, strings.Join(args, " "))
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, fmt.Errorf("eksctl command %s %s exited with code %d: %s", bin, strings.Join(args, " "), result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	if runErr != nil {
		return result, fmt.Errorf("executing eksctl command %s %s: %w", bin, strings.Join(args, " "), runErr)
	}

	result.ExitCode = 0
	return result, nil
}

// Version returns the raw output of `eksctl version`.
func (e *SubprocessExecutor) Version(ctx context.Context) (string, error) {
	result, err := e.Run(ctx, []string{"version"})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Stdout), nil
}
