package scanners

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"eks-upgrader/internal/readiness"
)

// kubentFinding is a best-effort subset of kubent's `-o json` output schema; verify
// against the installed kubent version, as the schema has changed across releases.
type kubentFinding struct {
	Kind        string `json:"ObjectName"`
	Namespace   string `json:"Namespace"`
	Name        string `json:"Name"`
	ApiVersion  string `json:"ApiVersion"`
	RuleSet     string `json:"RuleSet"`
	ReplaceWith string `json:"ReplaceWith"`
	Since       string `json:"Since"` // target Kubernetes version the API is removed in
}

// ScanWithKubent runs `kubent -o json` against the live cluster (using the kubeconfig
// already active in the environment) plus any local Helm chart directories. If kubent
// is not installed, it returns (nil, nil) so the caller can treat this signal as
// skipped rather than failing readiness outright.
func ScanWithKubent(ctx context.Context, runner ExternalToolRunner, helmChartDirs []string) ([]readiness.DeprecatedAPIFinding, error) {
	args := []string{"-o", "json"}
	for _, dir := range helmChartDirs {
		args = append(args, "--helm3", "--helm-paths", dir)
	}

	out, err := runner.Run(ctx, "kubent", args)
	if err != nil {
		if errors.Is(err, ErrToolNotInstalled) {
			return nil, nil
		}
		return nil, fmt.Errorf("running kubent: %w", err)
	}
	if len(out) == 0 {
		return nil, nil
	}

	var parsed []kubentFinding
	if err := json.Unmarshal(out, &parsed); err != nil {
		// Best-effort signal only; degrade gracefully rather than failing readiness.
		return nil, nil
	}

	findings := make([]readiness.DeprecatedAPIFinding, 0, len(parsed))
	for _, item := range parsed {
		findings = append(findings, readiness.DeprecatedAPIFinding{
			Source: "kubent", ResourceKind: item.Kind, ResourceName: item.Name,
			APIVersion: item.ApiVersion, Replacement: item.ReplaceWith, RemovedInVersion: item.Since,
		})
	}
	return findings, nil
}
