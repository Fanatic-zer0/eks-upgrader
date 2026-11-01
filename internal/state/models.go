// Package state persists upgrade-run state so that runs are resumable, auditable,
// and introspectable via the `status` and `report` CLI commands.
package state

import "time"

// Phase mirrors the finite state machine phases defined in internal/upgrade.
type Phase string

const (
	PhaseInit                 Phase = "INIT"
	PhaseValidate             Phase = "VALIDATE"
	PhaseStateReconciliation  Phase = "STATE_RECONCILIATION"
	PhaseReadinessAnalysis    Phase = "READINESS_ANALYSIS"
	PhasePreflight            Phase = "PREFLIGHT"
	PhaseApproval             Phase = "APPROVAL"
	PhaseControlPlaneUpgrade  Phase = "CONTROL_PLANE_UPGRADE"
	PhaseAddonUpgrade         Phase = "ADDON_UPGRADE"
	PhaseKarpenterUpgrade     Phase = "KARPENTER_UPGRADE"
	PhaseNodeGroupUpgrade     Phase = "NODE_GROUP_UPGRADE"
	PhasePostUpgradeCheck     Phase = "POST_UPGRADE_CHECK"
	PhaseConfigReconciliation Phase = "CONFIG_RECONCILIATION"
	PhaseComplete             Phase = "COMPLETE"
)

// RunStatus is the overall status of an upgrade run.
type RunStatus string

const (
	StatusRunning               RunStatus = "RUNNING"
	StatusPaused                RunStatus = "PAUSED"
	StatusInterrupted           RunStatus = "INTERRUPTED"
	StatusAborted               RunStatus = "ABORTED"
	StatusFailed                RunStatus = "FAILED"
	StatusCompleted             RunStatus = "COMPLETED"
	StatusCompletedWithWarnings RunStatus = "COMPLETE_WITH_WARNINGS"
)

// Run is the top-level persisted record for a single upgrade execution.
type Run struct {
	RunID         string
	Cluster       string
	Region        string
	SourceVersion string
	TargetVersion string
	CurrentPhase  Phase
	Status        RunStatus
	PauseReason   string
	AbortReason   string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// PhaseHistoryEntry records one transition into/out of a phase.
type PhaseHistoryEntry struct {
	RunID     string
	Phase     Phase
	Status    string // STARTED | COMPLETED | FAILED | PAUSED
	StartedAt time.Time
	EndedAt   *time.Time
	Detail    string
}

// Approval records a human approval event, e.g. for the control-plane gate.
type Approval struct {
	RunID      string
	Phase      Phase
	Approver   string
	Comment    string
	Token      string
	ApprovedAt time.Time
}

// EksctlCommandRecord captures full provenance of every eksctl invocation.
type EksctlCommandRecord struct {
	RunID      string
	Phase      Phase
	Args       []string
	ExitCode   int
	DurationMS int64
	StartedAt  time.Time
	Stdout     string
	Stderr     string
}

// NodeGroupStatus tracks per-node-group upgrade progress.
type NodeGroupStatus struct {
	RunID     string
	Name      string
	Status    string // PENDING | IN_PROGRESS | SUCCEEDED | FAILED | PAUSED
	StartedAt *time.Time
	EndedAt   *time.Time
	Detail    string
}

// ResumeAttempt records a single resume attempt for a given phase, used to drive the
// resume circuit breaker.
type ResumeAttempt struct {
	RunID    string
	Phase    Phase
	Attempt  int
	FailedAt time.Time
	Error    string
}
