package readiness

import (
	"fmt"

	"eks-upgrader/internal/aws"
	"eks-upgrader/internal/config"
)

// ClassifyInsights maps EKS Upgrade Insights into readiness findings per policy:
// ERROR insights for APIs removed in the target version are BLOCKING, WARNING insights
// require explicit acknowledgement, PASSING is safe, and UNKNOWN defaults to WARNING.
func ClassifyInsights(insights []aws.Insight, policy config.EKSUpgradeInsightsSpec) []Finding {
	if !policy.Enabled {
		return nil
	}

	var findings []Finding
	for _, ins := range insights {
		f := Finding{
			Source:   "eks-upgrade-insights",
			Category: ins.Category,
			Resource: ins.Name,
		}
		switch ins.Status {
		case "PASSING":
			f.Severity = SeveritySafe
			f.Message = fmt.Sprintf("insight %q is passing", ins.Name)
		case "ERROR":
			f.Severity = SeverityBlocking
			f.Message = fmt.Sprintf("insight %q reported ERROR: %s", ins.Name, ins.Description)
			f.Recommendation = "resolve the underlying issue before upgrading; see the EKS console Upgrade Insights tab for remediation guidance"
		case "WARNING":
			f.Severity = SeverityWarning
			f.Message = fmt.Sprintf("insight %q reported WARNING: %s", ins.Name, ins.Description)
			f.Recommendation = "review and acknowledge before proceeding"
		default: // UNKNOWN or any unrecognized status
			f.Severity = SeverityWarning
			f.Message = fmt.Sprintf("insight %q has an unrecognized status %q; treating as a warning", ins.Name, ins.Status)
		}

		if !policy.FailOnError && f.Severity == SeverityBlocking {
			f.Severity = SeverityWarning
			f.Message += " (downgraded to warning: preflight.eksUpgradeInsights.failOnError=false)"
		}
		findings = append(findings, f)
	}
	return findings
}
