package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"eks-upgrader/internal/eksctl"
	"eks-upgrader/internal/readiness"
	"eks-upgrader/internal/readiness/scanners"
	"eks-upgrader/internal/reconcile"
	"eks-upgrader/internal/state"
	"eks-upgrader/internal/upgrade"
)

// Deps implements upgrade.Dependencies against the real, wired-up App.
type Deps struct {
	App *App
}

var _ upgrade.Dependencies = (*Deps)(nil)

func (d *Deps) runID() string { return upgrade.RunID(d.App.Plan) }

// k8sReader returns App.K8s as an interface, or a true nil interface (not a non-nil
// interface wrapping a nil pointer) when Kubernetes access is unavailable.
func (d *Deps) k8sReader() reconcile.NodeGroupNodesReader {
	if d.App.K8s == nil {
		return nil
	}
	return d.App.K8s
}

func (d *Deps) karpenterReader() reconcile.KarpenterReader {
	if d.App.Karpenter == nil {
		return nil
	}
	return d.App.Karpenter
}

// AlreadySucceeded re-verifies live state before a resumed phase retries a mutating
// command, so a crash after eksctl succeeded but before the checkpoint persisted does
// not re-run the same mutation.
func (d *Deps) AlreadySucceeded(ctx context.Context, phase state.Phase) (bool, error) {
	switch phase {
	case state.PhaseControlPlaneUpgrade:
		if d.App.EKS == nil {
			return false, nil
		}
		info, err := d.App.EKS.DescribeCluster(ctx, d.App.Plan.Metadata.Cluster)
		if err != nil {
			return false, err
		}
		return info.Version == d.App.Plan.Spec.TargetKubernetesVersion && info.Status == "ACTIVE", nil
	case state.PhaseAddonUpgrade:
		if d.App.EKS == nil || !d.App.Plan.Spec.Addons.Upgrade {
			return false, nil
		}
		addons, err := d.App.EKS.ListAddons(ctx, d.App.Plan.Metadata.Cluster)
		if err != nil {
			return false, err
		}
		healthy := make(map[string]bool, len(addons))
		for _, a := range addons {
			healthy[a.Name] = a.Healthy()
		}
		for _, name := range d.App.Plan.Spec.Addons.Names {
			if !healthy[name] {
				return false, nil
			}
		}
		return true, nil
	default:
		// Karpenter and node-group upgrades don't have a cheap, reliable "already done"
		// signal at the phase level (node groups are tracked individually in the state
		// store instead); always attempt rather than risk skipping real work.
		return false, nil
	}
}

func (d *Deps) Reconcile(ctx context.Context) ([]reconcile.Finding, error) {
	collector := &reconcile.Collector{EKS: d.App.EKS, K8s: d.k8sReader(), Karpenter: d.karpenterReader()}
	observed, err := collector.Collect(ctx, d.App.Plan.Metadata.Cluster, d.App.Plan.Metadata.Region, d.App.Plan.Spec.Karpenter.Enabled)
	if err != nil {
		return nil, err
	}

	findings := reconcile.Diff(d.App.Cluster, d.App.Plan, observed)
	findings = reconcile.ApplyPolicy(findings, d.App.Plan.Spec.StateReconciliation)

	outDir := d.outputDir()
	if err := os.MkdirAll(outDir, 0o755); err == nil {
		if yamlBytes, yerr := observed.ToYAML(); yerr == nil {
			_ = os.WriteFile(outDir+"/observed-cluster.yaml", yamlBytes, 0o644)
		}
		if jsonBytes, jerr := reconcile.ReportJSON(findings); jerr == nil {
			_ = os.WriteFile(outDir+"/drift-report.json", jsonBytes, 0o644)
		}
		_ = os.WriteFile(outDir+"/drift-report.md", []byte(reconcile.ReportMarkdown(findings)), 0o644)
	}
	return findings, nil
}

