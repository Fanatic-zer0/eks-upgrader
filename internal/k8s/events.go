package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EventInfo is a simplified Kubernetes Event used for diagnostics and remediation hints.
type EventInfo struct {
	Namespace     string
	Name          string
	Reason        string
	Message       string
	Type          string
	Count         int32
	LastTimestamp string
}

// ListRecentEvents returns all events in namespace (or all namespaces if empty),
// most useful for diagnosing node/pod/Karpenter failures during a paused run.
func (c *Client) ListRecentEvents(ctx context.Context, namespace string) ([]EventInfo, error) {
	list, err := c.Clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing events in namespace %q: %w", namespace, err)
	}

	result := make([]EventInfo, 0, len(list.Items))
	for _, e := range list.Items {
		result = append(result, EventInfo{
			Namespace:     e.Namespace,
			Name:          e.Name,
			Reason:        e.Reason,
			Message:       e.Message,
			Type:          e.Type,
			Count:         e.Count,
			LastTimestamp: e.LastTimestamp.String(),
		})
	}
	return result, nil
}
