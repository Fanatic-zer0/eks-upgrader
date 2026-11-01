package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version/Commit/Date are overridden at build time via -ldflags (see Makefile).
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print eks-upgrader version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "eks-upgrader %s (commit %s, built %s)\n", Version, Commit, Date)
			return nil
		},
	}
}
