package cli

import (
	"context"
	"fmt"
	"log/slog"

	"eks-upgrader/internal/audit"
	"eks-upgrader/internal/config"
	"eks-upgrader/internal/lock"
	"eks-upgrader/internal/notify"
	"eks-upgrader/internal/state"
	"eks-upgrader/internal/upgrade"
)

// minimalEngine opens just the state store at statePath and builds a lightweight
// Engine around whatever run it finds, without requiring the original
// upgrade.yaml/cluster.yaml. This backs status/abort/pause/approve/report, which per
// the buildplan's CLI surface take only --state.
func minimalEngine(ctx context.Context, statePath string) (*upgrade.Engine, *state.Run, func(), error) {
	store, err := state.OpenSQLiteStore(statePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("opening state store %q: %w", statePath, err)
	}
	closeFn := func() { _ = store.Close() }

	run, err := store.LatestRun(ctx)
	if err != nil {
		closeFn()
		return nil, nil, nil, fmt.Errorf("loading latest run: %w", err)
	}
	if run == nil {
		closeFn()
		return nil, nil, nil, fmt.Errorf("no run found in %q", statePath)
	}

	plan := &config.UpgradePlan{Metadata: config.PlanMetadata{
		Cluster: run.Cluster, Region: run.Region, RunID: run.RunID,
	}}

	engine := &upgrade.Engine{
		Plan:   plan,
		Store:  store,
		Lock:   lock.NewLocalLocker("./state/locks"),
		Audit:  audit.NopLogger{},
		Notify: &notify.MultiNotifier{},
		Logger: slog.Default(),
	}
	return engine, run, closeFn, nil
}
