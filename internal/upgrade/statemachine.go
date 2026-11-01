// Package upgrade implements the upgrade orchestration engine: the finite state
// machine, phase sequencing, health gates, canary evaluation, rollback guidance, and
// final report generation. It depends on config/state/lock/audit/notify directly, and
// on the heavier external systems (eksctl, AWS, Kubernetes, Karpenter, Helm) only
// through the Dependencies interface, so the engine itself is fully testable without
// live infrastructure.
package upgrade

import (
	"eks-upgrader/internal/config"
	"eks-upgrader/internal/state"
)

// PhaseOrder is the fixed phase sequence of the upgrade state machine. CANARY is
// intentionally not a top-level phase: canary checks are an embedded sub-step within
// NODE_GROUP_UPGRADE and POST_UPGRADE_CHECK.
var PhaseOrder = []state.Phase{
	state.PhaseInit,
	state.PhaseValidate,
	state.PhaseStateReconciliation,
	state.PhaseReadinessAnalysis,
	state.PhasePreflight,
	state.PhaseApproval,
	state.PhaseControlPlaneUpgrade,
	state.PhaseAddonUpgrade,
	state.PhaseKarpenterUpgrade,
	state.PhaseNodeGroupUpgrade,
	state.PhasePostUpgradeCheck,
	state.PhaseConfigReconciliation,
	state.PhaseComplete,
}

// NextPhase returns the phase immediately following current in PhaseOrder.
func NextPhase(current state.Phase) state.Phase {
	for i, p := range PhaseOrder {
		if p == current && i+1 < len(PhaseOrder) {
			return PhaseOrder[i+1]
		}
	}
	return state.PhaseComplete
}

// FailureAction resolves what should happen to the run when phase fails, per the
// buildplan's fixed failure-transition table combined with the plan's configurable
// fallback policy.
func FailureAction(phase state.Phase, fallback config.FallbackSpec) state.RunStatus {
	switch phase {
	case state.PhaseValidate:
		return state.StatusFailed
	case state.PhaseStateReconciliation:
		return actionToStatus(fallback.OnStateDrift)
	case state.PhaseReadinessAnalysis:
		return state.StatusAborted
	case state.PhasePreflight:
		return actionToStatus(fallback.OnPreflightFailure)
	case state.PhaseApproval:
		return state.StatusPaused
	case state.PhaseControlPlaneUpgrade:
		return actionToStatus(fallback.OnControlPlaneFailure)
	case state.PhaseAddonUpgrade:
		return actionToStatus(fallback.OnAddonFailure)
	case state.PhaseKarpenterUpgrade:
		return actionToStatus(fallback.OnKarpenterFailure)
	case state.PhaseNodeGroupUpgrade:
		return actionToStatus(fallback.OnNodeGroupFailure)
	case state.PhasePostUpgradeCheck:
		return actionToStatus(fallback.OnPostCheckFailure)
	case state.PhaseConfigReconciliation:
		return state.StatusCompletedWithWarnings
	default:
		return state.StatusPaused
	}
}

func actionToStatus(action string) state.RunStatus {
	if action == "abort" {
		return state.StatusAborted
	}
	return state.StatusPaused
}
