package readiness

import (
	"fmt"

	"eks-upgrader/internal/config"
)

// DeprecatedAPIFinding is a single raw signal from any deprecated-API scanner (metrics
// scrape, Pluto, kubent). ClassifyDeprecatedAPIs converts these into readiness Findings.
type DeprecatedAPIFinding struct {
	Source           string // "apiserver-metrics" | "pluto" | "kubent"
	ResourceKind     string
	ResourceName     string
	APIVersion       string
	Replacement      string // recommended replacement apiVersion, if known
	RemovedInVersion string // Kubernetes version the API is removed in, if known; empty if only deprecated
}

// ClassifyDeprecatedAPIs applies the buildplan's severity policy:
//   - API removed in target Kubernetes version -> BLOCKING
//   - API deprecated but not removed in target version -> WARNING
//   - Live client still calling a deprecated API (apiserver-metrics source) -> WARNING or
//     BLOCKING depending on policy.FailOnRemovedInTargetVers / policy.FailOnFound
func ClassifyDeprecatedAPIs(raw []DeprecatedAPIFinding, targetVersion string, policy config.DeprecatedAPIsSpec) []Finding {
	if !policy.FailOnFound && !policy.WarnOnDeprecatedBefRemove && !policy.FailOnRemovedInTargetVers {
		return nil
	}

	var findings []Finding
	for _, r := range raw {
		f := Finding{
			Source:   r.Source,
			Category: "deprecated-api",
			Resource: fmt.Sprintf("%s/%s", r.ResourceKind, r.ResourceName),
			Message:  fmt.Sprintf("%s uses deprecated API %s", fmt.Sprintf("%s/%s", r.ResourceKind, r.ResourceName), r.APIVersion),
		}
		if r.Replacement != "" {
			f.Recommendation = fmt.Sprintf("migrate to %s", r.Replacement)
		}

		switch {
		case r.RemovedInVersion != "" && r.RemovedInVersion == targetVersion:
			f.Severity = SeverityBlocking
			f.Message = fmt.Sprintf("API %s used by %s is removed in target Kubernetes version %s", r.APIVersion, f.Resource, targetVersion)
			if !policy.FailOnRemovedInTargetVers {
				f.Severity = SeverityWarning
			}
		case r.RemovedInVersion != "":
			// Removed in some future version, not the immediate target: still deprecated now.
			f.Severity = SeverityWarning
		default:
			if !policy.WarnOnDeprecatedBefRemove {
				continue
			}
			f.Severity = SeverityWarning
		}
		findings = append(findings, f)
	}
	return findings
}
