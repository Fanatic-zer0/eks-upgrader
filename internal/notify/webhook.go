package notify

import (
	"context"
	"fmt"
)

// WebhookNotifier posts the full Notification as JSON to a generic URL read from the
// environment variable named WebhookEnvVar.
type WebhookNotifier struct {
	WebhookEnvVar string
}

func (w *WebhookNotifier) Notify(ctx context.Context, n Notification) error {
	url, err := resolveEnv(w.WebhookEnvVar)
	if err != nil {
		return fmt.Errorf("webhook notifier: %w", err)
	}
	if err := httpPostJSON(ctx, url, n); err != nil {
		return fmt.Errorf("webhook notifier: %w", err)
	}
	return nil
}
