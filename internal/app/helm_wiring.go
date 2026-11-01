package app

import (
	"time"

	"eks-upgrader/internal/config"
	"eks-upgrader/internal/helm"
)

// helmUpgradeOptionsFromPlan maps spec.karpenter.controller into a helm.UpgradeOptions.
func helmUpgradeOptionsFromPlan(k config.KarpenterSpec, timeout time.Duration) helm.UpgradeOptions {
	return helm.UpgradeOptions{
		ReleaseName:     "karpenter",
		Namespace:       k.Namespace,
		ChartRepository: k.Controller.ChartRepository,
		ChartName:       k.Controller.ChartName,
		ChartVersion:    k.Controller.ChartVersion,
		Wait:            true,
		Timeout:         timeout,
	}
}
