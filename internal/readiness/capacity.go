package readiness

import (
	"fmt"

	"eks-upgrader/internal/aws"
)

// CheckSubnetCapacity flags any subnet with fewer available IPs than minAvailableIPs,
// which would block Karpenter/managed-node-group replacement capacity during rollout.
func CheckSubnetCapacity(subnets []aws.SubnetCapacity, minAvailableIPs int) []Finding {
	var findings []Finding
	for _, s := range subnets {
		if int(s.AvailableIPAddresses) < minAvailableIPs {
			findings = append(findings, Finding{
				Source: "capacity", Category: "subnet-ip", Resource: s.SubnetID, Severity: SeverityBlocking,
				Message:        fmt.Sprintf("subnet %s (%s) has only %d available IPs, below the required minimum of %d", s.SubnetID, s.AvailabilityZone, s.AvailableIPAddresses, minAvailableIPs),
				Recommendation: "free up IP capacity or provision additional subnets before upgrading",
			})
		}
	}
	return findings
}

// CheckPDBIssues converts raw PDB validation issue strings (k8s.ValidatePDBsForNamespaces)
// into blocking readiness findings.
func CheckPDBIssues(issues []string) []Finding {
	var findings []Finding
	for _, issue := range issues {
		findings = append(findings, Finding{
			Source: "pdb", Category: "pod-disruption-budget", Severity: SeverityBlocking,
			Message:        issue,
			Recommendation: "adjust the PDB's minAvailable/maxUnavailable or scale up replicas so at least one disruption is permitted",
		})
	}
	return findings
}

// CheckNodeReadiness flags any NotReady node as blocking.
func CheckNodeReadiness(allReady bool, notReadyNodes []string) []Finding {
	if allReady {
		return nil
	}
	var findings []Finding
	for _, n := range notReadyNodes {
		findings = append(findings, Finding{
			Source: "node-readiness", Category: "node", Resource: n, Severity: SeverityBlocking,
			Message:        fmt.Sprintf("node %s is not Ready", n),
			Recommendation: "investigate and resolve node health before upgrading",
		})
	}
	return findings
}

// CheckPendingPods flags pending-pod count above maxPendingPods as blocking.
func CheckPendingPods(pendingCount, maxPendingPods int) []Finding {
	if pendingCount <= maxPendingPods {
		return nil
	}
	return []Finding{{
		Source: "pending-pods", Category: "scheduling", Severity: SeverityBlocking,
		Message:        fmt.Sprintf("%d pods are Pending, exceeding the allowed maximum of %d", pendingCount, maxPendingPods),
		Recommendation: "investigate scheduling failures (capacity, taints, affinity) before upgrading",
	}}
}
