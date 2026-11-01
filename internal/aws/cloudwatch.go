package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// CloudWatchReader is the read-only surface of the CloudWatch API this application
// depends on for control-plane health signals.
//
// Note: EKS does not natively emit API-server error-rate metrics to CloudWatch unless
// control-plane logging or Container Insights is enabled; callers should treat a zero
// result here as inconclusive rather than "healthy" and fall back to a direct
// apiserver /metrics scrape (see internal/readiness/scanners).
type CloudWatchReader interface {
	// SumMetric returns the sum statistic for namespace/metricName/dimensions over the
	// trailing window.
	SumMetric(ctx context.Context, namespace, metricName string, dimensions map[string]string, window time.Duration) (float64, error)
}

// CloudWatchClient implements CloudWatchReader using the real AWS SDK v2 client.
type CloudWatchClient struct {
	client *cloudwatch.Client
}

func NewCloudWatchClient(client *cloudwatch.Client) *CloudWatchClient {
	return &CloudWatchClient{client: client}
}

func (c *CloudWatchClient) SumMetric(ctx context.Context, namespace, metricName string, dimensions map[string]string, window time.Duration) (float64, error) {
	now := time.Now().UTC()

	dims := make([]types.Dimension, 0, len(dimensions))
	for k, v := range dimensions {
		dims = append(dims, types.Dimension{Name: aws.String(k), Value: aws.String(v)})
	}

	out, err := c.client.GetMetricStatistics(ctx, &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String(namespace),
		MetricName: aws.String(metricName),
		Dimensions: dims,
		StartTime:  aws.Time(now.Add(-window)),
		EndTime:    aws.Time(now),
		Period:     aws.Int32(int32(window.Seconds())),
		Statistics: []types.Statistic{types.StatisticSum},
	})
	if err != nil {
		return 0, fmt.Errorf("querying CloudWatch metric %s/%s: %w", namespace, metricName, err)
	}

	var total float64
	for _, dp := range out.Datapoints {
		if dp.Sum != nil {
			total += *dp.Sum
		}
	}
	return total, nil
}
