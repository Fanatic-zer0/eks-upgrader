package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"eks-upgrader/internal/app"
	"eks-upgrader/internal/upgrade"
)

func newResumeCmd() *cobra.Command {
	var planPath, clusterConfigPath, kubeconfigPath, eksctlPath, helmPath, owner, statePath string
	var force bool

	cmd := &cobra.Command{
		Use:   "resume",
		Short: "Continue a paused or interrupted run from its last persisted checkpoint",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := commandContext()
			defer cancel()
			out := cmd.OutOrStdout()

			a, err := app.New(app.Options{
				ClusterConfigPath: clusterConfigPath, PlanPath: planPath, KubeconfigPath: kubeconfigPath,
				EksctlBinaryPath: eksctlPath, HelmBinaryPath: helmPath, StateFilePath: statePath,
				LogLevel: flagLogLevel, LogFormat: flagLogFormat, Owner: owner,
			})
			if err != nil {
				return internalExit(fmt.Errorf("initializing clients: %w", err))
			}
			defer a.Close()

			engine := &upgrade.Engine{
				Cluster: a.Cluster, Plan: a.Plan, Store: a.Store, Lock: a.Lock, Audit: a.Audit,
				Notify: a.Notifier, Deps: &app.Deps{App: a}, Logger: a.Logger, Owner: a.Options.Owner,
			}

			if err := engine.Resume(ctx, force); err != nil {
				if errors.Is(err, upgrade.ErrResumeAttemptsExceeded) {
					return blockingExit(err)
				}
				return internalExit(err)
			}
			fmt.Fprintln(out, "resume completed")
			return nil
		},
	}

	cmd.Flags().StringVar(&planPath, "plan", "", "path to upgrade.yaml (required)")
	cmd.Flags().StringVar(&statePath, "state", "", "path to the run's state database (defaults to spec.execution.stateFile)")
	cmd.Flags().StringVar(&clusterConfigPath, "cluster-config", "", "path to cluster.yaml")
	cmd.Flags().StringVar(&kubeconfigPath, "kubeconfig", os.Getenv("KUBECONFIG"), "path to kubeconfig")
	cmd.Flags().StringVar(&eksctlPath, "eksctl-path", "", "path to the eksctl binary")
	cmd.Flags().StringVar(&helmPath, "helm-path", "", "path to the helm binary")
	cmd.Flags().StringVar(&owner, "owner", defaultOwner(), "identity recorded as the lock owner")
	cmd.Flags().BoolVar(&force, "force", false, "override the resume circuit breaker")
	_ = cmd.MarkFlagRequired("plan")
	return cmd
}
