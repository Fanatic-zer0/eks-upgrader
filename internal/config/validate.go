package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ValidationResult holds the outcome of Phase 1 validation.
type ValidationResult struct {
	Blocking []string
	Warnings []string
}

func (r *ValidationResult) OK() bool {
	return len(r.Blocking) == 0
}

func (r *ValidationResult) addBlocking(format string, args ...interface{}) {
	r.Blocking = append(r.Blocking, fmt.Sprintf(format, args...))
}

func (r *ValidationResult) addWarning(format string, args ...interface{}) {
	r.Warnings = append(r.Warnings, fmt.Sprintf(format, args...))
}

// Validate runs all Phase 1 structural checks against the loaded cluster config and
// upgrade plan. It performs no mutation and no network calls.
func Validate(cfg *ClusterConfig, plan *UpgradePlan) *ValidationResult {
	r := &ValidationResult{}

	validateAPIVersionKind(cfg, plan, r)
	validateVersionStep(cfg, plan, r)
	validateNodeGroupOrder(cfg, plan, r)
	validateReferencedFiles(plan, r)
	validateSecretsNotInline(plan, r)
	validateKarpenterManifests(plan, r)

	return r
}

func validateAPIVersionKind(cfg *ClusterConfig, plan *UpgradePlan, r *ValidationResult) {
	if cfg.APIVersion == "" || !strings.HasPrefix(cfg.APIVersion, "eksctl.io/") {
		r.addBlocking("cluster.yaml: unexpected apiVersion %q, expected an eksctl.io/* ClusterConfig", cfg.APIVersion)
	}
	if cfg.Kind != "ClusterConfig" {
		r.addBlocking("cluster.yaml: unexpected kind %q, expected ClusterConfig", cfg.Kind)
	}
	if plan.APIVersion == "" || !strings.HasPrefix(plan.APIVersion, "platform.io/") {
		r.addBlocking("upgrade.yaml: unexpected apiVersion %q, expected platform.io/*", plan.APIVersion)
	}
	if plan.Kind != "EKSUpgradePlan" {
		r.addBlocking("upgrade.yaml: unexpected kind %q, expected EKSUpgradePlan", plan.Kind)
	}
}

// parseMinorVersion parses a Kubernetes "MAJOR.MINOR" string, e.g. "1.30".
func parseMinorVersion(v string) (major, minor int, err error) {
	parts := strings.SplitN(strings.TrimSpace(v), ".", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("version %q is not in MAJOR.MINOR form", v)
	}
	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("version %q has non-numeric major component: %w", v, err)
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("version %q has non-numeric minor component: %w", v, err)
	}
	return major, minor, nil
}

// IsExactlyOneMinorAhead reports whether target is exactly current + 1 minor version.
func IsExactlyOneMinorAhead(current, target string) (bool, error) {
	curMajor, curMinor, err := parseMinorVersion(current)
	if err != nil {
		return false, err
	}
	tgtMajor, tgtMinor, err := parseMinorVersion(target)
	if err != nil {
		return false, err
	}
	return tgtMajor == curMajor && tgtMinor == curMinor+1, nil
}

func validateVersionStep(cfg *ClusterConfig, plan *UpgradePlan, r *ValidationResult) {
	current := cfg.Metadata.Version
	target := plan.Spec.TargetKubernetesVersion

	if current == "" {
		r.addBlocking("cluster.yaml: metadata.version is required")
		return
	}
	if target == "" {
		r.addBlocking("upgrade.yaml: spec.targetKubernetesVersion is required")
		return
	}

	ok, err := IsExactlyOneMinorAhead(current, target)
	if err != nil {
		r.addBlocking("version comparison failed: %v", err)
		return
	}
	if !ok {
		r.addBlocking("target Kubernetes version %q must be exactly one minor version ahead of current %q (skipping versions is not allowed)", target, current)
	}

	for _, ng := range cfg.ManagedNodeGroups {
		if ng.InstanceType == "" {
			r.addBlocking("managed node group %q is missing instanceType", ng.Name)
		}
		if ng.MinSize > ng.MaxSize {
			r.addBlocking("managed node group %q has minSize (%d) greater than maxSize (%d)", ng.Name, ng.MinSize, ng.MaxSize)
		}
		if ng.DesiredCapacity < ng.MinSize || ng.DesiredCapacity > ng.MaxSize {
			r.addWarning("managed node group %q desiredCapacity (%d) is outside [minSize, maxSize]", ng.Name, ng.DesiredCapacity)
		}
	}
}

