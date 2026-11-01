package karpenter

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// EC2NodeClass is the subset of karpenter.k8s.aws/v1 EC2NodeClass status used for
// AMI-drift preflight and live-state reconciliation checks.
type EC2NodeClass struct {
	Name           string
	Healthy        bool
	AMIAlias       string
	AMISelectorIDs []string
}

// ListEC2NodeClasses returns all EC2NodeClass objects in the cluster.
func (c *Client) ListEC2NodeClasses(ctx context.Context) ([]EC2NodeClass, error) {
	list, err := c.Dynamic.Resource(EC2NodeClassGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing Karpenter EC2NodeClasses: %w", err)
	}

	result := make([]EC2NodeClass, 0, len(list.Items))
	for _, item := range list.Items {
		nc := EC2NodeClass{
			Name:    item.GetName(),
			Healthy: conditionTrue(&item, "Ready"),
		}

		amiSelectorTerms, found, _ := unstructured.NestedSlice(item.Object, "spec", "amiSelectorTerms")
		if found {
			for _, t := range amiSelectorTerms {
				term, ok := t.(map[string]interface{})
				if !ok {
					continue
				}
				if alias, ok := term["alias"].(string); ok && alias != "" {
					nc.AMIAlias = alias
				}
				if id, ok := term["id"].(string); ok && id != "" {
					nc.AMISelectorIDs = append(nc.AMISelectorIDs, id)
				}
			}
		}
		result = append(result, nc)
	}
	return result, nil
}
