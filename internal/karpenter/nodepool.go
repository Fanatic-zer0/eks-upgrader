package karpenter

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// NodePool is the subset of karpenter.sh/v1 NodePool status used for preflight checks.
type NodePool struct {
	Name                  string
	Healthy               bool
	Limits                map[string]string
	DisruptionBudgetNodes []string
}

func conditionTrue(obj *unstructured.Unstructured, condType string) bool {
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found {
		return false
	}
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] == condType {
			return cond["status"] == "True"
		}
	}
	return false
}

// ListNodePools returns all NodePool objects in the cluster.
func (c *Client) ListNodePools(ctx context.Context) ([]NodePool, error) {
	list, err := c.Dynamic.Resource(NodePoolGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing Karpenter NodePools: %w", err)
	}

	result := make([]NodePool, 0, len(list.Items))
	for _, item := range list.Items {
		np := NodePool{
			Name:    item.GetName(),
			Healthy: conditionTrue(&item, "Ready"),
		}
		limits, found, _ := unstructured.NestedStringMap(item.Object, "spec", "limits")
		if found {
			np.Limits = limits
		}

		budgets, found, _ := unstructured.NestedSlice(item.Object, "spec", "disruption", "budgets")
		if found {
			for _, b := range budgets {
				budget, ok := b.(map[string]interface{})
				if !ok {
					continue
				}
				if nodes, ok := budget["nodes"].(string); ok {
					np.DisruptionBudgetNodes = append(np.DisruptionBudgetNodes, nodes)
				}
			}
		}
		result = append(result, np)
	}
	return result, nil
}
