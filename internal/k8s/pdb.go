package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PDBInfo is the subset of PodDisruptionBudget state used for health-gate checks.
type PDBInfo struct {
	Namespace          string
	Name               string
	DisruptionsAllowed int32
	CurrentHealthy     int32
	DesiredHealthy     int32
	ExpectedPods       int32
}

// ListPDBs returns all PodDisruptionBudgets in namespace.
func (c *Client) ListPDBs(ctx context.Context, namespace string) ([]PDBInfo, error) {
	list, err := c.Clientset.PolicyV1().PodDisruptionBudgets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing PDBs in namespace %q: %w", namespace, err)
	}

	result := make([]PDBInfo, 0, len(list.Items))
	for _, p := range list.Items {
		result = append(result, PDBInfo{
			Namespace:          p.Namespace,
			Name:               p.Name,
			DisruptionsAllowed: p.Status.DisruptionsAllowed,
			CurrentHealthy:     p.Status.CurrentHealthy,
			DesiredHealthy:     p.Status.DesiredHealthy,
			ExpectedPods:       p.Status.ExpectedPods,
		})
	}
	return result, nil
}

// ValidatePDBsForNamespaces checks that every PDB in the given critical namespaces
// currently permits at least one voluntary disruption, returning human-readable issue
// descriptions for any that do not (a hard blocker for rolling node-group upgrades).
func (c *Client) ValidatePDBsForNamespaces(ctx context.Context, namespaces []string) ([]string, error) {
	var issues []string
	for _, ns := range namespaces {
		pdbs, err := c.ListPDBs(ctx, ns)
		if err != nil {
			return nil, err
		}
		for _, p := range pdbs {
			if p.DisruptionsAllowed <= 0 {
				issues = append(issues, fmt.Sprintf("PDB %s/%s currently allows 0 disruptions (desiredHealthy=%d currentHealthy=%d)",
					p.Namespace, p.Name, p.DesiredHealthy, p.CurrentHealthy))
			}
		}
	}
	return issues, nil
}
