package reconcile

import (
	"testing"

	"eks-upgrader/internal/config"
)

func TestApplyPolicy_IgnoredFieldsForceInSync(t *testing.T) {
	findings := []Finding{{Field: "managedNodeGroups[system].labels", Classification: Warning}}
	policy := config.StateReconciliationSpec{IgnoredFields: []string{"managedNodeGroups[*].labels"}}

	out := ApplyPolicy(findings, policy)
	if out[0].Classification != InSync {
		t.Fatalf("expected IN_SYNC after ignoredFields override, got %s", out[0].Classification)
	}
}

func TestApplyPolicy_WarnOnlyCapsSeverity(t *testing.T) {
	findings := []Finding{{Field: "managedNodeGroups[rogue]", Classification: Unmanaged}}
	policy := config.StateReconciliationSpec{WarnOnlyOn: []string{"managedNodeGroups[*]"}}

	out := ApplyPolicy(findings, policy)
	if out[0].Classification != Warning {
		t.Fatalf("expected WARNING after warnOnlyOn cap, got %s", out[0].Classification)
	}
}

func TestApplyPolicy_BlockOnForcesBlocking(t *testing.T) {
	findings := []Finding{{Field: "cluster.tags", Classification: Warning}}
	policy := config.StateReconciliationSpec{BlockOn: []string{"cluster.tags"}}

	out := ApplyPolicy(findings, policy)
	if out[0].Classification != Blocking {
		t.Fatalf("expected BLOCKING after blockOn override, got %s", out[0].Classification)
	}
}

func TestApplyPolicy_NoMatchKeepsDefault(t *testing.T) {
	findings := []Finding{{Field: "cluster.version", Classification: Blocking}}
	out := ApplyPolicy(findings, config.StateReconciliationSpec{})
	if out[0].Classification != Blocking {
		t.Fatalf("expected default classification preserved, got %s", out[0].Classification)
	}
}
