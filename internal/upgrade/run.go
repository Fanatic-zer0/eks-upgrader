package upgrade

import (
	"context"
	"errors"
	"fmt"
	"time"

	"eks-upgrader/internal/audit"
	"eks-upgrader/internal/notify"
	"eks-upgrader/internal/state"
)

// Run starts a brand-new upgrade run. dryRun mode performs no mutation: no lock, no
// state writes, only a plan report (see dryrun.go).
func (e *Engine) Run(ctx context.Context, dryRun bool) error {
	if dryRun {
		_, err := e.DryRunReport(ctx)
		return err
	}

	rc := e.runID()
	existing, err := e.Store.GetRun(ctx, rc)
	if err != nil {
		return fmt.Errorf("checking for existing run: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("run %q already exists (status=%s); use resume to continue or set a different runID", rc, existing.Status)
	}

	run := &state.Run{
		RunID: rc, Cluster: e.Plan.Metadata.Cluster, Region: e.Plan.Metadata.Region,
		SourceVersion: e.Cluster.Metadata.Version, TargetVersion: e.Plan.Spec.TargetKubernetesVersion,
		CurrentPhase: state.PhaseInit, Status: state.StatusRunning,
	}
	if err := e.Store.CreateRun(ctx, run); err != nil {
		return fmt.Errorf("creating run record: %w", err)
	}

	_ = e.Audit.Emit(audit.Event{RunID: rc, Cluster: run.Cluster, Region: run.Region, Type: audit.EventRunStarted, Message: "run started"})
	e.notifyEvent(ctx, "RUN_STARTED", notify.SeverityInfo, "upgrade run started")

	return e.runLoop(ctx, state.PhaseInit)
}

// Resume continues a paused or interrupted run from its last persisted phase, after
// re-verifying live state and checking the resume circuit breaker.
func (e *Engine) Resume(ctx context.Context, force bool) error {
	rc := e.runID()
	run, err := e.Store.GetRun(ctx, rc)
	if err != nil {
		return fmt.Errorf("loading run: %w", err)
	}
	if run == nil {
		return fmt.Errorf("no run found with id %q", rc)
	}
	if run.Status != state.StatusPaused && run.Status != state.StatusInterrupted {
		return fmt.Errorf("run %q is not resumable from status %s", rc, run.Status)
	}

	attemptCount, err := e.Store.CountResumeAttempts(ctx, rc, run.CurrentPhase)
	if err != nil {
		return fmt.Errorf("counting resume attempts: %w", err)
	}
	if !force && attemptCount >= e.maxResumeAttempts() {
		return ErrResumeAttemptsExceeded
	}

	if _, err := e.Lock.Acquire(ctx, run.Cluster, rc, e.Owner, e.Plan.Spec.Execution.Lock.TTL.Duration); err != nil {
		return fmt.Errorf("%w: %v", ErrLockHeldByOther, err)
	}

	startPhase := run.CurrentPhase

	// Live-state re-verification: if the in-flight phase's mutating command actually
	// already succeeded out-of-band, skip straight to the next phase instead of
	// re-running it.
	if already, aerr := e.Deps.AlreadySucceeded(ctx, run.CurrentPhase); aerr == nil && already {
		_ = e.Store.CompletePhaseHistory(ctx, rc, run.CurrentPhase, "COMPLETED", "already succeeded (verified on resume)")
		startPhase = NextPhase(run.CurrentPhase)
	}

	if err := e.Store.UpdateRunStatus(ctx, rc, state.StatusRunning, ""); err != nil {
		return fmt.Errorf("marking run as running: %w", err)
	}
	_ = e.Audit.Emit(audit.Event{RunID: rc, Type: audit.EventResumed, Phase: string(run.CurrentPhase), Message: "run resumed"})
	e.notifyEvent(ctx, "RESUMED", notify.SeverityInfo, fmt.Sprintf("run resumed at phase %s", run.CurrentPhase))

	runErr := e.runLoop(ctx, startPhase)
	if runErr != nil && !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, context.DeadlineExceeded) {
		_ = e.Store.RecordResumeAttempt(ctx, state.ResumeAttempt{
			RunID: rc, Phase: run.CurrentPhase, Attempt: attemptCount + 1, FailedAt: time.Now().UTC(), Error: runErr.Error(),
		})
	}
	return runErr
}

