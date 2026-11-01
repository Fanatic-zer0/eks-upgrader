// Package config defines the strongly typed representations of cluster.yaml
// (eksctl ClusterConfig subset) and upgrade.yaml (EKSUpgradePlan).
package config

import (
	"fmt"
	"time"
)

// Duration wraps time.Duration with YAML (de)serialization from strings like "90m", "4h".
type Duration struct {
	time.Duration
}

// UnmarshalYAML implements yaml.v3 unmarshaling from a Go duration string.
func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	if s == "" {
		return nil
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = parsed
	return nil
}

// MarshalYAML implements yaml.v3 marshaling back to a Go duration string.
func (d Duration) MarshalYAML() (interface{}, error) {
	return d.Duration.String(), nil
}

// ClusterConfig is the subset of the eksctl ClusterConfig that eks-upgrader reads.
// It is never written back to disk by this application.
type ClusterConfig struct {
	APIVersion        string             `yaml:"apiVersion"`
	Kind              string             `yaml:"kind"`
	Metadata          ClusterMetadata    `yaml:"metadata"`
	ManagedNodeGroups []ManagedNodeGroup `yaml:"managedNodeGroups"`

	// SourcePath records where this config was loaded from for patch/report generation.
	SourcePath string `yaml:"-"`
}

type ClusterMetadata struct {
	Name    string            `yaml:"name"`
	Region  string            `yaml:"region"`
	Version string            `yaml:"version"`
	Tags    map[string]string `yaml:"tags"`
}

type ManagedNodeGroup struct {
	Name              string            `yaml:"name"`
	InstanceType      string            `yaml:"instanceType"`
	DesiredCapacity   int               `yaml:"desiredCapacity"`
	MinSize           int               `yaml:"minSize"`
	MaxSize           int               `yaml:"maxSize"`
	PrivateNetworking bool              `yaml:"privateNetworking"`
	Labels            map[string]string `yaml:"labels"`
	Taints            []NodeTaint       `yaml:"taints,omitempty"`
	LaunchTemplate    *LaunchTemplate   `yaml:"launchTemplate,omitempty"`
}

type NodeTaint struct {
	Key    string `yaml:"key"`
	Value  string `yaml:"value"`
	Effect string `yaml:"effect"`
}

type LaunchTemplate struct {
	ID      string `yaml:"id,omitempty"`
	Name    string `yaml:"name,omitempty"`
	Version string `yaml:"version,omitempty"`
}

// UpgradePlan is the strongly typed representation of upgrade.yaml (EKSUpgradePlan).
type UpgradePlan struct {
	APIVersion string       `yaml:"apiVersion"`
	Kind       string       `yaml:"kind"`
	Metadata   PlanMetadata `yaml:"metadata"`
	Spec       PlanSpec     `yaml:"spec"`

	// SourcePath records where this plan was loaded from.
	SourcePath string `yaml:"-"`
}

type PlanMetadata struct {
	Name    string `yaml:"name"`
	Cluster string `yaml:"cluster"`
	Region  string `yaml:"region"`
	RunID   string `yaml:"runID"`
}

type PlanSpec struct {
	ClusterConfig           string                  `yaml:"clusterConfig"`
	TargetKubernetesVersion string                  `yaml:"targetKubernetesVersion"`
	Execution               ExecutionSpec           `yaml:"execution"`
	StateReconciliation     StateReconciliationSpec `yaml:"stateReconciliation"`
	Preflight               PreflightSpec           `yaml:"preflight"`
	ControlPlane            ControlPlaneSpec        `yaml:"controlPlane"`
	Addons                  AddonsSpec              `yaml:"addons"`
	Karpenter               KarpenterSpec           `yaml:"karpenter"`
	NodeGroups              NodeGroupsSpec          `yaml:"nodeGroups"`
	Canary                  CanarySpec              `yaml:"canary"`
	Fallback                FallbackSpec            `yaml:"fallback"`
	PostUpgrade             PostUpgradeSpec         `yaml:"postUpgrade"`
}

type LockSpec struct {
	Enabled bool     `yaml:"enabled"`
	Backend string   `yaml:"backend"` // local | dynamodb
	TTL     Duration `yaml:"ttl"`
}

type ExecutionSpec struct {
	Mode                    string   `yaml:"mode"` // sequential | interactive
	DryRun                  bool     `yaml:"dryRun"`
	ResumeOnFailure         bool     `yaml:"resumeOnFailure"`
	MaxConcurrentNodeGroups int      `yaml:"maxConcurrentNodeGroups"`
	StateBackend            string   `yaml:"stateBackend"`
	StateFile               string   `yaml:"stateFile"`
	Lock                    LockSpec `yaml:"lock"`
}

