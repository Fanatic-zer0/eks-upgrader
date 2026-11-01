//go:build integration

package aws

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/iam"
)

// These tests exercise the real AWS SDK v2 clients against a AWS-API-compatible
// endpoint (e.g. floci/LocalStack on :4566). Run with:
//
//	AWS_ENDPOINT_URL=http://localhost:4566 AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test \
//	  go test -tags integration ./internal/aws/... -v
func skipUnlessEndpointConfigured(t *testing.T) {
	t.Helper()
	if os.Getenv("AWS_ENDPOINT_URL") == "" {
		t.Skip("AWS_ENDPOINT_URL not set; skipping AWS integration test")
	}
}

func TestIAMClient_Integration_GetRole(t *testing.T) {
	skipUnlessEndpointConfigured(t)
	ctx := context.Background()

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		t.Fatalf("LoadDefaultConfig: %v", err)
	}
	client := NewIAMClient(iam.NewFromConfig(cfg))

	info, err := client.GetRole(ctx, "eks-upgrader-test-cluster-role")
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if !info.Exists || info.Arn == "" {
		t.Fatalf("unexpected role info: %+v", info)
	}
}

func TestIAMClient_Integration_GetRole_NotFound(t *testing.T) {
	skipUnlessEndpointConfigured(t)
	ctx := context.Background()

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		t.Fatalf("LoadDefaultConfig: %v", err)
	}
	client := NewIAMClient(iam.NewFromConfig(cfg))

	info, err := client.GetRole(ctx, "role-that-does-not-exist")
	if err == nil {
		t.Fatal("expected an error for a nonexistent role")
	}
	if info.Exists {
		t.Fatalf("expected Exists=false, got %+v", info)
	}
}

func TestEC2Client_Integration_DescribeSubnetCapacity(t *testing.T) {
	skipUnlessEndpointConfigured(t)
	ctx := context.Background()

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		t.Fatalf("LoadDefaultConfig: %v", err)
	}
	ec2Raw := ec2.NewFromConfig(cfg)

	vpcOut, err := ec2Raw.CreateVpc(ctx, &ec2.CreateVpcInput{CidrBlock: aws.String("10.42.0.0/16")})
	if err != nil {
		t.Fatalf("CreateVpc: %v", err)
	}
	subnetOut, err := ec2Raw.CreateSubnet(ctx, &ec2.CreateSubnetInput{
		VpcId: vpcOut.Vpc.VpcId, CidrBlock: aws.String("10.42.1.0/24"),
	})
	if err != nil {
		t.Fatalf("CreateSubnet: %v", err)
	}

	client := NewEC2Client(ec2Raw)
	capacities, err := client.DescribeSubnetCapacity(ctx, []string{aws.ToString(subnetOut.Subnet.SubnetId)})
	if err != nil {
		t.Fatalf("DescribeSubnetCapacity: %v", err)
	}
	if len(capacities) != 1 || capacities[0].AvailableIPAddresses <= 0 {
		t.Fatalf("unexpected subnet capacity: %+v", capacities)
	}
}

func TestCloudWatchClient_Integration_SumMetric(t *testing.T) {
	skipUnlessEndpointConfigured(t)
	ctx := context.Background()

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		t.Fatalf("LoadDefaultConfig: %v", err)
	}
	cwRaw := cloudwatch.NewFromConfig(cfg)

	_, err = cwRaw.PutMetricData(ctx, &cloudwatch.PutMetricDataInput{
		Namespace: aws.String("eks-upgrader/integration-test"),
		MetricData: []types.MetricDatum{
			{MetricName: aws.String("ApiServer5xx"), Value: aws.Float64(3)},
		},
	})
	if err != nil {
		t.Fatalf("PutMetricData: %v", err)
	}

	client := NewCloudWatchClient(cwRaw)
	// CloudWatch in community LocalStack editions may not fully simulate datapoint
	// aggregation windows; this call is primarily verifying wire-level compatibility
	// (the request is accepted and parsed) rather than exact aggregation correctness.
	if _, err := client.SumMetric(ctx, "eks-upgrader/integration-test", "ApiServer5xx", nil, time.Hour); err != nil {
		t.Fatalf("SumMetric: %v", err)
	}
}
