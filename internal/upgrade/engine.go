package upgrade

import (
	"context"
	"fmt"
	"log/slog"

	"eks-upgrader/internal/audit"
	"eks-upgrader/internal/config"
	"eks-upgrader/internal/lock"
	"eks-upgrader/internal/notify"
	"eks-upgrader/internal/readiness"
	"eks-upgrader/internal/reconcile"
	"eks-upgrader/internal/state"
)

// Dependencies is the seam between the orchestration engine (sequencing, state,
// gating, audit, notifications) and every heavier external system (eksctl, AWS,
// Kubernetes, Karpenter, Helm, Prometheus). A single implementation composing the
// lower-level packages lives in internal/app; tests use a fake.
type Dependencies interface {
	// AlreadySucceeded lets resume re-verify live state before retrying a mutating
	// command, in case the previous attempt actually succeeded out-of-band.
	AlreadySucceeded(ctx context.Context, phase state.Phase) (bool, error)

	Reconcile(ctx context.Context) ([]reconcile.Finding, error)
	Readiness(ctx context.Context) (*readiness.Report, error)
	Preflight(ctx context.Context) ([]readiness.Finding, error)

	UpgradeControlPlane(ctx context.Context) error
	UpgradeAddons(ctx context.Context) error
	UpgradeKarpenter(ctx context.Context) error

	UpgradeNodeGroup(ctx context.Context, nodeGroupName string) error
	NodeGroupHealthGate(ctx context.Context, nodeGroupName string) (HealthGateResult, error)
	RunCanary(ctx context.Context, namespaces []string) ([]CanaryCheckResult, bool, error)

	PostUpgradeCheck(ctx context.Context) ([]readiness.Finding, error)
	GenerateConfigPatches(ctx context.Context) error

	// EksctlVersionSupports reports whether the installed eksctl supports targetVersion,
	// used by Phase 1 validation.
	EksctlVersionSupports(ctx context.Context, targetVersion string) (bool, string, error)
}

// ErrAwaitingApproval is returned by the Approval phase handler when no approval
// record exists yet; the engine treats this as a normal pause, not a hard failure.
var ErrAwaitingApproval = fmt.Errorf("awaiting approval")

// ErrLockHeldByOther is returned by Run/Resume when the cluster lock is held by a
// different run.
var ErrLockHeldByOther = fmt.Errorf("cluster lock is held by a different run")

// ErrResumeAttemptsExceeded is returned by Resume when the per-phase resume circuit
// breaker has tripped and force was not requested.
var ErrResumeAttemptsExceeded = fmt.Errorf("maximum resume attempts exceeded for this phase; pass force=true to override")

// DefaultMaxResumeAttempts is the default resume circuit-breaker threshold.
const DefaultMaxResumeAttempts = 3

// Engine orchestrates a single upgrade run against the fixed phase sequence.
type Engine struct {
	Cluster *config.ClusterConfig
	Plan    *config.UpgradePlan
	Store   state.Store
	Lock    lock.Locker
	Audit   audit.Logger
	Notify  notify.Notifier
	Deps    Dependencies
	Logger  *slog.Logger

	// Owner identifies the process/operator acquiring the lock (e.g. hostname or CI job id).
	Owner string
	// MaxResumeAttempts overrides DefaultMaxResumeAttempts when non-zero.
	MaxResumeAttempts int
}

func (e *Engine) maxResumeAttempts() int {
	if e.MaxResumeAttempts > 0 {
		return e.MaxResumeAttempts
	}
	return DefaultMaxResumeAttempts
}

func (e *Engine) runID() string {
	return RunID(e.Plan)
}

// RunID derives the canonical run identifier for a plan: the explicit
// metadata.runID if set, otherwise a deterministic "<cluster>-<targetVersion>"
// fallback. Exported so internal/app can correlate state-store lookups and output
// file names with the exact run the engine itself is operating on.
func RunID(plan *config.UpgradePlan) string {
	if plan.Metadata.RunID != "" {
		return plan.Metadata.RunID
	}
	return plan.Metadata.Cluster + "-" + plan.Spec.TargetKubernetesVersion
}

func (e *Engine) logger() *slog.Logger {
	if e.Logger != nil {
		return e.Logger
	}
	return slog.Default()
}

func (e *Engine) notifyEvent(ctx context.Context, event string, severity notify.Severity, message string) {
	if e.Notify == nil {
		return
	}
	_ = e.Notify.Notify(ctx, notify.Notification{
		RunID: e.runID(), Cluster: e.Plan.Metadata.Cluster, Region: e.Plan.Metadata.Region,
		Event: event, Message: message, Severity: severity,
	})
}

func hasBlockingReadiness(findings []readiness.Finding) bool {
	for _, f := range findings {
		if f.Severity == readiness.SeverityBlocking {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
