package readiness

import (
	"testing"

	"eks-upgrader/internal/aws"
	"eks-upgrader/internal/config"
)

func TestClassifyInsights(t *testing.T) {
	policy := config.EKSUpgradeInsightsSpec{Enabled: true, FailOnError: true}
	insights := []aws.Insight{
		{Name: "flow-schema-check", Status: "ERROR", Description: "uses removed API"},
		{Name: "kube-proxy-check", Status: "WARNING", Description: "should upgrade after control plane"},
		{Name: "all-good", Status: "PASSING"},
		{Name: "weird", Status: "UNKNOWN"},
	}

	findings := ClassifyInsights(insights, policy)
	if len(findings) != 4 {
		t.Fatalf("expected 4 findings, got %d", len(findings))
	}

	want := map[string]Severity{
		"flow-schema-check": SeverityBlocking,
		"kube-proxy-check":  SeverityWarning,
		"all-good":          SeveritySafe,
		"weird":             SeverityWarning,
	}
	for _, f := range findings {
		if f.Severity != want[f.Resource] {
			t.Fatalf("insight %q: got severity %s, want %s", f.Resource, f.Severity, want[f.Resource])
		}
	}
}

func TestClassifyInsights_FailOnErrorFalseDowngradesToWarning(t *testing.T) {
	policy := config.EKSUpgradeInsightsSpec{Enabled: true, FailOnError: false}
	insights := []aws.Insight{{Name: "flow-schema-check", Status: "ERROR"}}

	findings := ClassifyInsights(insights, policy)
	if len(findings) != 1 || findings[0].Severity != SeverityWarning {
		t.Fatalf("expected downgraded WARNING, got %+v", findings)
	}
}

func TestClassifyInsights_DisabledReturnsNil(t *testing.T) {
	findings := ClassifyInsights([]aws.Insight{{Name: "x", Status: "ERROR"}}, config.EKSUpgradeInsightsSpec{Enabled: false})
	if findings != nil {
		t.Fatalf("expected nil findings when disabled, got %+v", findings)
	}
}
