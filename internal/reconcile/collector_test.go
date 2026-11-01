package reconcile

import (
	"context"
	"fmt"
	"testing"

	"eks-upgrader/internal/aws"
	"eks-upgrader/internal/k8s"
	"eks-upgrader/internal/karpenter"
)

type fakeEKSReader struct {
	cluster    *aws.ClusterInfo
	nodegroups map[string]*aws.NodeGroupInfo
	addons     []aws.AddonInfo
	addonsErr  error
}

func (f *fakeEKSReader) DescribeCluster(ctx context.Context, name string) (*aws.ClusterInfo, error) {
	return f.cluster, nil
}
func (f *fakeEKSReader) ListNodegroupNames(ctx context.Context, clusterName string) ([]string, error) {
	var names []string
	for n := range f.nodegroups {
		names = append(names, n)
	}
	return names, nil
}
func (f *fakeEKSReader) DescribeNodegroup(ctx context.Context, clusterName, nodegroupName string) (*aws.NodeGroupInfo, error) {
	return f.nodegroups[nodegroupName], nil
}
func (f *fakeEKSReader) ListAddons(ctx context.Context, clusterName string) ([]aws.AddonInfo, error) {
	if f.addonsErr != nil {
		return nil, f.addonsErr
	}
	return f.addons, nil
}

type fakeNodeGroupNodesReader struct{}

func (f *fakeNodeGroupNodesReader) NodesForNodeGroup(ctx context.Context, nodegroupName string) ([]k8s.NodeInfo, error) {
	return nil, nil
}

type fakeKarpenterReader struct {
	nodeClasses []karpenter.EC2NodeClass
	nodePools   []karpenter.NodePool
	drifted     []karpenter.NodeClaim
}

func (f *fakeKarpenterReader) ListEC2NodeClasses(ctx context.Context) ([]karpenter.EC2NodeClass, error) {
	return f.nodeClasses, nil
}
func (f *fakeKarpenterReader) ListNodePools(ctx context.Context) ([]karpenter.NodePool, error) {
	return f.nodePools, nil
}
func (f *fakeKarpenterReader) DriftedNodeClaims(ctx context.Context) ([]karpenter.NodeClaim, error) {
	return f.drifted, nil
}

func TestCollector_Collect(t *testing.T) {
	eksReader := &fakeEKSReader{
		cluster: &aws.ClusterInfo{Name: "prod", Version: "1.30", Status: "ACTIVE", Endpoint: "https://example.com"},
		nodegroups: map[string]*aws.NodeGroupInfo{
			"system": {Name: "system", Status: "ACTIVE", KubernetesVersion: "1.30", DesiredSize: 2, MinSize: 2, MaxSize: 4},
		},
		addons: []aws.AddonInfo{{Name: "vpc-cni", Version: "v1.19.0", Status: "ACTIVE"}},
	}
	karpReader := &fakeKarpenterReader{
		nodeClasses: []karpenter.EC2NodeClass{{Name: "default", Healthy: true, AMIAlias: "al2023@v20250918"}},
		nodePools:   []karpenter.NodePool{{Name: "general-purpose", Healthy: true, DisruptionBudgetNodes: []string{"10%"}}},
		drifted:     []karpenter.NodeClaim{{Name: "claim-1", NodePool: "general-purpose", Drifted: true}},
	}

	c := &Collector{EKS: eksReader, K8s: &fakeNodeGroupNodesReader{}, Karpenter: karpReader}

	observed, err := c.Collect(context.Background(), "prod", "ap-south-1", true)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if observed.Cluster.CurrentKubernetesVersion != "1.30" {
		t.Fatalf("unexpected cluster version: %+v", observed.Cluster)
	}
	if len(observed.ManagedNodeGroups) != 1 || observed.ManagedNodeGroups[0].Name != "system" {
		t.Fatalf("unexpected node groups: %+v", observed.ManagedNodeGroups)
	}
	if len(observed.Addons) != 1 || observed.Addons[0].Health != "HEALTHY" {
		t.Fatalf("unexpected addons: %+v", observed.Addons)
	}
	if !observed.Karpenter.Installed || len(observed.Karpenter.NodePools) != 1 || observed.Karpenter.NodePools[0].DriftedNodeClaims != 1 {
		t.Fatalf("unexpected karpenter state: %+v", observed.Karpenter)
	}
}

func TestCollector_Collect_KarpenterDisabledSkipsCollection(t *testing.T) {
	eksReader := &fakeEKSReader{cluster: &aws.ClusterInfo{Version: "1.30", Status: "ACTIVE"}, nodegroups: map[string]*aws.NodeGroupInfo{}}
	karpReader := &fakeKarpenterReader{}

	c := &Collector{EKS: eksReader, K8s: &fakeNodeGroupNodesReader{}, Karpenter: karpReader}
	observed, err := c.Collect(context.Background(), "prod", "ap-south-1", false)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if observed.Karpenter.Installed {
		t.Fatal("expected Karpenter.Installed to be false when karpenterEnabled=false")
	}
}

func TestCollector_Collect_AddonListingFailureDegradesGracefully(t *testing.T) {
	// Regression test: discovered via floci/LocalStack integration testing, where
	// ListAddons returned a malformed response and previously aborted the entire
	// collection even though cluster/node-group data was otherwise available.
	eksReader := &fakeEKSReader{
		cluster: &aws.ClusterInfo{Version: "1.30", Status: "ACTIVE"},
		nodegroups: map[string]*aws.NodeGroupInfo{
			"system": {Name: "system", Status: "ACTIVE", KubernetesVersion: "1.30"},
		},
		addonsErr: fmt.Errorf("simulated ListAddons failure"),
	}

	c := &Collector{EKS: eksReader, K8s: &fakeNodeGroupNodesReader{}, Karpenter: &fakeKarpenterReader{}}
	observed, err := c.Collect(context.Background(), "prod", "ap-south-1", false)
	if err != nil {
		t.Fatalf("expected Collect to succeed despite ListAddons failure, got: %v", err)
	}
	if len(observed.ManagedNodeGroups) != 1 {
		t.Fatalf("expected node group data to still be collected, got: %+v", observed.ManagedNodeGroups)
	}
	if observed.Addons != nil {
		t.Fatalf("expected nil/empty addons after a ListAddons failure, got: %+v", observed.Addons)
	}
}
