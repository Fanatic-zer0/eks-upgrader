package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"eks-upgrader/internal/app"
	"eks-upgrader/internal/upgrade"
)

func newUpgradeCmd() *cobra.Command {
	var planPath, clusterConfigPath, kubeconfigPath, eksctlPath, helmPath, owner string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Execute the EKS upgrade state machine",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := commandContext()
			defer cancel()
			out := cmd.OutOrStdout()

			a, err := app.New(app.Options{
				ClusterConfigPath: clusterConfigPath, PlanPath: planPath, KubeconfigPath: kubeconfigPath,
				EksctlBinaryPath: eksctlPath, HelmBinaryPath: helmPath, LogLevel: flagLogLevel, LogFormat: flagLogFormat, Owner: owner,
			})
			if err != nil {
				return internalExit(fmt.Errorf("initializing clients: %w", err))
			}
			defer a.Close()

			engine := &upgrade.Engine{
				Cluster: a.Cluster, Plan: a.Plan, Store: a.Store, Lock: a.Lock, Audit: a.Audit,
				Notify: a.Notifier, Deps: &app.Deps{App: a}, Logger: a.Logger, Owner: a.Options.Owner,
			}

			if dryRun {
				report, err := engine.DryRunReport(ctx)
				if err != nil {
					return internalExit(err)
				}
				printDryRunReport(out, report)
				if len(report.ValidationBlocking) > 0 {
					return blockingExit(fmt.Errorf("dry-run validation found %d blocking issue(s)", len(report.ValidationBlocking)))
				}
				return nil
			}

			if err := engine.Run(ctx, false); err != nil {
				return internalExit(err)
			}
			fmt.Fprintln(out, "upgrade run completed")
			return nil
		},
	}

	cmd.Flags().StringVar(&planPath, "plan", "", "path to upgrade.yaml (required)")
	cmd.Flags().StringVar(&clusterConfigPath, "cluster-config", "", "path to cluster.yaml")
	cmd.Flags().StringVar(&kubeconfigPath, "kubeconfig", os.Getenv("KUBECONFIG"), "path to kubeconfig")
	cmd.Flags().StringVar(&eksctlPath, "eksctl-path", "", "path to the eksctl binary (defaults to $PATH)")
	cmd.Flags().StringVar(&helmPath, "helm-path", "", "path to the helm binary (defaults to $PATH)")
	cmd.Flags().StringVar(&owner, "owner", defaultOwner(), "identity recorded as the lock owner")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the plan without mutating anything")
	_ = cmd.MarkFlagRequired("plan")
	return cmd
}

func printDryRunReport(out io.Writer, report *upgrade.DryRunReportData) {
	fmt.Fprintln(out, "=== Validation ===")
	for _, b := range report.ValidationBlocking {
		fmt.Fprintln(out, "BLOCKING:", b)
	}
	for _, w := range report.ValidationWarnings {
		fmt.Fprintln(out, "WARNING:", w)
	}
	fmt.Fprintln(out, "\n=== Expected phase sequence ===")
	for _, p := range report.ExpectedSequence {
		fmt.Fprintln(out, "-", p)
	}
	fmt.Fprintln(out, "\n=== Planned eksctl commands ===")
	for _, c := range report.PlannedEksctlCommands {
		fmt.Fprintln(out, "$", c)
	}
}

func defaultOwner() string {
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return "eks-upgrader"
}