func validateNodeGroupOrder(cfg *ClusterConfig, plan *UpgradePlan, r *ValidationResult) {
	order := plan.Spec.NodeGroups.Order

	seen := make(map[string]int, len(order))
	for _, name := range order {
		seen[name]++
	}
	for name, count := range seen {
		if count > 1 {
			r.addBlocking("nodeGroups.order contains duplicate entry %q", name)
		}
	}

	desired := make(map[string]bool, len(cfg.ManagedNodeGroups))
	for _, ng := range cfg.ManagedNodeGroups {
		desired[ng.Name] = true
	}

	for _, name := range order {
		if !desired[name] {
			r.addBlocking("nodeGroups.order references %q which does not exist in cluster.yaml managedNodeGroups", name)
		}
	}
	for name := range desired {
		if _, ok := seen[name]; !ok {
			r.addBlocking("managed node group %q exists in cluster.yaml but is missing from nodeGroups.order", name)
		}
	}
}

func validateReferencedFiles(plan *UpgradePlan, r *ValidationResult) {
	clusterConfigPath := ResolveClusterConfigPath(plan)
	if _, err := os.Stat(clusterConfigPath); err != nil {
		r.addBlocking("spec.clusterConfig reference %q does not exist: %v", clusterConfigPath, err)
	}

	if plan.Spec.PostUpgrade.RunSmokeTests {
		for _, cmd := range plan.Spec.PostUpgrade.SmokeTestCommand {
			if cmd == "" {
				continue
			}
			// Only validate the first token (the executable/script path) if it looks like a path.
			if strings.HasPrefix(cmd, "./") || strings.HasPrefix(cmd, "/") {
				if _, err := os.Stat(cmd); err != nil {
					r.addWarning("postUpgrade.smokeTestCommand references %q which does not exist (may be resolved at run time)", cmd)
				}
			}
			break
		}
	}
}

func validateKarpenterManifests(plan *UpgradePlan, r *ValidationResult) {
	if !plan.Spec.Karpenter.Enabled {
		return
	}
	for _, nc := range plan.Spec.Karpenter.NodeClasses {
		if nc.ManifestPath == "" {
			continue
		}
		if _, err := os.Stat(nc.ManifestPath); err != nil {
			r.addBlocking("karpenter.nodeClasses[%s].manifestPath %q does not exist: %v", nc.Name, nc.ManifestPath, err)
		}
	}
	for _, np := range plan.Spec.Karpenter.NodePools {
		if np.ManifestPath == "" {
			continue
		}
		if _, err := os.Stat(np.ManifestPath); err != nil {
			r.addBlocking("karpenter.nodePools[%s].manifestPath %q does not exist: %v", np.Name, np.ManifestPath, err)
		}
	}
}

// secretLikePatterns flags literal secret material that should never be committed to YAML.
var secretLikePatterns = []*regexp.Regexp{
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),                       // AWS access key id
	regexp.MustCompile(`https://hooks\.slack\.com/services/\S+`), // literal Slack webhook
	regexp.MustCompile(`xox[baprs]-[0-9A-Za-z-]+`),               // Slack token
	regexp.MustCompile(`(?i)-----BEGIN [A-Z ]*PRIVATE KEY-----`), // PEM private key
}

func validateSecretsNotInline(plan *UpgradePlan, r *ValidationResult) {
	raw, err := os.ReadFile(plan.SourcePath)
	if err != nil {
		r.addWarning("could not re-read %q to scan for inline secrets: %v", plan.SourcePath, err)
		return
	}
	for _, pattern := range secretLikePatterns {
		if pattern.Match(raw) {
			r.addBlocking("upgrade.yaml appears to contain inline secret material matching %q; use an *Env field referencing an environment variable instead", pattern.String())
		}
	}
}
