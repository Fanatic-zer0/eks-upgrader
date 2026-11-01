Build a production-grade, standalone Go application called `eks-upgrader`.

# Mission

Build a CLI-first Amazon EKS upgrade orchestrator that safely upgrades EKS clusters using eksctl as the only tool that mutates EKS and CloudFormation resources.

The application must consume two YAML inputs:

1. `cluster.yaml` — an eksctl-compatible `ClusterConfig`
2. `upgrade.yaml` — an upgrade plan containing target version, sequencing, safeguards, health gates, canary checks, notifications, fallback policy, live-state reconciliation policy, and upgrade-readiness policy

The application must support:

- eksctl-managed node groups
- Karpenter-managed compute
- EKS-managed add-ons
- Live cluster-state reconciliation
- Kubernetes API deprecation and compatibility checks
- Canary validation
- Approval workflows
- Pause, resume, abort, rollback guidance, and auditability

The app must be safe to run from CI/CD, a Kubernetes Job, or an operator workstation.

Do not use Terraform. Do not directly call AWS EKS mutation APIs except where strictly required for read-only validation. All EKS/CloudFormation mutations must be performed through eksctl.

# Core principles

- Git is the source of truth for desired state.
- eksctl owns EKS control plane, managed node groups, and CloudFormation mutations.
- Karpenter owns elastic node lifecycle and drift-based node replacement.
- `eks-upgrader` owns orchestration, policy enforcement, sequencing, state, validation, canaries, audit, notifications, and fallback decisions.
- The app must never silently rewrite `cluster.yaml`.
- After a successful upgrade, the app may generate a proposed Git patch or pull request, but a human must review and merge it.
- The app must upgrade only one Kubernetes minor version at a time.
- Every phase must be checkpointed and resumable.
- Every mutating command must be logged with full arguments, exit code, duration, user, cluster, region, and correlation ID.
- The app must fail closed: if a required safeguard cannot be verified, pause or abort rather than proceed.
- No AWS credentials, Slack tokens, webhook URLs, PagerDuty keys, or other secrets may be stored in YAML. Use environment variables, AWS IAM roles, IRSA, Secrets Manager, or CI OIDC.
- Do not hardcode cluster names, AWS account IDs, regions, AMI IDs, VPC IDs, subnet IDs, or secrets.

# Technology requirements

Use:

- Go 1.23+
- Cobra for CLI
- Direct YAML parsing for configuration via `sigs.k8s.io/yaml` or `gopkg.in/yaml.v3` (no Viper); secret-reference fields (e.g. `slackWebhookEnv`) are resolved explicitly by the config loader, not via env-binding frameworks
- `log/slog` for structured logging
- SQLite via `modernc.org/sqlite` for default local state
- Kubernetes client-go for read-only Kubernetes validation
- AWS SDK for Go v2 for read-only AWS validation
- Helm Go SDK or Helm CLI integration for optional Karpenter controller upgrade
- `testcontainers-go` or mocked executors for tests
- GitHub Actions for CI

The app must execute eksctl as a subprocess, capture stdout/stderr, parse exit codes, enforce timeouts using Go context, and emit structured audit events.

# Repository layout

Create this exact layout:

eks-upgrader/
├── cmd/
│   └── eks-upgrader/
│       └── main.go
├── internal/
│   ├── cli/
│   │   ├── root.go
│   │   ├── validate.go
│   │   ├── state.go
│   │   ├── readiness.go
│   │   ├── upgrade.go
│   │   ├── resume.go
│   │   ├── status.go
│   │   ├── abort.go
│   │   ├── pause.go
│   │   ├── approve.go
│   │   ├── lock.go
│   │   └── report.go
│   ├── app/
│   │   └── app.go
│   ├── config/
│   │   ├── loader.go
│   │   ├── schema.go
│   │   ├── validate.go
│   │   └── testdata/
│   ├── upgrade/
│   │   ├── engine.go
│   │   ├── statemachine.go
│   │   ├── phases.go
│   │   ├── gates.go
│   │   ├── canary.go
│   │   ├── rollback.go
│   │   └── report.go
│   ├── readiness/
│   │   ├── analyzer.go
│   │   ├── deprecated_apis.go
│   │   ├── compatibility.go
│   │   ├── insights.go
│   │   ├── capacity.go
│   │   ├── report.go
│   │   └── scanners/
│   │       ├── pluto.go
│   │       ├── kubent.go
│   │       └── metrics.go
│   ├── reconcile/
│   │   ├── collector.go
│   │   ├── differ.go
│   │   ├── normalizer.go
│   │   ├── patch.go
│   │   └── report.go
│   ├── eksctl/
│   │   ├── executor.go
│   │   ├── cluster.go
│   │   ├── nodegroup.go
│   │   ├── addons.go
│   │   └── mocks/
│   ├── helm/
│   │   └── client.go
│   ├── karpenter/
│   │   ├── client.go
│   │   ├── nodepool.go
│   │   ├── nodeclass.go
│   │   ├── drift.go
│   │   └── disruption.go
│   ├── k8s/
│   │   ├── client.go
│   │   ├── nodes.go
│   │   ├── pods.go
│   │   ├── pdb.go
│   │   ├── workloads.go
│   │   ├── addons.go
│   │   └── events.go
│   ├── aws/
│   │   ├── eks.go
│   │   ├── ec2.go
│   │   ├── iam.go
│   │   ├── cloudwatch.go
│   │   └── insights.go
│   ├── state/
│   │   ├── store.go
│   │   ├── sqlite.go
│   │   ├── models.go
│   │   └── migrations/
│   ├── notify/
│   │   ├── notifier.go
│   │   ├── slack.go
│   │   ├── webhook.go
│   │   └── pagerduty.go
│   ├── audit/
│   │   ├── logger.go
│   │   └── models.go
│   └── lock/
│       ├── locker.go
│       ├── local.go
│       └── dynamodb.go
├── configs/
│   └── examples/
│       ├── cluster.yaml
│       ├── upgrade.yaml
│       ├── karpenter-nodepool.yaml
│       └── karpenter-ec2nodeclass.yaml
├── docs/
│   ├── architecture.md
│   ├── upgrade-runbook.md
│   ├── fallback-runbook.md
│   ├── yaml-reference.md
│   ├── readiness-checks.md
│   └── iam-policy.md
├── scripts/
│   ├── install-tools.sh
│   └── smoke-test.sh
├── .github/
│   └── workflows/
│       ├── ci.yaml
│       └── release.yaml
├── Dockerfile
├── Makefile
├── go.mod
├── go.sum
└── README.md

