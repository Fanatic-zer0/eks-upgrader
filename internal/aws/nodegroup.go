package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
)

// NodeGroupInfo is the subset of managed node group state needed for live-state
// reconciliation and preflight checks. It is sourced from the read-only EKS
// DescribeNodegroup API (equivalent, non-mutating data to `eksctl get nodegroup -o json`).
type NodeGroupInfo struct {
	Name              string
	Status            string
	KubernetesVersion string
	InstanceTypes     []string
	DesiredSize       int32
	MinSize           int32
	MaxSize           int32
	AMIType           string
	Labels            map[string]string
	Taints            []NodeGroupTaint
	LaunchTemplateID  string
	LaunchTemplateVer string
}

type NodeGroupTaint struct {
	Key    string
	Value  string
	Effect string
}

// ListNodegroupNames returns the names of all managed node groups in the cluster.
func (c *EKSClient) ListNodegroupNames(ctx context.Context, clusterName string) ([]string, error) {
	var names []string
	var nextToken *string
	for {
		out, err := c.client.ListNodegroups(ctx, &eks.ListNodegroupsInput{ClusterName: aws.String(clusterName), NextToken: nextToken})
		if err != nil {
			return nil, fmt.Errorf("listing managed node groups for cluster %q: %w", clusterName, err)
		}
		names = append(names, out.Nodegroups...)
		if out.NextToken == nil || aws.ToString(out.NextToken) == "" {
			break
		}
		nextToken = out.NextToken
	}
	return names, nil
}

// DescribeNodegroup fetches full detail for a single managed node group.
func (c *EKSClient) DescribeNodegroup(ctx context.Context, clusterName, nodegroupName string) (*NodeGroupInfo, error) {
	out, err := c.client.DescribeNodegroup(ctx, &eks.DescribeNodegroupInput{
		ClusterName:   aws.String(clusterName),
		NodegroupName: aws.String(nodegroupName),
	})
	if err != nil {
		return nil, fmt.Errorf("describing node group %q: %w", nodegroupName, err)
	}
	ng := out.Nodegroup
	if ng == nil {
		return nil, fmt.Errorf("node group %q not found", nodegroupName)
	}

	info := &NodeGroupInfo{
		Name:          aws.ToString(ng.NodegroupName),
		Status:        string(ng.Status),
		Labels:        ng.Labels,
		InstanceTypes: ng.InstanceTypes,
		AMIType:       string(ng.AmiType),
	}
	if ng.Version != nil {
		info.KubernetesVersion = aws.ToString(ng.Version)
	}
	if ng.ScalingConfig != nil {
		info.DesiredSize = aws.ToInt32(ng.ScalingConfig.DesiredSize)
		info.MinSize = aws.ToInt32(ng.ScalingConfig.MinSize)
		info.MaxSize = aws.ToInt32(ng.ScalingConfig.MaxSize)
	}
	for _, t := range ng.Taints {
		info.Taints = append(info.Taints, NodeGroupTaint{
			Key: aws.ToString(t.Key), Value: aws.ToString(t.Value), Effect: string(t.Effect),
		})
	}
	if ng.LaunchTemplate != nil {
		info.LaunchTemplateID = aws.ToString(ng.LaunchTemplate.Id)
		info.LaunchTemplateVer = aws.ToString(ng.LaunchTemplate.Version)
	}
	return info, nil
}
