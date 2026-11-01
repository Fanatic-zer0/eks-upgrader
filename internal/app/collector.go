package app

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"k8s.io/client-go/dynamic"

	internalaws "eks-upgrader/internal/aws"
	"eks-upgrader/internal/k8s"
	"eks-upgrader/internal/karpenter"
	"eks-upgrader/internal/reconcile"
)

// CollectorOptions configures a standalone live-state collector for the `state pull`
// and `state diff` commands, which operate on just --cluster/--region/--output and
// deliberately do not require upgrade.yaml.
type CollectorOptions struct {
	Cluster        string
	Region         string
	KubeconfigPath string
}

// NewCollector builds a reconcile.Collector directly from AWS/kubeconfig credentials,
// without loading any eks-upgrader YAML config. Kubernetes/Karpenter access is
// best-effort: a missing/unreachable kubeconfig only disables those sources rather
// than failing collection outright (EKS/EC2/IAM data is still gathered).
func NewCollector(opts CollectorOptions) (*reconcile.Collector, error) {
	if opts.Cluster == "" {
		return nil, fmt.Errorf("--cluster is required")
	}
	if opts.Region == "" {
		return nil, fmt.Errorf("--region is required")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(opts.Region))
	if err != nil {
		return nil, fmt.Errorf("loading AWS SDK default config: %w", err)
	}
	eksClient := internalaws.NewEKSClient(eks.NewFromConfig(awsCfg))

	var k8sReader reconcile.NodeGroupNodesReader
	var karpReader reconcile.KarpenterReader
	if k8sClient, err := k8s.NewClientFromKubeconfig(opts.KubeconfigPath); err == nil {
		k8sReader = k8sClient
		if restCfg, err := k8s.RESTConfigFromKubeconfig(opts.KubeconfigPath); err == nil {
			if dynClient, err := dynamic.NewForConfig(restCfg); err == nil {
				karpReader = karpenter.NewClient(dynClient)
			}
		}
	}

	return &reconcile.Collector{EKS: eksClient, K8s: k8sReader, Karpenter: karpReader}, nil
}
