package reconcile

import (
	"fmt"
	"os"
	"regexp"

	"github.com/pmezard/go-difflib/difflib"

	"eks-upgrader/internal/config"
)

// GenerateClusterPatch proposes a unified diff updating metadata.version in
// cluster.yaml. It never writes to the original file; eks-upgrader never silently
// rewrites cluster.yaml — the returned patch text is written to output/cluster.patch
// for a human to review and apply with `git apply`.
func GenerateClusterPatch(cfg *config.ClusterConfig, targetVersion string) (string, error) {
	original, err := os.ReadFile(cfg.SourcePath)
	if err != nil {
		return "", fmt.Errorf("reading original cluster config for patch generation: %w", err)
	}

	versionPattern := regexp.MustCompile(`(?m)^(\s*version:\s*)"?` + regexp.QuoteMeta(cfg.Metadata.Version) + `"?\s*$`)
	modified := versionPattern.ReplaceAll(original, []byte(`${1}"`+targetVersion+`"`))

	if string(modified) == string(original) {
		return "", fmt.Errorf("could not locate metadata.version: %q in %s to patch", cfg.Metadata.Version, cfg.SourcePath)
	}

	return unifiedDiff(string(original), string(modified), "a/cluster.yaml", "b/cluster.yaml")
}

// GenerateKarpenterNodeClassPatch proposes a unified diff updating an EC2NodeClass
// manifest's AMI alias from oldAlias to newAlias.
func GenerateKarpenterNodeClassPatch(manifestPath, oldAlias, newAlias string) (string, error) {
	original, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", fmt.Errorf("reading karpenter manifest for patch generation: %w", err)
	}

	aliasPattern := regexp.MustCompile(`(?m)^(\s*(?:- )?alias:\s*)"?` + regexp.QuoteMeta(oldAlias) + `"?\s*$`)
	modified := aliasPattern.ReplaceAll(original, []byte(`${1}"`+newAlias+`"`))

	if string(modified) == string(original) {
		return "", fmt.Errorf("could not locate alias %q in %s to patch", oldAlias, manifestPath)
	}

	return unifiedDiff(string(original), string(modified), "a/"+manifestPath, "b/"+manifestPath)
}

func unifiedDiff(original, modified, fromFile, toFile string) (string, error) {
	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(original),
		B:        difflib.SplitLines(modified),
		FromFile: fromFile,
		ToFile:   toFile,
		Context:  3,
	}
	return difflib.GetUnifiedDiffString(diff)
}
