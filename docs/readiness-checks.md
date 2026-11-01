# Readiness Checks

The Upgrade Readiness Analyzer (`internal/readiness`) aggregates independent signals
into a single `READY` / `READY_WITH_WARNINGS` / `NOT_READY` verdict before the
`APPROVAL` phase gates the control-plane mutation.

## EKS Upgrade Insights

`internal/aws.EKSClient.ListInsights` calls the real `ListInsights`/`DescribeInsight`
EKS APIs (equivalent to `aws eks list-insights`). `readiness.ClassifyInsights` applies:

| Insight status | Default severity | Policy override |
|---|---|---|
| `PASSING` | `SAFE` | — |
| `ERROR` | `BLOCKING` | `preflight.eksUpgradeInsights.failOnError: false` downgrades to `WARNING` |
| `WARNING` | `WARNING` | always requires acknowledgement before `upgrade` proceeds past `APPROVAL` |
| `UNKNOWN` / anything else | `WARNING` | no override currently modeled |

Disabled entirely via `preflight.eksUpgradeInsights.enabled: false`.

## Deprecated API checks

Three independent signals feed `readiness.ClassifyDeprecatedAPIs`:

1. **apiserver metrics scrape** (`internal/readiness/scanners/metrics.go`) — parses the
   `apiserver_requested_deprecated_apis` gauge directly from `/metrics` (the same
   mechanism as `kubectl get --raw /metrics`). This is the strongest signal: it only
   fires for APIs a **live client has actually called**, with a `removed_release`
   label when known.
2. **Pluto** (`internal/readiness/scanners/pluto.go`) — invoked as an external CLI
   (`pluto detect-files <path> -o json --target-versions k8s=v<target>`). If the
   `pluto` binary is not on `$PATH`, this signal is silently skipped (not a hard
   failure) — readiness still proceeds on the metrics-scrape and EKS Insights signals.
3. **kubent** (`internal/readiness/scanners/kubent.go`) — invoked as `kubent -o json`
   against the live cluster/Helm releases, with the same skip-if-missing behavior.

Both external tools are invoked via subprocess rather than embedded as Go libraries
(see the open question in the original build plan); `internal/readiness/scanners.
ExternalToolRunner` is the seam for swapping either approach later.

### Severity policy

| Condition | Severity |
|---|---|
| API removed in the **exact** target Kubernetes version | `BLOCKING` (downgradable via `preflight.deprecatedAPIs.failOnRemovedInTargetVersion: false`) |
| API removed in some **later** (non-target) version | `WARNING` |
| API merely deprecated, not removed | `WARNING`, only emitted if `preflight.deprecatedAPIs.warnOnDeprecatedBeforeRemoval: true` |

If `preflight.deprecatedAPIs.failOnFound`, `warnOnDeprecatedBeforeRemoval`, and
`failOnRemovedInTargetVersion` are all false, deprecated-API scanning is skipped
entirely.

## Compatibility matrix approach

`internal/readiness/compatibility.go` deliberately avoids fabricating exact version
matrices it cannot verify:

- **Karpenter**: compared against a small, explicitly-maintained minimum-version table
  (`karpenterMinVersionForK8s`) that must be updated as new Kubernetes/Karpenter
  releases ship. An unrecognized target Kubernetes version produces a `WARNING`
  ("manually verify"), not a fabricated pass/fail.
- **EKS add-ons**: always emits a `WARNING` reminder to check the AWS add-on
  compatibility documentation. EKS itself enforces compatibility at `eksctl update
  addon` time, so this is a proactive reminder rather than the sole gate.

## Severity policy summary

| Source | Blocking condition |
|---|---|
| EKS Upgrade Insights | `ERROR` status for the target version |
| Deprecated API scan | API removed in the exact target version |
| Karpenter compatibility | installed controller version below the known minimum for the target version |
| Node readiness | any `NotReady` node |
| Pending pods | pending count exceeds the configured maximum (default 0) |
| PDBs | any PDB in a critical namespace currently allows 0 disruptions |

Everything else (EKS add-on compatibility reminders, unknown-version Karpenter checks,
merely-deprecated APIs, `WARNING`-level insights) is a `WARNING` requiring
acknowledgement, not an automatic abort.
