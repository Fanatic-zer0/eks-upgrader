package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newPauseCmd() *cobra.Command {
	var statePath, reason string

	cmd := &cobra.Command{
		Use:   "pause",
		Short: "Operator-initiated pause of a currently running run at the next safe checkpoint",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := commandContext()
			defer cancel()

			eng, _, closeFn, err := minimalEngine(ctx, statePath)
			if err != nil {
				return internalExit(err)
			}
			defer closeFn()

			if err := eng.Pause(ctx, reason); err != nil {
				return internalExit(err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "run paused")
			return nil
		},
	}

	cmd.Flags().StringVar(&statePath, "state", "", "path to the run's state database (required)")
	cmd.Flags().StringVar(&reason, "reason", "manual pause for investigation", "reason recorded for the pause")
	_ = cmd.MarkFlagRequired("state")
	return cmd
}
