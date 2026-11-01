// Command eks-upgrader is a CLI-first Amazon EKS upgrade orchestrator that performs
// all EKS/CloudFormation mutations exclusively through eksctl.
package main

import (
	"fmt"
	"os"

	"eks-upgrader/internal/cli"
)

func main() {
	err := cli.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
	}
	os.Exit(cli.ExitCode(err))
}