type StateReconciliationSpec struct {
	Enabled       bool     `yaml:"enabled"`
	FailOnDrift   bool     `yaml:"failOnDrift"`
	IgnoredFields []string `yaml:"ignoredFields"`
	WarnOnlyOn    []string `yaml:"warnOnlyOn"`
	BlockOn       []string `yaml:"blockOn"`
}

type DeprecatedAPIsSpec struct {
	FailOnFound               bool `yaml:"failOnFound"`
	FailOnRemovedInTargetVers bool `yaml:"failOnRemovedInTargetVersion"`
	WarnOnDeprecatedBefRemove bool `yaml:"warnOnDeprecatedBeforeRemoval"`
}

type EKSUpgradeInsightsSpec struct {
	Enabled             bool `yaml:"enabled"`
	FailOnError         bool `yaml:"failOnError"`
	AcknowledgeWarnings bool `yaml:"acknowledgeWarnings"`
}

type CloudWatchPreflightSpec struct {
	Enabled                 bool `yaml:"enabled"`
	APIServer5xxThreshold   int  `yaml:"apiServer5xxThreshold"`
	EvaluationWindowMinutes int  `yaml:"evaluationWindowMinutes"`
}

type PreflightSpec struct {
	RequiredNodeGroupVersionsMatchControlPlane bool                    `yaml:"requiredNodeGroupVersionsMatchControlPlane"`
	RequireAllNodesReady                       bool                    `yaml:"requireAllNodesReady"`
	RequirePDBsForCriticalWorkloads            bool                    `yaml:"requirePDBsForCriticalWorkloads"`
	CriticalNamespaces                         []string                `yaml:"criticalNamespaces"`
	MinAvailableIPsPerSubnet                   int                     `yaml:"minAvailableIPsPerSubnet"`
	RequireSubnetCapacity                      bool                    `yaml:"requireSubnetCapacity"`
	DeprecatedAPIs                             DeprecatedAPIsSpec      `yaml:"deprecatedAPIs"`
	EKSUpgradeInsights                         EKSUpgradeInsightsSpec  `yaml:"eksUpgradeInsights"`
	CloudWatch                                 CloudWatchPreflightSpec `yaml:"cloudwatch"`
}

type ApprovalSpec struct {
	Required        bool     `yaml:"required"`
	Approvers       []string `yaml:"approvers"`
	ApprovalTimeout Duration `yaml:"approvalTimeout"`
}

type WaitSpec struct {
	Interval Duration `yaml:"interval"`
	Timeout  Duration `yaml:"timeout"`
}

type ControlPlaneSpec struct {
	Timeout  Duration     `yaml:"timeout"`
	Approval ApprovalSpec `yaml:"approval"`
	Wait     WaitSpec     `yaml:"wait"`
}

type AddonsSpec struct {
	Upgrade  bool     `yaml:"upgrade"`
	Names    []string `yaml:"names"`
	Strategy string   `yaml:"strategy"`
	Timeout  Duration `yaml:"timeout"`
}

type KarpenterControllerSpec struct {
	UpgradeStrategy string   `yaml:"upgradeStrategy"` // helm
	ChartRepository string   `yaml:"chartRepository"`
	ChartName       string   `yaml:"chartName"`
	ChartVersion    string   `yaml:"chartVersion"`
	WaitTimeout     Duration `yaml:"waitTimeout"`
}

type KarpenterNodeClassSpec struct {
	Name         string `yaml:"name"`
	ManifestPath string `yaml:"manifestPath"`
	AMIStrategy  string `yaml:"amiStrategy"`
	AMIAlias     string `yaml:"amiAlias"`
}

type DisruptionBudget struct {
	Nodes    string   `yaml:"nodes"`
	Schedule string   `yaml:"schedule,omitempty"`
	Duration Duration `yaml:"duration,omitempty"`
	Reasons  []string `yaml:"reasons,omitempty"`
}

type KarpenterNodePoolSpec struct {
	Name                      string             `yaml:"name"`
	ManifestPath              string             `yaml:"manifestPath"`
	DriftEnabled              bool               `yaml:"driftEnabled"`
	MaxUnavailablePercentage  int                `yaml:"maxUnavailablePercentage"`
	MaxConcurrentDriftedNodes int                `yaml:"maxConcurrentDriftedNodes"`
	DisruptionBudgets         []DisruptionBudget `yaml:"disruptionBudgets"`
}

