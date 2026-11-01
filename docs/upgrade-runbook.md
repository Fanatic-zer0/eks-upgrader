# Upgrade Runbook

## Pre-upgrade checklist

1. Confirm `cluster.yaml` and `upgrade.yaml` are committed and reviewed; `upgrade.yaml`
   `spec.targetKubernetesVersion` must be exactly one minor version ahead of
   `cluster.yaml`'s `metadata.version`.
2. Confirm `spec.nodeGroups.order` lists every managed node group in `cluster.yaml`
   exactly once — `eks-upgrader validate` will reject anything else.
3. Export any secrets referenced by `*Env` fields (`SLACK_WEBHOOK_URL`,
   `PAGERDUTY_ROUTING_KEY`, `PROMETHEUS_TOKEN`, etc.) in the environment that will run
   `eks-upgrader`; never place them in YAML.
4. Ensure AWS credentials (IAM role, IRSA, or CI OIDC) have at least the read-only
   permissions in [iam-policy.md](iam-policy.md), and that `eksctl`/`helm`/`kubectl`
   are installed and authenticated against the target cluster.
5. Run:
   ```bash
   eks-upgrader validate --plan upgrade.yaml --cluster-config cluster.yaml --use-live-state
   eks-upgrader readiness --plan upgrade.yaml --cluster-config cluster.yaml --target-version <target> --output ./output
   eks-upgrader upgrade --plan upgrade.yaml --cluster-config cluster.yaml --dry-run
   ```
   Resolve every `BLOCKING` finding before continuing. Acknowledge and understand every
   `WARNING`.

## Execution steps

1. `eks-upgrader upgrade --plan upgrade.yaml --cluster-config cluster.yaml`
2. The run acquires the cluster lock, then proceeds through `VALIDATE` ->
   `STATE_RECONCILIATION` -> `READINESS_ANALYSIS` -> `PREFLIGHT` automatically.
3. If `spec.controlPlane.approval.required: true`, the run pauses at `APPROVAL` and
   emits a notification. Approve it from another terminal/session:
   ```bash
   eks-upgrader approve --state ./state/upgrade.db --approver "<you>" --comment "approved for maintenance window"
   eks-upgrader resume --plan upgrade.yaml --state ./state/upgrade.db
   ```
4. The run then proceeds through `CONTROL_PLANE_UPGRADE` -> `ADDON_UPGRADE` ->
   `KARPENTER_UPGRADE` (if enabled) -> `NODE_GROUP_UPGRADE` (strictly sequential, one
   node group at a time by default) -> `POST_UPGRADE_CHECK` -> `CONFIG_RECONCILIATION`.

## Monitoring steps

- Tail structured logs (`--log-format=json`) for `phase`, `eksctlCommand`, and
  `nodeGroup` fields.
- `eks-upgrader status --state ./state/upgrade.db` at any time shows the current
  phase, completed phases, node-group progress, approvals, and the recommended next
  action.
- The audit log (`./state/audit.jsonl` by default) is an append-only JSON-lines trail
  of every phase/approval/eksctl-command/pause/resume event, suitable for ingestion
  into a SIEM.

## Pause/resume instructions

- **Operator-initiated pause** (no failure has occurred):
  ```bash
  eks-upgrader pause --state ./state/upgrade.db --reason "investigating a dependency"
  ```
- **Resume** (after a pause, an interruption, or a transient failure):
  ```bash
  eks-upgrader resume --plan upgrade.yaml --state ./state/upgrade.db
  ```
  `resume` re-verifies live state for the in-flight phase before retrying any mutating
  command. After 3 consecutive failed resume attempts on the same phase, it refuses to
  continue automatically; pass `--force` only after you have manually investigated the
  root cause.
- **Stuck lock with no active process** (e.g. the process was killed without a chance
  to release its lock):
  ```bash
  eks-upgrader lock release --cluster <cluster> --region <region> --confirm
  ```
  Only do this after confirming no `eks-upgrader upgrade`/`resume` process is actually
  running against the cluster.

## Post-upgrade checklist

1. `eks-upgrader status --state ./state/upgrade.db` shows `Status: COMPLETED`.
2. Review `output/upgrade-report.md`, `output/upgrade-readiness-report.md`, and
   `output/drift-report.md`.
3. Review and apply the generated config patch:
   ```bash
   git apply output/cluster.patch
   git apply output/karpenter.patch   # if Karpenter is enabled and AMI drift occurred
   ```
4. Open a pull request with the patched `cluster.yaml`/Karpenter manifests — a human
   must review and merge it; `eks-upgrader` never commits on its own.
5. If anything failed or paused along the way, see
   [fallback-runbook.md](fallback-runbook.md).
