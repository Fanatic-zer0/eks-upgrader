package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadClusterConfig reads and parses an eksctl-compatible cluster.yaml.
func LoadClusterConfig(path string) (*ClusterConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading cluster config %q: %w", path, err)
	}

	var cfg ClusterConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parsing cluster config %q: %w", path, err)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	cfg.SourcePath = abs
	return &cfg, nil
}

// LoadUpgradePlan reads and parses an upgrade.yaml (EKSUpgradePlan).
func LoadUpgradePlan(path string) (*UpgradePlan, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading upgrade plan %q: %w", path, err)
	}

	var plan UpgradePlan
	if err := yaml.Unmarshal(raw, &plan); err != nil {
		return nil, fmt.Errorf("parsing upgrade plan %q: %w", path, err)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	plan.SourcePath = abs
	return &plan, nil
}

// ResolveClusterConfigPath resolves the plan's spec.clusterConfig reference relative
// to the directory the upgrade.yaml was loaded from, unless it is already absolute.
func ResolveClusterConfigPath(plan *UpgradePlan) string {
	if filepath.IsAbs(plan.Spec.ClusterConfig) {
		return plan.Spec.ClusterConfig
	}
	return filepath.Join(filepath.Dir(plan.SourcePath), plan.Spec.ClusterConfig)
}
