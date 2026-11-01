package upgrade

import (
	"fmt"

	"eks-upgrader/internal/config"
)

// HealthGateResult is the outcome of evaluating a node group's post-upgrade health gates.
type HealthGateResult struct {
	Passed bool
	Issues []string
}

// HealthGateInput bundles the raw signals health gates are evaluated against.
type HealthGateInput struct {
	AllNodesReady   bool
	NotReadyNodes   []string
	AllPodsReady    bool
	NotReadyPods    []string
	PendingPodCount int
	PDBIssues       []string
}

// EvaluateHealthGates applies spec.nodeGroups.healthGates to a single node group's
// post-upgrade signals, blocking progression to the next node group on any failure.
func EvaluateHealthGates(gates config.HealthGatesSpec, in HealthGateInput) HealthGateResult {
	var issues []string

	if gates.RequireNodesReady && !in.AllNodesReady {
		issues = append(issues, fmt.Sprintf("not all nodes are Ready: %v", in.NotReadyNodes))
	}
	if gates.RequirePodsReady && !in.AllPodsReady {
		issues = append(issues, fmt.Sprintf("not all pods are Ready: %v", in.NotReadyPods))
	}
	if in.PendingPodCount > gates.MaxPendingPods {
		issues = append(issues, fmt.Sprintf("%d pods pending, exceeding maxPendingPods=%d", in.PendingPodCount, gates.MaxPendingPods))
	}
	if gates.RequirePDBRespected && len(in.PDBIssues) > 0 {
		issues = append(issues, in.PDBIssues...)
	}

	return HealthGateResult{Passed: len(issues) == 0, Issues: issues}
}
