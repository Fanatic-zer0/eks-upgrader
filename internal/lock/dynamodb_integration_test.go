//go:build integration

package lock

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// These tests exercise DynamoDBLocker against a real DynamoDB-API-compatible endpoint
// (e.g. floci/LocalStack on :4566). Run with:
//
//	AWS_ENDPOINT_URL=http://localhost:4566 AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test \
//	  go test -tags integration ./internal/lock/... -run DynamoDB -v
//
// The table must already exist (see scripts/localstack-bootstrap.sh):
//
//	aws --endpoint-url http://localhost:4566 dynamodb create-table \
//	  --table-name eks-upgrader-locks \
//	  --attribute-definitions AttributeName=cluster,AttributeType=S \
//	  --key-schema AttributeName=cluster,KeyType=HASH \
//	  --billing-mode PAY_PER_REQUEST
func newIntegrationLocker(t *testing.T) *DynamoDBLocker {
	t.Helper()
	if os.Getenv("AWS_ENDPOINT_URL") == "" {
		t.Skip("AWS_ENDPOINT_URL not set; skipping DynamoDB integration test")
	}
	cfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		t.Fatalf("LoadDefaultConfig: %v", err)
	}
	return NewDynamoDBLocker(dynamodb.NewFromConfig(cfg), "eks-upgrader-locks")
}

func TestDynamoDBLocker_Integration_AcquireReleaseLifecycle(t *testing.T) {
	ctx := context.Background()
	l := newIntegrationLocker(t)
	cluster := "integration-test-cluster-1"

	_ = l.ForceRelease(ctx, cluster) // best-effort cleanup from a prior run

	lk, err := l.Acquire(ctx, cluster, "run-int-1", "ci", time.Hour)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if lk.RunID != "run-int-1" {
		t.Fatalf("unexpected runID: %s", lk.RunID)
	}

	if _, err := l.Acquire(ctx, cluster, "run-int-2", "ci", time.Hour); !errors.Is(err, ErrHeld) {
		t.Fatalf("expected ErrHeld for concurrent acquire, got %v", err)
	}

	status, err := l.Status(ctx, cluster)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status == nil || status.RunID != "run-int-1" {
		t.Fatalf("unexpected status: %+v", status)
	}

	if err := l.Release(ctx, cluster, "run-int-1"); err != nil {
		t.Fatalf("Release: %v", err)
	}

	status, err = l.Status(ctx, cluster)
	if err != nil {
		t.Fatalf("Status after release: %v", err)
	}
	if status != nil {
		t.Fatalf("expected no lock after release, got %+v", status)
	}
}

func TestDynamoDBLocker_Integration_ExpiredLockCanBeStolen(t *testing.T) {
	ctx := context.Background()
	l := newIntegrationLocker(t)
	cluster := "integration-test-cluster-2"

	_ = l.ForceRelease(ctx, cluster)

	if _, err := l.Acquire(ctx, cluster, "run-int-1", "ci", -time.Minute); err != nil {
		t.Fatalf("Acquire (pre-expired): %v", err)
	}

	lk, err := l.Acquire(ctx, cluster, "run-int-2", "ci", time.Hour)
	if err != nil {
		t.Fatalf("expected Acquire to succeed over an expired lock, got %v", err)
	}
	if lk.RunID != "run-int-2" {
		t.Fatalf("unexpected runID: %s", lk.RunID)
	}

	_ = l.ForceRelease(ctx, cluster)
}

func TestDynamoDBLocker_Integration_ForceRelease(t *testing.T) {
	ctx := context.Background()
	l := newIntegrationLocker(t)
	cluster := "integration-test-cluster-3"

	_ = l.ForceRelease(ctx, cluster)

	if _, err := l.Acquire(ctx, cluster, "run-int-1", "ci", time.Hour); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := l.ForceRelease(ctx, cluster); err != nil {
		t.Fatalf("ForceRelease: %v", err)
	}
	if err := l.ForceRelease(ctx, cluster); !errors.Is(err, ErrNotHeld) {
		t.Fatalf("expected ErrNotHeld on second ForceRelease, got %v", err)
	}
}