# CLI requirements

Implement these Cobra commands:

```bash
eks-upgrader validate \
  --plan configs/upgrade.yaml \
  --cluster-config configs/cluster.yaml \
  --use-live-state

eks-upgrader state pull \
  --cluster prod-cluster \
  --region ap-south-1 \
  --output ./output

eks-upgrader state diff \
  --cluster-config configs/cluster.yaml \
  --karpenter-config ./karpenter \
  --observed ./output/observed-cluster.yaml

eks-upgrader readiness \
  --plan configs/upgrade.yaml \
  --cluster-config configs/cluster.yaml \
  --target-version 1.31 \
  --output ./output

eks-upgrader upgrade \
  --plan configs/upgrade.yaml \
  --cluster-config configs/cluster.yaml \
  --dry-run

eks-upgrader upgrade \
  --plan configs/upgrade.yaml \
  --cluster-config configs/cluster.yaml

eks-upgrader resume \
  --plan configs/upgrade.yaml \
  --state state/upgrade.db

eks-upgrader status \
  --state state/upgrade.db

eks-upgrader abort \
  --state state/upgrade.db \
  --reason "manual abort"

eks-upgrader pause \
  --state state/upgrade.db \
  --reason "manual pause for investigation"

eks-upgrader lock release \
  --cluster prod-cluster \
  --region ap-south-1 \
  --confirm

eks-upgrader approve \
  --state state/upgrade.db \
  --approver "platform-oncall" \
  --comment "approved for maintenance window"

eks-upgrader report \
  --state state/upgrade.db \
  --format json|yaml|markdown

eks-upgrader version
```

Requirements:

- `validate` must perform no mutation.
- `state pull` must perform no mutation.
- `state diff` must perform no mutation.
- `readiness` must perform no mutation.
- `upgrade --dry-run` must print the exact eksctl commands that would run, validation checks, expected sequence, blocking issues, warnings, and proposed config patch.
- `upgrade` must execute the full state machine.
- `resume` must continue from the last persisted successful checkpoint.
- `status` must show current phase, completed phases, node-group progress, Karpenter progress, failures, approvals, and next action.
- `abort` must release locks, mark the run aborted, and send notifications.
- `pause` must mark the run paused at the next safe checkpoint without requiring a failure; it is operator-initiated.
- `lock release` must forcibly release a stuck cluster lock; it requires `--confirm` and is only safe when no run process is actually active.
- `approve` must record an approval event and allow a paused control-plane upgrade to proceed.
- Every command must support `--log-level`, `--log-format=json|text`, and `--timeout`.
- Every command must exit with a stable, CI-scriptable code: `0` success/ready, `1` warnings (e.g. `READY_WITH_WARNINGS` or non-blocking drift), `2` blocking failure, `3` internal/unexpected error.

# YAML contracts

## 1. `cluster.yaml`

This must remain compatible with eksctl `ClusterConfig`.

Example:

```yaml
apiVersion: eksctl.io/v1alpha5
kind: ClusterConfig

metadata:
  name: prod-cluster
  region: ap-south-1
  version: "1.30"
  tags:
    environment: production
    owner: platform-team

managedNodeGroups:
  - name: system
    instanceType: m6i.large
    desiredCapacity: 2
    minSize: 2
    maxSize: 4
    privateNetworking: true
    labels:
      workload: system

  - name: app-a
    instanceType: m6i.xlarge
    desiredCapacity: 3
    minSize: 3
    maxSize: 6
    privateNetworking: true
    labels:
      workload: app
      pool: a

  - name: app-b
    instanceType: m6i.xlarge
    desiredCapacity: 3
    minSize: 3
    maxSize: 6
    privateNetworking: true
    labels:
      workload: app
      pool: b
```

## 2. `upgrade.yaml`

Create a strongly typed Go representation for this schema:

