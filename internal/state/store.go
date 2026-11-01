package state

import "context"

// Store persists all data needed to resume, audit, and report on an upgrade run.
// Implementations must be safe for concurrent reads while a single writer (the active
// upgrade/resume process) holds the store open.
type Store interface {
	CreateRun(ctx context.Context, run *Run) error
	GetRun(ctx context.Context, runID string) (*Run, error)
	LatestRunForCluster(ctx context.Context, cluster string) (*Run, error)
	LatestRun(ctx context.Context) (*Run, error)
	UpdateRunPhase(ctx context.Context, runID string, phase Phase, status RunStatus) error
	UpdateRunStatus(ctx context.Context, runID string, status RunStatus, reason string) error

	AppendPhaseHistory(ctx context.Context, entry PhaseHistoryEntry) error
	CompletePhaseHistory(ctx context.Context, runID string, phase Phase, status, detail string) error
	GetPhaseHistory(ctx context.Context, runID string) ([]PhaseHistoryEntry, error)

	RecordApproval(ctx context.Context, approval Approval) error
	GetApprovals(ctx context.Context, runID string) ([]Approval, error)

	RecordEksctlCommand(ctx context.Context, rec EksctlCommandRecord) error
	GetEksctlCommands(ctx context.Context, runID string) ([]EksctlCommandRecord, error)

	UpsertNodeGroupStatus(ctx context.Context, status NodeGroupStatus) error
	GetNodeGroupStatuses(ctx context.Context, runID string) ([]NodeGroupStatus, error)

	RecordResumeAttempt(ctx context.Context, attempt ResumeAttempt) error
	CountResumeAttempts(ctx context.Context, runID string, phase Phase) (int, error)

	Close() error
}
