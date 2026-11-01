package karpenter

import (
	"testing"
	"time"

	"eks-upgrader/internal/config"
)

func mustDuration(t *testing.T, s string) config.Duration {
	t.Helper()
	d, err := time.ParseDuration(s)
	if err != nil {
		t.Fatalf("parsing duration %q: %v", s, err)
	}
	return config.Duration{Duration: d}
}

func TestActiveBudgetsForReason_FiltersByReasonAndSchedule(t *testing.T) {
	// Monday 17:00 UTC, within the "0 17 * * mon-fri" window.
	now := time.Date(2026, 11, 2, 17, 30, 0, 0, time.UTC)

	budgets := []config.DisruptionBudget{
		{Nodes: "10%"}, // always active, no reason filter
		{Nodes: "0", Schedule: "0 17 * * mon-fri", Duration: mustDuration(t, "16h"), Reasons: []string{"Drifted"}},
		{Nodes: "0", Reasons: []string{"Empty"}}, // different reason, should not apply
	}

	active, err := ActiveBudgetsForReason(budgets, "Drifted", now)
	if err != nil {
		t.Fatalf("ActiveBudgetsForReason: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("expected 2 active budgets, got %d: %+v", len(active), active)
	}
}

func TestActiveBudgetsForReason_OutsideScheduleWindow(t *testing.T) {
	// Saturday, outside the "mon-fri" window.
	now := time.Date(2026, 11, 7, 17, 30, 0, 0, time.UTC)

	budgets := []config.DisruptionBudget{
		{Nodes: "0", Schedule: "0 17 * * mon-fri", Duration: mustDuration(t, "16h"), Reasons: []string{"Drifted"}},
	}

	active, err := ActiveBudgetsForReason(budgets, "Drifted", now)
	if err != nil {
		t.Fatalf("ActiveBudgetsForReason: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected 0 active budgets outside the schedule window, got %d", len(active))
	}
}

func TestMostRestrictiveBudget(t *testing.T) {
	tests := []struct {
		name    string
		budgets []config.DisruptionBudget
		total   int
		want    int
	}{
		{"no budgets means unconstrained", nil, 10, 10},
		{"percentage budget", []config.DisruptionBudget{{Nodes: "10%"}}, 10, 1},
		{"absolute budget", []config.DisruptionBudget{{Nodes: "2"}}, 10, 2},
		{"zero budget blocks everything", []config.DisruptionBudget{{Nodes: "0"}}, 10, 0},
		{"most restrictive of multiple wins", []config.DisruptionBudget{{Nodes: "50%"}, {Nodes: "1"}}, 10, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			allowed, _, err := MostRestrictiveBudget(tc.budgets, tc.total)
			if err != nil {
				t.Fatalf("MostRestrictiveBudget: %v", err)
			}
			if allowed != tc.want {
				t.Fatalf("got %d, want %d", allowed, tc.want)
			}
		})
	}
}
