package eksctl

import (
	"context"
	"log/slog"
	"time"

	"eks-upgrader/internal/audit"
	"eks-upgrader/internal/state"
)

// RunContext carries the identifiers every instrumented eksctl invocation must be
// tagged with for audit trail and state-store correlation.
type RunContext struct {
	RunID   string
	Cluster string
	Region  string
	Phase   state.Phase
}

// InstrumentedExecutor decorates an Executor with structured logging, audit events,
// and state-store persistence, and enforces a per-command timeout via context.
type InstrumentedExecutor struct {
	Inner  Executor
	Audit  audit.Logger
	Store  state.Store
	Logger *slog.Logger
}

// RunWithTimeout executes args with a bounded context timeout, recording an audit
// trail entry and a state-store record before and after execution.
func (e *InstrumentedExecutor) RunWithTimeout(ctx context.Context, rc RunContext, args []string, timeout time.Duration) (*CommandResult, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	e.Logger.Info("eksctl command started",
		"runID", rc.RunID, "cluster", rc.Cluster, "region", rc.Region, "phase", rc.Phase, "args", args)
	_ = e.Audit.Emit(audit.Event{
		RunID: rc.RunID, Cluster: rc.Cluster, Region: rc.Region,
		Type: audit.EventEksctlCommandStarted, Phase: string(rc.Phase),
		Message: "eksctl command started",
		Data:    map[string]interface{}{"args": args},
	})

	result, runErr := e.Inner.Run(cmdCtx, args)

	exitCode := 0
	stdout, stderr := "", ""
	var durationMS int64
	startedAt := time.Now().UTC()
	if result != nil {
		exitCode = result.ExitCode
		stdout = result.Stdout
		stderr = result.Stderr
		durationMS = result.Duration.Milliseconds()
		startedAt = result.StartedAt
	}

	if e.Store != nil {
		if err := e.Store.RecordEksctlCommand(ctx, state.EksctlCommandRecord{
			RunID: rc.RunID, Phase: rc.Phase, Args: args,
			ExitCode: exitCode, DurationMS: durationMS, StartedAt: startedAt,
			Stdout: stdout, Stderr: stderr,
		}); err != nil {
			e.Logger.Warn("failed to persist eksctl command record", "error", err)
		}
	}

	status := "SUCCEEDED"
	level := slog.LevelInfo
	if runErr != nil {
		status = "FAILED"
		level = slog.LevelError
	}
	e.Logger.Log(ctx, level, "eksctl command completed",
		"runID", rc.RunID, "phase", rc.Phase, "exitCode", exitCode, "durationMS", durationMS, "status", status)
	_ = e.Audit.Emit(audit.Event{
		RunID: rc.RunID, Cluster: rc.Cluster, Region: rc.Region,
		Type: audit.EventEksctlCommandCompleted, Phase: string(rc.Phase),
		Message: "eksctl command completed",
		Data: map[string]interface{}{
			"exitCode": exitCode, "durationMS": durationMS, "status": status,
		},
	})

	return result, runErr
}

func (e *InstrumentedExecutor) Version(ctx context.Context) (string, error) {
	return e.Inner.Version(ctx)
}
