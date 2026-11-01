package eksctl

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestBuildUpgradeClusterArgs(t *testing.T) {
	args := BuildUpgradeClusterArgs("/tmp/cluster.yaml")
	want := []string{"upgrade", "cluster", "--config-file", "/tmp/cluster.yaml", "--approve"}
	assertArgsEqual(t, want, args)
}

func TestBuildUpgradeNodeGroupArgs(t *testing.T) {
	t.Run("without launch template", func(t *testing.T) {
		args := BuildUpgradeNodeGroupArgs(NodeGroupUpgradeOptions{
			Cluster: "prod", Region: "ap-south-1", NodeGroup: "app-a", KubernetesVersion: "1.31",
		})
		want := []string{"upgrade", "nodegroup", "--cluster", "prod", "--region", "ap-south-1", "--name", "app-a", "--kubernetes-version", "1.31", "--wait"}
		assertArgsEqual(t, want, args)
	})

	t.Run("with launch template", func(t *testing.T) {
		args := BuildUpgradeNodeGroupArgs(NodeGroupUpgradeOptions{
			Cluster: "prod", Region: "ap-south-1", NodeGroup: "app-a", KubernetesVersion: "1.31", LaunchTemplateVersion: "3",
		})
		want := []string{"upgrade", "nodegroup", "--cluster", "prod", "--region", "ap-south-1", "--name", "app-a", "--kubernetes-version", "1.31", "--launch-template-version", "3", "--wait"}
		assertArgsEqual(t, want, args)
	})
}

func TestBuildUpdateAddonArgs(t *testing.T) {
	args := BuildUpdateAddonArgs(AddonUpgradeOptions{Cluster: "prod", Region: "ap-south-1", Name: "vpc-cni", Version: "v1.19.0"})
	want := []string{"update", "addon", "--cluster", "prod", "--region", "ap-south-1", "--name", "vpc-cni", "--version", "v1.19.0", "--wait"}
	assertArgsEqual(t, want, args)
}

func assertArgsEqual(t *testing.T, want, got []string) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("arg length mismatch: want %v got %v", want, got)
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("arg %d mismatch: want %q got %q (full: want=%v got=%v)", i, want[i], got[i], want, got)
		}
	}
}

// writeFakeEksctlScript writes a small shell script masquerading as `eksctl` that
// behaves according to its first argument, so SubprocessExecutor can be tested without
// a real eksctl binary.
func writeFakeEksctlScript(t *testing.T, behavior string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake eksctl script test requires a POSIX shell")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "eksctl")

	var script string
	switch behavior {
	case "success":
		script = "#!/bin/sh\necho 'all good'\nexit 0\n"
	case "failure":
		script = "#!/bin/sh\necho 'boom' 1>&2\nexit 2\n"
	case "sleep":
		script = "#!/bin/sh\nsleep 5\necho 'too slow'\nexit 0\n"
	}
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake eksctl script: %v", err)
	}
	return path
}

func TestSubprocessExecutor_Success(t *testing.T) {
	bin := writeFakeEksctlScript(t, "success")
	e := &SubprocessExecutor{BinaryPath: bin}

	result, err := e.Run(context.Background(), []string{"version"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Stdout == "" {
		t.Fatal("expected non-empty stdout")
	}
}

func TestSubprocessExecutor_NonZeroExit(t *testing.T) {
	bin := writeFakeEksctlScript(t, "failure")
	e := &SubprocessExecutor{BinaryPath: bin}

	result, err := e.Run(context.Background(), []string{"upgrade", "cluster"})
	if err == nil {
		t.Fatal("expected error for non-zero exit code")
	}
	if result.ExitCode != 2 {
		t.Fatalf("expected exit code 2, got %d", result.ExitCode)
	}
}

func TestSubprocessExecutor_Timeout(t *testing.T) {
	bin := writeFakeEksctlScript(t, "sleep")
	e := &SubprocessExecutor{BinaryPath: bin}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := e.Run(ctx, []string{"upgrade", "cluster"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
