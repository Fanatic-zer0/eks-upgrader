package k8s

import "context"

// FetchRawMetrics returns the raw Prometheus-format text from the apiserver /metrics
// endpoint, equivalent to `kubectl get --raw /metrics`. Used to detect deprecated API
// usage via the apiserver_requested_deprecated_apis gauge.
func (c *Client) FetchRawMetrics(ctx context.Context) ([]byte, error) {
	return c.Clientset.Discovery().RESTClient().Get().AbsPath("/metrics").DoRaw(ctx)
}
