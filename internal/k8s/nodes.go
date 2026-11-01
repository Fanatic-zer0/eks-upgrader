package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeInfo is the subset of Node state used for readiness/preflight checks.
type NodeInfo struct {
	Name           string
	KubeletVersion string
	Ready          bool
	Unschedulable  bool
	Labels         map[string]string
	Taints         []corev1.Taint
}

// ListNodes returns all nodes in the cluster with their readiness and version info.
func (c *Client) ListNodes(ctx context.Context) ([]NodeInfo, error) {
	list, err := c.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	result := make([]NodeInfo, 0, len(list.Items))
	for _, n := range list.Items {
		ready := false
		for _, cond := range n.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		result = append(result, NodeInfo{
			Name:           n.Name,
			KubeletVersion: n.Status.NodeInfo.KubeletVersion,
			Ready:          ready,
			Unschedulable:  n.Spec.Unschedulable,
			Labels:         n.Labels,
			Taints:         n.Spec.Taints,
		})
	}
	return result, nil
}

// NodesForNodeGroup returns nodes belonging to the given EKS managed node group, using
// the well-known `eks.amazonaws.com/nodegroup` label.
func (c *Client) NodesForNodeGroup(ctx context.Context, nodegroupName string) ([]NodeInfo, error) {
	list, err := c.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: "eks.amazonaws.com/nodegroup=" + nodegroupName,
	})
	if err != nil {
		return nil, fmt.Errorf("listing nodes for node group %q: %w", nodegroupName, err)
	}

	result := make([]NodeInfo, 0, len(list.Items))
	for _, n := range list.Items {
		ready := false
		for _, cond := range n.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		result = append(result, NodeInfo{
			Name:           n.Name,
			KubeletVersion: n.Status.NodeInfo.KubeletVersion,
			Ready:          ready,
			Unschedulable:  n.Spec.Unschedulable,
			Labels:         n.Labels,
			Taints:         n.Spec.Taints,
		})
	}
	return result, nil
}

// AllNodesReady reports whether every node is Ready, returning the names of any that
// are not for diagnostics.
func (c *Client) AllNodesReady(ctx context.Context) (bool, []string, error) {
	nodes, err := c.ListNodes(ctx)
	if err != nil {
		return false, nil, err
	}
	var notReady []string
	for _, n := range nodes {
		if !n.Ready {
			notReady = append(notReady, n.Name)
		}
	}
	return len(notReady) == 0, notReady, nil
}
