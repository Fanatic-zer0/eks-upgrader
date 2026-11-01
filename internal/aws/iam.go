package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
)

// RoleInfo is the subset of IAM role state needed for preflight/reconciliation checks.
type RoleInfo struct {
	RoleName string
	Arn      string
	Exists   bool
}

// IAMReader is the read-only surface of the IAM API this application depends on.
type IAMReader interface {
	GetRole(ctx context.Context, roleName string) (*RoleInfo, error)
}

// IAMClient implements IAMReader using the real AWS SDK v2 IAM client.
type IAMClient struct {
	client *iam.Client
}

func NewIAMClient(client *iam.Client) *IAMClient {
	return &IAMClient{client: client}
}

func (c *IAMClient) GetRole(ctx context.Context, roleName string) (*RoleInfo, error) {
	out, err := c.client.GetRole(ctx, &iam.GetRoleInput{RoleName: aws.String(roleName)})
	if err != nil {
		return &RoleInfo{RoleName: roleName, Exists: false}, fmt.Errorf("getting IAM role %q: %w", roleName, err)
	}
	if out.Role == nil {
		return &RoleInfo{RoleName: roleName, Exists: false}, nil
	}
	return &RoleInfo{
		RoleName: aws.ToString(out.Role.RoleName),
		Arn:      aws.ToString(out.Role.Arn),
		Exists:   true,
	}, nil
}