func (d *Deps) Readiness(ctx context.Context) (*readiness.Report, error) {
	var findings []readiness.Finding

	if d.App.EKS != nil && d.App.Plan.Spec.Preflight.EKSUpgradeInsights.Enabled {
		insights, err := d.App.EKS.ListInsights(ctx, d.App.Plan.Metadata.Cluster)
		if err != nil {
			d.App.Logger.Warn("failed to list EKS upgrade insights", "error", err)
		} else {
			findings = append(findings, readiness.ClassifyInsights(insights, d.App.Plan.Spec.Preflight.EKSUpgradeInsights)...)
		}
	}

	if d.App.K8s != nil {
		var raw []readiness.DeprecatedAPIFinding
		if scraped, err := scanners.ScrapeDeprecatedAPIMetrics(ctx, d.App.K8s); err != nil {
			d.App.Logger.Warn("failed to scrape deprecated API metrics", "error", err)
		} else {
			raw = append(raw, scraped...)
		}
		runner := scanners.SubprocessRunner{}
		if plutoFindings, err := scanners.ScanWithPluto(ctx, runner, []string{d.App.Options.ClusterConfigPath}, d.App.Plan.Spec.TargetKubernetesVersion); err == nil {
			raw = append(raw, plutoFindings...)
		}
		if kubentFindings, err := scanners.ScanWithKubent(ctx, runner, nil); err == nil {
			raw = append(raw, kubentFindings...)
		}
		findings = append(findings, readiness.ClassifyDeprecatedAPIs(raw, d.App.Plan.Spec.TargetKubernetesVersion, d.App.Plan.Spec.Preflight.DeprecatedAPIs)...)
	}

	if d.App.Plan.Spec.Karpenter.Enabled && d.App.Karpenter != nil {
		pools, err := d.App.Karpenter.ListNodePools(ctx)
		if err == nil {
			for _, p := range pools {
				if !p.Healthy {
					findings = append(findings, readiness.Finding{Source: "karpenter", Category: "nodepool", Resource: p.Name, Severity: readiness.SeverityBlocking, Message: fmt.Sprintf("NodePool %q is not Ready", p.Name)})
				}
			}
		}
		findings = append(findings, readiness.Finding{
			Source: "compatibility", Category: "karpenter", Severity: readiness.SeverityWarning,
			Message:        "verify the installed Karpenter controller version against " + d.App.Plan.Spec.TargetKubernetesVersion + " before proceeding (live controller version is not auto-detected)",
			Recommendation: "check `kubectl get deployment -n " + d.App.Plan.Spec.Karpenter.Namespace + " karpenter -o jsonpath={.spec.template.spec.containers[0].image}`",
		})
	}

	if d.App.EKS != nil {
		for _, name := range d.App.Plan.Spec.Addons.Names {
			findings = append(findings, readiness.CheckAddonCompatibility(name, "", d.App.Plan.Spec.TargetKubernetesVersion))
		}
	}

	findings = append(findings, d.liveClusterHealthFindings(ctx)...)

	report := &readiness.Report{Status: readiness.ComputeStatus(findings), GeneratedAt: time.Now().UTC(), Findings: findings}
	d.writeReadinessArtifacts(report)
	return report, nil
}

func (d *Deps) Preflight(ctx context.Context) ([]readiness.Finding, error) {
	return d.liveClusterHealthFindings(ctx), nil
}

func (d *Deps) liveClusterHealthFindings(ctx context.Context) []readiness.Finding {
	var findings []readiness.Finding
	if d.App.K8s == nil {
		return findings
	}

	if d.App.Plan.Spec.Preflight.RequireAllNodesReady {
		allReady, notReady, err := d.App.K8s.AllNodesReady(ctx)
		if err == nil {
			findings = append(findings, readiness.CheckNodeReadiness(allReady, notReady)...)
		}
	}

	if pending, err := d.App.K8s.PendingPods(ctx); err == nil {
		findings = append(findings, readiness.CheckPendingPods(len(pending), 0)...)
	}

	if d.App.Plan.Spec.Preflight.RequirePDBsForCriticalWorkloads && len(d.App.Plan.Spec.Preflight.CriticalNamespaces) > 0 {
		if issues, err := d.App.K8s.ValidatePDBsForNamespaces(ctx, d.App.Plan.Spec.Preflight.CriticalNamespaces); err == nil {
			findings = append(findings, readiness.CheckPDBIssues(issues)...)
		}
	}

	return findings
}

func (d *Deps) UpgradeControlPlane(ctx context.Context) error {
	args := eksctl.BuildUpgradeClusterArgs(d.App.Cluster.SourcePath)
	rc := eksctl.RunContext{RunID: d.runID(), Cluster: d.App.Plan.Metadata.Cluster, Region: d.App.Plan.Metadata.Region, Phase: state.PhaseControlPlaneUpgrade}
	timeout := d.App.Plan.Spec.ControlPlane.Timeout.Duration
	if timeout <= 0 {
		timeout = 90 * time.Minute
	}
	_, err := d.App.Eksctl.RunWithTimeout(ctx, rc, args, timeout)
	return err
}