```yaml
apiVersion: platform.io/v1
kind: EKSUpgradePlan

metadata:
  name: prod-cluster-1-31
  cluster: prod-cluster
  region: ap-south-1
  runID: optional-custom-run-id

spec:
  clusterConfig: ./cluster.yaml
  targetKubernetesVersion: "1.31"

  execution:
    mode: sequential   # orchestration context only (sequential|interactive); concurrency/rollout style is governed by maxConcurrentNodeGroups below and nodeGroups.strategy, not by this field
    dryRun: false
    resumeOnFailure: true
    maxConcurrentNodeGroups: 1
    stateBackend: sqlite
    stateFile: ./state/upgrade.db
    lock:
      enabled: true
      backend: local
      ttl: 4h

  stateReconciliation:
    enabled: true
    failOnDrift: true
    ignoredFields:
      - metadata.tags["last-updated"]
      - managedNodeGroups[*].status
    warnOnlyOn:
      - cluster.tags
      - nodeGroup.labels
    blockOn:
      - cluster.version
      - managedNodeGroups[*].kubernetesVersion
      - managedNodeGroups[*].desiredSize
      - managedNodeGroups[*].minSize
      - managedNodeGroups[*].maxSize
      - karpenter.nodeClasses[*].amiSelectorTerms
      - karpenter.nodePools[*].disruption.budgets

  preflight:
    requiredNodeGroupVersionsMatchControlPlane: true
    requireAllNodesReady: true
    requirePDBsForCriticalWorkloads: true
    criticalNamespaces:
      - kube-system
      - checkout
      - payments
    minAvailableIPsPerSubnet: 10
    requireSubnetCapacity: true
    deprecatedAPIs:
      failOnFound: true
      failOnRemovedInTargetVersion: true
      warnOnDeprecatedBeforeRemoval: true
    eksUpgradeInsights:
      enabled: true
      failOnError: true
      acknowledgeWarnings: true
    cloudwatch:
      enabled: true   # requires control-plane logging enabled or Container Insights; falls back to a direct /metrics scrape if unavailable (see docs/readiness-checks.md)
      apiServer5xxThreshold: 5
      evaluationWindowMinutes: 10

  controlPlane:
    timeout: 90m
    approval:
      required: true
      approvers:
        - platform-oncall
      approvalTimeout: 12h
    wait:
      interval: 30s
      timeout: 90m

  addons:
    upgrade: true
    names:
      - vpc-cni
      - coredns
      - kube-proxy
    strategy: latestCompatible
    timeout: 30m

  karpenter:
    enabled: true
    namespace: karpenter
    controller:
      upgradeStrategy: helm
      chartRepository: https://charts.karpenter.sh
      chartName: karpenter
      chartVersion: "1.6.0"
      waitTimeout: 20m
    nodeClasses:
      - name: default
        manifestPath: ./karpenter/ec2nodeclass.yaml
        amiStrategy: alias
        amiAlias: "al2023@v20250918"
    nodePools:
      - name: general-purpose
        manifestPath: ./karpenter/nodepool.yaml
        driftEnabled: true
        maxUnavailablePercentage: 20
        maxConcurrentDriftedNodes: 2
        disruptionBudgets:
          - nodes: "10%"
          - nodes: "0"
            schedule: "0 17 * * mon-fri"
            duration: 16h
            reasons:
              - Drifted
    safeguards:
      pauseConsolidationDuringUpgrade: true
      requirePDBs: true
      requireNodeCapacityHeadroom: true
      minFreeIPsPerSubnet: 10

  nodeGroups:
    order:
      - system
      - app-a
      - app-b
    strategy: rolling   # rollout style per node group; concurrency across node groups is capped by execution.maxConcurrentNodeGroups above
    maxUnavailablePercentage: 33
    perNodeGroupTimeout: 60m
    drainTimeout: 15m
    waitBetweenGroups: 5m
    healthGates:
      requireNodesReady: true
      requirePodsReady: true
      maxPendingPods: 0
      requirePDBRespected: true

  canary:
    enabled: true
    namespaces:
      - checkout
      - payments
    prometheusURL: https://prometheus.example.com
    prometheusBearerTokenEnv: PROMETHEUS_TOKEN
    prometheusTLSSkipVerify: false
    comparisonMode: absolute   # absolute | baseline (compares against a control group of not-yet-upgraded nodes)
    checks:
      - name: error-rate
        query: "sum(rate(http_requests_total{status=~'5..'}[5m])) / sum(rate(http_requests_total[5m]))"
        threshold: 0.01
        comparison: lessThan
        duration: 5m
      - name: p99-latency
        query: "histogram_quantile(0.99, sum(rate(http_request_duration_seconds_bucket[5m])) by (le))"
        threshold: 1.5
        comparison: lessThan
        duration: 5m

  fallback:
    onPreflightFailure: abort
    onReadinessBlockingFailure: abort
    onStateDrift: pause
    onControlPlaneFailure: pause
    onAddonFailure: pause
    onKarpenterFailure: pause
    onNodeGroupFailure: pause
    onCanaryFailure: pause
    onPostCheckFailure: pause
    preserveFailedNodeGroup: true
    blueGreenNodeGroups:
      enabled: false
      suffix: "-v{targetMinor}"
    notify:
      slackWebhookEnv: SLACK_WEBHOOK_URL
      pagerdutyRoutingKeyEnv: PAGERDUTY_ROUTING_KEY
      webhookEnv: UPGRADE_WEBHOOK_URL

  postUpgrade:
    verifyNodes: true
    verifyWorkloads: true
    verifyIngress: true
    verifyDNS: true
    verifyPVCs: true
    runSmokeTests: true
    smokeTestCommand: ["./scripts/smoke-test.sh"]
    generateConfigPatch: true
    outputDir: ./output
```

