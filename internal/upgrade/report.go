package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"eks-upgrader/internal/state"
)

// RunReport aggregates everything persisted for a run, used by both `status` (a
// summary) and `report` (full json/yaml/markdown output).
type RunReport struct {
	Run            *state.Run                  `json:"run"`
	PhaseHistory   []state.PhaseHistoryEntry   `json:"phaseHistory"`
	NodeGroups     []state.NodeGroupStatus     `json:"nodeGroups"`
	Approvals      []state.Approval            `json:"approvals"`
	EksctlCommands []state.EksctlCommandRecord `json:"eksctlCommands"`
}

// BuildRunReport assembles a RunReport from the state store.
func BuildRunReport(ctx context.Context, store state.Store, runID string) (*RunReport, error) {
	run, err := store.GetRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("loading run: %w", err)
	}
	if run == nil {
		return nil, fmt.Errorf("no run found with id %q", runID)
	}

	history, err := store.GetPhaseHistory(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("loading phase history: %w", err)
	}
	nodeGroups, err := store.GetNodeGroupStatuses(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("loading node group statuses: %w", err)
	}
	approvals, err := store.GetApprovals(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("loading approvals: %w", err)
	}
	commands, err := store.GetEksctlCommands(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("loading eksctl commands: %w", err)
	}

	return &RunReport{Run: run, PhaseHistory: history, NodeGroups: nodeGroups, Approvals: approvals, EksctlCommands: commands}, nil
}

// JSON renders the report as indented JSON for output/upgrade-report.json.
func (r *RunReport) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// EksctlCommandsJSON renders just the eksctl command history for output/eksctl-commands.json.
func (r *RunReport) EksctlCommandsJSON() ([]byte, error) {
	return json.MarshalIndent(r.EksctlCommands, "", "  ")
}

// Markdown renders a human-readable summary for `status`/`report --format markdown`
// and for output/upgrade-report.md.
func (r *RunReport) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Upgrade Run Report: %s\n\n", r.Run.RunID)
	fmt.Fprintf(&b, "- Cluster: %s (%s)\n", r.Run.Cluster, r.Run.Region)
	fmt.Fprintf(&b, "- Versions: %s -> %s\n", r.Run.SourceVersion, r.Run.TargetVersion)
	fmt.Fprintf(&b, "- Status: **%s**\n", r.Run.Status)
	fmt.Fprintf(&b, "- Current phase: %s\n", r.Run.CurrentPhase)
	if r.Run.PauseReason != "" {
		fmt.Fprintf(&b, "- Pause reason: %s\n", r.Run.PauseReason)
	}
	if r.Run.AbortReason != "" {
		fmt.Fprintf(&b, "- Abort reason: %s\n", r.Run.AbortReason)
	}
	fmt.Fprintf(&b, "- Next action: %s\n\n", r.NextAction())

	b.WriteString("## Phase history\n\n")
	b.WriteString("| Phase | Status | Started | Ended | Detail |\n|---|---|---|---|---|\n")
	for _, h := range r.PhaseHistory {
		ended := ""
		if h.EndedAt != nil {
			ended = h.EndedAt.Format("2006-01-02T15:04:05Z")
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", h.Phase, h.Status, h.StartedAt.Format("2006-01-02T15:04:05Z"), ended, h.Detail)
	}

	if len(r.NodeGroups) > 0 {
		b.WriteString("\n## Node group progress\n\n")
		b.WriteString("| Name | Status | Detail |\n|---|---|---|\n")
		for _, n := range r.NodeGroups {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", n.Name, n.Status, n.Detail)
		}
	}

	if len(r.Approvals) > 0 {
		b.WriteString("\n## Approvals\n\n")
		for _, a := range r.Approvals {
			fmt.Fprintf(&b, "- %s approved by **%s** at %s: %s\n", a.Phase, a.Approver, a.ApprovedAt.Format("2006-01-02T15:04:05Z"), a.Comment)
		}
	}

	if len(r.EksctlCommands) > 0 {
		b.WriteString("\n## eksctl command history\n\n")
		for _, c := range r.EksctlCommands {
			fmt.Fprintf(&b, "- [%s] exit=%d duration=%dms: `%s`\n", c.Phase, c.ExitCode, c.DurationMS, strings.Join(c.Args, " "))
		}
	}

	return b.String()
}

// NextAction gives a short human-readable hint of what the operator should do next.
func (r *RunReport) NextAction() string {
	switch r.Run.Status {
	case state.StatusPaused:
		if r.Run.CurrentPhase == state.PhaseApproval {
			return "run `eks-upgrader approve` then `eks-upgrader resume`"
		}
		return "investigate the pause reason, then `eks-upgrader resume`"
	case state.StatusInterrupted:
		return "run `eks-upgrader resume`"
	case state.StatusAborted:
		return "none; run aborted"
	case state.StatusFailed:
		return "none; validation failed, fix cluster.yaml/upgrade.yaml and start a new run"
	case state.StatusCompleted, state.StatusCompletedWithWarnings:
		return "review output/upgrade-report.md and apply the generated config patch"
	default:
		return "none; run in progress"
	}
}
