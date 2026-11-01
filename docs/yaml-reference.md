# upgrade.yaml Reference

`upgrade.yaml` is parsed into `config.UpgradePlan` (`internal/config/schema.go`) via
`gopkg.in/yaml.v3`. Duration-typed fields (`timeout`, `ttl`, `interval`, `duration`,
`waitTimeout`, etc.) accept any Go duration string (`"90m"`, `"4h"`, `"30s"`).

## Top level

| Field | Type | Required | Default | Notes |
|---|---|---|---|---|
| `apiVersion` | string | yes | — | must be `platform.io/v1` |
| `kind` | string | yes | — | must be `EKSUpgradePlan` |
| `metadata.name` | string | no | — | human-readable run name |
| `metadata.cluster` | string | yes | — | EKS cluster name |
| `metadata.region` | string | yes | — | AWS region |
| `metadata.runID` | string | no | `<cluster>-<targetKubernetesVersion>` | overrides the derived run ID |

## `spec`

| Field | Type | Required | Default | Notes |
|---|---|---|---|---|
| `spec.clusterConfig` | string | yes | — | path to `cluster.yaml`, resolved relative to `upgrade.yaml`'s directory if not absolute |
| `spec.targetKubernetesVersion` | string | yes | — | must be exactly one minor version ahead of `cluster.yaml`'s `metadata.version` |

### `spec.execution`

| Field | Default | Notes |
|---|---|---|
| `mode` | `sequential` | orchestration context only (`sequential`\|`interactive`); concurrency/rollout style is governed by `maxConcurrentNodeGroups` and `nodeGroups.strategy`, not this field |
| `dryRun` | `false` | mirrors `--dry-run`; the CLI flag takes precedence |
| `resumeOnFailure` | `true` | informational; `resume` is always available regardless |
| `maxConcurrentNodeGroups` | `1` | node groups beyond 1 require every group to pass preflight |
| `stateBackend` | `sqlite` | only `sqlite` is implemented |
| `stateFile` | — | path to the SQLite state DB |
| `lock.enabled` | `true` | |
| `lock.backend` | `local` | `local` (file-based, single host) or `dynamodb` (safe across CI runners; table `eks-upgrader-locks`, partition key `cluster`) |
| `lock.ttl` | — | lock TTL; an expired lock can be acquired by a new run or force-released via `lock release` |

### `spec.stateReconciliation`

| Field | Notes |
|---|---|
| `enabled` | runs the `STATE_RECONCILIATION` phase |
| `failOnDrift` | if true, any remaining `BLOCKING` finding fails the phase |
| `ignoredFields` | field patterns (supports a `[*]` wildcard segment) forced to `IN_SYNC` |
| `warnOnlyOn` | field patterns capped at `WARNING` even if otherwise `BLOCKING`/`UNMANAGED` |
| `blockOn` | field patterns forced to `BLOCKING` |

### `spec.preflight`

| Field | Notes |
|---|---|
| `requiredNodeGroupVersionsMatchControlPlane` | reserved for future stricter checking; node-group/control-plane version drift is already surfaced via reconciliation |
| `requireAllNodesReady` | blocks on any `NotReady` node |
| `requirePDBsForCriticalWorkloads` | blocks if any PDB in `criticalNamespaces` currently allows 0 disruptions |
| `criticalNamespaces` | namespaces checked for PDB health |
| `minAvailableIPsPerSubnet` / `requireSubnetCapacity` | reserved; `cluster.yaml` does not currently model subnet IDs, so this check is a no-op until subnet IDs are supplied out-of-band |
| `deprecatedAPIs.failOnFound` | gates whether deprecated-API findings are emitted at all |
| `deprecatedAPIs.failOnRemovedInTargetVersion` | if false, downgrades a target-version removal from `BLOCKING` to `WARNING` |
| `deprecatedAPIs.warnOnDeprecatedBeforeRemoval` | if false, suppresses findings for APIs deprecated but not yet removed |
| `eksUpgradeInsights.enabled` | runs `aws eks list-insights` classification |
| `eksUpgradeInsights.failOnError` | if false, downgrades `ERROR` insights from `BLOCKING` to `WARNING` |
| `cloudwatch.enabled` | requires control-plane logging or Container Insights; see [readiness-checks.md](readiness-checks.md) |

### `spec.controlPlane`

| Field | Notes |
|---|---|
| `timeout` | context timeout for `eksctl upgrade cluster` |
| `approval.required` | pauses before `CONTROL_PLANE_UPGRADE` until `eks-upgrader approve` is run |
| `approval.approvers` | informational allow-list; not currently enforced against the `--approver` value |
| `approval.approvalTimeout` | reserved; not currently auto-enforced as a timeout-triggered pause |
| `wait.interval` / `wait.timeout` | reserved for future polling-interval tuning |

### `spec.addons`

