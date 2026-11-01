package reconcile

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ToYAML serializes the observed cluster snapshot for output/observed-cluster.yaml.
func (o *ObservedCluster) ToYAML() ([]byte, error) {
	return yaml.Marshal(o)
}

// LoadObservedCluster parses a previously written output/observed-cluster.yaml, used
// by `state diff` to compare against desired state without re-collecting live data.
func LoadObservedCluster(path string) (*ObservedCluster, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading observed cluster state %q: %w", path, err)
	}
	var o ObservedCluster
	if err := yaml.Unmarshal(raw, &o); err != nil {
		return nil, fmt.Errorf("parsing observed cluster state %q: %w", path, err)
	}
	return &o, nil
}

// ReportJSON serializes findings for output/drift-report.json.
func ReportJSON(findings []Finding) ([]byte, error) {
	return json.MarshalIndent(findings, "", "  ")
}

// classificationOrder controls the display order in the Markdown report (most severe first).
var classificationOrder = map[Classification]int{
	Blocking: 0, Warning: 1, Unmanaged: 2, Info: 3, InSync: 4,
}

// ReportMarkdown renders a human-readable drift report for output/drift-report.md.
func ReportMarkdown(findings []Finding) string {
	sorted := make([]Finding, len(findings))
	copy(sorted, findings)
	sort.SliceStable(sorted, func(i, j int) bool {
		return classificationOrder[sorted[i].Classification] < classificationOrder[sorted[j].Classification]
	})

	var b strings.Builder
	b.WriteString("# Live-State Drift Report\n\n")

	counts := map[Classification]int{}
	for _, f := range findings {
		counts[f.Classification]++
	}
	b.WriteString(fmt.Sprintf("Blocking: %d | Warning: %d | Unmanaged: %d | Info: %d | In sync: %d\n\n",
		counts[Blocking], counts[Warning], counts[Unmanaged], counts[Info], counts[InSync]))

	if counts[Blocking] > 0 {
		b.WriteString("**Upgrade is blocked until blocking drift is resolved.**\n\n")
	}

	b.WriteString("| Classification | Field | Desired | Observed | Message |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, f := range sorted {
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n", f.Classification, f.Field, f.Desired, f.Observed, f.Message))
	}
	return b.String()
}
