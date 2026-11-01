package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"eks-upgrader/internal/config"
)

// loadKarpenterDesiredFromDir parses NodePool/EC2NodeClass manifests in dir (as used
// by `state diff --karpenter-config`) into the same desired-state shapes used
// elsewhere, without requiring a full upgrade.yaml.
func loadKarpenterDesiredFromDir(dir string) ([]config.KarpenterNodeClassSpec, []config.KarpenterNodePoolSpec, error) {
	var nodeClasses []config.KarpenterNodeClassSpec
	var nodePools []config.KarpenterNodePoolSpec

	if dir == "" {
		return nodeClasses, nodePools, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("reading karpenter config directory %q: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !(strings.HasSuffix(entry.Name(), ".yaml") || strings.HasSuffix(entry.Name(), ".yml")) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("reading manifest %q: %w", path, err)
		}

		var doc map[string]interface{}
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			continue // skip non-manifest files silently
		}

		kind, _ := doc["kind"].(string)
		metadata, _ := doc["metadata"].(map[string]interface{})
		name, _ := metadata["name"].(string)
		spec, _ := doc["spec"].(map[string]interface{})
		if name == "" || spec == nil {
			continue
		}

		switch kind {
		case "EC2NodeClass":
			nc := config.KarpenterNodeClassSpec{Name: name, ManifestPath: path}
			if terms, ok := spec["amiSelectorTerms"].([]interface{}); ok {
				for _, t := range terms {
					if term, ok := t.(map[string]interface{}); ok {
						if alias, ok := term["alias"].(string); ok && alias != "" {
							nc.AMIAlias = alias
						}
					}
				}
			}
			nodeClasses = append(nodeClasses, nc)
		case "NodePool":
			np := config.KarpenterNodePoolSpec{Name: name, ManifestPath: path}
			if disruption, ok := spec["disruption"].(map[string]interface{}); ok {
				if budgets, ok := disruption["budgets"].([]interface{}); ok {
					for _, b := range budgets {
						if budget, ok := b.(map[string]interface{}); ok {
							var db config.DisruptionBudget
							if nodes, ok := budget["nodes"].(string); ok {
								db.Nodes = nodes
							}
							np.DisruptionBudgets = append(np.DisruptionBudgets, db)
						}
					}
				}
			}
			nodePools = append(nodePools, np)
		}
	}

	return nodeClasses, nodePools, nil
}
