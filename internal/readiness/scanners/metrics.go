// Package scanners implements the individual deprecated-API detection signals:
// a direct apiserver /metrics scrape, and optional external Pluto/kubent CLI scans.
package scanners

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"eks-upgrader/internal/readiness"
)

// MetricsFetcher fetches the raw Prometheus-format apiserver metrics text, e.g. via
// `kubectl get --raw /metrics` or the equivalent REST client call.
type MetricsFetcher interface {
	FetchRawMetrics(ctx context.Context) ([]byte, error)
}

// deprecatedAPIMetricLine matches lines like:
// apiserver_requested_deprecated_apis{group="flowcontrol.apiserver.k8s.io",removed_release="1.29",resource="flowschemas",subresource="",version="v1beta3"} 1
var deprecatedAPIMetricLine = regexp.MustCompile(`^apiserver_requested_deprecated_apis\{([^}]*)\}\s+(\d+)`)
var labelPair = regexp.MustCompile(`(\w+)="([^"]*)"`)

// ScrapeDeprecatedAPIMetrics parses the apiserver_requested_deprecated_apis gauge,
// which reports every deprecated API a live client has actually called — the strongest
// signal that migration is still required, independent of static manifest scans.
func ScrapeDeprecatedAPIMetrics(ctx context.Context, fetcher MetricsFetcher) ([]readiness.DeprecatedAPIFinding, error) {
	raw, err := fetcher.FetchRawMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("scraping apiserver deprecated-API metrics: %w", err)
	}

	var findings []readiness.DeprecatedAPIFinding
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		m := deprecatedAPIMetricLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if m[2] == "0" {
			continue // zero requests observed; not actually in use
		}

		labels := map[string]string{}
		for _, lm := range labelPair.FindAllStringSubmatch(m[1], -1) {
			labels[lm[1]] = lm[2]
		}

		apiVersion := labels["version"]
		if labels["group"] != "" {
			apiVersion = labels["group"] + "/" + labels["version"]
		}

		findings = append(findings, readiness.DeprecatedAPIFinding{
			Source:           "apiserver-metrics",
			ResourceKind:     labels["resource"],
			ResourceName:     "(live client)",
			APIVersion:       apiVersion,
			RemovedInVersion: labels["removed_release"],
		})
	}
	return findings, nil
}
