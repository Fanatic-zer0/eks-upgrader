package upgrade

import "testing"

func TestBlueGreenNodeGroupName(t *testing.T) {
	name, err := BlueGreenNodeGroupName("app-a", "-v{targetMinor}", "1.31")
	if err != nil {
		t.Fatalf("BlueGreenNodeGroupName: %v", err)
	}
	if name != "app-a-v31" {
		t.Fatalf("got %q, want %q", name, "app-a-v31")
	}
}

func TestBlueGreenNodeGroupName_InvalidVersion(t *testing.T) {
	if _, err := BlueGreenNodeGroupName("app-a", "-v{targetMinor}", "badversion"); err == nil {
		t.Fatal("expected error for malformed version")
	}
}

func TestBuildKarpenterAMIRollbackPlan(t *testing.T) {
	plan := BuildKarpenterAMIRollbackPlan("default", "al2023@v20250815")
	if plan.RevertToAMIAlias != "al2023@v20250815" {
		t.Fatalf("unexpected revert alias: %s", plan.RevertToAMIAlias)
	}
	if plan.RecommendedBudget.Nodes != "0" {
		t.Fatalf("expected recommended budget nodes=0, got %q", plan.RecommendedBudget.Nodes)
	}
	if len(plan.Steps) == 0 {
		t.Fatal("expected non-empty remediation steps")
	}
}
