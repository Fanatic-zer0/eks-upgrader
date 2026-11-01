package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"eks-upgrader/internal/upgrade"
)

func newStatusCmd() *cobra.Command {
	var statePath string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current phase, progress, failures, approvals, and next action for a run",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := commandContext()
			defer cancel()

			eng, run, closeFn, err := minimalEngine(ctx, statePath)
			if err != nil {
				return internalExit(err)
			}
			defer closeFn()

			report, err := upgrade.BuildRunReport(ctx, eng.Store, run.RunID)
			if err != nil {
				return internalExit(err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Run:            %s\n", report.Run.RunID)
			fmt.Fprintf(out, "Cluster:        %s (%s)\n", report.Run.Cluster, report.Run.Region)
			fmt.Fprintf(out, "Versions:       %s -> %s\n", report.Run.SourceVersion, report.Run.TargetVersion)
			fmt.Fprintf(out, "Status:         %s\n", report.Run.Status)
			fmt.Fprintf(out, "Current phase:  %s\n", report.Run.CurrentPhase)
			if report.Run.PauseReason != "" {
				fmt.Fprintf(out, "Pause reason:   %s\n", report.Run.PauseReason)
			}
			if report.Run.AbortReason != "" {
				fmt.Fprintf(out, "Abort reason:   %s\n", report.Run.AbortReason)
			}
			fmt.Fprintf(out, "Completed phases:\n")
			for _, h := range report.PhaseHistory {
				if h.Status == "COMPLETED" {
					fmt.Fprintf(out, "  - %s\n", h.Phase)
				}
			}
			if len(report.NodeGroups) > 0 {
				fmt.Fprintf(out, "Node group progress:\n")
				for _, n := range report.NodeGroups {
					fmt.Fprintf(out, "  - %s: %s\n", n.Name, n.Status)
				}
			}
			if len(report.Approvals) > 0 {
				fmt.Fprintf(out, "Approvals:\n")
				for _, a := range report.Approvals {
					fmt.Fprintf(out, "  - %s approved by %s\n", a.Phase, a.Approver)
				}
			}
			fmt.Fprintf(out, "Next action:    %s\n", report.NextAction())
			return nil
		},
	}

	cmd.Flags().StringVar(&statePath, "state", "", "path to the run's state database (required)")
	_ = cmd.MarkFlagRequired("state")
	return cmd
}
