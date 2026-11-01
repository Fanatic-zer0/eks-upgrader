package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodInfo is the subset of Pod state used for readiness/preflight checks.
type PodInfo struct {
	Namespace string
	Name      string
	NodeName  string
	Phase     corev1.PodPhase
	Ready     bool
}

// ListPods returns all pods in namespace, or across all namespaces if empty.
func (c *Client) ListPods(ctx context.Context, namespace string) ([]PodInfo, error) {
	list, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pods in namespace %q: %w", namespace, err)
	}

	result := make([]PodInfo, 0, len(list.Items))
	for _, p := range list.Items {
		ready := false
		for _, cond := range p.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		result = append(result, PodInfo{
			Namespace: p.Namespace,
			Name:      p.Name,
			NodeName:  p.Spec.NodeName,
			Phase:     p.Status.Phase,
			Ready:     ready,
		})
	}
	return result, nil
}

// PendingPods returns all pods across all namespaces stuck in the Pending phase.
func (c *Client) PendingPods(ctx context.Context) ([]PodInfo, error) {
	all, err := c.ListPods(ctx, "")
	if err != nil {
		return nil, err
	}
	var pending []PodInfo
	for _, p := range all {
		if p.Phase == corev1.PodPending {
			pending = append(pending, p)
		}
	}
	return pending, nil
}