| Field | Notes |
|---|---|
| `upgrade` | gates the `ADDON_UPGRADE` phase |
| `names` | add-ons upgraded one at a time, in list order, via `eksctl update addon` |
| `strategy` | informational (`latestCompatible`); no explicit `--version` is passed, letting EKS resolve it |
| `timeout` | per add-on context timeout |

### `spec.karpenter`

| Field | Notes |
|---|---|
| `enabled` | gates `KARPENTER_UPGRADE` and all Karpenter-aware reconciliation/readiness checks |
| `namespace` | Karpenter controller namespace |
| `controller.upgradeStrategy` | only `helm` is implemented |
| `controller.chartRepository` / `chartName` / `chartVersion` | passed to `helm upgrade --install --repo ...` |
| `controller.waitTimeout` | helm `--timeout` |
| `nodeClasses[].name` / `manifestPath` / `amiStrategy` / `amiAlias` | desired AMI selector; drift against live `EC2NodeClass` objects is `BLOCKING` |
| `nodePools[].name` / `manifestPath` / `driftEnabled` / `maxUnavailablePercentage` / `maxConcurrentDriftedNodes` | drift/disruption planning inputs |
| `nodePools[].disruptionBudgets[].nodes` / `schedule` / `duration` / `reasons` | evaluated by `internal/karpenter/disruption.go`; `nodes: "0"` blocks voluntary disruption for the matching reasons |
| `safeguards.*` | informational; enforced qualitatively through the preflight/readiness capacity checks |

### `spec.nodeGroups`

| Field | Notes |
|---|---|
| `order` | **required to be a set-equal match** (no missing, no extra, no duplicates) against `cluster.yaml`'s `managedNodeGroups` — enforced in `VALIDATE` |
| `strategy` | `rolling`; concurrency is capped by `execution.maxConcurrentNodeGroups` |
| `maxUnavailablePercentage` | passed through to eksctl's own node-group update behavior |
| `perNodeGroupTimeout` | context timeout for each `eksctl upgrade nodegroup` |
| `drainTimeout` | informational; eksctl manages its own drain timeout |
| `waitBetweenGroups` | pause duration between successive node groups |
| `healthGates.*` | evaluated by `internal/upgrade.EvaluateHealthGates` after each node group |

### `spec.canary`

| Field | Notes |
|---|---|
| `enabled` | gates canary evaluation after each node group and in `POST_UPGRADE_CHECK` |
| `namespaces` | informational scope hint; queries themselves determine actual scope |
| `prometheusURL` | base URL; queried via `/api/v1/query` |
| `prometheusBearerTokenEnv` | env var name holding a bearer token (never put the token itself in YAML) |
| `prometheusTLSSkipVerify` | reserved; the default HTTP client does not currently skip verification |
| `comparisonMode` | `absolute` (implemented) or `baseline` (schema reserved for a future control-group comparison; currently falls back to absolute evaluation) |
| `checks[].name` / `query` / `threshold` / `comparison` (`lessThan`\|`greaterThan`) / `duration` | one instant Prometheus query per check |

### `spec.fallback`

| Field | Notes |
|---|---|
| `onPreflightFailure` / `onReadinessBlockingFailure` / `onStateDrift` / `onControlPlaneFailure` / `onAddonFailure` / `onKarpenterFailure` / `onNodeGroupFailure` / `onCanaryFailure` / `onPostCheckFailure` | `abort` or `pause`; `READINESS_ANALYSIS` blocking findings and `VALIDATE` failures always abort/fail regardless of this policy |
| `preserveFailedNodeGroup` | informational; the engine never deletes a node group automatically regardless of this flag |
| `blueGreenNodeGroups.enabled` / `suffix` | consumed by `internal/upgrade.BlueGreenNodeGroupName`; blue/green creation itself is a documented runbook procedure, not yet an automated phase |
| `notify.slackWebhookEnv` / `pagerdutyRoutingKeyEnv` / `webhookEnv` | environment variable **names**, not secrets — resolved at send time |

### `spec.postUpgrade`

| Field | Notes |
|---|---|
| `verifyNodes` / `verifyWorkloads` | covered by the shared post-upgrade health findings (node readiness, pending pods, PDBs) |
| `verifyIngress` / `verifyDNS` / `verifyPVCs` | reserved; not yet implemented as distinct checks |
| `runSmokeTests` / `smokeTestCommand` | reserved; not yet invoked by the engine |
| `generateConfigPatch` | gates `CONFIG_RECONCILIATION` patch writing |
| `outputDir` | default `./output`; all generated reports/patches are written here |

## Secrets

No field in this schema accepts a literal secret value. Fields ending in `Env` hold an
environment variable **name**; `eks-upgrader validate` scans the raw YAML for common
secret patterns (AWS access keys, Slack webhook URLs, Slack tokens, PEM private keys)
and fails closed if any are found.
