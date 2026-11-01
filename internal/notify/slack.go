package notify

import (
	"context"
	"fmt"
)

// SlackNotifier posts to an incoming webhook URL whose value is read from the
// environment variable named WebhookEnvVar.
type SlackNotifier struct {
	WebhookEnvVar string
}

type slackPayload struct {
	Text string `json:"text"`
}

func (s *SlackNotifier) Notify(ctx context.Context, n Notification) error {
	url, err := resolveEnv(s.WebhookEnvVar)
	if err != nil {
		return fmt.Errorf("slack notifier: %w", err)
	}

	text := fmt.Sprintf("[%s] *%s* cluster=%s region=%s run=%s\n%s",
		n.Severity, n.Event, n.Cluster, n.Region, n.RunID, n.Message)

	if err := httpPostJSON(ctx, url, slackPayload{Text: text}); err != nil {
		return fmt.Errorf("slack notifier: %w", err)
	}
	return nil
}
