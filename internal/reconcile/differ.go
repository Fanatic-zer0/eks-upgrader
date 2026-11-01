package reconcile

import (
	"fmt"

	"eks-upgrader/internal/config"
)

// Classification is the drift severity assigned to a single diff finding.
type Classification string

const (
	InSync    Classification = "IN_SYNC"
	Info      Classification = "INFO"
	Warning   Classification = "WARNING"
	Blocking  Classification = "BLOCKING"
	Unmanaged Classification = "UNMANAGED"
)

// Finding is a single classified difference between Git-declared desired state and
// live observed state.
type Finding struct {
	Field          string         `json:"field"`
	Classification Classification `json:"classification"`
	Desired        string         `json:"desired,omitempty"`
	Observed       string         `json:"observed,omitempty"`
	Message        string         `json:"message"`
}

// Diff compares cfg/plan (Git desired state) against observed (live state) and returns
// every classified finding. Classification here is the differ's default judgment;
// ApplyPolicy subsequently reconciles it against the plan's stateReconciliation policy.
func Diff(cfg *config.ClusterConfig, plan *config.UpgradePlan, observed *ObservedCluster) []Finding {
	var findings []Finding

	findings = append(findings, diffClusterVersion(cfg, observed)...)
	findings = append(findings, diffClusterTags(cfg, observed)...)
	findings = append(findings, diffNodeGroups(cfg, observed)...)
	if plan.Spec.Karpenter.Enabled {
		findings = append(findings, diffKarpenter(plan, observed)...)
	}
	return findings
}

func diffClusterVersion(cfg *config.ClusterConfig, observed *ObservedCluster) []Finding {
	if cfg.Metadata.Version == observed.Cluster.CurrentKubernetesVersion {
		return []Finding{{Field: "cluster.version", Classification: InSync, Desired: cfg.Metadata.Version, Observed: observed.Cluster.CurrentKubernetesVersion}}
	}
	return []Finding{{
		Field: "cluster.version", Classification: Blocking,
		Desired: cfg.Metadata.Version, Observed: observed.Cluster.CurrentKubernetesVersion,
		Message: "live EKS control-plane version differs from cluster.yaml metadata.version",
	}}
}

func diffClusterTags(cfg *config.ClusterConfig, observed *ObservedCluster) []Finding {
	if mapsEqual(cfg.Metadata.Tags, observed.Cluster.Tags) {
		return nil
	}
	return []Finding{{
		Field: "cluster.tags", Classification: Warning,
		Message: "cluster tags differ between cluster.yaml and the live cluster (cosmetic drift)",
	}}
}

func diffNodeGroups(cfg *config.ClusterConfig, observed *ObservedCluster) []Finding {
	var findings []Finding

	desired := make(map[string]config.ManagedNodeGroup, len(cfg.ManagedNodeGroups))
	for _, ng := range cfg.ManagedNodeGroups {
		desired[ng.Name] = ng
	}
	live := make(map[string]ObservedNodeGroup, len(observed.ManagedNodeGroups))
	for _, ng := range observed.ManagedNodeGroups {
		live[ng.Name] = ng
	}

	for name, d := range desired {
		obs, ok := live[name]
		if !ok {
			findings = append(findings, Finding{
				Field: fmt.Sprintf("managedNodeGroups[%s]", name), Classification: Warning,
				Message: "declared in cluster.yaml but not found live (may not be created yet)",
			})
			continue
		}

		if obs.KubernetesVersion != observed.Cluster.CurrentKubernetesVersion {
			findings = append(findings, Finding{
				Field: fmt.Sprintf("managedNodeGroups[%s].kubernetesVersion", name), Classification: Blocking,
				Desired: observed.Cluster.CurrentKubernetesVersion, Observed: obs.KubernetesVersion,
				Message: "managed node group Kubernetes version does not match the live control-plane version",
			})
		}

		if int32(d.DesiredCapacity) != obs.DesiredSize || int32(d.MinSize) != obs.MinSize || int32(d.MaxSize) != obs.MaxSize {
			findings = append(findings, Finding{
				Field: fmt.Sprintf("managedNodeGroups[%s].size", name), Classification: Blocking,
				Desired:  fmt.Sprintf("min=%d max=%d desired=%d", d.MinSize, d.MaxSize, d.DesiredCapacity),
				Observed: fmt.Sprintf("min=%d max=%d desired=%d", obs.MinSize, obs.MaxSize, obs.DesiredSize),
				Message:  "managed node group scaling config differs from cluster.yaml",
			})
		}

		if !mapsEqual(d.Labels, obs.Labels) {
			findings = append(findings, Finding{
				Field: fmt.Sprintf("managedNodeGroups[%s].labels", name), Classification: Warning,
				Message: "node group labels differ from cluster.yaml (non-critical unless workload scheduling depends on them)",
			})
		}
	}

	for name := range live {
		if _, ok := desired[name]; !ok {
			findings = append(findings, Finding{
				Field: fmt.Sprintf("managedNodeGroups[%s]", name), Classification: Unmanaged,
				Message: "live managed node group is not declared in cluster.yaml",
			})
		}
	}

	return findings
}

