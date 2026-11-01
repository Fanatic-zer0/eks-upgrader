// Package notify fans out upgrade lifecycle events to Slack, generic webhooks, and
// PagerDuty. No channel URLs or tokens are ever read from YAML; each channel resolves
// its secret from an environment variable named in the plan's fallback.notify spec.
package notify

import (
	"context"
	"fmt"
	"time"
)

// Severity classifies a notification for routing/filtering by the receiving system.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Notification is a single upgrade lifecycle event to be delivered to operators.
type Notification struct {
	RunID     string
	Cluster   string
	Region    string
	Event     string // e.g. RUN_STARTED, PHASE_COMPLETED, PAUSED, APPROVAL_REQUESTED, RUN_COMPLETED
	Message   string
	Severity  Severity
	Timestamp time.Time
}

// Notifier delivers a Notification to a single channel.
type Notifier interface {
	Notify(ctx context.Context, n Notification) error
}

// MultiNotifier fans a Notification out to every configured channel, collecting (not
// short-circuiting on) individual delivery failures.
type MultiNotifier struct {
	Notifiers []Notifier
}

func (m *MultiNotifier) Notify(ctx context.Context, n Notification) error {
	var errs []error
	for _, notifier := range m.Notifiers {
		if err := notifier.Notify(ctx, n); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("notification delivery failed for %d/%d channels: %v", len(errs), len(m.Notifiers), errs)
}
