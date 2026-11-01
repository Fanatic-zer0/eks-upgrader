// Package reconcile implements the read-only live-state collector, the differ that
// compares observed state against Git-declared desired state, and proposed (never
// auto-applied) config patch generation.
package reconcile

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"eks-upgrader/internal/aws"
	"eks-upgrader/internal/k8s"
	"eks-upgrader/internal/karpenter"
)

// ObservedCluster is the full live-state snapshot written to output/observed-cluster.yaml.
type ObservedCluster struct {
	APIVersion        string              `yaml:"apiVersion"`
	Kind              string              `yaml:"kind"`
	Metadata          ObservedMetadata    `yaml:"metadata"`
	Cluster           ObservedClusterInfo `yaml:"cluster"`
	ManagedNodeGroups []ObservedNodeGroup `yaml:"managedNodeGroups"`
	Karpenter         ObservedKarpenter   `yaml:"karpenter"`
	Addons            []ObservedAddon     `yaml:"addons"`
}

type ObservedMetadata struct {
	Cluster     string    `yaml:"cluster"`
	Region      string    `yaml:"region"`
	CollectedAt time.Time `yaml:"collectedAt"`
}

type ObservedClusterInfo struct {
	CurrentKubernetesVersion string            `yaml:"currentKubernetesVersion"`
	Status                   string            `yaml:"status"`
	Endpoint                 string            `yaml:"endpoint"`
	OIDCIssuer               string            `yaml:"oidcIssuer"`
	Tags                     map[string]string `yaml:"tags"`
}

type ObservedNodeGroup struct {
	Name              string            `yaml:"name"`
	Status            string            `yaml:"status"`
	KubernetesVersion string            `yaml:"kubernetesVersion"`
	DesiredSize       int32             `yaml:"desiredSize"`
	MinSize           int32             `yaml:"minSize"`
	MaxSize           int32             `yaml:"maxSize"`
	InstanceTypes     []string          `yaml:"instanceTypes"`
	AMIType           string            `yaml:"amiType"`
	Labels            map[string]string `yaml:"labels,omitempty"`
	LaunchTemplateVer string            `yaml:"launchTemplateVersion,omitempty"`
}

type ObservedKarpenter struct {
	Installed   bool                `yaml:"installed"`
	NodeClasses []ObservedNodeClass `yaml:"nodeClasses"`
	NodePools   []ObservedNodePool  `yaml:"nodePools"`
}

type ObservedNodeClass struct {
	Name     string `yaml:"name"`
	Healthy  bool   `yaml:"healthy"`
	AMIAlias string `yaml:"amiAlias,omitempty"`
}

type ObservedNodePool struct {
	Name                  string   `yaml:"name"`
	Healthy               bool     `yaml:"healthy"`
	DriftedNodeClaims     int      `yaml:"driftedNodeClaims"`
	DisruptionBudgetNodes []string `yaml:"disruptionBudgetNodes,omitempty"`
}

type ObservedAddon struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	Health  string `yaml:"health"`
}

// EKSReader is the narrow read-only EKS surface the Collector depends on, satisfied by
// *aws.EKSClient and easily faked in tests.
type EKSReader interface {
	DescribeCluster(ctx context.Context, name string) (*aws.ClusterInfo, error)
	ListNodegroupNames(ctx context.Context, clusterName string) ([]string, error)
	DescribeNodegroup(ctx context.Context, clusterName, nodegroupName string) (*aws.NodeGroupInfo, error)
	ListAddons(ctx context.Context, clusterName string) ([]aws.AddonInfo, error)
}

// NodeGroupNodesReader resolves representative node labels for a managed node group.
type NodeGroupNodesReader interface {
	NodesForNodeGroup(ctx context.Context, nodegroupName string) ([]k8s.NodeInfo, error)
}

// KarpenterReader is the narrow read-only Karpenter surface the Collector depends on.
type KarpenterReader interface {
	ListEC2NodeClasses(ctx context.Context) ([]karpenter.EC2NodeClass, error)
	ListNodePools(ctx context.Context) ([]karpenter.NodePool, error)
	DriftedNodeClaims(ctx context.Context) ([]karpenter.NodeClaim, error)
}

// Collector aggregates read-only state from AWS, Kubernetes, and Karpenter into a
// single ObservedCluster snapshot.
type Collector struct {
	EKS       EKSReader
	K8s       NodeGroupNodesReader
	Karpenter KarpenterReader
	// Logger receives non-fatal warnings (e.g. a supplementary signal like add-on
	// listing failing); defaults to slog.Default() when nil.
	Logger *slog.Logger
}

