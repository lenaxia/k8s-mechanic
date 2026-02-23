package cascade

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

func TestNewChecker(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		wantErr bool
	}{
		{"enabled", true, false},
		{"disabled", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Enabled:                 tt.enabled,
				NamespaceFailurePercent: 50,
				NodeCacheTTL:            30 * time.Second,
			}
			checker, err := NewChecker(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewChecker() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if checker == nil {
				t.Error("NewChecker() returned nil checker")
			}
		})
	}
}

func TestChecker_ShouldSuppress_NodeFailure(t *testing.T) {
	scheme := newTestScheme()

	// Create a NotReady node
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
			UID:  types.UID("node-1-uid"),
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionFalse,
					Reason: "KubeletNotReady",
				},
			},
		},
	}

	// Create a pod on that node
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       types.UID("test-pod-uid"),
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodFailed,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "app",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 1,
						},
					},
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(node, pod).
		Build()

	cfg := Config{
		Enabled:                 true,
		NamespaceFailurePercent: 50,
		NodeCacheTTL:            30 * time.Second,
	}
	checker, err := NewChecker(cfg)
	if err != nil {
		t.Fatalf("NewChecker() error = %v", err)
	}

	errorsJSON, _ := json.Marshal([]struct {
		Text string `json:"text"`
	}{
		{Text: "container app: terminated with exit code 1"},
	})

	finding := &domain.Finding{
		Kind:         "Pod",
		Name:         "test-pod",
		Namespace:    "default",
		ParentObject: "test-pod",
		Errors:       string(errorsJSON),
		SourceRef: domain.SourceRef{
			APIVersion: "v1",
			Kind:       "Pod",
			Name:       "test-pod",
			Namespace:  "default",
		},
	}

	suppress, reason, err := checker.ShouldSuppress(context.Background(), finding, client)
	if err != nil {
		t.Fatalf("ShouldSuppress() error = %v", err)
	}
	if !suppress {
		t.Error("ShouldSuppress() = false, want true for pod on NotReady node")
	}
	if reason == "" {
		t.Error("ShouldSuppress() returned empty reason")
	}
}

func TestChecker_ShouldSuppress_NodeMemoryPressure(t *testing.T) {
	scheme := newTestScheme()

	// Create a node with MemoryPressure
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
			UID:  types.UID("node-1-uid"),
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeMemoryPressure,
					Status: corev1.ConditionTrue,
					Reason: "KubeletHasInsufficientMemory",
				},
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	// Create an OOMKilled pod on that node
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       types.UID("test-pod-uid"),
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodFailed,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "app",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 137,
							Reason:   "OOMKilled",
						},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 137,
							Reason:   "OOMKilled",
						},
					},
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(node, pod).
		Build()

	cfg := Config{
		Enabled:                 true,
		NamespaceFailurePercent: 50,
		NodeCacheTTL:            30 * time.Second,
	}
	checker, err := NewChecker(cfg)
	if err != nil {
		t.Fatalf("NewChecker() error = %v", err)
	}

	errorsJSON, _ := json.Marshal([]struct {
		Text string `json:"text"`
	}{
		{Text: "container app: CrashLoopBackOff (last exit: OOMKilled)"},
	})

	finding := &domain.Finding{
		Kind:         "Pod",
		Name:         "test-pod",
		Namespace:    "default",
		ParentObject: "test-pod",
		Errors:       string(errorsJSON),
		SourceRef: domain.SourceRef{
			APIVersion: "v1",
			Kind:       "Pod",
			Name:       "test-pod",
			Namespace:  "default",
		},
	}

	suppress, reason, err := checker.ShouldSuppress(context.Background(), finding, client)
	if err != nil {
		t.Fatalf("ShouldSuppress() error = %v", err)
	}
	if !suppress {
		t.Error("ShouldSuppress() = false, want true for OOMKilled pod on node with MemoryPressure")
	}
	if reason == "" {
		t.Error("ShouldSuppress() returned empty reason")
	}
}

