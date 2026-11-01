package eksctl

// NodeGroupUpgradeOptions configures a single managed node group upgrade command.
type NodeGroupUpgradeOptions struct {
	Cluster               string
	Region                string
	NodeGroup             string
	KubernetesVersion     string
	LaunchTemplateVersion string // optional; empty means "do not pass --launch-template-version"
}

// BuildUpgradeNodeGroupArgs builds the args for `eksctl upgrade nodegroup`, optionally
// including a launch-template version when the node group's plan specifies one.
func BuildUpgradeNodeGroupArgs(opts NodeGroupUpgradeOptions) []string {
	args := []string{
		"upgrade", "nodegroup",
		"--cluster", opts.Cluster,
		"--region", opts.Region,
		"--name", opts.NodeGroup,
		"--kubernetes-version", opts.KubernetesVersion,
	}
	if opts.LaunchTemplateVersion != "" {
		args = append(args, "--launch-template-version", opts.LaunchTemplateVersion)
	}
	args = append(args, "--wait")
	return args
}
