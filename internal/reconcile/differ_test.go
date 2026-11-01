package reconcile

import (
	"testing"

	"eks-upgrader/internal/config"
)

func baseClusterConfig() *config.ClusterConfig {
	return &config.ClusterConfig{
		Metadata: config.ClusterMetadata{Version: "1.30", Tags: map[string]string{"environment": "production"}},
		ManagedNodeGroups: []config.ManagedNodeGroup{
			{Name: "system", DesiredCapacity: 2, MinSize: 2, MaxSize: 4, Labels: map[string]string{"workload": "system"}},
		},
	}
}

func baseObserved() *ObservedCluster {
	return &ObservedCluster{
		Cluster: ObservedClusterInfo{CurrentKubernetesVersion: "1.30", Tags: map[string]string{"environment": "production"}},
		ManagedNodeGroups: []ObservedNodeGroup{
			{Name: "system", KubernetesVersion: "1.30", DesiredSize: 2, MinSize: 2, MaxSize: 4, Labels: map[string]string{"workload": "system"}},
		},
	}
}

func TestDiff_InSyncWhenMatching(t *testing.T) {
	plan := &config.UpgradePlan{}
	findings := Diff(baseClusterConfig(), plan, baseObserved())
	if HasBlocking(findings) {
		t.Fatalf("expected no blocking findings, got: %+v", findings)
	}
}

func TestDiff_BlocksOnClusterVersionDrift(t *testing.T) {
	cfg := baseClusterConfig()
	observed := baseObserved()
	observed.Cluster.CurrentKubernetesVersion = "1.29"

	findings := Diff(cfg, &config.UpgradePlan{}, observed)
	if !HasBlocking(findings) {
		t.Fatal("expected blocking finding for cluster version drift")
	}
}

func TestDiff_BlocksOnNodeGroupSizeDrift(t *testing.T) {
	cfg := baseClusterConfig()
	observed := baseObserved()
	observed.ManagedNodeGroups[0].DesiredSize = 5

	findings := Diff(cfg, &config.UpgradePlan{}, observed)
	if !HasBlocking(findings) {
		t.Fatal("expected blocking finding for node group size drift")
	}
}

func TestDiff_WarnsOnTagDrift(t *testing.T) {
	cfg := baseClusterConfig()
	observed := baseObserved()
	observed.Cluster.Tags = map[string]string{"environment": "staging"}

	findings := Diff(cfg, &config.UpgradePlan{}, observed)
	var found bool
	for _, f := range findings {
		if f.Field == "cluster.tags" && f.Classification == Warning {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a WARNING finding for cluster.tags drift, got: %+v", findings)
	}
	if HasBlocking(findings) {
		t.Fatal("tag drift must never be blocking")
	}
}

func TestDiff_UnmanagedNodeGroupNotInGit(t *testing.T) {
	cfg := baseClusterConfig()
	observed := baseObserved()
	observed.ManagedNodeGroups = append(observed.ManagedNodeGroups, ObservedNodeGroup{Name: "rogue", KubernetesVersion: "1.30", DesiredSize: 1, MinSize: 1, MaxSize: 1})

	findings := Diff(cfg, &config.UpgradePlan{}, observed)
	var found bool
	for _, f := range findings {
		if f.Classification == Unmanaged && f.Field == "managedNodeGroups[rogue]" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected UNMANAGED finding for rogue node group, got: %+v", findings)
	}
}

func TestDiff_KarpenterAMIDrift(t *testing.T) {
	cfg := baseClusterConfig()
	observed := baseObserved()
	observed.Karpenter.NodeClasses = []ObservedNodeClass{{Name: "default", Healthy: true, AMIAlias: "al2023@v20250815"}}

	plan := &config.UpgradePlan{Spec: config.PlanSpec{Karpenter: config.KarpenterSpec{
		Enabled:     true,
		NodeClasses: []config.KarpenterNodeClassSpec{{Name: "default", AMIAlias: "al2023@v20250918"}},
	}}}

	findings := Diff(cfg, plan, observed)
	var found bool
	for _, f := range findings {
		if f.Classification == Blocking && f.Field == "karpenter.nodeClasses[default].amiSelectorTerms" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected blocking finding for Karpenter AMI drift, got: %+v", findings)
	}
}

func TestDiff_KarpenterDisruptionBudgetDrift(t *testing.T) {
	cfg := baseClusterConfig()
	observed := baseObserved()
	observed.Karpenter.NodePools = []ObservedNodePool{{Name: "general-purpose", Healthy: true, DisruptionBudgetNodes: []string{"10%"}}}

	plan := &config.UpgradePlan{Spec: config.PlanSpec{Karpenter: config.KarpenterSpec{
		Enabled: true,
		NodePools: []config.KarpenterNodePoolSpec{{
			Name: "general-purpose",
			DisruptionBudgets: []config.DisruptionBudget{
				{Nodes: "10%"}, {Nodes: "0", Reasons: []string{"Drifted"}},
			},
		}},
	}}}

	findings := Diff(cfg, plan, observed)
	var found bool
	for _, f := range findings {
		if f.Classification == Blocking && f.Field == "karpenter.nodePools[general-purpose].disruption.budgets" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected blocking finding for disruption budget count drift, got: %+v", findings)
	}
}
