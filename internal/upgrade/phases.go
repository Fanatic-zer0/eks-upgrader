package upgrade

import (
	"context"
	"fmt"
	"time"

	"eks-upgrader/internal/config"
	"eks-upgrader/internal/state"
)

// phaseHandler executes a single phase's logic; a non-nil error fails the phase.
type phaseHandler func(ctx context.Context) error

func (e *Engine) handlers() map[state.Phase]phaseHandler {
	return map[state.Phase]phaseHandler{
		state.PhaseInit:                 e.phaseInit,
		state.PhaseValidate:             e.phaseValidate,
		state.PhaseStateReconciliation:  e.phaseStateReconciliation,
		state.PhaseReadinessAnalysis:    e.phaseReadinessAnalysis,
		state.PhasePreflight:            e.phasePreflight,
		state.PhaseApproval:             e.phaseApproval,
		state.PhaseControlPlaneUpgrade:  e.phaseControlPlaneUpgrade,
		state.PhaseAddonUpgrade:         e.phaseAddonUpgrade,
		state.PhaseKarpenterUpgrade:     e.phaseKarpenterUpgrade,
		state.PhaseNodeGroupUpgrade:     e.phaseNodeGroupUpgrade,
		state.PhasePostUpgradeCheck:     e.phasePostUpgradeCheck,
		state.PhaseConfigReconciliation: e.phaseConfigReconciliation,
	}
}

func (e *Engine) phaseInit(ctx context.Context) error {
	return nil
}

func (e *Engine) phaseValidate(ctx context.Context) error {
	result := config.Validate(e.Cluster, e.Plan)
	if !result.OK() {
		return fmt.Errorf("validation failed: %v", result.Blocking)
	}

	if ok, installedVersion, err := e.Deps.EksctlVersionSupports(ctx, e.Plan.Spec.TargetKubernetesVersion); err != nil {
		e.logger().Warn("could not verify eksctl version compatibility", "error", err)
	} else if !ok {
		return fmt.Errorf("installed eksctl version %q does not support target Kubernetes version %q", installedVersion, e.Plan.Spec.TargetKubernetesVersion)
	}

	ttl := e.Plan.Spec.Execution.Lock.TTL.Duration
	if ttl <= 0 {
		ttl = time.Hour
	}
	if _, err := e.Lock.Acquire(ctx, e.Plan.Metadata.Cluster, e.runID(), e.Owner, ttl); err != nil {
		return fmt.Errorf("acquiring upgrade lock: %w", err)
	}
	return nil
}

func (e *Engine) phaseStateReconciliation(ctx context.Context) error {
	if !e.Plan.Spec.StateReconciliation.Enabled {
		return nil
	}
	findings, err := e.Deps.Reconcile(ctx)
	if err != nil {
		return fmt.Errorf("live-state reconciliation failed: %w", err)
	}
	if e.Plan.Spec.StateReconciliation.FailOnDrift {
		for _, f := range findings {
			if string(f.Classification) == "BLOCKING" {
				return fmt.Errorf("blocking live-state drift detected: %s: %s", f.Field, f.Message)
			}
		}
	}
	return nil
}

func (e *Engine) phaseReadinessAnalysis(ctx context.Context) error {
	report, err := e.Deps.Readiness(ctx)
	if err != nil {
		return fmt.Errorf("readiness analysis failed: %w", err)
	}
	if report.HasBlocking() {
		return fmt.Errorf("upgrade readiness is NOT_READY: %d blocking finding(s)", len(report.Findings))
	}
	return nil
}

func (e *Engine) phasePreflight(ctx context.Context) error {
	findings, err := e.Deps.Preflight(ctx)
	if err != nil {
		return fmt.Errorf("preflight checks failed: %w", err)
	}
	if hasBlockingReadiness(findings) {
		return fmt.Errorf("preflight has blocking findings: %d", len(findings))
	}
	return nil
}

func (e *Engine) phaseApproval(ctx context.Context) error {
	if !e.Plan.Spec.ControlPlane.Approval.Required {
		return nil
	}
	approvals, err := e.Store.GetApprovals(ctx, e.runID())
	if err != nil {
		return fmt.Errorf("checking approvals: %w", err)
	}
	for _, a := range approvals {
		if a.Phase == state.PhaseApproval {
			return nil
		}
	}
	return ErrAwaitingApproval
}

func (e *Engine) phaseControlPlaneUpgrade(ctx context.Context) error {
	if already, err := e.Deps.AlreadySucceeded(ctx, state.PhaseControlPlaneUpgrade); err == nil && already {
		return nil
	}
	if err := e.Deps.UpgradeControlPlane(ctx); err != nil {
		return fmt.Errorf("control-plane upgrade failed: %w", err)
	}
	return nil
}

