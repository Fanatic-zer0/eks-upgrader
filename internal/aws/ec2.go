package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// SubnetCapacity reports the available IP addresses in a single subnet.
type SubnetCapacity struct {
	SubnetID             string
	AvailabilityZone     string
	AvailableIPAddresses int32
}

// EC2Reader is the read-only surface of the EC2 API this application depends on.
type EC2Reader interface {
	DescribeSubnetCapacity(ctx context.Context, subnetIDs []string) ([]SubnetCapacity, error)
}

// EC2Client implements EC2Reader using the real AWS SDK v2 EC2 client.
type EC2Client struct {
	client *ec2.Client
}

func NewEC2Client(client *ec2.Client) *EC2Client {
	return &EC2Client{client: client}
}

func (c *EC2Client) DescribeSubnetCapacity(ctx context.Context, subnetIDs []string) ([]SubnetCapacity, error) {
	if len(subnetIDs) == 0 {
		return nil, nil
	}
	out, err := c.client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{SubnetIds: subnetIDs})
	if err != nil {
		return nil, fmt.Errorf("describing subnets %v: %w", subnetIDs, err)
	}

	result := make([]SubnetCapacity, 0, len(out.Subnets))
	for _, s := range out.Subnets {
		result = append(result, SubnetCapacity{
			SubnetID:             aws.ToString(s.SubnetId),
			AvailabilityZone:     aws.ToString(s.AvailabilityZone),
			AvailableIPAddresses: aws.ToInt32(s.AvailableIpAddressCount),
		})
	}
	return result, nil
}
