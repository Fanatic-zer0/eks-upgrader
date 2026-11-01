package k8s

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func int32ptr(v int32) *int32 { return &v }

func TestListNodes_AndAllNodesReady(t *testing.T) {
	cs := fake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				NodeInfo:   corev1.NodeSystemInfo{KubeletVersion: "v1.31.0"},
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
			Status: corev1.NodeStatus{
				NodeInfo:   corev1.NodeSystemInfo{KubeletVersion: "v1.30.0"},
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}},
			},
		},
	)
	c := &Client{Clientset: cs}

	nodes, err := c.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}

	ok, notReady, err := c.AllNodesReady(context.Background())
	if err != nil {
		t.Fatalf("AllNodesReady: %v", err)
	}
	if ok {
		t.Fatal("expected AllNodesReady to be false")
	}
	if len(notReady) != 1 || notReady[0] != "node-2" {
		t.Fatalf("unexpected not-ready list: %v", notReady)
	}
}

func TestPendingPods(t *testing.T) {
	cs := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "pod-a"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "pod-b"},
			Status:     corev1.PodStatus{Phase: corev1.PodPending},
		},
	)
	c := &Client{Clientset: cs}

	pending, err := c.PendingPods(context.Background())
	if err != nil {
		t.Fatalf("PendingPods: %v", err)
	}
	if len(pending) != 1 || pending[0].Name != "pod-b" {
		t.Fatalf("unexpected pending pods: %+v", pending)
	}
}

func TestValidatePDBsForNamespaces_DetectsZeroDisruptionsAllowed(t *testing.T) {
	cs := fake.NewSimpleClientset(
		&policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{Namespace: "checkout", Name: "checkout-pdb"},
			Status:     policyv1.PodDisruptionBudgetStatus{DisruptionsAllowed: 0, DesiredHealthy: 3, CurrentHealthy: 2},
		},
		&policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "payments-pdb"},
			Status:     policyv1.PodDisruptionBudgetStatus{DisruptionsAllowed: 1, DesiredHealthy: 3, CurrentHealthy: 3},
		},
	)
	c := &Client{Clientset: cs}

	issues, err := c.ValidatePDBsForNamespaces(context.Background(), []string{"checkout", "payments"})
	if err != nil {
		t.Fatalf("ValidatePDBsForNamespaces: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected exactly 1 PDB issue, got %d: %v", len(issues), issues)
	}
}

func TestWorkloadHealth(t *testing.T) {
	cs := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "coredns"},
			Spec:       appsv1.DeploymentSpec{Replicas: int32ptr(2)},
			Status:     appsv1.DeploymentStatus{ReadyReplicas: 1},
		},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "kube-proxy"},
			Status:     appsv1.DaemonSetStatus{DesiredNumberScheduled: 3, NumberReady: 3},
		},
	)
	c := &Client{Clientset: cs}

	deployments, err := c.ListDeployments(context.Background(), "kube-system")
	if err != nil {
		t.Fatalf("ListDeployments: %v", err)
	}
	if len(deployments) != 1 || deployments[0].Healthy {
		t.Fatalf("expected coredns to be unhealthy (1/2 ready), got %+v", deployments)
	}

	health, err := c.AddonWorkloadHealth(context.Background(), "kube-proxy")
	if err != nil {
		t.Fatalf("AddonWorkloadHealth: %v", err)
	}
	if !health.Healthy {
		t.Fatalf("expected kube-proxy daemonset to be healthy, got %+v", health)
	}
}
