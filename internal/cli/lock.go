package cli

import (
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/spf13/cobra"

	"eks-upgrader/internal/lock"
)

func newLockCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "lock", Short: "Manage the distributed cluster upgrade lock"}
	cmd.AddCommand(newLockReleaseCmd())
	return cmd
}

func newLockReleaseCmd() *cobra.Command {
	var cluster, region, backend string
	var confirm bool

	cmd := &cobra.Command{
		Use:   "release",
		Short: "Forcibly release a stuck cluster lock; only safe when no run process is actually active",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := commandContext()
			defer cancel()

			if !confirm {
				return internalExit(fmt.Errorf("refusing to force-release the lock for %q without --confirm", cluster))
			}

			var locker lock.Locker
			switch backend {
			case "dynamodb":
				awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
				if err != nil {
					return internalExit(fmt.Errorf("loading AWS config: %w", err))
				}
				locker = lock.NewDynamoDBLocker(dynamodb.NewFromConfig(awsCfg), "eks-upgrader-locks")
			default:
				locker = lock.NewLocalLocker("./state/locks")
			}

			status, err := locker.Status(ctx, cluster)
			if err != nil {
				return internalExit(err)
			}
			if status == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "no lock currently held for cluster %q\n", cluster)
				return nil
			}

			if err := locker.ForceRelease(ctx, cluster); err != nil {
				return internalExit(err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "lock for cluster %q (previously held by runID=%s owner=%s) forcibly released\n", cluster, status.RunID, status.Owner)
			return nil
		},
	}

	cmd.Flags().StringVar(&cluster, "cluster", "", "EKS cluster name (required)")
	cmd.Flags().StringVar(&region, "region", "", "AWS region (required for --lock-backend=dynamodb)")
	cmd.Flags().StringVar(&backend, "lock-backend", "local", "lock backend: local|dynamodb")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "required to actually force-release the lock")
	_ = cmd.MarkFlagRequired("cluster")
	return cmd
}
