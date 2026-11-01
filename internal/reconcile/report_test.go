package reconcile

import (
	"os"
	"strings"
	"testing"
)

func TestReportMarkdown_SortsBlockingFirst(t *testing.T) {
	findings := []Finding{
		{Field: "a", Classification: InSync},
		{Field: "b", Classification: Blocking, Message: "bad"},
		{Field: "c", Classification: Warning},
	}
	md := ReportMarkdown(findings)
	if !strings.Contains(md, "Upgrade is blocked") {
		t.Fatal("expected blocking banner in markdown report")
	}
	bIdx := strings.Index(md, "| BLOCKING | b |")
	cIdx := strings.Index(md, "| WARNING | c |")
	aIdx := strings.Index(md, "| IN_SYNC | a |")
	if bIdx == -1 || cIdx == -1 || aIdx == -1 {
		t.Fatalf("expected all classifications present in table:\n%s", md)
	}
	if !(bIdx < cIdx && cIdx < aIdx) {
		t.Fatalf("expected BLOCKING before WARNING before IN_SYNC, got order in:\n%s", md)
	}
}

func TestReportJSON(t *testing.T) {
	findings := []Finding{{Field: "cluster.version", Classification: Blocking}}
	data, err := ReportJSON(findings)
	if err != nil {
		t.Fatalf("ReportJSON: %v", err)
	}
	if !strings.Contains(string(data), "cluster.version") {
		t.Fatalf("expected JSON to contain field name, got: %s", data)
	}
}

func TestObservedCluster_ToYAML(t *testing.T) {
	o := &ObservedCluster{Metadata: ObservedMetadata{Cluster: "prod"}}
	data, err := o.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML: %v", err)
	}
	if !strings.Contains(string(data), "cluster: prod") {
		t.Fatalf("expected YAML to contain cluster name, got: %s", data)
	}
}

func TestLoadObservedCluster_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/observed-cluster.yaml"

	original := &ObservedCluster{
		Metadata: ObservedMetadata{Cluster: "prod", Region: "ap-south-1"},
		Cluster:  ObservedClusterInfo{CurrentKubernetesVersion: "1.30"},
	}
	data, err := original.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("writing observed cluster file: %v", err)
	}

	loaded, err := LoadObservedCluster(path)
	if err != nil {
		t.Fatalf("LoadObservedCluster: %v", err)
	}
	if loaded.Metadata.Cluster != "prod" || loaded.Cluster.CurrentKubernetesVersion != "1.30" {
		t.Fatalf("unexpected round-tripped observed cluster: %+v", loaded)
	}
}
