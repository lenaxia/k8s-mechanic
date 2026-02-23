package cascade

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// Checker implements infrastructure cascade detection.
// It suppresses findings that are likely caused by broader infrastructure issues
// to avoid redundant investigations.
type Checker interface {
	// ShouldSuppress returns true if the finding should be suppressed due to
	// an infrastructure cascade. The reason string explains why it was suppressed.
	ShouldSuppress(ctx context.Context, finding *domain.Finding, k8sClient client.Client) (bool, string, error)
}

// Config holds cascade checker configuration.
type Config struct {
	// Enabled controls whether cascade checking is active.
	Enabled bool
	// NamespaceFailurePercent is the threshold for namespace-wide failure detection.
	// If > this percentage of pods in a namespace are failing, findings are suppressed.
	NamespaceFailurePercent int
	// NodeCacheTTL is the time-to-live for cached node state.
	NodeCacheTTL time.Duration
}

// checker implements the Checker interface.
type checker struct {
	config Config
	mu     sync.RWMutex
	cache  map[string]nodeCacheEntry
}

type nodeCacheEntry struct {
	node      *corev1.Node
	timestamp time.Time
}

// NewChecker creates a new cascade checker with the given configuration.
func NewChecker(config Config) (Checker, error) {
	if config.NamespaceFailurePercent < 0 || config.NamespaceFailurePercent > 100 {
		return nil, fmt.Errorf("NamespaceFailurePercent must be between 0 and 100, got %d", config.NamespaceFailurePercent)
	}
	if config.NodeCacheTTL < 0 {
		return nil, fmt.Errorf("NodeCacheTTL must be non-negative, got %v", config.NodeCacheTTL)
	}
	return &checker{
		config: config,
		cache:  make(map[string]nodeCacheEntry),
	}, nil
}

// ShouldSuppress implements the Checker interface.
func (c *checker) ShouldSuppress(ctx context.Context, finding *domain.Finding, k8sClient client.Client) (bool, string, error) {
	if !c.config.Enabled {
		return false, "", nil
	}

	// Only check pod findings for cascade suppression
	if finding.Kind != "Pod" {
		return false, "", nil
	}

	// Rule 1: Node failure detection
	suppress, reason, err := c.checkNodeFailure(ctx, finding, k8sClient)
	if err != nil {
		return false, "", err
	}
	if suppress {
		return true, reason, nil
	}

	// Rule 2: Node pressure correlation
	suppress, reason, err = c.checkNodePressure(ctx, finding, k8sClient)
	if err != nil {
		return false, "", err
	}
	if suppress {
		return true, reason, nil
	}

	// Rule 3: Namespace-wide failure detection
	suppress, reason, err = c.checkNamespaceWideFailure(ctx, finding, k8sClient)
	if err != nil {
		return false, "", err
	}
	if suppress {
		return true, reason, nil
	}

	return false, "", nil
}

// checkNodeFailure checks if the pod is on a NotReady node.
func (c *checker) checkNodeFailure(ctx context.Context, finding *domain.Finding, k8sClient client.Client) (bool, string, error) {
	pod, err := c.getPod(ctx, finding.Namespace, finding.Name, k8sClient)
	if err != nil {
		if errors.IsNotFound(err) {
			// Pod doesn't exist anymore, no need to suppress
			return false, "", nil
		}
		return false, "", fmt.Errorf("getting pod %s/%s: %w", finding.Namespace, finding.Name, err)
	}

	if pod.Spec.NodeName == "" {
		// Pod not scheduled yet
		return false, "", nil
	}

	node, err := c.getNode(ctx, pod.Spec.NodeName, k8sClient)
	if err != nil {
		if errors.IsNotFound(err) {
			// Node doesn't exist
			return true, fmt.Sprintf("node %s not found", pod.Spec.NodeName), nil
		}
		return false, "", fmt.Errorf("getting node %s: %w", pod.Spec.NodeName, err)
	}

	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			if cond.Status == corev1.ConditionFalse || cond.Status == corev1.ConditionUnknown {
				return true, fmt.Sprintf("node %s is %s: %s", node.Name, cond.Status, cond.Reason), nil
			}
			break
		}
	}

	return false, "", nil
}

