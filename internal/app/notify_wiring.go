package app

import (
	"eks-upgrader/internal/config"
	"eks-upgrader/internal/notify"
)

// buildNotifier fans out to every channel configured in fallback.notify. Channels
// whose *Env variable is unset simply fail at send time (logged, not fatal), per the
// "no secrets in YAML" principle — presence of the field does not require the
// corresponding secret to be configured in every environment (e.g. local dry-runs).
func buildNotifier(spec config.FallbackNotifySpec) notify.Notifier {
	var notifiers []notify.Notifier
	if spec.SlackWebhookEnv != "" {
		notifiers = append(notifiers, &notify.SlackNotifier{WebhookEnvVar: spec.SlackWebhookEnv})
	}
	if spec.WebhookEnv != "" {
		notifiers = append(notifiers, &notify.WebhookNotifier{WebhookEnvVar: spec.WebhookEnv})
	}
	if spec.PagerdutyRoutingKeyEnv != "" {
		notifiers = append(notifiers, &notify.PagerDutyNotifier{RoutingKeyEnvVar: spec.PagerdutyRoutingKeyEnv})
	}
	return &notify.MultiNotifier{Notifiers: notifiers}
}
