// Package aws provides thin, read-only wrappers around AWS SDK v2 service clients.
// This application never calls EKS/CloudFormation mutation APIs directly; all mutation
// goes through eksctl. These clients exist solely for live-state collection and
// read-only preflight/readiness validation.
package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
)

// ClusterInfo is the subset of EKS cluster state needed for reconciliation/preflight.
type ClusterInfo struct {
	Name            string
	Status          string
	Version         string
	Endpoint        string
	OIDCIssuer      string
	Tags            map[string]string
	LoggingEnabled  []string
	PlatformVersion string
}

// EKSReader is the read-only surface of the EKS API this application depends on.
type EKSReader interface {
	DescribeCluster(ctx context.Context, name string) (*ClusterInfo, error)
}

// EKSClient implements EKSReader using the real AWS SDK v2 EKS client.
type EKSClient struct {
	client *eks.Client
}

func NewEKSClient(client *eks.Client) *EKSClient {
	return &EKSClient{client: client}
}

func (c *EKSClient) DescribeCluster(ctx context.Context, name string) (*ClusterInfo, error) {
	out, err := c.client.DescribeCluster(ctx, &eks.DescribeClusterInput{Name: aws.String(name)})
	if err != nil {
		return nil, fmt.Errorf("describing EKS cluster %q: %w", name, err)
	}
	if out.Cluster == nil {
		return nil, fmt.Errorf("EKS cluster %q not found", name)
	}
	cl := out.Cluster

	info := &ClusterInfo{
		Name:   aws.ToString(cl.Name),
		Status: string(cl.Status),
		Tags:   cl.Tags,
	}
	if cl.Version != nil {
		info.Version = aws.ToString(cl.Version)
	}
	if cl.Endpoint != nil {
		info.Endpoint = aws.ToString(cl.Endpoint)
	}
	if cl.Identity != nil && cl.Identity.Oidc != nil {
		info.OIDCIssuer = aws.ToString(cl.Identity.Oidc.Issuer)
	}
	if cl.PlatformVersion != nil {
		info.PlatformVersion = aws.ToString(cl.PlatformVersion)
	}
	if cl.Logging != nil {
		for _, lg := range cl.Logging.ClusterLogging {
			if lg.Enabled != nil && *lg.Enabled {
				for _, t := range lg.Types {
					info.LoggingEnabled = append(info.LoggingEnabled, string(t))
				}
			}
		}
	}
	return info, nil
}

// ControlPlaneLoggingEnabled reports whether any control-plane log type is enabled,
// which is a prerequisite for CloudWatch-Logs-derived API server error metrics.
func ControlPlaneLoggingEnabled(enabled []string) bool {
	return len(enabled) > 0
}
