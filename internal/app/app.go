// Package app wires the orchestration engine (internal/upgrade) to every real
// external system: eksctl, AWS, Kubernetes, Karpenter, Helm, and Prometheus. It is the
// single place where concrete clients are constructed; internal/cli depends only on
// this package and internal/upgrade, never on the lower-level packages directly.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"k8s.io/client-go/dynamic"

	"eks-upgrader/internal/audit"
	internalaws "eks-upgrader/internal/aws"
	"eks-upgrader/internal/config"
	"eks-upgrader/internal/eksctl"
	"eks-upgrader/internal/helm"
	"eks-upgrader/internal/k8s"
	"eks-upgrader/internal/karpenter"
	"eks-upgrader/internal/lock"
	"eks-upgrader/internal/notify"
	"eks-upgrader/internal/state"
)

// Options configures how App resolves its external connections. Every field maps to a
// global CLI flag or environment variable; no secrets are read from YAML.
type Options struct {
	ClusterConfigPath string
	PlanPath          string
	KubeconfigPath    string
	StateFilePath     string
	AuditLogPath      string
	EksctlBinaryPath  string
	HelmBinaryPath    string
	LogLevel          string
	LogFormat         string
	Owner             string
}

// App bundles every constructed client/dependency needed to run the engine.
type App struct {
	Cluster *config.ClusterConfig
	Plan    *config.UpgradePlan

	Store state.Store
	Lock  lock.Locker
	Audit audit.Logger

	Logger   *slog.Logger
	Notifier notify.Notifier

	Eksctl     *eksctl.InstrumentedExecutor
	EKS        *internalaws.EKSClient
	EC2        *internalaws.EC2Client
	IAM        *internalaws.IAMClient
	CloudWatch *internalaws.CloudWatchClient
	K8s        *k8s.Client
	Karpenter  *karpenter.Client
	Helm       helm.Upgrader

	Options Options
}

// New constructs an App from Options, loading cluster.yaml/upgrade.yaml and wiring
// every external client. Kubernetes/Karpenter clients are optional (nil) when no
// kubeconfig is reachable, so that AWS-only commands (e.g. `state pull` without
// cluster access) still function; callers must nil-check before use.
func New(opts Options) (*App, error) {
	logger := buildLogger(opts.LogLevel, opts.LogFormat)

	plan, err := config.LoadUpgradePlan(opts.PlanPath)
	if err != nil {
		return nil, fmt.Errorf("loading upgrade plan: %w", err)
	}
	clusterConfigPath := opts.ClusterConfigPath
	if clusterConfigPath == "" {
		clusterConfigPath = config.ResolveClusterConfigPath(plan)
	}
	cluster, err := config.LoadClusterConfig(clusterConfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading cluster config: %w", err)
	}

	stateFile := opts.StateFilePath
	if stateFile == "" {
		stateFile = resolveRelativeToPlan(plan, plan.Spec.Execution.StateFile)
	}
	store, err := state.OpenSQLiteStore(stateFile)
	if err != nil {
		return nil, fmt.Errorf("opening state store: %w", err)
	}

	auditPath := opts.AuditLogPath
	if auditPath == "" {
		auditPath = resolveRelativeToPlan(plan, "./state/audit.jsonl")
	}
	auditLogger, err := audit.NewFileLogger(auditPath)
	if err != nil {
		return nil, fmt.Errorf("opening audit log: %w", err)
	}

	var locker lock.Locker
	switch plan.Spec.Execution.Lock.Backend {
	case "dynamodb":
		awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
		if err != nil {
			return nil, fmt.Errorf("loading AWS config for dynamodb lock backend: %w", err)
		}
		locker = lock.NewDynamoDBLocker(dynamodb.NewFromConfig(awsCfg), "eks-upgrader-locks")
	default:
		locker = lock.NewLocalLocker(resolveRelativeToPlan(plan, "./state/locks"))
	}

	notifier := buildNotifier(plan.Spec.Fallback.Notify)

	app := &App{
		Cluster: cluster, Plan: plan, Store: store, Lock: locker, Audit: auditLogger,
		Logger: logger, Notifier: notifier, Options: opts,
	}

	eksctlExec := &eksctl.SubprocessExecutor{BinaryPath: opts.EksctlBinaryPath}
	app.Eksctl = &eksctl.InstrumentedExecutor{Inner: eksctlExec, Audit: auditLogger, Store: store, Logger: logger}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(plan.Metadata.Region))
	if err != nil {
		logger.Warn("could not load AWS SDK default config; AWS-dependent checks will fail", "error", err)
	} else {
		app.EKS = internalaws.NewEKSClient(eks.NewFromConfig(awsCfg))
		app.EC2 = internalaws.NewEC2Client(ec2.NewFromConfig(awsCfg))
		app.IAM = internalaws.NewIAMClient(iam.NewFromConfig(awsCfg))
		app.CloudWatch = internalaws.NewCloudWatchClient(cloudwatch.NewFromConfig(awsCfg))
	}

	if k8sClient, err := k8s.NewClientFromKubeconfig(opts.KubeconfigPath); err != nil {
		logger.Warn("could not build Kubernetes client; cluster-dependent checks will be skipped", "error", err)
	} else {
		app.K8s = k8sClient
		if dynClient, err := dynamicClientFromKubeconfig(opts.KubeconfigPath); err != nil {
			logger.Warn("could not build Karpenter dynamic client", "error", err)
		} else {
			app.Karpenter = karpenter.NewClient(dynClient)
		}
	}

	app.Helm = &helm.CLIClient{BinaryPath: opts.HelmBinaryPath}

	return app, nil
}

// Close releases held resources (state store, audit log). Locks are released by the
// engine itself on completion/abort, not here.
func (a *App) Close() {
	if a.Store != nil {
		_ = a.Store.Close()
	}
	if a.Audit != nil {
		_ = a.Audit.Close()
	}
}

func dynamicClientFromKubeconfig(path string) (dynamic.Interface, error) {
	cfg, err := k8s.RESTConfigFromKubeconfig(path)
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(cfg)
}

// resolveRelativeToPlan resolves a relative path (e.g. spec.execution.stateFile)
// against the directory upgrade.yaml was loaded from, rather than the process's
// current working directory, mirroring config.ResolveClusterConfigPath so that state/
// audit/lock artifacts stay co-located with the plan regardless of where the CLI is
// invoked from.
func resolveRelativeToPlan(plan *config.UpgradePlan, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(filepath.Dir(plan.SourcePath), path)
}
