package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// httpPostJSON posts a JSON body to url with a bounded timeout, returning an error for
// non-2xx responses or transport failures.
func httpPostJSON(ctx context.Context, url string, body interface{}) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling notification payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("building notification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("notification endpoint returned status %d", resp.StatusCode)
	}
	return nil
}

// resolveEnv reads a required environment variable by name, used to resolve the
// *Env-suffixed fields in fallback.notify (no secrets are ever read from YAML).
func resolveEnv(envVar string) (string, error) {
	if envVar == "" {
		return "", fmt.Errorf("no environment variable name configured")
	}
	v := os.Getenv(envVar)
	if v == "" {
		return "", fmt.Errorf("environment variable %q is not set", envVar)
	}
	return v, nil
}
