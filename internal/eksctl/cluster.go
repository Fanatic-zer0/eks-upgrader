package eksctl

// BuildUpgradeClusterArgs builds the args for upgrading the EKS control plane.
// `--config-file` keeps eksctl itself as the sole source of cluster mutation intent.
func BuildUpgradeClusterArgs(configFilePath string) []string {
	return []string{
		"upgrade", "cluster",
		"--config-file", configFilePath,
		"--approve",
	}
}
