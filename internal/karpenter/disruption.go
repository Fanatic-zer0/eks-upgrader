package karpenter

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"eks-upgrader/internal/config"
)

// ActiveBudgetsForReason returns the subset of configured disruption budgets that
// currently apply to reason (e.g. "Drifted"), honoring each budget's optional
// schedule/duration activation window.
func ActiveBudgetsForReason(budgets []config.DisruptionBudget, reason string, now time.Time) ([]config.DisruptionBudget, error) {
	var active []config.DisruptionBudget
	for _, b := range budgets {
		if len(b.Reasons) > 0 && !containsReasonFold(b.Reasons, reason) {
			continue
		}
		if b.Schedule == "" {
			active = append(active, b)
			continue
		}
		within, err := scheduleWindowActive(b.Schedule, b.Duration.Duration, now)
		if err != nil {
			return nil, err
		}
		if within {
			active = append(active, b)
		}
	}
	return active, nil
}

func containsReasonFold(reasons []string, reason string) bool {
	for _, r := range reasons {
		if strings.EqualFold(r, reason) {
			return true
		}
	}
	return false
}

// scheduleWindowActive reports whether now falls within [activation, activation+duration]
// for the most recent cron activation at or before now.
func scheduleWindowActive(schedule string, duration time.Duration, now time.Time) (bool, error) {
	sched, err := cron.ParseStandard(schedule)
	if err != nil {
		return false, fmt.Errorf("parsing disruption budget schedule %q: %w", schedule, err)
	}
	probe := now.Add(-duration)
	next := sched.Next(probe)
	return !next.After(now) && next.After(probe), nil
}

// MostRestrictiveBudget returns the tightest node-count allowance among the active
// budgets (Karpenter itself applies the most restrictive matching budget). No active
// budgets means voluntary disruption is unconstrained.
func MostRestrictiveBudget(active []config.DisruptionBudget, totalNodes int) (allowed int, raw string, err error) {
	if len(active) == 0 {
		return totalNodes, "", nil
	}
	allowed = totalNodes
	for _, b := range active {
		n, err := resolveNodesBudget(b.Nodes, totalNodes)
		if err != nil {
			return 0, "", err
		}
		if raw == "" || n < allowed {
			allowed = n
			raw = b.Nodes
		}
	}
	return allowed, raw, nil
}

func resolveNodesBudget(nodes string, total int) (int, error) {
	nodes = strings.TrimSpace(nodes)
	if strings.HasSuffix(nodes, "%") {
		pctStr := strings.TrimSuffix(nodes, "%")
		pct, err := strconv.Atoi(pctStr)
		if err != nil {
			return 0, fmt.Errorf("invalid percentage disruption budget %q: %w", nodes, err)
		}
		return (total * pct) / 100, nil
	}
	n, err := strconv.Atoi(nodes)
	if err != nil {
		return 0, fmt.Errorf("invalid disruption budget %q: %w", nodes, err)
	}
	return n, nil
}
