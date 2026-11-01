package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkloadHealth summarizes replica health for a Deployment, StatefulSet, or DaemonSet.
type WorkloadHealth struct {
	Namespace string
	Name      string
	Kind      string
	Desired   int32
	Ready     int32
	Healthy   bool
}

// ListDeployments returns health summaries for all Deployments in namespace.
func (c *Client) ListDeployments(ctx context.Context, namespace string) ([]WorkloadHealth, error) {
	list, err := c.Clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing deployments in namespace %q: %w", namespace, err)
	}
	result := make([]WorkloadHealth, 0, len(list.Items))
	for _, d := range list.Items {
		desired := int32(1)
		if d.Spec.Replicas != nil {
			desired = *d.Spec.Replicas
		}
		result = append(result, WorkloadHealth{
			Namespace: d.Namespace, Name: d.Name, Kind: "Deployment",
			Desired: desired, Ready: d.Status.ReadyReplicas, Healthy: d.Status.ReadyReplicas >= desired,
		})
	}
	return result, nil
}

// ListStatefulSets returns health summaries for all StatefulSets in namespace.
func (c *Client) ListStatefulSets(ctx context.Context, namespace string) ([]WorkloadHealth, error) {
	list, err := c.Clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing statefulsets in namespace %q: %w", namespace, err)
	}
	result := make([]WorkloadHealth, 0, len(list.Items))
	for _, s := range list.Items {
		desired := int32(1)
		if s.Spec.Replicas != nil {
			desired = *s.Spec.Replicas
		}
		result = append(result, WorkloadHealth{
			Namespace: s.Namespace, Name: s.Name, Kind: "StatefulSet",
			Desired: desired, Ready: s.Status.ReadyReplicas, Healthy: s.Status.ReadyReplicas >= desired,
		})
	}
	return result, nil
}

// ListDaemonSets returns health summaries for all DaemonSets in namespace.
func (c *Client) ListDaemonSets(ctx context.Context, namespace string) ([]WorkloadHealth, error) {
	list, err := c.Clientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing daemonsets in namespace %q: %w", namespace, err)
	}
	result := make([]WorkloadHealth, 0, len(list.Items))
	for _, d := range list.Items {
		result = append(result, WorkloadHealth{
			Namespace: d.Namespace, Name: d.Name, Kind: "DaemonSet",
			Desired: d.Status.DesiredNumberScheduled, Ready: d.Status.NumberReady,
			Healthy: d.Status.NumberReady >= d.Status.DesiredNumberScheduled,
		})
	}
	return result, nil
}
