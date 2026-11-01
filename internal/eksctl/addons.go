package eksctl

// AddonUpgradeOptions configures a single EKS-managed add-on upgrade command.
type AddonUpgradeOptions struct {
	Cluster string
	Region  string
	Name    string
	Version string // optional; empty lets eksctl resolve the latest compatible version
}

// BuildUpdateAddonArgs builds the args for `eksctl update addon`, used to upgrade
// VPC CNI, CoreDNS, and kube-proxy one at a time after the control-plane upgrade.
func BuildUpdateAddonArgs(opts AddonUpgradeOptions) []string {
	args := []string{
		"update", "addon",
		"--cluster", opts.Cluster,
		"--region", opts.Region,
		"--name", opts.Name,
	}
	if opts.Version != "" {
		args = append(args, "--version", opts.Version)
	}
	args = append(args, "--wait")
	return args
}
