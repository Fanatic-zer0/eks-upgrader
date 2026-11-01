package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "upgrade.db")
	s, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("OpenSQLiteStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSQLiteStore_RunLifecycle(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	run := &Run{
		RunID:         "run-1",
		Cluster:       "prod-cluster",
		Region:        "ap-south-1",
		SourceVersion: "1.30",
		TargetVersion: "1.31",
		CurrentPhase:  PhaseInit,
		Status:        StatusRunning,
	}
	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	got, err := s.GetRun(ctx, "run-1")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got == nil || got.Cluster != "prod-cluster" {
		t.Fatalf("unexpected run: %+v", got)
	}

	if err := s.UpdateRunPhase(ctx, "run-1", PhaseValidate, StatusRunning); err != nil {
		t.Fatalf("UpdateRunPhase: %v", err)
	}
	got, _ = s.GetRun(ctx, "run-1")
	if got.CurrentPhase != PhaseValidate {
		t.Fatalf("expected phase VALIDATE, got %s", got.CurrentPhase)
	}

	if err := s.UpdateRunStatus(ctx, "run-1", StatusPaused, "manual pause"); err != nil {
		t.Fatalf("UpdateRunStatus: %v", err)
	}
	got, _ = s.GetRun(ctx, "run-1")
	if got.Status != StatusPaused || got.PauseReason != "manual pause" {
		t.Fatalf("unexpected run after pause: %+v", got)
	}

	latest, err := s.LatestRunForCluster(ctx, "prod-cluster")
	if err != nil {
		t.Fatalf("LatestRunForCluster: %v", err)
	}
	if latest == nil || latest.RunID != "run-1" {
		t.Fatalf("unexpected latest run: %+v", latest)
	}

	latestAny, err := s.LatestRun(ctx)
	if err != nil {
		t.Fatalf("LatestRun: %v", err)
	}
	if latestAny == nil || latestAny.RunID != "run-1" {
		t.Fatalf("unexpected LatestRun result: %+v", latestAny)
	}
}

func TestSQLiteStore_PhaseHistoryAndApprovals(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	run := &Run{RunID: "run-2", Cluster: "c", Region: "r", SourceVersion: "1.30", TargetVersion: "1.31", CurrentPhase: PhaseInit, Status: StatusRunning}
	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	if err := s.AppendPhaseHistory(ctx, PhaseHistoryEntry{RunID: "run-2", Phase: PhaseValidate, Status: "STARTED", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("AppendPhaseHistory: %v", err)
	}
	if err := s.CompletePhaseHistory(ctx, "run-2", PhaseValidate, "COMPLETED", "ok"); err != nil {
		t.Fatalf("CompletePhaseHistory: %v", err)
	}

	history, err := s.GetPhaseHistory(ctx, "run-2")
	if err != nil {
		t.Fatalf("GetPhaseHistory: %v", err)
	}
	if len(history) != 1 || history[0].Status != "COMPLETED" || history[0].EndedAt == nil {
		t.Fatalf("unexpected phase history: %+v", history)
	}

	if err := s.RecordApproval(ctx, Approval{RunID: "run-2", Phase: PhaseApproval, Approver: "oncall", Token: "tok-1", ApprovedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("RecordApproval: %v", err)
	}
	approvals, err := s.GetApprovals(ctx, "run-2")
	if err != nil {
		t.Fatalf("GetApprovals: %v", err)
	}
	if len(approvals) != 1 || approvals[0].Approver != "oncall" {
		t.Fatalf("unexpected approvals: %+v", approvals)
	}
}

func TestSQLiteStore_EksctlCommandsAndNodeGroups(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	run := &Run{RunID: "run-3", Cluster: "c", Region: "r", SourceVersion: "1.30", TargetVersion: "1.31", CurrentPhase: PhaseInit, Status: StatusRunning}
	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	rec := EksctlCommandRecord{
		RunID: "run-3", Phase: PhaseNodeGroupUpgrade,
		Args:     []string{"eksctl", "upgrade", "nodegroup", "--name", "app-a"},
		ExitCode: 0, DurationMS: 1234, StartedAt: time.Now().UTC(), Stdout: "ok",
	}
	if err := s.RecordEksctlCommand(ctx, rec); err != nil {
		t.Fatalf("RecordEksctlCommand: %v", err)
	}
	cmds, err := s.GetEksctlCommands(ctx, "run-3")
	if err != nil {
		t.Fatalf("GetEksctlCommands: %v", err)
	}
	if len(cmds) != 1 || len(cmds[0].Args) != 5 {
		t.Fatalf("unexpected eksctl commands: %+v", cmds)
	}

	start := time.Now().UTC()
	if err := s.UpsertNodeGroupStatus(ctx, NodeGroupStatus{RunID: "run-3", Name: "app-a", Status: "IN_PROGRESS", StartedAt: &start}); err != nil {
		t.Fatalf("UpsertNodeGroupStatus (insert): %v", err)
	}
	end := time.Now().UTC()
	if err := s.UpsertNodeGroupStatus(ctx, NodeGroupStatus{RunID: "run-3", Name: "app-a", Status: "SUCCEEDED", EndedAt: &end, Detail: "all healthy"}); err != nil {
		t.Fatalf("UpsertNodeGroupStatus (update): %v", err)
	}

	statuses, err := s.GetNodeGroupStatuses(ctx, "run-3")
	if err != nil {
		t.Fatalf("GetNodeGroupStatuses: %v", err)
	}
	if len(statuses) != 1 || statuses[0].Status != "SUCCEEDED" || statuses[0].StartedAt == nil || statuses[0].EndedAt == nil {
		t.Fatalf("unexpected node group statuses: %+v", statuses)
	}
}

func TestSQLiteStore_ResumeAttemptCircuitBreaker(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	run := &Run{RunID: "run-4", Cluster: "c", Region: "r", SourceVersion: "1.30", TargetVersion: "1.31", CurrentPhase: PhaseInit, Status: StatusRunning}
	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	for i := 1; i <= 3; i++ {
		if err := s.RecordResumeAttempt(ctx, ResumeAttempt{RunID: "run-4", Phase: PhaseControlPlaneUpgrade, Attempt: i, FailedAt: time.Now().UTC(), Error: "timeout"}); err != nil {
			t.Fatalf("RecordResumeAttempt: %v", err)
		}
	}

	count, err := s.CountResumeAttempts(ctx, "run-4", PhaseControlPlaneUpgrade)
	if err != nil {
		t.Fatalf("CountResumeAttempts: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 resume attempts, got %d", count)
	}
}
