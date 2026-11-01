package scanners

import (
	"context"
	"testing"
)

type fakeToolRunner struct {
	stdout []byte
	err    error
	calls  [][]string
}

func (f *fakeToolRunner) Run(ctx context.Context, binary string, args []string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{binary}, args...))
	return f.stdout, f.err
}

func TestScanWithPluto_ParsesFindings(t *testing.T) {
	runner := &fakeToolRunner{stdout: []byte(`{
		"items": [
			{"name": "example", "namespace": "default", "kind": "FlowSchema", "api-version": "flowcontrol.apiserver.k8s.io/v1beta3", "replacement-api": "flowcontrol.apiserver.k8s.io/v1", "removed-in": "1.31", "deprecated": true, "removed": true}
		]
	}`)}

	findings, err := ScanWithPluto(context.Background(), runner, []string{"./manifests"}, "1.31")
	if err != nil {
		t.Fatalf("ScanWithPluto: %v", err)
	}
	if len(findings) != 1 || findings[0].RemovedInVersion != "1.31" {
		t.Fatalf("unexpected findings: %+v", findings)
	}
}

func TestScanWithPluto_NotInstalledReturnsNilNil(t *testing.T) {
	runner := &fakeToolRunner{err: ErrToolNotInstalled}
	findings, err := ScanWithPluto(context.Background(), runner, []string{"./manifests"}, "1.31")
	if err != nil {
		t.Fatalf("expected nil error when pluto is not installed, got %v", err)
	}
	if findings != nil {
		t.Fatalf("expected nil findings when pluto is not installed, got %+v", findings)
	}
}

func TestScanWithKubent_ParsesFindings(t *testing.T) {
	runner := &fakeToolRunner{stdout: []byte(`[
		{"ObjectName": "CronJob", "Namespace": "default", "Name": "job-1", "ApiVersion": "batch/v1beta1", "ReplaceWith": "batch/v1", "Since": "1.25"}
	]`)}

	findings, err := ScanWithKubent(context.Background(), runner, nil)
	if err != nil {
		t.Fatalf("ScanWithKubent: %v", err)
	}
	if len(findings) != 1 || findings[0].ResourceName != "job-1" {
		t.Fatalf("unexpected findings: %+v", findings)
	}
}

func TestScanWithKubent_NotInstalledReturnsNilNil(t *testing.T) {
	runner := &fakeToolRunner{err: ErrToolNotInstalled}
	findings, err := ScanWithKubent(context.Background(), runner, nil)
	if err != nil {
		t.Fatalf("expected nil error when kubent is not installed, got %v", err)
	}
	if findings != nil {
		t.Fatalf("expected nil findings when kubent is not installed, got %+v", findings)
	}
}
