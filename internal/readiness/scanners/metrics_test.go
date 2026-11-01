package scanners

import (
	"context"
	"testing"
)

type fakeMetricsFetcher struct {
	raw []byte
	err error
}

func (f *fakeMetricsFetcher) FetchRawMetrics(ctx context.Context) ([]byte, error) {
	return f.raw, f.err
}

func TestScrapeDeprecatedAPIMetrics(t *testing.T) {
	raw := `# HELP apiserver_requested_deprecated_apis deprecated api usage
# TYPE apiserver_requested_deprecated_apis gauge
apiserver_requested_deprecated_apis{group="flowcontrol.apiserver.k8s.io",removed_release="1.31",resource="flowschemas",subresource="",version="v1beta3"} 1
apiserver_requested_deprecated_apis{group="batch",removed_release="",resource="cronjobs",subresource="",version="v1beta1"} 0
`
	findings, err := ScrapeDeprecatedAPIMetrics(context.Background(), &fakeMetricsFetcher{raw: []byte(raw)})
	if err != nil {
		t.Fatalf("ScrapeDeprecatedAPIMetrics: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding (zero-count line skipped), got %d: %+v", len(findings), findings)
	}
	if findings[0].ResourceKind != "flowschemas" || findings[0].RemovedInVersion != "1.31" {
		t.Fatalf("unexpected finding: %+v", findings[0])
	}
	if findings[0].APIVersion != "flowcontrol.apiserver.k8s.io/v1beta3" {
		t.Fatalf("unexpected APIVersion: %s", findings[0].APIVersion)
	}
}
