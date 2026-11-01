// Package karpenter provides read-only access to Karpenter CRDs (NodePool,
// EC2NodeClass, NodeClaim) via the dynamic client, plus pure disruption-budget
// evaluation logic. eks-upgrader never creates/deletes Karpenter-managed nodes
// directly; Karpenter's own controller owns node lifecycle based on drift.
package karpenter

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	NodePoolGVR     = schema.GroupVersionResource{Group: "karpenter.sh", Version: "v1", Resource: "nodepools"}
	NodeClaimGVR    = schema.GroupVersionResource{Group: "karpenter.sh", Version: "v1", Resource: "nodeclaims"}
	EC2NodeClassGVR = schema.GroupVersionResource{Group: "karpenter.k8s.aws", Version: "v1", Resource: "ec2nodeclasses"}
)

// Client wraps a dynamic.Interface so k8s.io/client-go/dynamic/fake can be substituted
// directly in tests.
type Client struct {
	Dynamic dynamic.Interface
}

func NewClient(dyn dynamic.Interface) *Client {
	return &Client{Dynamic: dyn}
}
