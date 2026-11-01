// Package readiness implements the Upgrade Readiness Analyzer: EKS Upgrade Insights
// classification, deprecated-API detection, Karpenter/add-on compatibility checks, and
// capacity/PDB/node-readiness checks, aggregated into an overall READY /
// READY_WITH_WARNINGS / NOT_READY verdict.
package readiness

import "time"

// Severity is the per-finding readiness classification.
type Severity string

const (
	SeveritySafe     Severity = "SAFE"
	SeverityWarning  Severity = "WARNING"
	SeverityBlocking Severity = "BLOCKING"
)

// Status is the overall readiness verdict.
type Status string

const (
	StatusReady             Status = "READY"
	StatusReadyWithWarnings Status = "READY_WITH_WARNINGS"
	StatusNotReady          Status = "NOT_READY"
)

// Finding is a single readiness signal from any source (EKS Insights, deprecated API
// scan, compatibility check, capacity/PDB/node check).
type Finding struct {
	Source         string   `json:"source"`
	Category       string   `json:"category"`
	Resource       string   `json:"resource,omitempty"`
	Severity       Severity `json:"severity"`
	Message        string   `json:"message"`
	Recommendation string   `json:"recommendation,omitempty"`
}

// Report is the full Upgrade Readiness Analyzer output.
type Report struct {
	Status      Status    `json:"status"`
	GeneratedAt time.Time `json:"generatedAt"`
	Findings    []Finding `json:"findings"`
}

// HasBlocking reports whether any finding is BLOCKING.
func (r *Report) HasBlocking() bool {
	for _, f := range r.Findings {
		if f.Severity == SeverityBlocking {
			return true
		}
	}
	return false
}

// Warnings returns all WARNING-severity findings, used to drive the acknowledgement flow.
func (r *Report) Warnings() []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Severity == SeverityWarning {
			out = append(out, f)
		}
	}
	return out
}

// ComputeStatus derives the overall Status from a finding set.
func ComputeStatus(findings []Finding) Status {
	hasBlocking := false
	hasWarning := false
	for _, f := range findings {
		switch f.Severity {
		case SeverityBlocking:
			hasBlocking = true
		case SeverityWarning:
			hasWarning = true
		}
	}
	switch {
	case hasBlocking:
		return StatusNotReady
	case hasWarning:
		return StatusReadyWithWarnings
	default:
		return StatusReady
	}
}