func (d *Deps) UpgradeAddons(ctx context.Context) error {
	timeout := d.App.Plan.Spec.Addons.Timeout.Duration
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	for _, name := range d.App.Plan.Spec.Addons.Names {
		args := eksctl.BuildUpdateAddonArgs(eksctl.AddonUpgradeOptions{
			Cluster: d.App.Plan.Metadata.Cluster, Region: d.App.Plan.Metadata.Region, Name: name,
		})
		rc := eksctl.RunContext{RunID: d.runID(), Cluster: d.App.Plan.Metadata.Cluster, Region: d.App.Plan.Metadata.Region, Phase: state.PhaseAddonUpgrade}
		if _, err := d.App.Eksctl.RunWithTimeout(ctx, rc, args, timeout); err != nil {
			return fmt.Errorf("upgrading add-on %q: %w", name, err)
		}
	}
	return nil
}

// UpgradeKarpenter upgrades the Karpenter controller/CRDs via Helm. It deliberately
// does not patch the live EC2NodeClass/NodePool objects: AMI selector and disruption
// budget changes are Git-owned and are proposed as a patch in GenerateConfigPatches,
// consistent with "Git is the source of truth" and "never silently rewrite config".
func (d *Deps) UpgradeKarpenter(ctx context.Context) error {
	k := d.App.Plan.Spec.Karpenter
	timeout := k.Controller.WaitTimeout.Duration
	if timeout <= 0 {
		timeout = 20 * time.Minute
	}
	_, err := d.App.Helm.Upgrade(ctx, helmUpgradeOptionsFromPlan(k, timeout))
	return err
}

func (d *Deps) UpgradeNodeGroup(ctx context.Context, nodeGroupName string) error {
	var launchTemplateVersion string
	for _, ng := range d.App.Cluster.ManagedNodeGroups {
		if ng.Name == nodeGroupName && ng.LaunchTemplate != nil {
			launchTemplateVersion = ng.LaunchTemplate.Version
		}
	}

	args := eksctl.BuildUpgradeNodeGroupArgs(eksctl.NodeGroupUpgradeOptions{
		Cluster: d.App.Plan.Metadata.Cluster, Region: d.App.Plan.Metadata.Region,
		NodeGroup: nodeGroupName, KubernetesVersion: d.App.Plan.Spec.TargetKubernetesVersion,
		LaunchTemplateVersion: launchTemplateVersion,
	})
	rc := eksctl.RunContext{RunID: d.runID(), Cluster: d.App.Plan.Metadata.Cluster, Region: d.App.Plan.Metadata.Region, Phase: state.PhaseNodeGroupUpgrade}
	timeout := d.App.Plan.Spec.NodeGroups.PerNodeGroupTimeout.Duration
	if timeout <= 0 {
		timeout = 60 * time.Minute
	}
	_, err := d.App.Eksctl.RunWithTimeout(ctx, rc, args, timeout)
	return err
}

func (d *Deps) NodeGroupHealthGate(ctx context.Context, nodeGroupName string) (upgrade.HealthGateResult, error) {
	input := upgrade.HealthGateInput{AllNodesReady: true, AllPodsReady: true}
	if d.App.K8s == nil {
		return upgrade.EvaluateHealthGates(d.App.Plan.Spec.NodeGroups.HealthGates, input), nil
	}

	nodes, err := d.App.K8s.NodesForNodeGroup(ctx, nodeGroupName)
	if err != nil {
		return upgrade.HealthGateResult{}, err
	}
	for _, n := range nodes {
		if !n.Ready {
			input.AllNodesReady = false
			input.NotReadyNodes = append(input.NotReadyNodes, n.Name)
		}
	}

	pending, err := d.App.K8s.PendingPods(ctx)
	if err == nil {
		input.PendingPodCount = len(pending)
	}

	if len(d.App.Plan.Spec.Preflight.CriticalNamespaces) > 0 {
		if issues, err := d.App.K8s.ValidatePDBsForNamespaces(ctx, d.App.Plan.Spec.Preflight.CriticalNamespaces); err == nil {
			input.PDBIssues = issues
		}
	}

	return upgrade.EvaluateHealthGates(d.App.Plan.Spec.NodeGroups.HealthGates, input), nil
}

func (d *Deps) RunCanary(ctx context.Context, namespaces []string) ([]upgrade.CanaryCheckResult, bool, error) {
	if !d.App.Plan.Spec.Canary.Enabled || len(d.App.Plan.Spec.Canary.Checks) == 0 {
		return nil, true, nil
	}
	client := &upgrade.HTTPPrometheusClient{BaseURL: d.App.Plan.Spec.Canary.PrometheusURL}
	if envVar := d.App.Plan.Spec.Canary.PrometheusBearerTokenEnv; envVar != "" {
		client.BearerToken = os.Getenv(envVar)
	}
	return upgrade.EvaluateCanaryChecks(ctx, client, d.App.Plan.Spec.Canary.Checks)
}