func diffKarpenter(plan *config.UpgradePlan, observed *ObservedCluster) []Finding {
	var findings []Finding

	desiredNC := make(map[string]config.KarpenterNodeClassSpec, len(plan.Spec.Karpenter.NodeClasses))
	for _, nc := range plan.Spec.Karpenter.NodeClasses {
		desiredNC[nc.Name] = nc
	}
	for _, obs := range observed.Karpenter.NodeClasses {
		d, ok := desiredNC[obs.Name]
		if !ok {
			findings = append(findings, Finding{
				Field: fmt.Sprintf("karpenter.nodeClasses[%s]", obs.Name), Classification: Unmanaged,
				Message: "live EC2NodeClass is not declared in upgrade.yaml karpenter.nodeClasses",
			})
			continue
		}
		if d.AMIAlias != "" && d.AMIAlias != obs.AMIAlias {
			findings = append(findings, Finding{
				Field: fmt.Sprintf("karpenter.nodeClasses[%s].amiSelectorTerms", obs.Name), Classification: Blocking,
				Desired: d.AMIAlias, Observed: obs.AMIAlias,
				Message: "EC2NodeClass AMI selector differs from the Git-declared value",
			})
		}
	}

	desiredNP := make(map[string]config.KarpenterNodePoolSpec, len(plan.Spec.Karpenter.NodePools))
	for _, np := range plan.Spec.Karpenter.NodePools {
		desiredNP[np.Name] = np
	}
	for _, obs := range observed.Karpenter.NodePools {
		d, ok := desiredNP[obs.Name]
		if !ok {
			findings = append(findings, Finding{
				Field: fmt.Sprintf("karpenter.nodePools[%s]", obs.Name), Classification: Unmanaged,
				Message: "live NodePool is not declared in upgrade.yaml karpenter.nodePools",
			})
			continue
		}
		if len(d.DisruptionBudgets) != len(obs.DisruptionBudgetNodes) {
			findings = append(findings, Finding{
				Field: fmt.Sprintf("karpenter.nodePools[%s].disruption.budgets", obs.Name), Classification: Blocking,
				Desired: fmt.Sprintf("%d budgets", len(d.DisruptionBudgets)), Observed: fmt.Sprintf("%d budgets", len(obs.DisruptionBudgetNodes)),
				Message: "NodePool disruption budget count differs from the Git-declared value",
			})
		}
	}

	return findings
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// HasBlocking reports whether any finding is classified BLOCKING.
func HasBlocking(findings []Finding) bool {
	for _, f := range findings {
		if f.Classification == Blocking {
			return true
		}
	}
	return false
}
