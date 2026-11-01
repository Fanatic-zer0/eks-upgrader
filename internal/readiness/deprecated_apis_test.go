package readiness

import (
	"testing"

	"eks-upgrader/internal/config"
)

func TestClassifyDeprecatedAPIs_RemovedInTargetIsBlocking(t *testing.T) {
	policy := config.DeprecatedAPIsSpec{FailOnFound: true, FailOnRemovedInTargetVers: true, WarnOnDeprecatedBefRemove: true}
	raw := []DeprecatedAPIFinding{
		{Source: "apiserver-metrics", ResourceKind: "flowschemas", APIVersion: "flowcontrol.apiserver.k8s.io/v1beta3", RemovedInVersion: "1.31"},
	}

	findings := ClassifyDeprecatedAPIs(raw, "1.31", policy)
	if len(findings) != 1 || findings[0].Severity != SeverityBlocking {
		t.Fatalf("expected 1 blocking finding, got %+v", findings)
	}
}

func TestClassifyDeprecatedAPIs_RemovedInFutureVersionIsWarning(t *testing.T) {
	policy := config.DeprecatedAPIsSpec{FailOnFound: true, FailOnRemovedInTargetVers: true, WarnOnDeprecatedBefRemove: true}
	raw := []DeprecatedAPIFinding{
		{Source: "pluto", ResourceKind: "ingress", APIVersion: "networking.k8s.io/v1beta1", RemovedInVersion: "1.33"},
	}

	findings := ClassifyDeprecatedAPIs(raw, "1.31", policy)
	if len(findings) != 1 || findings[0].Severity != SeverityWarning {
		t.Fatalf("expected 1 warning finding for future removal, got %+v", findings)
	}
}

func TestClassifyDeprecatedAPIs_FailOnRemovedFalseDowngrades(t *testing.T) {
	policy := config.DeprecatedAPIsSpec{FailOnFound: true, FailOnRemovedInTargetVers: false, WarnOnDeprecatedBefRemove: true}
	raw := []DeprecatedAPIFinding{
		{Source: "apiserver-metrics", ResourceKind: "flowschemas", APIVersion: "flowcontrol.apiserver.k8s.io/v1beta3", RemovedInVersion: "1.31"},
	}

	findings := ClassifyDeprecatedAPIs(raw, "1.31", policy)
	if len(findings) != 1 || findings[0].Severity != SeverityWarning {
		t.Fatalf("expected downgraded WARNING, got %+v", findings)
	}
}

func TestClassifyDeprecatedAPIs_MerelyDeprecatedHonorsWarnFlag(t *testing.T) {
	raw := []DeprecatedAPIFinding{{Source: "pluto", ResourceKind: "cronjob", APIVersion: "batch/v1beta1"}}

	disabled := ClassifyDeprecatedAPIs(raw, "1.31", config.DeprecatedAPIsSpec{WarnOnDeprecatedBefRemove: false})
	if len(disabled) != 0 {
		t.Fatalf("expected no findings when warnOnDeprecatedBeforeRemoval is false, got %+v", disabled)
	}

	enabled := ClassifyDeprecatedAPIs(raw, "1.31", config.DeprecatedAPIsSpec{WarnOnDeprecatedBefRemove: true})
	if len(enabled) != 1 || enabled[0].Severity != SeverityWarning {
		t.Fatalf("expected 1 warning finding, got %+v", enabled)
	}
}
