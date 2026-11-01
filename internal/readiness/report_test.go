package readiness

import (
	"strings"
	"testing"
	"time"
)

func TestComputeStatus(t *testing.T) {
	tests := []struct {
		name     string
		findings []Finding
		want     Status
	}{
		{"all safe", []Finding{{Severity: SeveritySafe}}, StatusReady},
		{"has warning", []Finding{{Severity: SeveritySafe}, {Severity: SeverityWarning}}, StatusReadyWithWarnings},
		{"has blocking", []Finding{{Severity: SeverityWarning}, {Severity: SeverityBlocking}}, StatusNotReady},
		{"empty", nil, StatusReady},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ComputeStatus(tc.findings); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestReportMarkdown_NotReadyShowsAbortMessage(t *testing.T) {
	r := &Report{
		Status:      StatusNotReady,
		GeneratedAt: time.Now(),
		Findings: []Finding{
			{Severity: SeverityBlocking, Message: "Deprecated API removed in Kubernetes 1.31", Resource: "FlowSchema/example", Recommendation: "migrate to v1"},
		},
	}
	md := ReportMarkdown(r)
	if !strings.Contains(md, "NOT_READY") || !strings.Contains(md, "Upgrade aborted due to blocking findings.") {
		t.Fatalf("unexpected markdown report:\n%s", md)
	}
}

func TestRemediationPlanMarkdown_NoFindings(t *testing.T) {
	r := &Report{Status: StatusReady}
	plan := RemediationPlanMarkdown(r)
	if !strings.Contains(plan, "No remediation required.") {
		t.Fatalf("expected no-remediation message, got:\n%s", plan)
	}
}

func TestDeprecatedAPIFindingsJSON_FiltersByCategory(t *testing.T) {
	r := &Report{Findings: []Finding{
		{Category: "deprecated-api", Message: "a"},
		{Category: "capacity", Message: "b"},
	}}
	data, err := DeprecatedAPIFindingsJSON(r)
	if err != nil {
		t.Fatalf("DeprecatedAPIFindingsJSON: %v", err)
	}
	if strings.Contains(string(data), `"b"`) || !strings.Contains(string(data), `"a"`) {
		t.Fatalf("expected only deprecated-api findings, got: %s", data)
	}
}