# Live-state reconciliation

Implement a read-only State Collector that pulls the current state of the EKS environment and compares it with Git-defined desired state.

## State Collector sources

Collect data from:

- AWS EKS API:
  - Cluster name
  - Region
  - Kubernetes version
  - Cluster status
  - Endpoint
  - OIDC issuer
  - Logging configuration
  - Tags

- eksctl:
  - Managed node groups
  - Node-group names
  - Kubernetes versions
  - Instance types
  - Desired, minimum, and maximum sizes
  - Labels
  - Taints
  - AMI type
  - Launch-template details
  - Status

- EC2 API:
  - Subnets
  - Available IP addresses
  - Security groups
  - Route tables
  - Instance states

- IAM:
  - Cluster IAM role
  - Node instance roles
  - OIDC provider
  - Karpenter controller role
  - Karpenter node role

- Kubernetes API:
  - Nodes
  - Node versions
  - Node labels and taints
  - Pods
  - Pending pods
  - PDBs
  - Deployments
  - StatefulSets
  - DaemonSets
  - Ingresses
  - Services
  - PVCs
  - Events

- EKS add-ons:
  - Installed add-on names
  - Versions
  - Health

- Karpenter:
  - Controller version
  - Controller health
  - `NodePool` objects
  - `EC2NodeClass` objects
  - `NodeClaim` objects
  - Drifted NodeClaims
  - Disruption budgets

- CloudWatch:
  - Control-plane health metrics
  - API server error metrics
  - Node status metrics

  > Note: EKS does not natively emit API-server error-rate metrics to CloudWatch *Metrics* without either Container Insights or control-plane logging enabled with CloudWatch Logs metric filters. If control-plane logging is not enabled, fall back to scraping `apiserver_request_total{code=~"5.."}` directly via `kubectl get --raw /metrics` (the same mechanism already used for deprecated-API detection) and treat the CloudWatch check as optional/best-effort. Document this prerequisite in `docs/readiness-checks.md`.

## State pull output

`eks-upgrader state pull` must generate:

```text
output/observed-cluster.yaml
output/observed-karpenter.yaml
output/drift-report.json
output/drift-report.md
output/raw/
```

Example observed state:

```yaml
apiVersion: platform.io/v1
kind: EKSObservedClusterState

metadata:
  cluster: prod-cluster
  region: ap-south-1
  collectedAt: "2026-10-31T10:45:00Z"

cluster:
  currentKubernetesVersion: "1.30"
  status: ACTIVE
  endpoint: [https://example.eks.ap-south-1.amazonaws.com](https://example.eks.ap-south-1.amazonaws.com)
  oidcIssuer: [https://oidc.eks.ap-south-1.amazonaws.com/id/EXAMPLE](https://oidc.eks.ap-south-1.amazonaws.com/id/EXAMPLE)

managedNodeGroups:
  - name: system
    status: ACTIVE
    kubernetesVersion: "1.30"
    desiredSize: 2
    minSize: 2
    maxSize: 4
    instanceTypes: ["m6i.large"]
    amiType: AL2023_x86_64_STANDARD

  - name: app-a
    status: ACTIVE
    kubernetesVersion: "1.30"
    desiredSize: 3
    minSize: 3
    maxSize: 6

karpenter:
  installed: true
  controllerVersion: "1.5.0"
  nodeClasses:
    - name: default
      healthy: true
      amiSelectorTerms:
        - alias: al2023@v20250815
  nodePools:
    - name: general-purpose
      healthy: true
      driftedNodeClaims: 0
      disruptionBudgets:
        - nodes: "10%"

addons:
  - name: vpc-cni
    version: "1.19.0"
    health: HEALTHY
  - name: coredns
    version: "1.11.3"
    health: HEALTHY
  - name: kube-proxy
    version: "1.30.0"
    health: HEALTHY
```

## State diff behavior

`eks-upgrader state diff` must compare:

- Git `cluster.yaml`
- Git Karpenter `NodePool` and `EC2NodeClass` manifests
- Live EKS cluster state
- Live managed node groups
- Live Karpenter objects
- Installed EKS add-ons

Classify every difference as:

- `IN_SYNC`
- `INFO`
- `WARNING`
- `BLOCKING`
- `UNMANAGED`

Block the upgrade when live state differs from desired state for:

- EKS Kubernetes version
- Managed node group Kubernetes version
- Managed node group desired/min/max size
- Karpenter `EC2NodeClass` AMI selector
- Karpenter `NodePool` disruption budgets
- Critical node-group labels or taints

Warn only for cosmetic drift such as tags or non-critical labels.

Never automatically modify `cluster.yaml`. Generate a proposed patch only.

# Upgrade-readiness analysis

Before approval, run an Upgrade Readiness Analyzer.

The analyzer must produce an overall status:

```text
READY
READY_WITH_WARNINGS
NOT_READY
```

It must query and analyze:

- Amazon EKS Upgrade Insights using `aws eks list-insights`
- Kubernetes deprecated API usage
- Kubernetes API server deprecated API metrics
- Helm release manifests
- Live Kubernetes objects and CRDs
- Karpenter controller compatibility
- Karpenter CRD compatibility
- EKS add-on compatibility
- Kubelet version skew
- Subnet IP capacity
- PDB validity
- Node readiness
- Pending pods
- Managed-node-group health
- Control-plane health
- IAM and OIDC configuration
- EKS rollback-readiness insights, reported as informational only

## EKS Upgrade Insights policy

- Treat `ERROR` insights for APIs removed in the target Kubernetes version as `BLOCKING`.
- Treat `WARNING` insights as requiring explicit acknowledgement.
- Treat `PASSING` as safe.
- Treat `UNKNOWN` as a warning unless configured otherwise.

## Deprecated API detection

Use multiple signals:

1. EKS Upgrade Insights.
2. Pluto scan of manifests, Helm releases, and live Kubernetes API resources.
3. kubent scan of live Kubernetes objects and Helm releases.
4. Kubernetes API server metric:

```bash
kubectl get --raw /metrics | grep apiserver_requested_deprecated_apis
```

5. Kubernetes API deprecation guide rules.

Classify findings:

- API removed in target Kubernetes version → `BLOCKING`
- API deprecated but not removed in target version → `WARNING`
- CRD using deprecated API → `WARNING`
- Helm chart using deprecated API → `WARNING`
- Client still calling deprecated API → `WARNING` or `BLOCKING` based on configuration

## Readiness report output

Generate:

```text
output/upgrade-readiness-report.md
output/upgrade-readiness-report.json
output/deprecated-api-findings.json
output/remediation-plan.md
```

Example terminal warning:

```text
⚠️  UPGRADE READINESS: NOT READY

BLOCKING: Deprecated API removed in Kubernetes 1.31
  Resource: FlowSchema/example
  API: flowcontrol.apiserver.k8s.io/v1beta3
  Recommendation: migrate to flowcontrol.apiserver.k8s.io/v1

BLOCKING: Karpenter 1.5.0 is not compatible with Kubernetes 1.31
  Recommendation: upgrade Karpenter controller and CRDs first

WARNING: kube-proxy 1.30.2 should be upgraded after control plane
  Acknowledge? [y/N]

Upgrade aborted due to blocking findings.
```

The upgrade must not call `eksctl upgrade cluster --approve` unless the readiness report is `READY` or `READY_WITH_WARNINGS` and all required warnings have been acknowledged.

# Upgrade state machine

Implement this exact finite state machine:

```text
INIT
  → VALIDATE
  → STATE_RECONCILIATION
  → READINESS_ANALYSIS
  → PREFLIGHT
  → APPROVAL
  → CONTROL_PLANE_UPGRADE
  → ADDON_UPGRADE
  → KARPENTER_UPGRADE
  → NODE_GROUP_UPGRADE
  → POST_UPGRADE_CHECK
  → CONFIG_RECONCILIATION
  → COMPLETE
```

`CANARY` is not a top-level phase — canary checks are a mandatory sub-step embedded within `NODE_GROUP_UPGRADE` (after each node group) and `POST_UPGRADE_CHECK`. A canary failure fails the *enclosing* phase; the state machine never transitions into or out of a standalone `CANARY` state. Persist canary results against whichever phase invoked them.

Failure transitions:

```text
VALIDATE failure                   → FAILED
STATE_RECONCILIATION drift         → PAUSED or ABORTED based on policy
READINESS blocking finding         → ABORTED
PREFLIGHT failure                  → PAUSED or ABORTED based on policy
APPROVAL timeout                   → PAUSED
CONTROL_PLANE failure              → PAUSED
ADDON failure                      → PAUSED
KARPENTER failure                  → PAUSED
NODE_GROUP failure (incl. embedded canary failure)         → PAUSED
POST_UPGRADE_CHECK failure (incl. embedded canary failure) → PAUSED
CONFIG_RECONCILIATION failure      → COMPLETE_WITH_WARNINGS
```

Persist for every run:

- Run ID
- Cluster name and region
- Source version and target version
- Current phase
- Phase history with timestamps
- Approval records
- eksctl command history
- Node-group status
- Karpenter status
- State-drift findings
- Readiness findings
- Canary results
- Errors and remediation hints
- Final report paths

## Resume semantics

- `resume` must re-verify live state for the in-flight phase before retrying it — if the last `eksctl` command actually succeeded out-of-band (e.g. the process crashed after eksctl exited 0 but before the checkpoint was persisted), the engine must detect this via a live-state check and advance instead of re-running the mutating command.
- `resume` must track a per-phase retry counter; after a configurable maximum (default 3) consecutive failures resuming the same phase, the run auto-transitions to `PAUSED` and requires `--force` or operator intervention rather than retrying indefinitely.
- The process must trap `SIGINT`/`SIGTERM` and persist a safe checkpoint (marking the current phase `INTERRUPTED` rather than `FAILED`) before exiting, so Kubernetes Job evictions or CI cancellations don't corrupt run state.
- The SQLite state backend must use WAL mode so `status`/`report` can read the database concurrently while `upgrade`/`resume` holds it open.

# Upgrade sequence

## Phase 1: Validate

