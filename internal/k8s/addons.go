package k8s

import (
	"context"
	"fmt"
)

// knownAddonWorkloads maps EKS-managed add-on names to the namespace/name/kind of the
// workload object that implements them, so their health can be cross-checked against
// the EKS DescribeAddon status.
var knownAddonWorkloads = map[string]struct {
	Namespace string
	Name      string
	Kind      string // Deployment | DaemonSet
}{
	"coredns":    {Namespace: "kube-system", Name: "coredns", Kind: "Deployment"},
	"kube-proxy": {Namespace: "kube-system", Name: "kube-proxy", Kind: "DaemonSet"},
	"vpc-cni":    {Namespace: "kube-system", Name: "aws-node", Kind: "DaemonSet"},
}

// AddonWorkloadHealth checks the underlying Kubernetes workload for a known EKS
// add-on, complementing (not replacing) the authoritative AWS EKS DescribeAddon status.
func (c *Client) AddonWorkloadHealth(ctx context.Context, addonName string) (*WorkloadHealth, error) {
	ref, ok := knownAddonWorkloads[addonName]
	if !ok {
		return nil, fmt.Errorf("no known workload mapping for add-on %q", addonName)
	}

	switch ref.Kind {
	case "Deployment":
		deployments, err := c.ListDeployments(ctx, ref.Namespace)
		if err != nil {
			return nil, err
		}
		for _, d := range deployments {
			if d.Name == ref.Name {
				return &d, nil
			}
		}
	case "DaemonSet":
		daemonsets, err := c.ListDaemonSets(ctx, ref.Namespace)
		if err != nil {
			return nil, err
		}
		for _, d := range daemonsets {
			if d.Name == ref.Name {
				return &d, nil
			}
		}
	}
	return nil, fmt.Errorf("workload %s/%s not found for add-on %q", ref.Namespace, ref.Name, addonName)
}
