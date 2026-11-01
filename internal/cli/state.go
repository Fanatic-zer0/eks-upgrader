package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"eks-upgrader/internal/app"
	"eks-upgrader/internal/config"
	"eks-upgrader/internal/reconcile"
)

func newStateCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "state", Short: "Read-only live cluster state operations"}
	cmd.AddCommand(newStatePullCmd(), newStateDiffCmd())
	return cmd
}

func newStatePullCmd() *cobra.Command {
	var cluster, region, output, kubeconfigPath string
	var karpenterEnabled bool

	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull live EKS/Karpenter state into output/observed-cluster.yaml; performs no mutation",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := commandContext()
			defer cancel()

			collector, err := app.NewCollector(app.CollectorOptions{Cluster: cluster, Region: region, KubeconfigPath: kubeconfigPath})
			if err != nil {
				return internalExit(fmt.Errorf("initializing state collector: %w", err))
			}

			observed, err := collector.Collect(ctx, cluster, region, karpenterEnabled)
			if err != nil {
				return internalExit(fmt.Errorf("collecting live state: %w", err))
			}

			if err := os.MkdirAll(output, 0o755); err != nil {
				return internalExit(fmt.Errorf("creating output directory: %w", err))
			}
			yamlBytes, err := observed.ToYAML()
			if err != nil {
				return internalExit(err)
			}
			if err := os.WriteFile(filepath.Join(output, "observed-cluster.yaml"), yamlBytes, 0o644); err != nil {
				return internalExit(err)
			}
			if err := os.MkdirAll(filepath.Join(output, "raw"), 0o755); err != nil {
				return internalExit(err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", filepath.Join(output, "observed-cluster.yaml"))
			return nil
		},
	}

	cmd.Flags().StringVar(&cluster, "cluster", "", "EKS cluster name (required)")
	cmd.Flags().StringVar(&region, "region", "", "AWS region (required)")
	cmd.Flags().StringVar(&output, "output", "./output", "output directory")
	cmd.Flags().StringVar(&kubeconfigPath, "kubeconfig", os.Getenv("KUBECONFIG"), "path to kubeconfig")
	cmd.Flags().BoolVar(&karpenterEnabled, "karpenter", false, "also collect Karpenter NodePool/EC2NodeClass/NodeClaim state")
	_ = cmd.MarkFlagRequired("cluster")
	_ = cmd.MarkFlagRequired("region")
	return cmd
}

func newStateDiffCmd() *cobra.Command {
	var clusterConfigPath, karpenterConfigDir, observedPath string

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Diff Git-declared desired state against a previously pulled observed state; performs no mutation",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			cluster, err := config.LoadClusterConfig(clusterConfigPath)
			if err != nil {
				return internalExit(fmt.Errorf("loading cluster config: %w", err))
			}
			observed, err := reconcile.LoadObservedCluster(observedPath)
			if err != nil {
				return internalExit(err)
			}

			nodeClasses, nodePools, err := loadKarpenterDesiredFromDir(karpenterConfigDir)
			if err != nil {
				return internalExit(err)
			}

			plan := &config.UpgradePlan{Spec: config.PlanSpec{
				Karpenter: config.KarpenterSpec{Enabled: karpenterConfigDir != "", NodeClasses: nodeClasses, NodePools: nodePools},
			}}

			findings := reconcile.Diff(cluster, plan, observed)

			for _, f := range findings {
				fmt.Fprintf(out, "%s: %s", f.Classification, f.Field)
				if f.Desired != "" || f.Observed != "" {
					fmt.Fprintf(out, " (desired=%q observed=%q)", f.Desired, f.Observed)
				}
				fmt.Fprintf(out, ": %s\n", f.Message)
			}

			if reconcile.HasBlocking(findings) {
				return blockingExit(fmt.Errorf("blocking drift detected between desired and live state"))
			}
			fmt.Fprintln(out, "no blocking drift detected")
			return nil
		},
	}

	cmd.Flags().StringVar(&clusterConfigPath, "cluster-config", "", "path to cluster.yaml (required)")
	cmd.Flags().StringVar(&karpenterConfigDir, "karpenter-config", "", "directory of Karpenter NodePool/EC2NodeClass manifests")
	cmd.Flags().StringVar(&observedPath, "observed", "./output/observed-cluster.yaml", "path to a previously pulled observed-cluster.yaml")
	_ = cmd.MarkFlagRequired("cluster-config")
	return cmd
}
