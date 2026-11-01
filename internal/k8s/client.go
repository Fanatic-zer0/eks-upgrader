// Package k8s provides read-only Kubernetes API access used for preflight checks,
// live-state collection, and post-upgrade verification. This application never writes
// to the Kubernetes API directly for cluster-mutating purposes (eksctl and Karpenter
// own that); k8s is used strictly for observation and validation.
package k8s

import (
	"fmt"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client wraps a Kubernetes clientset interface so that k8s.io/client-go/kubernetes/fake
// can be substituted directly in tests.
type Client struct {
	Clientset kubernetes.Interface
}

// NewClientFromKubeconfig builds a Client from a kubeconfig file path (or the default
// loading rules if path is empty).
func NewClientFromKubeconfig(path string) (*Client, error) {
	cfg, err := RESTConfigFromKubeconfig(path)
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("building kubernetes clientset: %w", err)
	}
	return &Client{Clientset: cs}, nil
}

// RESTConfigFromKubeconfig loads a *rest.Config from a kubeconfig file path (or the
// default loading rules if path is empty), shared by the typed and dynamic clients.
func RESTConfigFromKubeconfig(path string) (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if path != "" {
		loadingRules.ExplicitPath = path
	}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("building kubeconfig client config: %w", err)
	}
	return cfg, nil
}

// NewInClusterClient builds a Client using in-cluster service account credentials,
// used when eks-upgrader runs as a Kubernetes Job.
func NewInClusterClient() (*Client, error) {
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); err != nil {
		return nil, fmt.Errorf("not running inside a Kubernetes pod (no service account token found): %w", err)
	}
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("loading in-cluster config: %w", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("building in-cluster kubernetes clientset: %w", err)
	}
	return &Client{Clientset: cs}, nil
}
