package karpenter

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func newFakeDynamicClient(objs ...runtime.Object) *Client {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		NodePoolGVR:     "NodePoolList",
		NodeClaimGVR:    "NodeClaimList",
		EC2NodeClassGVR: "EC2NodeClassList",
	}
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objs...)
	return NewClient(dyn)
}

func unstructuredNodePool(name string, ready bool, limits map[string]interface{}) *unstructured.Unstructured {
	status := "False"
	if ready {
		status = "True"
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "karpenter.sh/v1",
		"kind":       "NodePool",
		"metadata":   map[string]interface{}{"name": name},
		"spec":       map[string]interface{}{"limits": limits},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Ready", "status": status},
			},
		},
	}}
}

func unstructuredNodeClaim(name, nodePool string, drifted bool) *unstructured.Unstructured {
	driftedStatus := "False"
	if drifted {
		driftedStatus = "True"
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "karpenter.sh/v1",
		"kind":       "NodeClaim",
		"metadata": map[string]interface{}{
			"name":   name,
			"labels": map[string]interface{}{"karpenter.sh/nodepool": nodePool},
		},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Drifted", "status": driftedStatus},
				map[string]interface{}{"type": "Ready", "status": "True"},
			},
		},
	}}
}

func TestListNodePools(t *testing.T) {
	c := newFakeDynamicClient(
		unstructuredNodePool("general-purpose", true, map[string]interface{}{"cpu": "1000"}),
	)
	pools, err := c.ListNodePools(context.Background())
	if err != nil {
		t.Fatalf("ListNodePools: %v", err)
	}
	if len(pools) != 1 || !pools[0].Healthy || pools[0].Limits["cpu"] != "1000" {
		t.Fatalf("unexpected node pools: %+v", pools)
	}
}

func TestListEC2NodeClasses(t *testing.T) {
	nc := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "karpenter.k8s.aws/v1",
		"kind":       "EC2NodeClass",
		"metadata":   map[string]interface{}{"name": "default"},
		"spec": map[string]interface{}{
			"amiSelectorTerms": []interface{}{
				map[string]interface{}{"alias": "al2023@v20250918"},
			},
		},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Ready", "status": "True"},
			},
		},
	}}
	c := newFakeDynamicClient(nc)

	classes, err := c.ListEC2NodeClasses(context.Background())
	if err != nil {
		t.Fatalf("ListEC2NodeClasses: %v", err)
	}
	if len(classes) != 1 || classes[0].AMIAlias != "al2023@v20250918" || !classes[0].Healthy {
		t.Fatalf("unexpected EC2NodeClasses: %+v", classes)
	}
}

func TestDriftedNodeClaims(t *testing.T) {
	c := newFakeDynamicClient(
		unstructuredNodeClaim("claim-1", "general-purpose", true),
		unstructuredNodeClaim("claim-2", "general-purpose", false),
	)

	drifted, err := c.DriftedNodeClaims(context.Background())
	if err != nil {
		t.Fatalf("DriftedNodeClaims: %v", err)
	}
	if len(drifted) != 1 || drifted[0].Name != "claim-1" {
		t.Fatalf("unexpected drifted node claims: %+v", drifted)
	}
}
