package upgrade_test

import (
	"context"
	"errors"
	"testing"

	"eks-upgrader/internal/state"
	"eks-upgrader/internal/upgrade"
)

func TestEngine_Run_FullSuccessPath(t *testing.T) {
	deps := newFakeDeps()
	e, store := newTestEngine(t, deps)
	ctx := context.Background()

	if err := e.Run(ctx, false); err != nil {
		t.Fatalf("Run: %v", err)
	}

	run, err := store.GetRun(ctx, "test-run")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != state.StatusCompleted {
		t.Fatalf("expected status COMPLETED, got %s", run.Status)
	}
	if run.CurrentPhase != state.PhaseComplete {
		t.Fatalf("expected phase COMPLETE, got %s", run.CurrentPhase)
	}

	groups, err := store.GetNodeGroupStatuses(ctx, "test-run")
	if err != nil {
		t.Fatalf("GetNodeGroupStatuses: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 node group statuses, got %d", len(groups))
	}
	for _, g := range groups {
		if g.Status != "SUCCEEDED" {
			t.Fatalf("expected node group %s to be SUCCEEDED, got %s", g.Name, g.Status)
		}
	}
	if len(deps.nodeGroupCalls) != 2 || deps.nodeGroupCalls[0] != "system" || deps.nodeGroupCalls[1] != "app-a" {
		t.Fatalf("expected node groups upgraded in order [system, app-a], got %v", deps.nodeGroupCalls)
	}
}

func TestEngine_Run_NodeGroupFailurePausesAndStopsBlastRadius(t *testing.T) {
	deps := newFakeDeps()
	deps.failNodeGroup = "app-a"
	e, store := newTestEngine(t, deps)
	ctx := context.Background()

	err := e.Run(ctx, false)
	if err == nil {
		t.Fatal("expected error from failing node group upgrade")
	}

	run, err := store.GetRun(ctx, "test-run")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != state.StatusPaused {
		t.Fatalf("expected status PAUSED, got %s", run.Status)
	}
	if run.CurrentPhase != state.PhaseNodeGroupUpgrade {
		t.Fatalf("expected phase NODE_GROUP_UPGRADE, got %s", run.CurrentPhase)
	}

	groups, err := store.GetNodeGroupStatuses(ctx, "test-run")
	if err != nil {
		t.Fatalf("GetNodeGroupStatuses: %v", err)
	}
	byName := map[string]string{}
	for _, g := range groups {
		byName[g.Name] = g.Status
	}
	if byName["system"] != "SUCCEEDED" {
		t.Fatalf("expected system to be SUCCEEDED, got %s", byName["system"])
	}
	if byName["app-a"] != "FAILED" {
		t.Fatalf("expected app-a to be FAILED, got %s", byName["app-a"])
	}
}

func TestEngine_ApprovalGating_PausesThenResumesAfterApprove(t *testing.T) {
	deps := newFakeDeps()
	e, store := newTestEngine(t, deps)
	e.Plan.Spec.ControlPlane.Approval.Required = true
	ctx := context.Background()

	err := e.Run(ctx, false)
	if err == nil || !errors.Is(err, upgrade.ErrAwaitingApproval) {
		t.Fatalf("expected ErrAwaitingApproval, got %v", err)
	}

	run, _ := store.GetRun(ctx, "test-run")
	if run.Status != state.StatusPaused || run.CurrentPhase != state.PhaseApproval {
		t.Fatalf("expected PAUSED at APPROVAL, got status=%s phase=%s", run.Status, run.CurrentPhase)
	}

	if err := e.Approve(ctx, "platform-oncall", "approved for maintenance window"); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	if err := e.Resume(ctx, false); err != nil {
		t.Fatalf("Resume after approval: %v", err)
	}

	run, _ = store.GetRun(ctx, "test-run")
	if run.Status != state.StatusCompleted {
		t.Fatalf("expected COMPLETED after resume, got %s", run.Status)
	}

	approvals, err := store.GetApprovals(ctx, "test-run")
	if err != nil {
		t.Fatalf("GetApprovals: %v", err)
	}
	if len(approvals) != 1 || approvals[0].Approver != "platform-oncall" {
		t.Fatalf("unexpected approvals: %+v", approvals)
	}
}

