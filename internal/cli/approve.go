package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newApproveCmd() *cobra.Command {
	var statePath, approver, comment string

	cmd := &cobra.Command{
		Use:   "approve",
		Short: "Record an approval event and allow a paused control-plane upgrade to proceed on the next resume",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := commandContext()
			defer cancel()

			eng, _, closeFn, err := minimalEngine(ctx, statePath)
			if err != nil {
				return internalExit(err)
			}
			defer closeFn()

			if err := eng.Approve(ctx, approver, comment); err != nil {
				return internalExit(err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "approval recorded; run `eks-upgrader resume` to continue")
			return nil
		},
	}

	cmd.Flags().StringVar(&statePath, "state", "", "path to the run's state database (required)")
	cmd.Flags().StringVar(&approver, "approver", "", "identity of the approver (required)")
	cmd.Flags().StringVar(&comment, "comment", "", "optional approval comment")
	_ = cmd.MarkFlagRequired("state")
	_ = cmd.MarkFlagRequired("approver")
	return cmd
}
