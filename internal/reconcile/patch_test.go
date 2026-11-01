package reconcile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eks-upgrader/internal/config"
)

func TestGenerateClusterPatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cluster.yaml")
	content := "apiVersion: eksctl.io/v1alpha5\nkind: ClusterConfig\nmetadata:\n  name: prod\n  version: \"1.30\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test cluster.yaml: %v", err)
	}

	cfg := &config.ClusterConfig{SourcePath: path, Metadata: config.ClusterMetadata{Version: "1.30"}}
	patch, err := GenerateClusterPatch(cfg, "1.31")
	if err != nil {
		t.Fatalf("GenerateClusterPatch: %v", err)
	}
	if !strings.Contains(patch, `-  version: "1.30"`) || !strings.Contains(patch, `+  version: "1.31"`) {
		t.Fatalf("patch does not contain expected version change:\n%s", patch)
	}

	// Original file must remain untouched.
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading cluster.yaml: %v", err)
	}
	if string(original) != content {
		t.Fatal("GenerateClusterPatch must never modify the original file")
	}
}

func TestGenerateClusterPatch_VersionNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cluster.yaml")
	if err := os.WriteFile(path, []byte("metadata:\n  name: prod\n"), 0o644); err != nil {
		t.Fatalf("writing test cluster.yaml: %v", err)
	}

	cfg := &config.ClusterConfig{SourcePath: path, Metadata: config.ClusterMetadata{Version: "1.30"}}
	if _, err := GenerateClusterPatch(cfg, "1.31"); err == nil {
		t.Fatal("expected error when version string cannot be located")
	}
}

func TestGenerateKarpenterNodeClassPatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ec2nodeclass.yaml")
	content := "spec:\n  amiSelectorTerms:\n    - alias: al2023@v20250815\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test manifest: %v", err)
	}

	patch, err := GenerateKarpenterNodeClassPatch(path, "al2023@v20250815", "al2023@v20250918")
	if err != nil {
		t.Fatalf("GenerateKarpenterNodeClassPatch: %v", err)
	}
	if !strings.Contains(patch, "al2023@v20250815") || !strings.Contains(patch, "al2023@v20250918") {
		t.Fatalf("patch does not contain expected alias change:\n%s", patch)
	}
	var sawRemoval, sawAddition bool
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "-") && strings.Contains(line, "al2023@v20250815") {
			sawRemoval = true
		}
		if strings.HasPrefix(line, "+") && strings.Contains(line, "al2023@v20250918") {
			sawAddition = true
		}
	}
	if !sawRemoval || !sawAddition {
		t.Fatalf("expected a removal line for old alias and addition line for new alias:\n%s", patch)
	}
}