// checkNodePressure checks if OOMKilled pods are on nodes with MemoryPressure.
func (c *checker) checkNodePressure(ctx context.Context, finding *domain.Finding, k8sClient client.Client) (bool, string, error) {
	// Check if the finding contains OOMKilled errors
	if !c.containsOOMKilledError(finding.Errors) {
		return false, "", nil
	}

	pod, err := c.getPod(ctx, finding.Namespace, finding.Name, k8sClient)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("getting pod %s/%s: %w", finding.Namespace, finding.Name, err)
	}

	if pod.Spec.NodeName == "" {
		return false, "", nil
	}

	node, err := c.getNode(ctx, pod.Spec.NodeName, k8sClient)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("getting node %s: %w", pod.Spec.NodeName, err)
	}

	// Check for MemoryPressure
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeMemoryPressure && cond.Status == corev1.ConditionTrue {
			return true, fmt.Sprintf("node %s has MemoryPressure: %s", node.Name, cond.Reason), nil
		}
	}

	return false, "", nil
}

// checkNamespaceWideFailure checks if > threshold% of pods in the namespace are failing.
func (c *checker) checkNamespaceWideFailure(ctx context.Context, finding *domain.Finding, k8sClient client.Client) (bool, string, error) {
	if c.config.NamespaceFailurePercent == 0 {
		return false, "", nil
	}

	var podList corev1.PodList
	if err := k8sClient.List(ctx, &podList, client.InNamespace(finding.Namespace)); err != nil {
		return false, "", fmt.Errorf("listing pods in namespace %s: %w", finding.Namespace, err)
	}

	if len(podList.Items) == 0 {
		return false, "", nil
	}

	failingPods := 0
	for _, pod := range podList.Items {
		if c.isPodFailing(&pod) {
			failingPods++
		}
	}

	failurePercent := (failingPods * 100) / len(podList.Items)
	if failurePercent > c.config.NamespaceFailurePercent {
		return true, fmt.Sprintf("%d%% of pods in namespace %s are failing (%d/%d)",
			failurePercent, finding.Namespace, failingPods, len(podList.Items)), nil
	}

	return false, "", nil
}

// containsOOMKilledError checks if the errors JSON contains OOMKilled references.
func (c *checker) containsOOMKilledError(errorsJSON string) bool {
	if errorsJSON == "" {
		return false
	}

	var errors []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(errorsJSON), &errors); err != nil {
		return false
	}

	for _, e := range errors {
		if strings.Contains(strings.ToLower(e.Text), "oomkilled") {
			return true
		}
	}
	return false
}

// isPodFailing returns true if the pod is in a failing state.
func (c *checker) isPodFailing(pod *corev1.Pod) bool {
	// Check pod phase
	if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodUnknown {
		return true
	}

	// Check container statuses
	allStatuses := append(pod.Status.ContainerStatuses, pod.Status.InitContainerStatuses...)
	for _, cs := range allStatuses {
		// Check waiting state for failure reasons
		if cs.State.Waiting != nil {
			reason := cs.State.Waiting.Reason
			if reason == "CrashLoopBackOff" || reason == "ImagePullBackOff" ||
				reason == "ErrImagePull" || reason == "CreateContainerConfigError" ||
				reason == "InvalidImageName" || reason == "RunContainerError" ||
				reason == "CreateContainerError" {
				return true
			}
		}

		// Check terminated state with non-zero exit code
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
			return true
		}
	}

	// Check for unschedulable pending pods
	if pod.Status.Phase == corev1.PodPending {
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodScheduled &&
				cond.Status == corev1.ConditionFalse &&
				cond.Reason == "Unschedulable" {
				return true
			}
		}
	}

	return false
}

// getPod retrieves a pod with caching.
func (c *checker) getPod(ctx context.Context, namespace, name string, k8sClient client.Client) (*corev1.Pod, error) {
	var pod corev1.Pod
	key := types.NamespacedName{Namespace: namespace, Name: name}
	if err := k8sClient.Get(ctx, key, &pod); err != nil {
		return nil, err
	}
	return &pod, nil
}

// getNode retrieves a node with caching.
func (c *checker) getNode(ctx context.Context, name string, k8sClient client.Client) (*corev1.Node, error) {
	// Check cache first
	c.mu.RLock()
	if entry, found := c.cache[name]; found {
		if time.Since(entry.timestamp) < c.config.NodeCacheTTL {
			c.mu.RUnlock()
			return entry.node, nil
		}
	}
	c.mu.RUnlock()

	// Fetch from API
	var node corev1.Node
	key := types.NamespacedName{Name: name}
	if err := k8sClient.Get(ctx, key, &node); err != nil {
		return nil, err
	}

	// Update cache
	c.mu.Lock()
	c.cache[name] = nodeCacheEntry{
		node:      &node,
		timestamp: time.Now(),
	}
	c.mu.Unlock()

	return &node, nil
}
