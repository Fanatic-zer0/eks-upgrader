package config

import (
	"testing"
)

func mustLoadValid(t *testing.T) (*ClusterConfig, *UpgradePlan) {
	t.Helper()
	cfg, err := LoadClusterConfig("testdata/valid-cluster.yaml")
	if err != nil {
		t.Fatalf("LoadClusterConfig: %v", err)
	}
	plan, err := LoadUpgradePlan("testdata/valid-upgrade.yaml")
	if err != nil {
		t.Fatalf("LoadUpgradePlan: %v", err)
	}
	return cfg, plan
}

func TestValidate_ValidConfigPasses(t *testing.T) {
	cfg, plan := mustLoadValid(t)
	result := Validate(cfg, plan)
	if !result.OK() {
		t.Fatalf("expected no blocking issues, got: %v", result.Blocking)
	}
}

func TestIsExactlyOneMinorAhead(t *testing.T) {
	tests := []struct {
		name    string
		current string
		target  string
		want    bool
		wantErr bool
	}{
		{"one minor ahead", "1.30", "1.31", true, false},
		{"same version", "1.30", "1.30", false, false},
		{"skip a minor version", "1.29", "1.31", false, false},
		{"downgrade", "1.31", "1.30", false, false},
		{"malformed current", "bad", "1.31", false, true},
		{"malformed target", "1.30", "bad", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := IsExactlyOneMinorAhead(tc.current, tc.target)
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected error state: err=%v wantErr=%v", err, tc.wantErr)
			}
			if err == nil && got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidate_RejectsVersionSkip(t *testing.T) {
	cfg, plan := mustLoadValid(t)
	plan.Spec.TargetKubernetesVersion = "1.32" // skips 1.31
	result := Validate(cfg, plan)
	if result.OK() {
		t.Fatal("expected blocking issue for version skip, got none")
	}
}

func TestValidate_RejectsNodeGroupOrderMismatch(t *testing.T) {
	cfg, plan := mustLoadValid(t)
	plan.Spec.NodeGroups.Order = []string{"system"} // missing app-a
	result := Validate(cfg, plan)
	if result.OK() {
		t.Fatal("expected blocking issue for missing node group in order, got none")
	}
}

func TestValidate_RejectsDuplicateNodeGroupOrder(t *testing.T) {
	cfg, plan := mustLoadValid(t)
	plan.Spec.NodeGroups.Order = []string{"system", "system", "app-a"}
	result := Validate(cfg, plan)
	if result.OK() {
		t.Fatal("expected blocking issue for duplicate node group order entry, got none")
	}
}

func TestValidate_RejectsUnknownNodeGroupInOrder(t *testing.T) {
	cfg, plan := mustLoadValid(t)
	plan.Spec.NodeGroups.Order = []string{"system", "app-a", "does-not-exist"}
	result := Validate(cfg, plan)
	if result.OK() {
		t.Fatal("expected blocking issue for unknown node group in order, got none")
	}
}

func TestValidate_RejectsMissingClusterConfigReference(t *testing.T) {
	cfg, plan := mustLoadValid(t)
	plan.Spec.ClusterConfig = "./does-not-exist.yaml"
	result := Validate(cfg, plan)
	if result.OK() {
		t.Fatal("expected blocking issue for missing cluster config reference, got none")
	}
}

func TestValidate_RejectsInlineSecrets(t *testing.T) {
	cfg, err := LoadClusterConfig("testdata/valid-cluster.yaml")
	if err != nil {
		t.Fatalf("LoadClusterConfig: %v", err)
	}
	plan, err := LoadUpgradePlan("testdata/secret-upgrade.yaml")
	if err != nil {
		t.Fatalf("LoadUpgradePlan: %v", err)
	}
	result := Validate(cfg, plan)
	if result.OK() {
		t.Fatal("expected blocking issue for inline secret, got none")
	}
}

func TestValidate_RejectsWrongAPIVersionKind(t *testing.T) {
	cfg, plan := mustLoadValid(t)
	cfg.Kind = "NotAClusterConfig"
	plan.Kind = "NotAnUpgradePlan"
	result := Validate(cfg, plan)
	if result.OK() {
		t.Fatal("expected blocking issues for wrong apiVersion/kind, got none")
	}
	if len(result.Blocking) < 2 {
		t.Fatalf("expected at least 2 blocking issues, got %d: %v", len(result.Blocking), result.Blocking)
	}
}