- Parse and validate both YAML files.
- Verify `apiVersion` and `kind`.
- Verify target Kubernetes version is exactly current version plus one minor.
- Verify all managed node groups are compatible with the target version.
- Verify node-group order contains no duplicates and is a set-equal match against `managedNodeGroups` in `cluster.yaml` (fail if any node group is missing from `order` or `order` references a node group that doesn't exist).
- Verify the installed `eksctl` version supports `targetKubernetesVersion`.
- Verify Karpenter manifests parse successfully if enabled.
- Verify all referenced files exist.
- Verify secrets are supplied through environment variables, not YAML.
- Acquire an upgrade lock for the cluster.
- Emit `VALIDATION_COMPLETED`.

## Phase 2: State reconciliation

- Pull live state.
- Generate observed configuration.
- Diff observed state against Git desired state.
- Emit drift findings.
- If blocking drift is found, pause or abort based on `fallback.onStateDrift`.
- Persist the reconciliation report.

## Phase 3: Readiness analysis

- Run EKS Upgrade Insights.
- Run deprecated API scans.
- Run compatibility checks.
- Run capacity and PDB checks.
- Generate readiness report.
- Print blocking issues and warnings.
- Require acknowledgement for warnings.
- Abort if blocking findings exist.

## Phase 4: Preflight

Run read-only checks:

- Get current EKS cluster version and status.
- List managed node groups and their Kubernetes versions.
- Confirm all managed node groups are on the current control-plane minor version.
- Confirm all nodes are `Ready`.
- Confirm no unexpected pending pods.
- Validate PDBs for critical namespaces.
- Check subnet available IP capacity.
- Check CloudWatch API server error metrics.
- If Karpenter is enabled:
  - Verify Karpenter controller health.
  - Verify all `NodePool` and `EC2NodeClass` objects are healthy.
  - Verify no unexpected drifted NodeClaims.
  - Verify capacity headroom exists for replacement nodes.
  - Verify disruption budgets permit the planned rollout.

If any blocking check fails, follow `fallback.onPreflightFailure`.

## Phase 5: Approval

If `controlPlane.approval.required` is true:

- Pause before mutating the control plane.
- Emit a pending-approval event.
- Wait for `eks-upgrader approve` or an external approval callback.
- Record approver identity, timestamp, comment, and approval token.
- Enforce approval timeout.

## Phase 6: Control-plane upgrade

Run:

```bash
eksctl upgrade cluster \
  --config-file cluster.yaml \
  --approve
```

Requirements:

- Use a context timeout from `controlPlane.timeout`.
- Stream eksctl output into structured logs.
- Poll EKS until cluster status is `ACTIVE`.
- Verify API server health.
- Verify nodes remain Ready.
- Persist checkpoint.
- On failure, pause and do not proceed to add-ons or node groups.

## Phase 7: Add-on upgrade

Upgrade EKS networking add-ons after control-plane success.

Upgrade one add-on at a time, in this order (verify against current AWS guidance for the specific source/target version pair before relying on it, since recommended sequencing has changed across EKS releases):

- VPC CNI
- CoreDNS
- kube-proxy

Requirements:

- Wait for each add-on to become healthy.
- Verify DNS resolution and CNI functionality after each upgrade.
- Persist checkpoint.
- On failure, pause.

## Phase 8: Karpenter upgrade

If `spec.karpenter.enabled` is true:

1. Verify Karpenter compatibility with target Kubernetes version.
2. Upgrade Karpenter controller and CRDs using the configured Helm strategy.
3. Wait for Karpenter deployment readiness.
4. Update `EC2NodeClass` AMI selector or alias.
5. Temporarily constrain consolidation if `pauseConsolidationDuringUpgrade` is true.
6. Apply conservative drift disruption budgets.
7. Observe drifted `NodeClaim` objects.
8. Ensure replacement nodes become Ready.
9. Validate workloads after each replacement wave.
10. Respect `maxConcurrentDriftedNodes`.
11. Persist checkpoint.

Important Karpenter behavior:

- Karpenter detects drift when a `NodeClaim` no longer matches its owning `NodePool` or `EC2NodeClass`.
- Drifted nodes are disrupted through Karpenter’s termination controller, which taints and drains nodes and respects PDBs and termination grace periods.
- NodePool disruption budgets rate-limit voluntary disruption from drift, emptiness, consolidation, and node repair.
- A disruption budget of `nodes: "0"` blocks voluntary disruption for matching nodes.
- Do not use expiration as a fallback mechanism; expiration can terminate nodes without pre-spinning replacements.
- For static Karpenter NodePools, ensure `limits.nodes` has headroom above `replicas` so drifted nodes can be replaced.

## Phase 9: Managed node-group upgrade

Upgrade managed node groups strictly sequentially in `spec.nodeGroups.order`.

For each node group:

1. Confirm the previous node group passed health gates.
2. Run node-group-specific preflight.
3. Optionally pause Cluster Autoscaler.
4. Run:

```bash
eksctl upgrade nodegroup \
  --cluster <cluster> \
  --region <region> \
  --name <nodegroup> \
  --kubernetes-version <target-version> \
  --wait
```

5. If the node group uses a launch template and the plan specifies one, run:

```bash
eksctl upgrade nodegroup \
  --cluster <cluster> \
  --region <region> \
  --name <nodegroup> \
  --launch-template-version <version> \
  --kubernetes-version <target-version> \
  --wait
```

6. Wait for completion.
7. Verify all nodes are Ready and running the target Kubernetes version.
8. Verify pods are Ready.
9. Verify PDBs were respected.
10. Run canary checks.
11. Persist checkpoint.
12. Wait `waitBetweenGroups`.
13. Move to the next node group.

Never upgrade multiple managed node groups concurrently unless `maxConcurrentNodeGroups` is explicitly greater than one and every node group passes preflight.

## Phase 10: Post-upgrade checks

Run:

- All nodes Ready.
- No unexpected pending pods.
- Critical Deployments and StatefulSets healthy.
- PDBs healthy.
- Ingress and Services reachable where configured.
- DNS checks.
- PVC-bound pods healthy.
- Smoke tests if enabled.
- Canary checks.
- CloudWatch health checks.

## Phase 11: Config reconciliation

Do not directly modify `cluster.yaml`.

Generate:

```text
output/cluster.patch
output/karpenter.patch
output/upgrade-report.md
output/upgrade-report.json
output/eksctl-commands.json
output/next-actions.md
```

The patch must propose updates to:

- `metadata.version` in `cluster.yaml`
- Managed node-group version references, if present
- Launch-template version references, if applicable
- Karpenter `EC2NodeClass` AMI alias/selector
- Karpenter controller chart version
- Any documented compatibility versions

Then print:

```text
Upgrade complete.
Review and commit the generated configuration patch:
  git apply output/cluster.patch
  git apply output/karpenter.patch
```

# Operational safeguards

Implement all of these:

| Safeguard | Requirement |
|---|---|
| One minor version | Reject skipping Kubernetes versions |
| Live-state reconciliation | Compare Git desired state with actual cluster state |
| Drift protection | Block on critical version, capacity, AMI, or disruption-budget drift |
| Dry-run | Show exact eksctl commands and checks without mutating |
| Approval gate | Required before control-plane mutation when configured |
| Distributed lock | Prevent concurrent upgrades for the same cluster |
| Idempotency | Safe rerun and resume from last checkpoint |
| Timeout | Every eksctl command and phase has a deadline |
| Audit trail | Immutable append-only JSON audit events |
| Node readiness | Block progression if nodes are NotReady |
| Pod readiness | Block progression if critical pods are not Ready |
| PDB enforcement | Block rollout if PDBs would be violated |
| Capacity guard | Check node capacity, pending pods, and subnet IPs |
| Blast-radius control | One node group at a time by default |
| Karpenter drift control | Use disruption budgets and concurrency limits |
| Canary analysis | Block progression if SLO checks fail |
| Deprecated API scan | Block on APIs removed in target version |
| Compatibility scan | Block on incompatible Karpenter or add-on versions |
| Notification | Notify on start, phase completion, pause, failure, approval request, and completion |
| Secret safety | No secrets in YAML or logs |
| Correlation ID | Attach a run ID to every log and audit event |
| Structured output | JSON and Markdown reports |
| Graceful interruption | Trap SIGINT/SIGTERM and checkpoint safely instead of corrupting state |
| Resume safety | Re-verify live state for the in-flight phase before retrying a mutating command |
| Resume circuit breaker | Auto-pause after repeated consecutive resume failures on the same phase |
| Tooling compatibility | Verify installed `eksctl` version supports the target Kubernetes version before any mutation |
| State-store concurrency | SQLite in WAL mode so read commands don't block the active run |

# Fallback and rollback

## Managed node groups

- On node-group failure, pause and preserve the failed node group.
- Do not automatically delete the old node group.
- Support blue/green node groups:
  - Create a new node group with suffix `-v<targetMinor>`.
  - Validate the new node group.
  - Drain workloads from the old node group.
  - Delete the old node group only after a *separate* explicit approval event (distinct from the control-plane approval) and successful validation — deletion is destructive and must never ride on an earlier approval.
- If a new node group fails, retain the old node group and mark the run paused.

## Karpenter

- On AMI or drift failure, revert the `EC2NodeClass` AMI selector/alias in Git and allow Karpenter to drift nodes back on the next replacement.
- Set the NodePool disruption budget to `nodes: "0"` for `Drifted` to stop further voluntary drift disruption.
- Do not force-delete Karpenter nodes.
- Inspect and report `NodeClaim`, `Node`, Karpenter events, and controller logs.
- If capacity replacement fails, recommend increasing NodePool limits or temporarily disabling consolidation.

## Control plane

- There is no reliable in-place Kubernetes version rollback.
- On control-plane failure, pause immediately.
- Do not upgrade add-ons or node groups.
- Provide a remediation report with EKS update status, eksctl output, CloudWatch signals, and recommended next steps.

# Observability

Emit structured logs with:

```json
{
  "time": "2026-10-31T10:35:00Z",
  "level": "INFO",
  "runID": "upgrade-abc123",
  "cluster": "prod-cluster",
  "region": "ap-south-1",
  "phase": "NODE_GROUP_UPGRADE",
  "nodeGroup": "app-a",
  "message": "starting managed node group upgrade",
  "eksctlCommand": ["eksctl", "upgrade", "nodegroup", "..."]
}
```

Emit audit events for:

- Run started
- Validation completed
- State reconciliation completed
- Drift detected
- Readiness analysis completed
- Blocking finding found
- Warning acknowledged
- Approval requested
- Approval granted
- eksctl command started
- eksctl command completed
- Phase completed
- Health gate passed/failed
- Canary passed/failed
- Pause/resume/abort
- Run completed

# Testing requirements

Implement:

- Unit tests for YAML loading, validation, version checks, state transitions, and command construction.
- Table-driven tests for invalid configurations.
- Mock eksctl executor tests.
- Fake Kubernetes client tests.
- Integration tests using mocked AWS/eksctl environments.
- End-to-end tests using a local kind cluster or mocked Karpenter objects where feasible.
- Tests for live-state collector and drift detection.
- Tests for readiness analyzer and deprecated API classification.
- Tests for report and patch generation.
- Tests for SIGINT/SIGTERM handling and checkpoint-on-interrupt.
- Tests for resume re-verifying live state before retrying a mutating command.
- Tests for the resume circuit breaker (auto-pause after max consecutive failures).
- Tests for forced lock release and operator-initiated pause.

Test these failure paths:

- Wrong current version
- Skipping minor version
- Node group not matching control plane
- Missing PDB
- Insufficient subnet IPs
- eksctl command timeout
- eksctl nonzero exit
- Canary failure
- Resume after simulated crash
- Concurrent upgrade lock conflict
- Karpenter drift blocked by disruption budget
- Deprecated API removed in target version
- Karpenter controller incompatible with target version
- eksctl version incompatible with target Kubernetes version
- Live state drift in cluster version
- Live state drift in node-group capacity
- Live state drift in Karpenter AMI selector
- Resume re-verifying a phase whose mutating command already succeeded out-of-band
- Interrupt signal received mid-phase (graceful checkpoint, not corruption)

Minimum test coverage target: 80% for:

- `internal/config`
- `internal/upgrade`
- `internal/state`
- `internal/eksctl`
- `internal/reconcile`
- `internal/readiness`

# Documentation

Create:

1. `README.md`
   - What the app does
   - Installation
   - Quickstart
   - YAML reference
   - CLI reference
   - Security model
   - Limitations

2. `docs/architecture.md`
   - Component diagram
   - State machine
   - Config ownership
   - eksctl boundary
   - Karpenter integration
   - State reconciliation design
   - Readiness analyzer design

3. `docs/upgrade-runbook.md`
   - Pre-upgrade checklist
   - Execution steps
   - Monitoring steps
   - Pause/resume instructions
   - Post-upgrade checklist

4. `docs/fallback-runbook.md`
   - Control-plane failure
   - Add-on failure
   - Managed-node-group failure
   - Karpenter failure
   - Blue/green node-group recovery
   - Karpenter AMI rollback

5. `docs/yaml-reference.md`
   - Every field in `upgrade.yaml`
   - Defaults
   - Required fields
   - Examples

6. `docs/readiness-checks.md`
   - EKS Upgrade Insights
   - Deprecated API checks
   - Pluto and kubent integration
   - Compatibility matrix approach
   - Severity policy

7. `docs/iam-policy.md`
   - Minimum read-only IAM policy for `eks-upgrader` itself (EKS/EC2/IAM/CloudWatch describe + `eks:ListInsights`)
   - Explicit boundary between this policy and eksctl's own (broader, mutating) IAM requirements

# Deliverables

Implement a working MVP with:

- Complete Go project structure
- Compiling CLI
- YAML parsing and validation
- eksctl executor
- State machine
- SQLite state store
- Live-state collector
- State diff and drift report
- Upgrade-readiness analyzer
- Deprecated API detection
- Compatibility checks
- Managed-node-group upgrade flow
- Karpenter drift-aware flow
- Health gates
- Canary framework
- Slack/webhook notifications
- Audit logging
- Dry-run mode
- Resume and abort
- Report and config-patch generation
- Dockerfile
- Makefile
- GitHub Actions CI
- Example YAML files
- Full documentation

Use clean architecture, dependency injection, interfaces for all external systems, and small focused packages. Avoid global mutable state. Make every external dependency mockable.

Before writing application code, create:

1. `docs/architecture.md`
2. `docs/yaml-reference.md`
3. `docs/readiness-checks.md`
4. `configs/examples/upgrade.yaml`
5. `configs/examples/cluster.yaml`

Then implement the application incrementally in this order:

1. Config loading and validation
2. CLI skeleton
3. State store
4. eksctl executor
5. Live-state collector
6. State diff and drift detection
7. Readiness analyzer
8. Read-only preflight checks
9. Control-plane upgrade
10. Add-on upgrade
11. Managed-node-group upgrade
12. Karpenter integration
13. Canary and post-checks
14. Reports and config-patch generation
15. Notifications, audit, locking
16. Tests and CI
17. Documentation

Ask me before making assumptions about:

- Production cluster names or AWS account IDs
- Exact Karpenter version
- Prometheus query semantics
- Approval workflow integration
- Whether managed node groups use custom launch templates
- Whether EKS Auto Mode is used
- Whether Pluto or kubent should be embedded as binaries or invoked as external CLI tools
- Whether the state backend should remain SQLite or use DynamoDB in production
- Whether `resume` should require re-approval if the paused phase occurred after `APPROVAL`, or whether the original approval still counts
- Whether the lock TTL should only passively expire or also trigger an alert/notification when a lock outlives its TTL while no run is actually active
- Whether blue/green node-group deletion requires a separate named approver group from control-plane approval, or the same approvers list