func (e *Engine) phaseAddonUpgrade(ctx context.Context) error {
	if !e.Plan.Spec.Addons.Upgrade {
		return nil
	}
	if already, err := e.Deps.AlreadySucceeded(ctx, state.PhaseAddonUpgrade); err == nil && already {
		return nil
	}
	if err := e.Deps.UpgradeAddons(ctx); err != nil {
		return fmt.Errorf("add-on upgrade failed: %w", err)
	}
	return nil
}

func (e *Engine) phaseKarpenterUpgrade(ctx context.Context) error {
	if !e.Plan.Spec.Karpenter.Enabled {
		return nil
	}
	if already, err := e.Deps.AlreadySucceeded(ctx, state.PhaseKarpenterUpgrade); err == nil && already {
		return nil
	}
	if err := e.Deps.UpgradeKarpenter(ctx); err != nil {
		return fmt.Errorf("karpenter upgrade failed: %w", err)
	}
	return nil
}

func (e *Engine) phaseNodeGroupUpgrade(ctx context.Context) error {
	rc := e.runID()
	existing, err := e.Store.GetNodeGroupStatuses(ctx, rc)
	if err != nil {
		return fmt.Errorf("loading node group statuses: %w", err)
	}
	succeeded := make(map[string]bool, len(existing))
	for _, s := range existing {
		if s.Status == "SUCCEEDED" {
			succeeded[s.Name] = true
		}
	}

	for _, name := range e.Plan.Spec.NodeGroups.Order {
		if succeeded[name] {
			continue
		}
		if err := e.upgradeOneNodeGroup(ctx, name); err != nil {
			return err
		}
		if e.Plan.Spec.NodeGroups.WaitBetweenGroups.Duration > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(e.Plan.Spec.NodeGroups.WaitBetweenGroups.Duration):
			}
		}
	}
	return nil
}

func (e *Engine) upgradeOneNodeGroup(ctx context.Context, name string) error {
	rc := e.runID()
	start := time.Now().UTC()
	_ = e.Store.UpsertNodeGroupStatus(ctx, state.NodeGroupStatus{RunID: rc, Name: name, Status: "IN_PROGRESS", StartedAt: &start})

	fail := func(detail string) error {
		end := time.Now().UTC()
		_ = e.Store.UpsertNodeGroupStatus(ctx, state.NodeGroupStatus{RunID: rc, Name: name, Status: "FAILED", EndedAt: &end, Detail: detail})
		return fmt.Errorf("node group %q upgrade failed: %s", name, detail)
	}

	if already, err := e.Deps.AlreadySucceeded(ctx, state.PhaseNodeGroupUpgrade); err != nil || !already {
		if err := e.Deps.UpgradeNodeGroup(ctx, name); err != nil {
			return fail(err.Error())
		}
	}

	gate, err := e.Deps.NodeGroupHealthGate(ctx, name)
	if err != nil {
		return fail(fmt.Sprintf("health gate check error: %v", err))
	}
	if !gate.Passed {
		return fail(fmt.Sprintf("health gates failed: %v", gate.Issues))
	}

	if e.Plan.Spec.Canary.Enabled {
		_, passed, err := e.Deps.RunCanary(ctx, e.Plan.Spec.Canary.Namespaces)
		if err != nil {
			return fail(fmt.Sprintf("canary check error: %v", err))
		}
		if !passed {
			return fail("canary checks failed")
		}
	}

	end := time.Now().UTC()
	_ = e.Store.UpsertNodeGroupStatus(ctx, state.NodeGroupStatus{RunID: rc, Name: name, Status: "SUCCEEDED", EndedAt: &end})
	return nil
}

func (e *Engine) phasePostUpgradeCheck(ctx context.Context) error {
	findings, err := e.Deps.PostUpgradeCheck(ctx)
	if err != nil {
		return fmt.Errorf("post-upgrade checks failed: %w", err)
	}
	if hasBlockingReadiness(findings) {
		return fmt.Errorf("post-upgrade checks have blocking findings: %d", len(findings))
	}
	if e.Plan.Spec.Canary.Enabled {
		_, passed, err := e.Deps.RunCanary(ctx, e.Plan.Spec.Canary.Namespaces)
		if err != nil {
			return fmt.Errorf("post-upgrade canary check error: %w", err)
		}
		if !passed {
			return fmt.Errorf("post-upgrade canary checks failed")
		}
	}
	return nil
}

func (e *Engine) phaseConfigReconciliation(ctx context.Context) error {
	if err := e.Deps.GenerateConfigPatches(ctx); err != nil {
		return fmt.Errorf("config patch generation failed: %w", err)
	}
	return nil
}
