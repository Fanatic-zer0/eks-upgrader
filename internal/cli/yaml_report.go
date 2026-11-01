package cli

import (
	"gopkg.in/yaml.v3"

	"eks-upgrader/internal/upgrade"
)

func reportYAML(r *upgrade.RunReport) ([]byte, error) {
	return yaml.Marshal(r)
}
