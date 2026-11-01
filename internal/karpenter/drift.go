package karpenter

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeClaim is the subset of karpenter.sh/v1 NodeClaim status used for drift-aware
// node replacement orchestration.
type NodeClaim struct {
	Name     string
	NodePool string
	Drifted  bool
	Ready    bool
}

// ListNodeClaims returns all NodeClaim objects in the cluster.
func (c *Client) ListNodeClaims(ctx context.Context) ([]NodeClaim, error) {
	list, err := c.Dynamic.Resource(NodeClaimGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing Karpenter NodeClaims: %w", err)
	}

	result := make([]NodeClaim, 0, len(list.Items))
	for _, item := range list.Items {
		labels := item.GetLabels()
		result = append(result, NodeClaim{
			Name:     item.GetName(),
			NodePool: labels["karpenter.sh/nodepool"],
			Drifted:  conditionTrue(&item, "Drifted"),
			Ready:    conditionTrue(&item, "Ready"),
		})
	}
	return result, nil
}

// DriftedNodeClaims returns only the NodeClaims currently marked Drifted.
func (c *Client) DriftedNodeClaims(ctx context.Context) ([]NodeClaim, error) {
	all, err := c.ListNodeClaims(ctx)
	if err != nil {
		return nil, err
	}
	var drifted []NodeClaim
	for _, n := range all {
		if n.Drifted {
			drifted = append(drifted, n)
		}
	}
	return drifted, nil
}
