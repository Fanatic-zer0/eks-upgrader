package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
)

// AddonInfo is the subset of EKS-managed add-on state needed for reconciliation and
// post-upgrade health checks.
type AddonInfo struct {
	Name    string
	Version string
	Status  string // ACTIVE | CREATING | DEGRADED | DELETING | CREATE_FAILED | UPDATE_FAILED | ...
}

// Healthy reports whether the add-on's reported AWS status indicates a healthy state.
func (a AddonInfo) Healthy() bool {
	return a.Status == "ACTIVE"
}

// ListAddons returns the installed EKS-managed add-ons for the cluster with version
// and health detail.
func (c *EKSClient) ListAddons(ctx context.Context, clusterName string) ([]AddonInfo, error) {
	listOut, err := c.client.ListAddons(ctx, &eks.ListAddonsInput{ClusterName: aws.String(clusterName)})
	if err != nil {
		return nil, fmt.Errorf("listing EKS add-ons for cluster %q: %w", clusterName, err)
	}

	result := make([]AddonInfo, 0, len(listOut.Addons))
	for _, name := range listOut.Addons {
		descOut, err := c.client.DescribeAddon(ctx, &eks.DescribeAddonInput{
			ClusterName: aws.String(clusterName),
			AddonName:   aws.String(name),
		})
		if err != nil {
			return nil, fmt.Errorf("describing add-on %q: %w", name, err)
		}
		info := AddonInfo{Name: name}
		if descOut.Addon != nil {
			info.Version = aws.ToString(descOut.Addon.AddonVersion)
			info.Status = string(descOut.Addon.Status)
		}
		result = append(result, info)
	}
	return result, nil
}
