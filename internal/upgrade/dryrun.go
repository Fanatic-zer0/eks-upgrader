package upgrade

import (
	"context"
	"strings"

	"eks-upgrader/internal/config"
	"eks-upgrader/internal/eksctl"
	"eks-upgrader/internal/state"
)

// DryRunReportData is the full `upgrade --dry-run` output: validation results, the
// expected phase sequence, and the exact eksctl commands that would run. No field here
// is ever produced by actually mutating anything.
type DryRunReportData struct {
	ValidationBlocking    []string
	ValidationWarnings    []string
	ExpectedSequence      []string
	PlannedEksctlCommands []string
}

// DryRunReport builds the dry-run output without acquiring a lock, creating a run
// record, or invoking any Dependencies method that could mutate state.
func (e *Engine) DryRunReport(ctx context.Context) (*DryRunReportData, error) {
	result := config.Validate(e.Cluster, e.Plan)
	report := &DryRunReportData{ValidationBlocking: result.Blocking, ValidationWarnings: result.Warnings}

	for _, p := range PhaseOrder {
		if p == state.PhaseInit || p == state.PhaseComplete {
			continue
		}
		report.ExpectedSequence = append(report.ExpectedSequence, string(p))
	}

	report.PlannedEksctlCommands = append(report.PlannedEksctlCommands,
		"eksctl "+strings.Join(eksctl.BuildUpgradeClusterArgs(e.Cluster.SourcePath), " "))

	if e.Plan.Spec.Addons.Upgrade {
		for _, name := range e.Plan.Spec.Addons.Names {
			args := eksctl.BuildUpdateAddonArgs(eksctl.AddonUpgradeOptions{
				Cluster: e.Plan.Metadata.Cluster, Region: e.Plan.Metadata.Region, Name: name,
			})
			report.PlannedEksctlCommands = append(report.PlannedEksctlCommands, "eksctl "+strings.Join(args, " "))
		}
	}

	for _, name := range e.Plan.Spec.NodeGroups.Order {
		args := eksctl.BuildUpgradeNodeGroupArgs(eksctl.NodeGroupUpgradeOptions{
			Cluster: e.Plan.Metadata.Cluster, Region: e.Plan.Metadata.Region,
			NodeGroup: name, KubernetesVersion: e.Plan.Spec.TargetKubernetesVersion,
		})
		report.PlannedEksctlCommands = append(report.PlannedEksctlCommands, "eksctl "+strings.Join(args, " "))
	}

	return report, nil
}
