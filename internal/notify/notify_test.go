package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSlackNotifier_PostsWebhook(t *testing.T) {
	var received slackPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("TEST_SLACK_WEBHOOK", srv.URL)
	n := &SlackNotifier{WebhookEnvVar: "TEST_SLACK_WEBHOOK"}

	err := n.Notify(context.Background(), Notification{
		RunID: "run-1", Cluster: "c", Region: "r", Event: "RUN_STARTED",
		Message: "started", Severity: SeverityInfo, Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if received.Text == "" {
		t.Fatal("expected non-empty slack message text")
	}
}

func TestSlackNotifier_MissingEnvVarFails(t *testing.T) {
	n := &SlackNotifier{WebhookEnvVar: "UNSET_SLACK_WEBHOOK_ENV"}
	if err := n.Notify(context.Background(), Notification{}); err == nil {
		t.Fatal("expected error when webhook env var is unset")
	}
}

func TestWebhookNotifier_PostsFullNotification(t *testing.T) {
	var received Notification
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	t.Setenv("TEST_GENERIC_WEBHOOK", srv.URL)
	n := &WebhookNotifier{WebhookEnvVar: "TEST_GENERIC_WEBHOOK"}

	err := n.Notify(context.Background(), Notification{RunID: "run-2", Event: "PAUSED"})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if received.RunID != "run-2" || received.Event != "PAUSED" {
		t.Fatalf("unexpected payload received: %+v", received)
	}
}

func TestPagerDutyNotifier_SkipsInfoSeverity(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := &PagerDutyNotifier{RoutingKeyEnvVar: "UNSET_ROUTING_KEY"}
	if err := n.Notify(context.Background(), Notification{Severity: SeverityInfo}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if called {
		t.Fatal("expected PagerDuty endpoint not to be called for Info severity")
	}
}

func TestMultiNotifier_AggregatesErrorsButDeliversToAll(t *testing.T) {
	var secondCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("TEST_MULTI_WEBHOOK", srv.URL)

	m := &MultiNotifier{Notifiers: []Notifier{
		&SlackNotifier{WebhookEnvVar: "UNSET_ENV_VAR"},
		&WebhookNotifier{WebhookEnvVar: "TEST_MULTI_WEBHOOK"},
	}}

	err := m.Notify(context.Background(), Notification{Event: "RUN_STARTED", Severity: SeverityInfo})
	if err == nil {
		t.Fatal("expected aggregated error from the failing notifier")
	}
	if !secondCalled {
		t.Fatal("expected the second notifier to still be called despite the first failing")
	}
}
