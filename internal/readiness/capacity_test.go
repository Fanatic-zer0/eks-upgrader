package readiness

import (
	"testing"

	"eks-upgrader/internal/aws"
)

func TestCheckSubnetCapacity(t *testing.T) {
	subnets := []aws.SubnetCapacity{
		{SubnetID: "subnet-a", AvailableIPAddresses: 20},
		{SubnetID: "subnet-b", AvailableIPAddresses: 5},
	}
	findings := CheckSubnetCapacity(subnets, 10)
	if len(findings) != 1 || findings[0].Resource != "subnet-b" {
		t.Fatalf("expected 1 blocking finding for subnet-b, got %+v", findings)
	}
}

func TestCheckPDBIssues(t *testing.T) {
	findings := CheckPDBIssues([]string{"PDB checkout/checkout-pdb allows 0 disruptions"})
	if len(findings) != 1 || findings[0].Severity != SeverityBlocking {
		t.Fatalf("expected 1 blocking finding, got %+v", findings)
	}
}

func TestCheckNodeReadiness(t *testing.T) {
	if findings := CheckNodeReadiness(true, nil); findings != nil {
		t.Fatalf("expected no findings when all nodes ready, got %+v", findings)
	}
	findings := CheckNodeReadiness(false, []string{"node-2"})
	if len(findings) != 1 || findings[0].Resource != "node-2" {
		t.Fatalf("expected 1 blocking finding for node-2, got %+v", findings)
	}
}

func TestCheckPendingPods(t *testing.T) {
	if findings := CheckPendingPods(0, 0); findings != nil {
		t.Fatalf("expected no findings for 0 pending pods, got %+v", findings)
	}
	findings := CheckPendingPods(3, 0)
	if len(findings) != 1 {
		t.Fatalf("expected 1 blocking finding, got %+v", findings)
	}
}
