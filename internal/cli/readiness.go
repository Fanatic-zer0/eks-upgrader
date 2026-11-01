package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"eks-upgrader/internal/app"
	"eks-upgrader/internal/readiness"
)

func newReadinessCmd() *cobra.Command {
	var planPath, clusterConfigPath, targetVersion, output, kubeconfigPath string

	cmd := &cobra.Command{
		Use:   "readiness",
		Short: "Run the Upgrade Readiness Analyzer; performs no mutation",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := commandContext()
			defer cancel()
			out := cmd.OutOrStdout()

			a, err := app.New(app.Options{ClusterConfigPath: clusterConfigPath, PlanPath: planPath, KubeconfigPath: kubeconfigPath, LogLevel: flagLogLevel, LogFormat: flagLogFormat})
			if err != nil {
				return internalExit(fmt.Errorf("initializing clients: %w", err))
			}
			defer a.Close()

			if targetVersion != "" {
				a.Plan.Spec.TargetKubernetesVersion = targetVersion
			}
			if output != "" {
				a.Plan.Spec.PostUpgrade.OutputDir = output
			}

			deps := &app.Deps{App: a}
			report, err := deps.Readiness(ctx)
			if err != nil {
				return internalExit(fmt.Errorf("readiness analysis failed: %w", err))
			}

			fmt.Fprintln(out, readiness.ReportMarkdown(report))
			fmt.Fprintf(out, "readiness reports written to %s\n", output)

			switch report.Status {
			case readiness.StatusNotReady:
				return blockingExit(fmt.Errorf("upgrade readiness is NOT_READY"))
			case readiness.StatusReadyWithWarnings:
				return warningExit(fmt.Errorf("upgrade readiness is READY_WITH_WARNINGS"))
			default:
				return nil
			}
		},
	}

	cmd.Flags().StringVar(&planPath, "plan", "", "path to upgrade.yaml (required)")
	cmd.Flags().StringVar(&clusterConfigPath, "cluster-config", "", "path to cluster.yaml")
	cmd.Flags().StringVar(&targetVersion, "target-version", "", "override spec.targetKubernetesVersion")
	cmd.Flags().StringVar(&output, "output", "./output", "output directory for readiness reports")
	cmd.Flags().StringVar(&kubeconfigPath, "kubeconfig", os.Getenv("KUBECONFIG"), "path to kubeconfig")
	_ = cmd.MarkFlagRequired("plan")
	return cmd
}
