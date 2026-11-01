package eksctl_test

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"eks-upgrader/internal/audit"
	"eks-upgrader/internal/eksctl"
	"eks-upgrader/internal/eksctl/mocks"
	"eks-upgrader/internal/state"
)

func TestInstrumentedExecutor_RecordsToStoreAndAudit(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	store, err := state.OpenSQLiteStore(filepath.Join(dir, "upgrade.db"))
	if err != nil {
		t.Fatalf("OpenSQLiteStore: %v", err)
	}
	defer store.Close()

	auditLogger, err := audit.NewFileLogger(filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}
	defer auditLogger.Close()

	if err := store.CreateRun(ctx, &state.Run{
		RunID: "run-1", Cluster: "prod", Region: "ap-south-1",
		SourceVersion: "1.30", TargetVersion: "1.31",
		CurrentPhase: state.PhaseNodeGroupUpgrade, Status: state.StatusRunning,
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	fake := mocks.NewFakeExecutor()
	instrumented := &eksctl.InstrumentedExecutor{
		Inner:  fake,
		Audit:  auditLogger,
		Store:  store,
		Logger: slog.Default(),
	}

	args := eksctl.BuildUpgradeNodeGroupArgs(eksctl.NodeGroupUpgradeOptions{
		Cluster: "prod", Region: "ap-south-1", NodeGroup: "app-a", KubernetesVersion: "1.31",
	})
	rc := eksctl.RunContext{RunID: "run-1", Cluster: "prod", Region: "ap-south-1", Phase: state.PhaseNodeGroupUpgrade}

	result, err := instrumented.RunWithTimeout(ctx, rc, args, 5*time.Second)
	if err != nil {
		t.Fatalf("RunWithTimeout: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
	if len(fake.Calls) != 1 {
		t.Fatalf("expected exactly 1 recorded call, got %d", len(fake.Calls))
	}

	commands, err := store.GetEksctlCommands(ctx, "run-1")
	if err != nil {
		t.Fatalf("GetEksctlCommands: %v", err)
	}
	if len(commands) != 1 {
		t.Fatalf("expected 1 persisted eksctl command, got %d", len(commands))
	}
	if commands[0].Args[0] != "upgrade" || commands[0].Args[1] != "nodegroup" {
		t.Fatalf("unexpected persisted args: %v", commands[0].Args)
	}
}

func TestInstrumentedExecutor_RecordsFailure(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	store, err := state.OpenSQLiteStore(filepath.Join(dir, "upgrade.db"))
	if err != nil {
		t.Fatalf("OpenSQLiteStore: %v", err)
	}
	defer store.Close()

	auditLogger := audit.NopLogger{}

	if err := store.CreateRun(ctx, &state.Run{
		RunID: "run-2", Cluster: "prod", Region: "ap-south-1",
		SourceVersion: "1.30", TargetVersion: "1.31",
		CurrentPhase: state.PhaseControlPlaneUpgrade, Status: state.StatusRunning,
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	fake := mocks.NewFakeExecutor()
	fake.DefaultResult = &eksctl.CommandResult{ExitCode: 1, Stderr: "boom"}
	fake.DefaultErr = context.DeadlineExceeded

	instrumented := &eksctl.InstrumentedExecutor{Inner: fake, Audit: auditLogger, Store: store, Logger: slog.Default()}
	rc := eksctl.RunContext{RunID: "run-2", Cluster: "prod", Region: "ap-south-1", Phase: state.PhaseControlPlaneUpgrade}

	_, err = instrumented.RunWithTimeout(ctx, rc, eksctl.BuildUpgradeClusterArgs("/tmp/cluster.yaml"), time.Second)
	if err == nil {
		t.Fatal("expected error to be propagated")
	}

	commands, err := store.GetEksctlCommands(ctx, "run-2")
	if err != nil {
		t.Fatalf("GetEksctlCommands: %v", err)
	}
	if len(commands) != 1 || commands[0].ExitCode != 1 {
		t.Fatalf("expected 1 persisted failed command, got %+v", commands)
	}
}
