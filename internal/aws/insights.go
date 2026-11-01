package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
)

// Insight is a single EKS Upgrade Insight finding, as returned by `aws eks list-insights`.
type Insight struct {
	ID                string
	Name              string
	Category          string
	KubernetesVersion string
	Status            string // PASSING | WARNING | ERROR | UNKNOWN
	Description       string
}

// EKSInsightsReader fetches EKS Upgrade Insights for a cluster. This is a read-only
// signal used by the Upgrade Readiness Analyzer; eks-upgrader never acts on insights
// except to classify and report them.
type EKSInsightsReader interface {
	ListInsights(ctx context.Context, clusterName string) ([]Insight, error)
}

func (c *EKSClient) ListInsights(ctx context.Context, clusterName string) ([]Insight, error) {
	var results []Insight
	var nextToken *string

	for {
		out, err := c.client.ListInsights(ctx, &eks.ListInsightsInput{
			ClusterName: aws.String(clusterName),
			NextToken:   nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("listing EKS upgrade insights for cluster %q: %w", clusterName, err)
		}
		for _, i := range out.Insights {
			ins := Insight{
				ID:   aws.ToString(i.Id),
				Name: aws.ToString(i.Name),
			}
			if i.Category != "" {
				ins.Category = string(i.Category)
			}
			if i.KubernetesVersion != nil {
				ins.KubernetesVersion = aws.ToString(i.KubernetesVersion)
			}
			if i.InsightStatus != nil {
				ins.Status = string(i.InsightStatus.Status)
				ins.Description = aws.ToString(i.InsightStatus.Reason)
			}
			results = append(results, ins)
		}
		if out.NextToken == nil || aws.ToString(out.NextToken) == "" {
			break
		}
		nextToken = out.NextToken
	}
	return results, nil
}
