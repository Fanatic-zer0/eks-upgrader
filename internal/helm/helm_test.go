package helm

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestBuildUpgradeArgs(t *testing.T) {
	args := BuildUpgradeArgs(UpgradeOptions{
		ReleaseName: "karpenter", Namespace: "karpenter",
		ChartRepository: "https://charts.karpenter.sh", ChartName: "karpenter",
		ChartVersion: "1.6.0", Wait: true, Timeout: 20 * time.Minute,
	})
	want := []string{
		"upgrade", "karpenter", "karpenter", "--install",
		"--repo", "https://charts.karpenter.sh",
		"--namespace", "karpenter", "--create-namespace",
		"--version", "1.6.0", "--wait", "--timeout", "20m0s",
	}
	if len(args) != len(want) {
		t.Fatalf("arg length mismatch: want %v got %v", want, args)
	}
	for i := range want {
		if want[i] != args[i] {
			t.Fatalf("arg %d mismatch: want %q got %q", i, want[i], args[i])
		}
	}
}

func TestCLIClient_Upgrade_Success(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "helm")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho 'Release \"karpenter\" has been upgraded'\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("writing fake helm script: %v", err)
	}

	c := &CLIClient{BinaryPath: path}
	result, err := c.Upgrade(context.Background(), UpgradeOptions{ReleaseName: "karpenter", Namespace: "karpenter", ChartRepository: "https://charts.karpenter.sh", ChartName: "karpenter"})
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
}

func TestCLIClient_Upgrade_Failure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "helm")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho 'boom' 1>&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("writing fake helm script: %v", err)
	}

	c := &CLIClient{BinaryPath: path}
	_, err := c.Upgrade(context.Background(), UpgradeOptions{ReleaseName: "karpenter", Namespace: "karpenter", ChartRepository: "https://charts.karpenter.sh", ChartName: "karpenter"})
	if err == nil {
		t.Fatal("expected error for non-zero exit code")
	}
}