// Pause marks a currently running run as paused at the operator's request. Because
// eks-upgrader is a CLI-first tool, an actively running `upgrade`/`resume` process
// checks for an externally-set PAUSED/ABORTED status between phases and stops
// gracefully rather than being preempted mid-phase.
func (e *Engine) Pause(ctx context.Context, reason string) error {
	rc := e.runID()
	run, err := e.Store.GetRun(ctx, rc)
	if err != nil {
		return fmt.Errorf("loading run: %w", err)
	}
	if run == nil {
		return fmt.Errorf("no run found with id %q", rc)
	}
	if run.Status != state.StatusRunning {
		return fmt.Errorf("run %q is not currently running (status=%s)", rc, run.Status)
	}
	if err := e.Store.UpdateRunStatus(ctx, rc, state.StatusPaused, reason); err != nil {
		return fmt.Errorf("marking run as paused: %w", err)
	}
	_ = e.Audit.Emit(audit.Event{RunID: rc, Type: audit.EventPaused, Message: reason})
	e.notifyEvent(ctx, "PAUSED", notify.SeverityWarning, "run paused: "+reason)
	return nil
}

// Abort releases the cluster lock, marks the run aborted, and notifies.
func (e *Engine) Abort(ctx context.Context, reason string) error {
	rc := e.runID()
	run, err := e.Store.GetRun(ctx, rc)
	if err != nil {
		return fmt.Errorf("loading run: %w", err)
	}
	if run == nil {
		return fmt.Errorf("no run found with id %q", rc)
	}
	if err := e.Store.UpdateRunStatus(ctx, rc, state.StatusAborted, reason); err != nil {
		return fmt.Errorf("marking run as aborted: %w", err)
	}
	if err := e.Lock.Release(ctx, run.Cluster, rc); err != nil {
		e.logger().Warn("failed to release lock on abort", "error", err)
	}
	_ = e.Audit.Emit(audit.Event{RunID: rc, Type: audit.EventAborted, Message: reason})
	e.notifyEvent(ctx, "ABORTED", notify.SeverityCritical, "run aborted: "+reason)
	return nil
}

