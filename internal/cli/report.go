package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"eks-upgrader/internal/upgrade"
)

func newReportCmd() *cobra.Command {
	var statePath, format string

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Render the full run report in json, yaml, or markdown",
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
			switch format {
			case "json":
				data, err := report.JSON()
				if err != nil {
					return internalExit(err)
				}
				fmt.Fprintln(out, string(data))
			case "markdown", "md":
				fmt.Fprintln(out, report.Markdown())
			case "yaml":
				data, err := reportYAML(report)
				if err != nil {
					return internalExit(err)
				}
				fmt.Fprintln(out, string(data))
			default:
				return internalExit(fmt.Errorf("unsupported --format %q (expected json|yaml|markdown)", format))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&statePath, "state", "", "path to the run's state database (required)")
	cmd.Flags().StringVar(&format, "format", "markdown", "output format: json|yaml|markdown")
	_ = cmd.MarkFlagRequired("state")
	return cmd
}
