package upgrade

import (
	"fmt"
	"strconv"
	"strings"

	"eks-upgrader/internal/config"
)

// BlueGreenNodeGroupName derives the new node group's name from the base name, the
// configured suffix template (e.g. "-v{targetMinor}"), and the target Kubernetes
// version's minor component.
func BlueGreenNodeGroupName(baseName, suffixTemplate, targetKubernetesVersion string) (string, error) {
	minor, err := targetMinorComponent(targetKubernetesVersion)
	if err != nil {
		return "", err
	}
	suffix := strings.ReplaceAll(suffixTemplate, "{targetMinor}", minor)
	return baseName + suffix, nil
}

func targetMinorComponent(version string) (string, error) {
	parts := strings.SplitN(version, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("version %q is not in MAJOR.MINOR form", version)
	}
	if _, err := strconv.Atoi(parts[1]); err != nil {
		return "", fmt.Errorf("version %q has non-numeric minor component: %w", version, err)
	}
	return parts[1], nil
}

// KarpenterAMIRollbackPlan describes the remediation steps recommended when a Karpenter
// AMI/drift rollout fails: revert the Git-declared AMI selector and stop further
// voluntary drift disruption until the revert has propagated.
type KarpenterAMIRollbackPlan struct {
	NodeClassName     string
	RevertToAMIAlias  string
	RecommendedBudget config.DisruptionBudget
	Steps             []string
}

// BuildKarpenterAMIRollbackPlan generates the remediation plan for a failed AMI/drift
// rollout. It never force-deletes Karpenter nodes; recovery happens by reverting Git
// and letting Karpenter's own drift reconciliation replace nodes back.
func BuildKarpenterAMIRollbackPlan(nodeClassName, previousAMIAlias string) KarpenterAMIRollbackPlan {
	return KarpenterAMIRollbackPlan{
		NodeClassName:     nodeClassName,
		RevertToAMIAlias:  previousAMIAlias,
		RecommendedBudget: config.DisruptionBudget{Nodes: "0", Reasons: []string{"Drifted"}},
		Steps: []string{
			fmt.Sprintf("Revert EC2NodeClass %q amiSelectorTerms.alias to %q in Git and merge.", nodeClassName, previousAMIAlias),
			"Set the NodePool disruption budget to nodes: \"0\" for reason Drifted to halt further voluntary disruption.",
			"Do not force-delete existing Karpenter nodes; allow drift reconciliation to replace them on the next eligible window.",
			"Inspect NodeClaim status, Node conditions, Karpenter controller logs, and recent events before resuming.",
			"Once healthy, restore the normal disruption budget and resume the upgrade.",
		},
	}
}

// ControlPlaneRemediation summarizes the fixed guidance for a failed control-plane
// upgrade: there is no reliable in-place rollback, so the run must pause and add-ons /
// node groups must not proceed.
var ControlPlaneRemediationSteps = []string{
	"Do not proceed to add-on or node-group upgrades.",
	"Check EKS cluster update status via the AWS console or `aws eks describe-update`.",
	"Review the captured eksctl stdout/stderr for the failed `eksctl upgrade cluster` command.",
	"Check CloudWatch control-plane signals (API server health, recent errors) if control-plane logging is enabled.",
	"There is no reliable in-place Kubernetes version rollback; engage AWS support if the control plane is in a degraded state.",
}
