package lock

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// DynamoDBLocker implements Locker using a DynamoDB table with a conditional write,
// safe for concurrent CI runners across machines. The table must have a string
// partition key named "cluster".
type DynamoDBLocker struct {
	client *dynamodb.Client
	table  string
}

// NewDynamoDBLocker constructs a DynamoDBLocker against the given table name.
func NewDynamoDBLocker(client *dynamodb.Client, table string) *DynamoDBLocker {
	return &DynamoDBLocker{client: client, table: table}
}

func (d *DynamoDBLocker) Acquire(ctx context.Context, cluster, runID, owner string, ttl time.Duration) (*Lock, error) {
	now := time.Now().UTC()
	lk := &Lock{
		Cluster:    cluster,
		RunID:      runID,
		Owner:      owner,
		AcquiredAt: now,
		ExpiresAt:  now.Add(ttl),
	}

	_, err := d.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(d.table),
		Item: map[string]types.AttributeValue{
			"cluster":    &types.AttributeValueMemberS{Value: cluster},
			"runID":      &types.AttributeValueMemberS{Value: runID},
			"owner":      &types.AttributeValueMemberS{Value: owner},
			"acquiredAt": &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
			"expiresAt":  &types.AttributeValueMemberS{Value: lk.ExpiresAt.Format(time.RFC3339)},
			"expiresAtUnix": &types.AttributeValueMemberN{
				Value: fmt.Sprintf("%d", lk.ExpiresAt.Unix()),
			},
		},
		// Allow the write if no item exists yet, the existing item is expired, or the
		// existing item is already held by this same run (e.g. `resume` refreshing its
		// own lock). "cluster" is a DynamoDB reserved keyword, so it must be referenced
		// via an ExpressionAttributeNames placeholder rather than literally.
		ConditionExpression: aws.String("attribute_not_exists(#cluster) OR expiresAtUnix < :now OR runID = :runID"),
		ExpressionAttributeNames: map[string]string{
			"#cluster": "cluster",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":now":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", now.Unix())},
			":runID": &types.AttributeValueMemberS{Value: runID},
		},
	})
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			existing, statusErr := d.Status(ctx, cluster)
			if statusErr == nil && existing != nil {
				return nil, fmt.Errorf("%w: held by runID=%s owner=%s since %s, expires %s",
					ErrHeld, existing.RunID, existing.Owner, existing.AcquiredAt.Format(time.RFC3339), existing.ExpiresAt.Format(time.RFC3339))
			}
			return nil, ErrHeld
		}
		return nil, fmt.Errorf("acquiring dynamodb lock: %w", err)
	}
	return lk, nil
}

func (d *DynamoDBLocker) Release(ctx context.Context, cluster, runID string) error {
	_, err := d.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(d.table),
		Key: map[string]types.AttributeValue{
			"cluster": &types.AttributeValueMemberS{Value: cluster},
		},
		ConditionExpression: aws.String("runID = :runID"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":runID": &types.AttributeValueMemberS{Value: runID},
		},
	})
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			return fmt.Errorf("lock for cluster %q is held by a different run, refusing to release", cluster)
		}
		return fmt.Errorf("releasing dynamodb lock: %w", err)
	}
	return nil
}

func (d *DynamoDBLocker) ForceRelease(ctx context.Context, cluster string) error {
	existing, err := d.Status(ctx, cluster)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrNotHeld
	}
	_, err = d.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(d.table),
		Key: map[string]types.AttributeValue{
			"cluster": &types.AttributeValueMemberS{Value: cluster},
		},
	})
	if err != nil {
		return fmt.Errorf("force releasing dynamodb lock: %w", err)
	}
	return nil
}

func (d *DynamoDBLocker) Status(ctx context.Context, cluster string) (*Lock, error) {
	out, err := d.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.table),
		Key: map[string]types.AttributeValue{
			"cluster": &types.AttributeValueMemberS{Value: cluster},
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("reading dynamodb lock status: %w", err)
	}
	if out.Item == nil {
		return nil, nil
	}

	lk := &Lock{Cluster: cluster}
	if v, ok := out.Item["runID"].(*types.AttributeValueMemberS); ok {
		lk.RunID = v.Value
	}
	if v, ok := out.Item["owner"].(*types.AttributeValueMemberS); ok {
		lk.Owner = v.Value
	}
	if v, ok := out.Item["acquiredAt"].(*types.AttributeValueMemberS); ok {
		lk.AcquiredAt, _ = time.Parse(time.RFC3339, v.Value)
	}
	if v, ok := out.Item["expiresAt"].(*types.AttributeValueMemberS); ok {
		lk.ExpiresAt, _ = time.Parse(time.RFC3339, v.Value)
	}
	return lk, nil
}
