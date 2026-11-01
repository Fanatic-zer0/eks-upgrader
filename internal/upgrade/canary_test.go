package upgrade

import (
	"context"
	"testing"

	"eks-upgrader/internal/config"
)

type fakePrometheusClient struct {
	values map[string]float64
	err    error
}

func (f *fakePrometheusClient) QueryInstant(ctx context.Context, query string) (float64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.values[query], nil
}

func TestEvaluateCanaryChecks_AllPass(t *testing.T) {
	client := &fakePrometheusClient{values: map[string]float64{
		"error_rate_query": 0.001,
		"p99_query":        0.8,
	}}
	checks := []config.CanaryCheck{
		{Name: "error-rate", Query: "error_rate_query", Threshold: 0.01, Comparison: "lessThan"},
		{Name: "p99-latency", Query: "p99_query", Threshold: 1.5, Comparison: "lessThan"},
	}

	results, passed, err := EvaluateCanaryChecks(context.Background(), client, checks)
	if err != nil {
		t.Fatalf("EvaluateCanaryChecks: %v", err)
	}
	if !passed || len(results) != 2 {
		t.Fatalf("expected all checks to pass, got passed=%v results=%+v", passed, results)
	}
}

func TestEvaluateCanaryChecks_FailsWhenThresholdExceeded(t *testing.T) {
	client := &fakePrometheusClient{values: map[string]float64{"error_rate_query": 0.05}}
	checks := []config.CanaryCheck{{Name: "error-rate", Query: "error_rate_query", Threshold: 0.01, Comparison: "lessThan"}}

	results, passed, err := EvaluateCanaryChecks(context.Background(), client, checks)
	if err != nil {
		t.Fatalf("EvaluateCanaryChecks: %v", err)
	}
	if passed || results[0].Passed {
		t.Fatalf("expected canary check to fail, got %+v", results)
	}
}

func TestEvaluateCanaryChecks_GreaterThanComparison(t *testing.T) {
	client := &fakePrometheusClient{values: map[string]float64{"uptime_query": 0.999}}
	checks := []config.CanaryCheck{{Name: "uptime", Query: "uptime_query", Threshold: 0.99, Comparison: "greaterThan"}}

	_, passed, err := EvaluateCanaryChecks(context.Background(), client, checks)
	if err != nil {
		t.Fatalf("EvaluateCanaryChecks: %v", err)
	}
	if !passed {
		t.Fatal("expected greaterThan comparison to pass")
	}
}

func TestEvaluateCanaryChecks_PropagatesQueryError(t *testing.T) {
	client := &fakePrometheusClient{err: context.DeadlineExceeded}
	checks := []config.CanaryCheck{{Name: "error-rate", Query: "q", Threshold: 0.01, Comparison: "lessThan"}}

	_, _, err := EvaluateCanaryChecks(context.Background(), client, checks)
	if err == nil {
		t.Fatal("expected error to propagate from a failing Prometheus query")
	}
}
