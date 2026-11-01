package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"eks-upgrader/internal/app"
	"eks-upgrader/internal/config"
	"eks-upgrader/internal/reconcile"
)

func newValidateCmd() *cobra.Command {
	var planPath, clusterConfigPath, kubeconfigPath string
	var useLiveState bool

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate upgrade.yaml and cluster.yaml; performs no mutation",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := commandContext()
			defer cancel()
			out := cmd.OutOrStdout()

			plan, err := config.LoadUpgradePlan(planPath)
			if err != nil {
				return internalExit(fmt.Errorf("loading upgrade plan: %w", err))
			}
			clusterPath := clusterConfigPath
			if clusterPath == "" {
				clusterPath = config.ResolveClusterConfigPath(plan)
			}
			cluster, err := config.LoadClusterConfig(clusterPath)
			if err != nil {
				return internalExit(fmt.Errorf("loading cluster config: %w", err))
			}

			result := config.Validate(cluster, plan)
			for _, w := range result.Warnings {
				fmt.Fprintln(out, "WARNING:", w)
			}
			for _, b := range result.Blocking {
				fmt.Fprintln(out, "BLOCKING:", b)
			}

			driftBlocking := false
			driftHasWarnings := false
			if useLiveState {
				a, err := app.New(app.Options{ClusterConfigPath: clusterPath, PlanPath: planPath, KubeconfigPath: kubeconfigPath, LogLevel: flagLogLevel, LogFormat: flagLogFormat})
				if err != nil {
					return internalExit(fmt.Errorf("initializing live-state clients: %w", err))
				}
				defer a.Close()

				deps := &app.Deps{App: a}
				findings, err := deps.Reconcile(ctx)
				if err != nil {
					return internalExit(fmt.Errorf("live-state reconciliation failed: %w", err))
				}
				for _, f := range findings {
					if f.Classification != reconcile.InSync {
						fmt.Fprintf(out, "%s (live-state): %s: %s\n", f.Classification, f.Field, f.Message)
					}
					if f.Classification == reconcile.Warning || f.Classification == reconcile.Unmanaged {
						driftHasWarnings = true
					}
				}
				driftBlocking = reconcile.HasBlocking(findings)
			}

			switch {
			case !result.OK() || driftBlocking:
				return blockingExit(fmt.Errorf("validation failed with %d blocking issue(s)", len(result.Blocking)))
			case len(result.Warnings) > 0 || driftHasWarnings:
				return warningExit(fmt.Errorf("validation passed with warnings"))
			default:
				fmt.Fprintln(out, "validation passed")
				return nil
			}
		},
	}

	cmd.Flags().StringVar(&planPath, "plan", "", "path to upgrade.yaml (required)")
	cmd.Flags().StringVar(&clusterConfigPath, "cluster-config", "", "path to cluster.yaml (defaults to spec.clusterConfig)")
	cmd.Flags().StringVar(&kubeconfigPath, "kubeconfig", os.Getenv("KUBECONFIG"), "path to kubeconfig (for --use-live-state)")
	cmd.Flags().BoolVar(&useLiveState, "use-live-state", false, "also reconcile against live cluster/Karpenter state")
	_ = cmd.MarkFlagRequired("plan")
	return cmd
}