// Approve records an approval event and allows a paused control-plane upgrade to
// proceed on the next `resume`. It does not itself resume the run.
func (e *Engine) Approve(ctx context.Context, approver, comment string) error {
	rc := e.runID()
	run, err := e.Store.GetRun(ctx, rc)
	if err != nil {
		return fmt.Errorf("loading run: %w", err)
	}
	if run == nil {
		return fmt.Errorf("no run found with id %q", rc)
	}
	if run.CurrentPhase != state.PhaseApproval {
		return fmt.Errorf("run %q is not currently awaiting approval (phase=%s)", rc, run.CurrentPhase)
	}

	token := fmt.Sprintf("%s-%d", rc, time.Now().UTC().UnixNano())
	if err := e.Store.RecordApproval(ctx, state.Approval{
		RunID: rc, Phase: state.PhaseApproval, Approver: approver, Comment: comment, Token: token, ApprovedAt: time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("recording approval: %w", err)
	}
	if err := e.Store.UpdateRunStatus(ctx, rc, state.StatusPaused, "approved; ready to resume"); err != nil {
		return fmt.Errorf("updating run status after approval: %w", err)
	}

	_ = e.Audit.Emit(audit.Event{RunID: rc, Type: audit.EventApprovalGranted, Phase: string(state.PhaseApproval), Message: fmt.Sprintf("approved by %s", approver)})
	e.notifyEvent(ctx, "APPROVAL_GRANTED", notify.SeverityInfo, fmt.Sprintf("approved by %s: %s", approver, comment))
	return nil
}

// runLoop drives phases from startPhase through PhaseComplete, checking for external
// pause/abort and context cancellation between each phase.
func (e *Engine) runLoop(ctx context.Context, startPhase state.Phase) error {
	rc := e.runID()
	phase := startPhase

	for phase != state.PhaseComplete {
		if current, err := e.Store.GetRun(ctx, rc); err == nil && current != nil {
			if current.Status == state.StatusPaused || current.Status == state.StatusAborted {
				return fmt.Errorf("run externally transitioned to %s (reason: %s)", current.Status,
					firstNonEmpty(current.PauseReason, current.AbortReason))
			}
		}

		select {
		case <-ctx.Done():
			return e.markInterrupted(ctx, phase, ctx.Err())
		default:
		}

		if err := e.runPhase(ctx, phase); err != nil {
			if ctx.Err() != nil {
				return e.markInterrupted(ctx, phase, ctx.Err())
			}
			return err
		}
		phase = NextPhase(phase)
	}

	return e.finish(ctx)
}

func (e *Engine) markInterrupted(ctx context.Context, phase state.Phase, cause error) error {
	rc := e.runID()
	_ = e.Store.CompletePhaseHistory(ctx, rc, phase, "INTERRUPTED", "interrupted: "+cause.Error())
	_ = e.Store.UpdateRunStatus(ctx, rc, state.StatusInterrupted, cause.Error())
	_ = e.Audit.Emit(audit.Event{RunID: rc, Type: audit.EventPaused, Phase: string(phase), Message: "run interrupted by signal/timeout"})
	e.notifyEvent(ctx, "INTERRUPTED", notify.SeverityWarning, fmt.Sprintf("run interrupted during phase %s; resumable", phase))
	return fmt.Errorf("run interrupted during phase %s: %w", phase, cause)
}

func (e *Engine) finish(ctx context.Context) error {
	rc := e.runID()
	if err := e.Store.UpdateRunPhase(ctx, rc, state.PhaseComplete, state.StatusCompleted); err != nil {
		return fmt.Errorf("marking run complete: %w", err)
	}
	if err := e.Lock.Release(ctx, e.Plan.Metadata.Cluster, rc); err != nil {
		e.logger().Warn("failed to release lock on completion", "error", err)
	}
	_ = e.Audit.Emit(audit.Event{RunID: rc, Type: audit.EventRunCompleted, Message: "run completed successfully"})
	e.notifyEvent(ctx, "RUN_COMPLETED", notify.SeverityInfo, "upgrade run completed successfully")
	return nil
}

// runPhase executes a single phase: persists STARTED/COMPLETED history, dispatches to
// the phase handler, and routes failures through handlePhaseFailure.
func (e *Engine) runPhase(ctx context.Context, phase state.Phase) error {
	rc := e.runID()

	if err := e.Store.UpdateRunPhase(ctx, rc, phase, state.StatusRunning); err != nil {
		return fmt.Errorf("updating run phase: %w", err)
	}
	if err := e.Store.AppendPhaseHistory(ctx, state.PhaseHistoryEntry{RunID: rc, Phase: phase, Status: "STARTED", StartedAt: time.Now().UTC()}); err != nil {
		e.logger().Warn("failed to append phase history", "error", err)
	}
	e.logger().Info("phase started", "runID", rc, "phase", phase)

	handler, ok := e.handlers()[phase]
	if !ok {
		return fmt.Errorf("no handler registered for phase %s", phase)
	}

	if err := handler(ctx); err != nil {
		return e.handlePhaseFailure(ctx, phase, err)
	}

	_ = e.Store.CompletePhaseHistory(ctx, rc, phase, "COMPLETED", "ok")
	_ = e.Audit.Emit(audit.Event{RunID: rc, Type: audit.EventPhaseCompleted, Phase: string(phase), Message: "phase completed"})
	e.notifyEvent(ctx, "PHASE_COMPLETED", notify.SeverityInfo, fmt.Sprintf("phase %s completed", phase))
	return nil
}

func (e *Engine) handlePhaseFailure(ctx context.Context, phase state.Phase, cause error) error {
	rc := e.runID()

	if phase == state.PhaseApproval && errors.Is(cause, ErrAwaitingApproval) {
		_ = e.Store.CompletePhaseHistory(ctx, rc, phase, "PAUSED", "awaiting approval")
		_ = e.Store.UpdateRunStatus(ctx, rc, state.StatusPaused, "awaiting approval")
		_ = e.Audit.Emit(audit.Event{RunID: rc, Type: audit.EventApprovalRequested, Phase: string(phase), Message: "awaiting approval"})
		e.notifyEvent(ctx, "APPROVAL_REQUESTED", notify.SeverityWarning, "control-plane upgrade is awaiting approval")
		return cause
	}

	_ = e.Store.CompletePhaseHistory(ctx, rc, phase, "FAILED", cause.Error())

	action := FailureAction(phase, e.Plan.Spec.Fallback)
	reason := cause.Error()
	_ = e.Store.UpdateRunStatus(ctx, rc, action, reason)

	eventType := audit.EventPaused
	severity := notify.SeverityWarning
	if action == state.StatusAborted || action == state.StatusFailed {
		eventType = audit.EventAborted
		severity = notify.SeverityCritical
	}
	_ = e.Audit.Emit(audit.Event{RunID: rc, Type: eventType, Phase: string(phase), Message: reason})
	e.notifyEvent(ctx, string(action), severity, fmt.Sprintf("phase %s failed: %s", phase, reason))

	if action == state.StatusAborted || action == state.StatusFailed {
		if err := e.Lock.Release(ctx, e.Plan.Metadata.Cluster, rc); err != nil {
			e.logger().Warn("failed to release lock after terminal failure", "error", err)
		}
	}
	return cause
}