func (c *Collector) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// Collect pulls the full live-state snapshot for cluster. karpenterEnabled controls
// whether Karpenter objects are collected at all (to avoid noisy errors on clusters
// that don't run it).
func (c *Collector) Collect(ctx context.Context, clusterName, region string, karpenterEnabled bool) (*ObservedCluster, error) {
	clusterInfo, err := c.EKS.DescribeCluster(ctx, clusterName)
	if err != nil {
		return nil, fmt.Errorf("collecting cluster state: %w", err)
	}

	observed := &ObservedCluster{
		APIVersion: "platform.io/v1",
		Kind:       "EKSObservedClusterState",
		Metadata:   ObservedMetadata{Cluster: clusterName, Region: region, CollectedAt: time.Now().UTC()},
		Cluster: ObservedClusterInfo{
			CurrentKubernetesVersion: clusterInfo.Version,
			Status:                   clusterInfo.Status,
			Endpoint:                 clusterInfo.Endpoint,
			OIDCIssuer:               clusterInfo.OIDCIssuer,
			Tags:                     clusterInfo.Tags,
		},
	}

	names, err := c.EKS.ListNodegroupNames(ctx, clusterName)
	if err != nil {
		return nil, fmt.Errorf("collecting managed node group names: %w", err)
	}
	for _, name := range names {
		ng, err := c.EKS.DescribeNodegroup(ctx, clusterName, name)
		if err != nil {
			return nil, fmt.Errorf("collecting managed node group %q: %w", name, err)
		}
		labels := ng.Labels
		if c.K8s != nil {
			if nodes, nerr := c.K8s.NodesForNodeGroup(ctx, name); nerr == nil && len(nodes) > 0 {
				// Representative labels from the first matching node; node-group-level
				// labels configured via eksctl/EKS are applied uniformly across nodes.
				labels = nodes[0].Labels
			}
		}
		observed.ManagedNodeGroups = append(observed.ManagedNodeGroups, ObservedNodeGroup{
			Name: ng.Name, Status: ng.Status, KubernetesVersion: ng.KubernetesVersion,
			DesiredSize: ng.DesiredSize, MinSize: ng.MinSize, MaxSize: ng.MaxSize,
			InstanceTypes: ng.InstanceTypes, AMIType: ng.AMIType, Labels: labels,
			LaunchTemplateVer: ng.LaunchTemplateVer,
		})
	}

	// Add-on health is a supplementary signal, not a safety-critical diff input (unlike
	// control-plane/node-group version and size, which must hard-fail collection on
	// error); a failure here degrades to a warning so the rest of the snapshot is still
	// usable, e.g. when the EKS add-ons API is unavailable or returns an unexpected
	// response.
	addons, err := c.EKS.ListAddons(ctx, clusterName)
	if err != nil {
		c.logger().Warn("failed to list EKS add-ons; continuing without add-on data", "cluster", clusterName, "error", err)
	} else {
		for _, a := range addons {
			health := "HEALTHY"
			if !a.Healthy() {
				health = "UNHEALTHY"
			}
			observed.Addons = append(observed.Addons, ObservedAddon{Name: a.Name, Version: a.Version, Health: health})
		}
	}

	if karpenterEnabled && c.Karpenter != nil {
		observed.Karpenter.Installed = true

		nodeClasses, err := c.Karpenter.ListEC2NodeClasses(ctx)
		if err != nil {
			return nil, fmt.Errorf("collecting Karpenter EC2NodeClasses: %w", err)
		}
		for _, nc := range nodeClasses {
			observed.Karpenter.NodeClasses = append(observed.Karpenter.NodeClasses, ObservedNodeClass{
				Name: nc.Name, Healthy: nc.Healthy, AMIAlias: nc.AMIAlias,
			})
		}

		nodePools, err := c.Karpenter.ListNodePools(ctx)
		if err != nil {
			return nil, fmt.Errorf("collecting Karpenter NodePools: %w", err)
		}
		drifted, err := c.Karpenter.DriftedNodeClaims(ctx)
		if err != nil {
			return nil, fmt.Errorf("collecting drifted Karpenter NodeClaims: %w", err)
		}
		driftedByPool := map[string]int{}
		for _, d := range drifted {
			driftedByPool[d.NodePool]++
		}
		for _, np := range nodePools {
			observed.Karpenter.NodePools = append(observed.Karpenter.NodePools, ObservedNodePool{
				Name: np.Name, Healthy: np.Healthy, DriftedNodeClaims: driftedByPool[np.Name],
				DisruptionBudgetNodes: np.DisruptionBudgetNodes,
			})
		}
	}

	return observed, nil
}
