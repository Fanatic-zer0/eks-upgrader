package upgrade

import (
	"testing"

	"eks-upgrader/internal/config"
)

func TestEvaluateHealthGates_Passes(t *testing.T) {
	gates := config.HealthGatesSpec{RequireNodesReady: true, RequirePodsReady: true, MaxPendingPods: 0, RequirePDBRespected: true}
	in := HealthGateInput{AllNodesReady: true, AllPodsReady: true, PendingPodCount: 0}

	result := EvaluateHealthGates(gates, in)
	if !result.Passed {
		t.Fatalf("expected health gates to pass, got issues: %v", result.Issues)
	}
}

func TestEvaluateHealthGates_FailsOnNotReadyNodes(t *testing.T) {
	gates := config.HealthGatesSpec{RequireNodesReady: true}
	in := HealthGateInput{AllNodesReady: false, NotReadyNodes: []string{"node-2"}}

	result := EvaluateHealthGates(gates, in)
	if result.Passed {
		t.Fatal("expected health gates to fail due to not-ready node")
	}
}

func TestEvaluateHealthGates_FailsOnPendingPods(t *testing.T) {
	gates := config.HealthGatesSpec{MaxPendingPods: 0}
	in := HealthGateInput{AllNodesReady: true, AllPodsReady: true, PendingPodCount: 2}

	result := EvaluateHealthGates(gates, in)
	if result.Passed {
		t.Fatal("expected health gates to fail due to pending pods")
	}
}

func TestEvaluateHealthGates_FailsOnPDBIssues(t *testing.T) {
	gates := config.HealthGatesSpec{RequirePDBRespected: true}
	in := HealthGateInput{AllNodesReady: true, AllPodsReady: true, PDBIssues: []string{"PDB x allows 0 disruptions"}}

	result := EvaluateHealthGates(gates, in)
	if result.Passed {
		t.Fatal("expected health gates to fail due to PDB issue")
	}
}