func TestChecker_ShouldSuppress_NamespaceWideFailure(t *testing.T) {
	scheme := newTestScheme()

	// Create 10 pods in namespace "test-ns", 6 of them failing
	var objects []client.Object
	for i := 0; i < 10; i++ {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-" + string(rune('a'+i)),
				Namespace: "test-ns",
				UID:       types.UID("pod-" + string(rune('a'+i)) + "-uid"),
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		}
		if i < 6 {
			// First 6 pods are failing
			pod.Status.Phase = corev1.PodFailed
			pod.Status.ContainerStatuses = []corev1.ContainerStatus{
				{
					Name: "app",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 1,
						},
					},
				},
			}
		} else {
			// Last 4 pods are healthy
			pod.Status.ContainerStatuses = []corev1.ContainerStatus{
				{
					Name:  "app",
					Ready: true,
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			}
		}
		objects = append(objects, pod)
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		Build()

	cfg := Config{
		Enabled:                 true,
		NamespaceFailurePercent: 50,
		NodeCacheTTL:            30 * time.Second,
	}
	checker, err := NewChecker(cfg)
	if err != nil {
		t.Fatalf("NewChecker() error = %v", err)
	}

	errorsJSON, _ := json.Marshal([]struct {
		Text string `json:"text"`
	}{
		{Text: "container app: terminated with exit code 1"},
	})

	finding := &domain.Finding{
		Kind:         "Pod",
		Name:         "pod-a",
		Namespace:    "test-ns",
		ParentObject: "pod-a",
		Errors:       string(errorsJSON),
		SourceRef: domain.SourceRef{
			APIVersion: "v1",
			Kind:       "Pod",
			Name:       "pod-a",
			Namespace:  "test-ns",
		},
	}

	suppress, reason, err := checker.ShouldSuppress(context.Background(), finding, client)
	if err != nil {
		t.Fatalf("ShouldSuppress() error = %v", err)
	}
	if !suppress {
		t.Error("ShouldSuppress() = false, want true for namespace with >50% pods failing")
	}
	if reason == "" {
		t.Error("ShouldSuppress() returned empty reason")
	}
}

func TestChecker_ShouldSuppress_Disabled(t *testing.T) {
	cfg := Config{
		Enabled:                 false,
		NamespaceFailurePercent: 50,
		NodeCacheTTL:            30 * time.Second,
	}
	checker, err := NewChecker(cfg)
	if err != nil {
		t.Fatalf("NewChecker() error = %v", err)
	}

	finding := &domain.Finding{
		Kind:         "Pod",
		Name:         "test-pod",
		Namespace:    "default",
		ParentObject: "test-pod",
		Errors:       `[{"text":"container app: terminated with exit code 1"}]`,
	}

	suppress, reason, err := checker.ShouldSuppress(context.Background(), finding, nil)
	if err != nil {
		t.Fatalf("ShouldSuppress() error = %v", err)
	}
	if suppress {
		t.Error("ShouldSuppress() = true, want false when disabled")
	}
	if reason != "" {
		t.Errorf("ShouldSuppress() reason = %q, want empty when disabled", reason)
	}
}

func TestChecker_ShouldSuppress_NonPodFinding(t *testing.T) {
	scheme := newTestScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	cfg := Config{
		Enabled:                 true,
		NamespaceFailurePercent: 50,
		NodeCacheTTL:            30 * time.Second,
	}
	checker, err := NewChecker(cfg)
	if err != nil {
		t.Fatalf("NewChecker() error = %v", err)
	}

	finding := &domain.Finding{
		Kind:         "Node",
		Name:         "node-1",
		Namespace:    "",
		ParentObject: "node-1",
		Errors:       `[{"text":"node node-1 has condition NodeReady (KubeletNotReady)"}]`,
		SourceRef: domain.SourceRef{
			APIVersion: "v1",
			Kind:       "Node",
			Name:       "node-1",
			Namespace:  "",
		},
	}

	suppress, reason, err := checker.ShouldSuppress(context.Background(), finding, client)
	if err != nil {
		t.Fatalf("ShouldSuppress() error = %v", err)
	}
	if suppress {
		t.Error("ShouldSuppress() = true, want false for non-pod finding")
	}
	if reason != "" {
		t.Errorf("ShouldSuppress() reason = %q, want empty for non-pod finding", reason)
	}
}

func TestChecker_CachePerformance(t *testing.T) {
	scheme := newTestScheme()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
			UID:  types.UID("node-1-uid"),
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       types.UID("test-pod-uid"),
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodFailed,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(node, pod).
		Build()

	cfg := Config{
		Enabled:                 true,
		NamespaceFailurePercent: 50,
		NodeCacheTTL:            30 * time.Second,
	}
	checker, err := NewChecker(cfg)
	if err != nil {
		t.Fatalf("NewChecker() error = %v", err)
	}

	errorsJSON, _ := json.Marshal([]struct {
		Text string `json:"text"`
	}{
		{Text: "container app: terminated with exit code 1"},
	})

	finding := &domain.Finding{
		Kind:         "Pod",
		Name:         "test-pod",
		Namespace:    "default",
		ParentObject: "test-pod",
		Errors:       string(errorsJSON),
		SourceRef: domain.SourceRef{
			APIVersion: "v1",
			Kind:       "Pod",
			Name:       "test-pod",
			Namespace:  "default",
		},
	}

	// First call should cache node state
	suppress1, _, err := checker.ShouldSuppress(context.Background(), finding, client)
	if err != nil {
		t.Fatalf("ShouldSuppress() error = %v", err)
	}
	if !suppress1 {
		t.Error("First ShouldSuppress() = false, want true")
	}

	// Second call should use cache
	suppress2, _, err := checker.ShouldSuppress(context.Background(), finding, client)
	if err != nil {
		t.Fatalf("ShouldSuppress() error = %v", err)
	}
	if !suppress2 {
		t.Error("Second ShouldSuppress() = false, want true")
	}
}

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	return scheme
}