func TestEngine_Abort_ReleasesLockAndMarksAborted(t *testing.T) {
	deps := newFakeDeps()
	deps.failControlPlane = true
	e, store := newTestEngine(t, deps)
	ctx := context.Background()

	if err := e.Run(ctx, false); err == nil {
		t.Fatal("expected error from failing control-plane upgrade")
	}

	if err := e.Abort(ctx, "manual abort for test"); err != nil {
		t.Fatalf("Abort: %v", err)
	}

	run, err := store.GetRun(ctx, "test-run")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != state.StatusAborted || run.AbortReason != "manual abort for test" {
		t.Fatalf("unexpected run after abort: %+v", run)
	}
}

func TestEngine_ReadinessBlockingFinding_AlwaysAborts(t *testing.T) {
	deps := newFakeDeps()
	deps.failReadiness = true
	e, store := newTestEngine(t, deps)
	ctx := context.Background()

	if err := e.Run(ctx, false); err == nil {
		t.Fatal("expected error from blocking readiness finding")
	}

	run, err := store.GetRun(ctx, "test-run")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != state.StatusAborted {
		t.Fatalf("expected ABORTED (fixed transition, not configurable), got %s", run.Status)
	}
}

func TestEngine_Run_DryRun_NoMutation(t *testing.T) {
	deps := newFakeDeps()
	e, store := newTestEngine(t, deps)
	ctx := context.Background()

	if err := e.Run(ctx, true); err != nil {
		t.Fatalf("dry-run Run: %v", err)
	}

	run, err := store.GetRun(ctx, "test-run")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run != nil {
		t.Fatalf("expected no run record to be created in dry-run mode, got %+v", run)
	}
	if deps.controlPlaneCalls != 0 || len(deps.nodeGroupCalls) != 0 {
		t.Fatalf("expected no Dependencies mutation calls in dry-run mode, got controlPlaneCalls=%d nodeGroupCalls=%v", deps.controlPlaneCalls, deps.nodeGroupCalls)
	}
}

func TestEngine_Resume_CircuitBreakerTripsAfterMaxAttempts(t *testing.T) {
	deps := newFakeDeps()
	deps.failControlPlane = true
	e, store := newTestEngine(t, deps)
	e.MaxResumeAttempts = 2
	ctx := context.Background()

	if err := e.Run(ctx, false); err == nil {
		t.Fatal("expected initial run to fail at control-plane upgrade")
	}

	// First resume attempt fails (control plane still fails) and is recorded.
	if err := e.Resume(ctx, false); err == nil {
		t.Fatal("expected first resume attempt to fail")
	}
	// Second resume attempt fails too, reaching the configured max of 2.
	if err := e.Resume(ctx, false); err == nil {
		t.Fatal("expected second resume attempt to fail")
	}

	// Third attempt should trip the circuit breaker before ever calling Dependencies again.
	callsBefore := deps.controlPlaneCalls
	err := e.Resume(ctx, false)
	if !errors.Is(err, upgrade.ErrResumeAttemptsExceeded) {
		t.Fatalf("expected ErrResumeAttemptsExceeded, got %v", err)
	}
	if deps.controlPlaneCalls != callsBefore {
		t.Fatalf("expected no further control-plane calls once circuit breaker trips, before=%d after=%d", callsBefore, deps.controlPlaneCalls)
	}

	// force=true should override the circuit breaker.
	deps.failControlPlane = false
	if err := e.Resume(ctx, true); err != nil {
		t.Fatalf("expected forced resume to succeed once the underlying failure is fixed, got %v", err)
	}

	run, _ := store.GetRun(ctx, "test-run")
	if run.Status != state.StatusCompleted {
		t.Fatalf("expected COMPLETED after forced resume, got %s", run.Status)
	}
}
