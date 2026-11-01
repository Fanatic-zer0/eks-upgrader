package upgrade_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"eks-upgrader/internal/audit"
	"eks-upgrader/internal/config"
	"eks-upgrader/internal/lock"
	"eks-upgrader/internal/readiness"
	"eks-upgrader/internal/reconcile"
	"eks-upgrader/internal/state"
	"eks-upgrader/internal/upgrade"
)

// fakeDeps is a fully scriptable upgrade.Dependencies fake.
type fakeDeps struct {
	failNodeGroup     string
	failControlPlane  bool
	failReadiness     bool
	alreadySucceeded  map[state.Phase]bool
	nodeGroupCalls    []string
	controlPlaneCalls int
}

func newFakeDeps() *fakeDeps {
	return &fakeDeps{alreadySucceeded: map[state.Phase]bool{}}
}

func (f *fakeDeps) AlreadySucceeded(ctx context.Context, phase state.Phase) (bool, error) {
	return f.alreadySucceeded[phase], nil
}
func (f *fakeDeps) Reconcile(ctx context.Context) ([]reconcile.Finding, error) { return nil, nil }
func (f *fakeDeps) Readiness(ctx context.Context) (*readiness.Report, error) {
	if f.failReadiness {
		return &readiness.Report{Status: readiness.StatusNotReady, Findings: []readiness.Finding{{Severity: readiness.SeverityBlocking, Message: "boom"}}}, nil
	}
	return &readiness.Report{Status: readiness.StatusReady}, nil
}
func (f *fakeDeps) Preflight(ctx context.Context) ([]readiness.Finding, error) { return nil, nil }
func (f *fakeDeps) UpgradeControlPlane(ctx context.Context) error {
	f.controlPlaneCalls++
	if f.failControlPlane {
		return errors.New("control plane upgrade failed")
	}
	return nil
}
func (f *fakeDeps) UpgradeAddons(ctx context.Context) error    { return nil }
func (f *fakeDeps) UpgradeKarpenter(ctx context.Context) error { return nil }
func (f *fakeDeps) UpgradeNodeGroup(ctx context.Context, name string) error {
	f.nodeGroupCalls = append(f.nodeGroupCalls, name)
	if name == f.failNodeGroup {
		return errors.New("node group upgrade failed")
	}
	return nil
}
func (f *fakeDeps) NodeGroupHealthGate(ctx context.Context, name string) (upgrade.HealthGateResult, error) {
	return upgrade.HealthGateResult{Passed: true}, nil
}
func (f *fakeDeps) RunCanary(ctx context.Context, namespaces []string) ([]upgrade.CanaryCheckResult, bool, error) {
	return nil, true, nil
}
func (f *fakeDeps) PostUpgradeCheck(ctx context.Context) ([]readiness.Finding, error) {
	return nil, nil
}
func (f *fakeDeps) GenerateConfigPatches(ctx context.Context) error { return nil }
func (f *fakeDeps) EksctlVersionSupports(ctx context.Context, targetVersion string) (bool, string, error) {
	return true, "0.200.0", nil
}

func testClusterConfig() *config.ClusterConfig {
	return &config.ClusterConfig{
		APIVersion: "eksctl.io/v1alpha5", Kind: "ClusterConfig",
		Metadata: config.ClusterMetadata{Name: "test-cluster", Region: "ap-south-1", Version: "1.30"},
		ManagedNodeGroups: []config.ManagedNodeGroup{
			{Name: "system", InstanceType: "m6i.large", DesiredCapacity: 2, MinSize: 2, MaxSize: 4},
			{Name: "app-a", InstanceType: "m6i.xlarge", DesiredCapacity: 3, MinSize: 3, MaxSize: 6},
		},
	}
}

func testUpgradePlan(t *testing.T, clusterConfigPath string) *config.UpgradePlan {
	t.Helper()
	return &config.UpgradePlan{
		APIVersion: "platform.io/v1", Kind: "EKSUpgradePlan",
		SourcePath: clusterConfigPath, // directory context for ResolveClusterConfigPath
		Metadata:   config.PlanMetadata{Name: "test", Cluster: "test-cluster", Region: "ap-south-1", RunID: "test-run"},
		Spec: config.PlanSpec{
			ClusterConfig:           "./cluster.yaml",
			TargetKubernetesVersion: "1.31",
			Execution:               config.ExecutionSpec{Lock: config.LockSpec{TTL: config.Duration{Duration: time.Hour}}},
			StateReconciliation:     config.StateReconciliationSpec{Enabled: false},
			ControlPlane:            config.ControlPlaneSpec{Approval: config.ApprovalSpec{Required: false}},
			NodeGroups:              config.NodeGroupsSpec{Order: []string{"system", "app-a"}},
			Fallback:                config.FallbackSpec{},
		},
	}
}

func newTestEngine(t *testing.T, deps *fakeDeps) (*upgrade.Engine, state.Store) {
	t.Helper()
	dir := t.TempDir()

	cluster := testClusterConfig()
	clusterPath := filepath.Join(dir, "cluster.yaml")
	cluster.SourcePath = clusterPath

	plan := testUpgradePlan(t, dir)
	plan.SourcePath = filepath.Join(dir, "upgrade.yaml")
	// clusterConfig resolves relative to upgrade.yaml's directory; write a stub file so
	// config.Validate's file-existence check passes.
	if err := os.WriteFile(clusterPath, []byte("apiVersion: eksctl.io/v1alpha5\n"), 0o644); err != nil {
		t.Fatalf("writing stub cluster.yaml: %v", err)
	}

	store, err := state.OpenSQLiteStore(filepath.Join(dir, "upgrade.db"))
	if err != nil {
		t.Fatalf("OpenSQLiteStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	locker := lock.NewLocalLocker(filepath.Join(dir, "locks"))

	e := &upgrade.Engine{
		Cluster: cluster, Plan: plan, Store: store, Lock: locker,
		Audit: audit.NopLogger{}, Notify: nil, Deps: deps, Owner: "test",
	}
	return e, store
}