type KarpenterSafeguardsSpec struct {
	PauseConsolidationDuringUpgrade bool `yaml:"pauseConsolidationDuringUpgrade"`
	RequirePDBs                     bool `yaml:"requirePDBs"`
	RequireNodeCapacityHeadroom     bool `yaml:"requireNodeCapacityHeadroom"`
	MinFreeIPsPerSubnet             int  `yaml:"minFreeIPsPerSubnet"`
}

type KarpenterSpec struct {
	Enabled     bool                     `yaml:"enabled"`
	Namespace   string                   `yaml:"namespace"`
	Controller  KarpenterControllerSpec  `yaml:"controller"`
	NodeClasses []KarpenterNodeClassSpec `yaml:"nodeClasses"`
	NodePools   []KarpenterNodePoolSpec  `yaml:"nodePools"`
	Safeguards  KarpenterSafeguardsSpec  `yaml:"safeguards"`
}

type HealthGatesSpec struct {
	RequireNodesReady   bool `yaml:"requireNodesReady"`
	RequirePodsReady    bool `yaml:"requirePodsReady"`
	MaxPendingPods      int  `yaml:"maxPendingPods"`
	RequirePDBRespected bool `yaml:"requirePDBRespected"`
}

type NodeGroupsSpec struct {
	Order                    []string        `yaml:"order"`
	Strategy                 string          `yaml:"strategy"`
	MaxUnavailablePercentage int             `yaml:"maxUnavailablePercentage"`
	PerNodeGroupTimeout      Duration        `yaml:"perNodeGroupTimeout"`
	DrainTimeout             Duration        `yaml:"drainTimeout"`
	WaitBetweenGroups        Duration        `yaml:"waitBetweenGroups"`
	HealthGates              HealthGatesSpec `yaml:"healthGates"`
}

type CanaryCheck struct {
	Name       string   `yaml:"name"`
	Query      string   `yaml:"query"`
	Threshold  float64  `yaml:"threshold"`
	Comparison string   `yaml:"comparison"` // lessThan | greaterThan
	Duration   Duration `yaml:"duration"`
}

type CanarySpec struct {
	Enabled                  bool          `yaml:"enabled"`
	Namespaces               []string      `yaml:"namespaces"`
	PrometheusURL            string        `yaml:"prometheusURL"`
	PrometheusBearerTokenEnv string        `yaml:"prometheusBearerTokenEnv"`
	PrometheusTLSSkipVerify  bool          `yaml:"prometheusTLSSkipVerify"`
	ComparisonMode           string        `yaml:"comparisonMode"` // absolute | baseline
	Checks                   []CanaryCheck `yaml:"checks"`
}

type BlueGreenNodeGroupsSpec struct {
	Enabled bool   `yaml:"enabled"`
	Suffix  string `yaml:"suffix"`
}

type FallbackNotifySpec struct {
	SlackWebhookEnv        string `yaml:"slackWebhookEnv"`
	PagerdutyRoutingKeyEnv string `yaml:"pagerdutyRoutingKeyEnv"`
	WebhookEnv             string `yaml:"webhookEnv"`
}

type FallbackSpec struct {
	OnPreflightFailure         string                  `yaml:"onPreflightFailure"`
	OnReadinessBlockingFailure string                  `yaml:"onReadinessBlockingFailure"`
	OnStateDrift               string                  `yaml:"onStateDrift"`
	OnControlPlaneFailure      string                  `yaml:"onControlPlaneFailure"`
	OnAddonFailure             string                  `yaml:"onAddonFailure"`
	OnKarpenterFailure         string                  `yaml:"onKarpenterFailure"`
	OnNodeGroupFailure         string                  `yaml:"onNodeGroupFailure"`
	OnCanaryFailure            string                  `yaml:"onCanaryFailure"`
	OnPostCheckFailure         string                  `yaml:"onPostCheckFailure"`
	PreserveFailedNodeGroup    bool                    `yaml:"preserveFailedNodeGroup"`
	BlueGreenNodeGroups        BlueGreenNodeGroupsSpec `yaml:"blueGreenNodeGroups"`
	Notify                     FallbackNotifySpec      `yaml:"notify"`
}

type PostUpgradeSpec struct {
	VerifyNodes         bool     `yaml:"verifyNodes"`
	VerifyWorkloads     bool     `yaml:"verifyWorkloads"`
	VerifyIngress       bool     `yaml:"verifyIngress"`
	VerifyDNS           bool     `yaml:"verifyDNS"`
	VerifyPVCs          bool     `yaml:"verifyPVCs"`
	RunSmokeTests       bool     `yaml:"runSmokeTests"`
	SmokeTestCommand    []string `yaml:"smokeTestCommand"`
	GenerateConfigPatch bool     `yaml:"generateConfigPatch"`
	OutputDir           string   `yaml:"outputDir"`
}
