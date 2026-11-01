package notify

import (
	"context"
	"fmt"
)

const pagerDutyEventsURL = "https://events.pagerduty.com/v2/enqueue"

// PagerDutyNotifier triggers a PagerDuty Events API v2 alert using a routing key read
// from the environment variable named RoutingKeyEnvVar. Only Critical/Warning
// severities trigger a page; Info notifications are skipped to avoid alert fatigue.
type PagerDutyNotifier struct {
	RoutingKeyEnvVar string
}

type pagerDutyPayload struct {
	RoutingKey  string             `json:"routing_key"`
	EventAction string             `json:"event_action"`
	Payload     pagerDutyEventBody `json:"payload"`
}

type pagerDutyEventBody struct {
	Summary   string `json:"summary"`
	Source    string `json:"source"`
	Severity  string `json:"severity"`
	Component string `json:"component,omitempty"`
	Group     string `json:"group,omitempty"`
	Class     string `json:"class,omitempty"`
}

func (p *PagerDutyNotifier) Notify(ctx context.Context, n Notification) error {
	if n.Severity == SeverityInfo {
		return nil
	}

	routingKey, err := resolveEnv(p.RoutingKeyEnvVar)
	if err != nil {
		return fmt.Errorf("pagerduty notifier: %w", err)
	}

	severity := "warning"
	if n.Severity == SeverityCritical {
		severity = "critical"
	}

	payload := pagerDutyPayload{
		RoutingKey:  routingKey,
		EventAction: "trigger",
		Payload: pagerDutyEventBody{
			Summary:   fmt.Sprintf("%s: %s (cluster=%s)", n.Event, n.Message, n.Cluster),
			Source:    "eks-upgrader",
			Severity:  severity,
			Component: n.Cluster,
			Group:     n.Region,
			Class:     n.Event,
		},
	}

	if err := httpPostJSON(ctx, pagerDutyEventsURL, payload); err != nil {
		return fmt.Errorf("pagerduty notifier: %w", err)
	}
	return nil
}
