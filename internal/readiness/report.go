package readiness

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

var severityOrder = map[Severity]int{SeverityBlocking: 0, SeverityWarning: 1, SeveritySafe: 2}

// ReportJSON serializes the full report for output/upgrade-readiness-report.json.
func ReportJSON(r *Report) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// DeprecatedAPIFindingsJSON serializes just the deprecated-api-category findings for
// output/deprecated-api-findings.json.
func DeprecatedAPIFindingsJSON(r *Report) ([]byte, error) {
	var deprecated []Finding
	for _, f := range r.Findings {
		if f.Category == "deprecated-api" {
			deprecated = append(deprecated, f)
		}
	}
	return json.MarshalIndent(deprecated, "", "  ")
}

// ReportMarkdown renders the human-readable terminal/file report for
// output/upgrade-readiness-report.md, matching the buildplan's terminal warning format.
func ReportMarkdown(r *Report) string {
	sorted := make([]Finding, len(r.Findings))
	copy(sorted, r.Findings)
	sort.SliceStable(sorted, func(i, j int) bool {
		return severityOrder[sorted[i].Severity] < severityOrder[sorted[j].Severity]
	})

	var b strings.Builder
	icon := "✅"
	if r.Status == StatusNotReady {
		icon = "⚠️ "
	} else if r.Status == StatusReadyWithWarnings {
		icon = "⚠️ "
	}
	fmt.Fprintf(&b, "%s UPGRADE READINESS: %s\n\n", icon, r.Status)

	for _, f := range sorted {
		if f.Severity == SeveritySafe {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", f.Severity, f.Message)
		if f.Resource != "" {
			fmt.Fprintf(&b, "  Resource: %s\n", f.Resource)
		}
		if f.Recommendation != "" {
			fmt.Fprintf(&b, "  Recommendation: %s\n", f.Recommendation)
		}
		b.WriteString("\n")
	}

	if r.Status == StatusNotReady {
		b.WriteString("Upgrade aborted due to blocking findings.\n")
	} else if r.Status == StatusReadyWithWarnings {
		b.WriteString("Upgrade may proceed once all warnings are acknowledged.\n")
	} else {
		b.WriteString("Upgrade is clear to proceed.\n")
	}
	return b.String()
}

// RemediationPlanMarkdown renders output/remediation-plan.md: one actionable item per
// non-safe finding.
func RemediationPlanMarkdown(r *Report) string {
	var b strings.Builder
	b.WriteString("# Remediation Plan\n\n")
	n := 0
	for _, f := range r.Findings {
		if f.Severity == SeveritySafe {
			continue
		}
		n++
		fmt.Fprintf(&b, "%d. **[%s]** %s\n", n, f.Severity, f.Message)
		if f.Recommendation != "" {
			fmt.Fprintf(&b, "   - %s\n", f.Recommendation)
		}
	}
	if n == 0 {
		b.WriteString("No remediation required.\n")
	}
	return b.String()
}
