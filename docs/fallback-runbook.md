# Fallback Runbook

## Control-plane failure

There is no reliable in-place Kubernetes control-plane version rollback. On
`CONTROL_PLANE_UPGRADE` failure the run pauses immediately and **does not** proceed to
add-ons, Karpenter, or node groups.

1. Check EKS cluster update status: `aws eks describe-update --name <cluster> --update-id <id>` (see the captured eksctl output for the update ID) or the EKS console.
2. Review the failed `eksctl upgrade cluster` invocation: `eks-upgrader report --state ./state/upgrade.db --format json` includes full stdout/stderr for every eksctl command.
3. Check CloudWatch control-plane signals if control-plane logging is enabled.
4. There is no reliable in-place rollback; engage AWS support if the control plane is
   in a degraded state. Once resolved, `eks-upgrader resume`.

## Add-on failure

1. Inspect the failed add-on's status: `aws eks describe-addon --cluster-name <cluster> --addon-name <name>`.
2. Check the underlying workload (`kubectl -n kube-system get deploy/coredns`,
   `daemonset/kube-proxy`, `daemonset/aws-node`) for CrashLoopBackOff or scheduling
   issues.
3. Resolve the underlying issue, then `eks-upgrader resume` — `AlreadySucceeded` will
   detect add-ons that are now healthy and skip re-running them.

## Managed-node-group failure

- The failed node group is **preserved**, never automatically deleted.
- Inspect node/pod status: `kubectl get nodes -l eks.amazonaws.com/nodegroup=<name>`,
  recent events, and the captured `eksctl upgrade nodegroup` stdout/stderr in
  `eks-upgrader report`.
- Resolve the root cause (capacity, PDBs, taints/tolerations, AMI issues), then
  `eks-upgrader resume`.

### Blue/green node-group recovery

Blue/green is a documented manual procedure using
`internal/upgrade.BlueGreenNodeGroupName` (`<base>-v<targetMinor>`) as the naming
convention, intended for node groups where in-place upgrade risk is unacceptable:

1. Create a new node group with the derived name (e.g. `app-a-v31`) in `cluster.yaml`
   at the target Kubernetes version, as a separate `eksctl create nodegroup` /
   `eksctl upgrade cluster --config-file` application (outside the automated
   sequential loop).
2. Validate the new node group's health gates pass.
3. Drain workloads from the old node group.
4. Obtain a **separate** explicit approval (distinct from the control-plane approval)
   before deleting the old node group — deletion is destructive and must never ride on
   an earlier approval.
5. Delete the old node group only after successful validation.
6. If the new node group fails, retain the old node group and leave the run paused.

## Karpenter failure

- **Do not force-delete Karpenter nodes.**
- On AMI/drift rollout failure, use
  `internal/upgrade.BuildKarpenterAMIRollbackPlan`'s guidance:
  1. Revert the `EC2NodeClass` `amiSelectorTerms.alias` to the previous value in Git
     and merge.
  2. Set the NodePool disruption budget to `nodes: "0"` for reason `Drifted` to halt
     further voluntary disruption.
  3. Allow Karpenter's own drift reconciliation to replace nodes back on the next
     eligible window — do not force-delete.
  4. Inspect `NodeClaim` status, `Node` conditions, Karpenter controller logs, and
     recent events before resuming.
  5. Once healthy, restore the normal disruption budget and `eks-upgrader resume`.
- If Karpenter controller Helm upgrade itself failed, check
  `helm status karpenter -n <namespace>` and `helm history karpenter -n <namespace>`
  before retrying.

## Karpenter AMI rollback

See step 1 above (Karpenter failure) — this is always a Git-side revert, never a
live-object patch from `eks-upgrader`, consistent with "Git is the source of truth."

## General

- `eks-upgrader status --state ./state/upgrade.db` always shows the current phase,
  pause/abort reason, and the recommended next action.
- `eks-upgrader abort --state ./state/upgrade.db --reason "<why>"` releases the lock,
  marks the run aborted, and sends a critical-severity notification — use this when a
  run must not be resumed (e.g. a decision to roll back manually instead).