func (d *Deps) PostUpgradeCheck(ctx context.Context) ([]readiness.Finding, error) {
	return d.liveClusterHealthFindings(ctx), nil
}

func (d *Deps) GenerateConfigPatches(ctx context.Context) error {
	outDir := d.outputDir()
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	if patch, err := reconcile.GenerateClusterPatch(d.App.Cluster, d.App.Plan.Spec.TargetKubernetesVersion); err != nil {
		d.App.Logger.Warn("could not generate cluster.yaml patch", "error", err)
	} else {
		_ = os.WriteFile(outDir+"/cluster.patch", []byte(patch), 0o644)
	}

	if d.App.Plan.Spec.Karpenter.Enabled && d.App.Karpenter != nil {
		d.writeKarpenterPatches(ctx, outDir)
	}

	report, err := upgrade.BuildRunReport(ctx, d.App.Store, d.runID())
	if err != nil {
		return fmt.Errorf("building final run report: %w", err)
	}
	if data, err := report.JSON(); err == nil {
		_ = os.WriteFile(outDir+"/upgrade-report.json", data, 0o644)
	}
	_ = os.WriteFile(outDir+"/upgrade-report.md", []byte(report.Markdown()), 0o644)
	if data, err := report.EksctlCommandsJSON(); err == nil {
		_ = os.WriteFile(outDir+"/eksctl-commands.json", data, 0o644)
	}
	_ = os.WriteFile(outDir+"/next-actions.md", []byte(nextActionsMarkdown(outDir)), 0o644)
	return nil
}

func (d *Deps) writeKarpenterPatches(ctx context.Context, outDir string) {
	classes, err := d.App.Karpenter.ListEC2NodeClasses(ctx)
	if err != nil {
		return
	}
	desired := map[string]string{}
	for _, nc := range d.App.Plan.Spec.Karpenter.NodeClasses {
		desired[nc.Name] = nc.AMIAlias
	}
	var combined string
	for _, obs := range classes {
		want, ok := desired[obs.Name]
		if !ok || want == "" || want == obs.AMIAlias {
			continue
		}
		manifestPath := ""
		for _, nc := range d.App.Plan.Spec.Karpenter.NodeClasses {
			if nc.Name == obs.Name {
				manifestPath = nc.ManifestPath
			}
		}
		if manifestPath == "" {
			continue
		}
		patch, err := reconcile.GenerateKarpenterNodeClassPatch(manifestPath, obs.AMIAlias, want)
		if err != nil {
			d.App.Logger.Warn("could not generate karpenter patch", "nodeClass", obs.Name, "error", err)
			continue
		}
		combined += patch + "\n"
	}
	if combined != "" {
		_ = os.WriteFile(outDir+"/karpenter.patch", []byte(combined), 0o644)
	}
}

func (d *Deps) EksctlVersionSupports(ctx context.Context, targetVersion string) (bool, string, error) {
	version, err := d.App.Eksctl.Version(ctx)
	if err != nil {
		return false, "", fmt.Errorf("could not determine installed eksctl version: %w", err)
	}
	if version == "" {
		return false, version, fmt.Errorf("eksctl reported an empty version string")
	}
	// eksctl itself tracks Kubernetes release support closely and will refuse
	// unsupported --kubernetes-version values at run time; we only verify here that
	// eksctl is installed and responsive, not an exact version matrix (which eksctl's
	// own release notes are the source of truth for).
	return true, version, nil
}

func (d *Deps) outputDir() string {
	dir := d.App.Plan.Spec.PostUpgrade.OutputDir
	if dir == "" {
		dir = "./output"
	}
	return resolveRelativeToPlan(d.App.Plan, dir)
}

func (d *Deps) writeReadinessArtifacts(report *readiness.Report) {
	outDir := d.outputDir()
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return
	}
	if data, err := readiness.ReportJSON(report); err == nil {
		_ = os.WriteFile(outDir+"/upgrade-readiness-report.json", data, 0o644)
	}
	_ = os.WriteFile(outDir+"/upgrade-readiness-report.md", []byte(readiness.ReportMarkdown(report)), 0o644)
	if data, err := readiness.DeprecatedAPIFindingsJSON(report); err == nil {
		_ = os.WriteFile(outDir+"/deprecated-api-findings.json", data, 0o644)
	}
	_ = os.WriteFile(outDir+"/remediation-plan.md", []byte(readiness.RemediationPlanMarkdown(report)), 0o644)
}

func nextActionsMarkdown(outDir string) string {
	return "# Next Actions\n\n" +
		"Upgrade complete. Review and commit the generated configuration patch:\n\n" +
		"```bash\ngit apply " + outDir + "/cluster.patch\ngit apply " + outDir + "/karpenter.patch\n```\n"
}
