package upgrade

import (
	"testing"

	"eks-upgrader/internal/config"
	"eks-upgrader/internal/state"
)

func TestNextPhase_FollowsFixedOrder(t *testing.T) {
	tests := []struct {
		current state.Phase
		want    state.Phase
	}{
		{state.PhaseInit, state.PhaseValidate},
		{state.PhaseValidate, state.PhaseStateReconciliation},
		{state.PhaseApproval, state.PhaseControlPlaneUpgrade},
		{state.PhaseNodeGroupUpgrade, state.PhasePostUpgradeCheck},
		{state.PhaseConfigReconciliation, state.PhaseComplete},
		{state.PhaseComplete, state.PhaseComplete},
	}
	for _, tc := range tests {
		if got := NextPhase(tc.current); got != tc.want {
			t.Fatalf("NextPhase(%s) = %s, want %s", tc.current, got, tc.want)
		}
	}
}

func TestFailureAction_FixedTransitions(t *testing.T) {
	fallback := config.FallbackSpec{}
	if got := FailureAction(state.PhaseValidate, fallback); got != state.StatusFailed {
		t.Fatalf("VALIDATE failure: got %s, want FAILED", got)
	}
	if got := FailureAction(state.PhaseReadinessAnalysis, fallback); got != state.StatusAborted {
		t.Fatalf("READINESS blocking: got %s, want ABORTED", got)
	}
	if got := FailureAction(state.PhaseApproval, fallback); got != state.StatusPaused {
		t.Fatalf("APPROVAL timeout: got %s, want PAUSED", got)
	}
	if got := FailureAction(state.PhaseConfigReconciliation, fallback); got != state.StatusCompletedWithWarnings {
		t.Fatalf("CONFIG_RECONCILIATION failure: got %s, want COMPLETE_WITH_WARNINGS", got)
	}
}

func TestFailureAction_ConfigurablePolicy(t *testing.T) {
	fallback := config.FallbackSpec{
		OnPreflightFailure: "abort", OnStateDrift: "pause", OnNodeGroupFailure: "pause", OnControlPlaneFailure: "pause",
	}
	if got := FailureAction(state.PhasePreflight, fallback); got != state.StatusAborted {
		t.Fatalf("PREFLIGHT with onPreflightFailure=abort: got %s, want ABORTED", got)
	}
	if got := FailureAction(state.PhaseStateReconciliation, fallback); got != state.StatusPaused {
		t.Fatalf("STATE_RECONCILIATION with onStateDrift=pause: got %s, want PAUSED", got)
	}
	if got := FailureAction(state.PhaseNodeGroupUpgrade, fallback); got != state.StatusPaused {
		t.Fatalf("NODE_GROUP_UPGRADE with onNodeGroupFailure=pause: got %s, want PAUSED", got)
	}
}
