package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"eks-upgrader/internal/config"
)

// PrometheusClient executes an instant query against a Prometheus-compatible API.
// Abstracted so tests can substitute a fake without a real Prometheus server.
type PrometheusClient interface {
	QueryInstant(ctx context.Context, query string) (float64, error)
}

// HTTPPrometheusClient queries a real Prometheus HTTP API (`/api/v1/query`).
type HTTPPrometheusClient struct {
	BaseURL     string
	BearerToken string
	HTTPClient  *http.Client
}

type prometheusResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Value [2]interface{} `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

func (c *HTTPPrometheusClient) QueryInstant(ctx context.Context, query string) (float64, error) {
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/v1/query", nil)
	if err != nil {
		return 0, fmt.Errorf("building prometheus query request: %w", err)
	}
	q := req.URL.Query()
	q.Set("query", query)
	req.URL.RawQuery = q.Encode()
	if c.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.BearerToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("querying prometheus: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("prometheus query returned status %d", resp.StatusCode)
	}

	var parsed prometheusResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, fmt.Errorf("decoding prometheus response: %w", err)
	}
	if parsed.Status != "success" {
		return 0, fmt.Errorf("prometheus query did not succeed: status=%s", parsed.Status)
	}
	if len(parsed.Data.Result) == 0 {
		return 0, fmt.Errorf("prometheus query returned no data points for %q", query)
	}

	valStr, ok := parsed.Data.Result[0].Value[1].(string)
	if !ok {
		return 0, fmt.Errorf("unexpected prometheus value type for query %q", query)
	}
	val, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing prometheus value %q: %w", valStr, err)
	}
	return val, nil
}

// CanaryCheckResult is the outcome of evaluating a single configured canary check.
type CanaryCheckResult struct {
	Name      string
	Passed    bool
	Value     float64
	Threshold float64
	Message   string
}

// EvaluateCanaryChecks runs every configured check and reports overall pass/fail.
// comparisonMode "baseline" is accepted by the schema for future comparative analysis
// (new-node-group vs. control-group metrics); only "absolute" threshold evaluation is
// implemented currently, and baseline mode falls back to absolute with a caveat.
func EvaluateCanaryChecks(ctx context.Context, client PrometheusClient, checks []config.CanaryCheck) ([]CanaryCheckResult, bool, error) {
	allPassed := true
	results := make([]CanaryCheckResult, 0, len(checks))

	for _, check := range checks {
		value, err := client.QueryInstant(ctx, check.Query)
		if err != nil {
			return nil, false, fmt.Errorf("evaluating canary check %q: %w", check.Name, err)
		}
		passed := compareThreshold(value, check.Threshold, check.Comparison)
		if !passed {
			allPassed = false
		}
		results = append(results, CanaryCheckResult{
			Name: check.Name, Passed: passed, Value: value, Threshold: check.Threshold,
			Message: fmt.Sprintf("%s: value=%.4f threshold=%.4f comparison=%s passed=%v", check.Name, value, check.Threshold, check.Comparison, passed),
		})
	}
	return results, allPassed, nil
}

func compareThreshold(value, threshold float64, comparison string) bool {
	switch comparison {
	case "greaterThan":
		return value > threshold
	case "lessThan":
		return value < threshold
	default:
		return value < threshold
	}
}
