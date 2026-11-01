package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newAbortCmd() *cobra.Command {
	var statePath, reason string

	cmd := &cobra.Command{
		Use:   "abort",
		Short: "Release locks, mark the run aborted, and send notifications",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := commandContext()
			defer cancel()

			eng, _, closeFn, err := minimalEngine(ctx, statePath)
			if err != nil {
				return internalExit(err)
			}
			defer closeFn()

			if err := eng.Abort(ctx, reason); err != nil {
				return internalExit(err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "run aborted")
			return nil
		},
	}

	cmd.Flags().StringVar(&statePath, "state", "", "path to the run's state database (required)")
	cmd.Flags().StringVar(&reason, "reason", "manual abort", "reason recorded for the abort")
	_ = cmd.MarkFlagRequired("state")
	return cmd
}
