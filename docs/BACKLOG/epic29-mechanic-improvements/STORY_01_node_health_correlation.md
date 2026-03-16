# STORY_01: Node Health Correlation and Filtering

## Status: Not Started

## Objective

Implement node health correlation to skip investigations for resources on unhealthy nodes. This addresses the observed problem where mechanic wasted cycles investigating pods on `NotReady` nodes (e.g., worker-02).

## Acceptance Criteria

- [ ] Node health cache with TTL-based invalidation
- [ ] Node health correlation in `ExtractFinding` for Pod, Deployment, StatefulSet providers
- [ ] Skip investigations for resources on `NotReady` nodes
- [ ] Group findings from same failed node into correlation group
- [ ] Configuration options for node health filtering
- [ ] Unit tests for node health correlation
- [ ] Integration tests with envtest
- [ ] `go test -timeout 30s -race ./...` passes

## Problem Analysis

### Current Issue
From operational analysis:
- Worker-02 was `NotReady` (kubelet stopped posting status)
- Multiple pods on worker-02 showed as failing
- Mechanic launched investigations for each pod individually
- Each investigation converged on "node is not ready" as root cause
- Wasted LLM tokens and compute cycles

### Desired Behavior
- Detect when a resource is on an unhealthy node
- Skip investigation for that resource (or group with node issue)
- Create single investigation for the node failure itself
- Reduce noise and wasted cycles

## Technical Design

### 1. Node Health Cache

**File**: `internal/filter/nodehealth/cache.go`

```go
package nodehealth

import (
    "context"
    "sync"
    "time"

    corev1 "k8s.io/api/core/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// NodeStatus represents the health status of a node.
type NodeStatus string

const (
    NodeStatusReady    NodeStatus = "Ready"
    NodeStatusNotReady NodeStatus = "NotReady"
    NodeStatusUnknown  NodeStatus = "Unknown"
)

// NodeHealth represents cached node health information.
type NodeHealth struct {
    Name      string
    Status    NodeStatus
    Conditions []corev1.NodeCondition
    LastCheck time.Time
    TTL       time.Duration
}

// IsHealthy returns true if the node is Ready.
func (nh NodeHealth) IsHealthy() bool {
    return nh.Status == NodeStatusReady
}

// IsExpired returns true if the cache entry has expired.
func (nh NodeHealth) IsExpired() bool {
    return time.Since(nh.LastCheck) > nh.TTL
}

// Cache manages node health information with TTL-based invalidation.
type Cache struct {
    client client.Client
    mu     sync.RWMutex
    nodes  map[string]NodeHealth
    ttl    time.Duration
}

// NewCache creates a new node health cache.
func NewCache(c client.Client, ttl time.Duration) *Cache {
    return &Cache{
        client: c,
        nodes:  make(map[string]NodeHealth),
        ttl:    ttl,
    }
}

// GetNodeHealth returns the health status of a node.
// If the cache entry is expired or missing, it fetches fresh data from the API.
func (c *Cache) GetNodeHealth(ctx context.Context, nodeName string) (NodeHealth, error) {
    // Check cache first
    c.mu.RLock()
    if nh, ok := c.nodes[nodeName]; ok && !nh.IsExpired() {
        c.mu.RUnlock()
        return nh, nil
    }
    c.mu.RUnlock()

    // Fetch from API
    node := &corev1.Node{}
    if err := c.client.Get(ctx, client.ObjectKey{Name: nodeName}, node); err != nil {
        return NodeHealth{}, err
    }

    // Determine status
    status := NodeStatusUnknown
    for _, cond := range node.Status.Conditions {
        if cond.Type == corev1.NodeReady {
            if cond.Status == corev1.ConditionTrue {
                status = NodeStatusReady
            } else {
                status = NodeStatusNotReady
            }
            break
        }
    }

    nh := NodeHealth{
        Name:       nodeName,
        Status:     status,
        Conditions: node.Status.Conditions,
        LastCheck:  time.Now(),
        TTL:        c.ttl,
    }

    // Update cache
    c.mu.Lock()
    c.nodes[nodeName] = nh
    c.mu.Unlock()

    return nh, nil
}

// GetNodeHealthBatch returns health status for multiple nodes.
// Optimized to minimize API calls by batching cache lookups.
func (c *Cache) GetNodeHealthBatch(ctx context.Context, nodeNames []string) (map[string]NodeHealth, error) {
    result := make(map[string]NodeHealth)
    var toFetch []string

    // First pass: check cache
    c.mu.RLock()
    for _, nodeName := range nodeNames {
        if nh, ok := c.nodes[nodeName]; ok && !nh.IsExpired() {
            result[nodeName] = nh
        } else {
            toFetch = append(toFetch, nodeName)
        }
    }
    c.mu.RUnlock()

    // Fetch remaining nodes
    for _, nodeName := range toFetch {
        nh, err := c.GetNodeHealth(ctx, nodeName)
        if err != nil {
            // If we can't fetch a node, mark it as unknown
            nh = NodeHealth{
                Name:      nodeName,
                Status:    NodeStatusUnknown,
                LastCheck: time.Now(),
                TTL:       c.ttl,
            }
        }
        result[nodeName] = nh
    }

    return result, nil
}

// Invalidate removes a node from the cache.
func (c *Cache) Invalidate(nodeName string) {
    c.mu.Lock()
    delete(c.nodes, nodeName)
    c.mu.Unlock()
}

// Clear removes all entries from the cache.
func (c *Cache) Clear() {
    c.mu.Lock()
    c.nodes = make(map[string]NodeHealth)
    c.mu.Unlock()
}
```

### 2. Node Health Filter

**File**: `internal/filter/nodehealth/filter.go`

```go
package nodehealth

import (
    "context"
    "fmt"

    "github.com/lenaxia/k8s-mendabot/internal/domain"
)

// FilterResult represents the outcome of node health filtering.
type FilterResult struct {
    // ShouldSkip indicates whether the investigation should be skipped.
    ShouldSkip bool
    
    // Reason provides the reason for skipping (if applicable).
    Reason string
    
    // NodeHealth contains the health status of the node.
    NodeHealth NodeHealth
    
    // CorrelationGroup suggests a correlation group ID for related findings.
    CorrelationGroup string
}

// Filter checks if a finding should be skipped based on node health.
type Filter struct {
    cache *Cache
}

// NewFilter creates a new node health filter.
func NewFilter(cache *Cache) *Filter {
    return &Filter{cache: cache}
}

// CheckPod checks if a pod finding should be skipped based on node health.
func (f *Filter) CheckPod(ctx context.Context, podName, namespace, nodeName string) (FilterResult, error) {
    if nodeName == "" {
        // Pod not scheduled yet - can't check node health
        return FilterResult{ShouldSkip: false}, nil
    }

    nh, err := f.cache.GetNodeHealth(ctx, nodeName)
    if err != nil {
        // If we can't determine node health, proceed with investigation
        return FilterResult{
            ShouldSkip: false,
            Reason:     fmt.Sprintf("could not determine node health: %v", err),
        }, nil
    }

    if !nh.IsHealthy() {
        return FilterResult{
            ShouldSkip:       true,
            Reason:          fmt.Sprintf("node %s is %s", nodeName, nh.Status),
            NodeHealth:      nh,
            CorrelationGroup: fmt.Sprintf("node-failure-%s", nodeName),
        }, nil
    }

    return FilterResult{ShouldSkip: false, NodeHealth: nh}, nil
}

// CheckDeployment checks if a deployment finding should be skipped based on node health.
// It checks the health of nodes where the deployment's pods are running.
func (f *Filter) CheckDeployment(ctx context.Context, client client.Client, deployName, namespace string) (FilterResult, error) {
    // Get pods for this deployment
    pods := &corev1.PodList{}
    labels := map[string]string{
        "app": deployName, // This is simplified - real implementation needs deployment selector
    }
    
    if err := client.List(ctx, pods, client.InNamespace(namespace), client.MatchingLabels(labels)); err != nil {
        return FilterResult{ShouldSkip: false}, nil // Proceed if we can't list pods
    }

    // Collect node names
    var nodeNames []string
    for _, pod := range pods.Items {
        if pod.Spec.NodeName != "" {
            nodeNames = append(nodeNames, pod.Spec.NodeName)
        }
    }

    if len(nodeNames) == 0 {
        return FilterResult{ShouldSkip: false}, nil
    }

    // Check health of all nodes
    nodeHealth, err := f.cache.GetNodeHealthBatch(ctx, nodeNames)
    if err != nil {
        return FilterResult{ShouldSkip: false}, nil
    }

    // Check if any node is unhealthy
    var unhealthyNodes []string
    for nodeName, nh := range nodeHealth {
        if !nh.IsHealthy() {
            unhealthyNodes = append(unhealthyNodes, nodeName)
        }
    }

    if len(unhealthyNodes) > 0 {
        return FilterResult{
            ShouldSkip:       true,
            Reason:          fmt.Sprintf("deployment pods running on unhealthy nodes: %v", unhealthyNodes),
            CorrelationGroup: fmt.Sprintf("node-failure-%s", unhealthyNodes[0]),
        }, nil
    }

    return FilterResult{ShouldSkip: false}, nil
}

// ShouldSkipFinding determines if a finding should be skipped based on node health.
func (f *Filter) ShouldSkipFinding(ctx context.Context, client client.Client, finding domain.Finding) (bool, string, error) {
    switch finding.Kind {
    case "Pod":
        // Extract node name from finding (added in STORY_00)
        // For now, we'll need to get it from the actual pod
        pod := &corev1.Pod{}
        if err := client.Get(ctx, client.ObjectKey{Name: finding.Name, Namespace: finding.Namespace}, pod); err != nil {
            return false, "", nil // Proceed if we can't get pod
        }
        
        result, err := f.CheckPod(ctx, finding.Name, finding.Namespace, pod.Spec.NodeName)
        if err != nil {
            return false, "", err
        }
        return result.ShouldSkip, result.Reason, nil

    case "Deployment":
        result, err := f.CheckDeployment(ctx, client, finding.Name, finding.Namespace)
        if err != nil {
            return false, "", err
        }
        return result.ShouldSkip, result.Reason, nil

    case "StatefulSet":
        // Similar to deployment check
        // Implementation omitted for brevity
        return false, "", nil

    default:
        // Other resource types don't have node affinity
        return false, "", nil
    }
}
```

### 3. Provider Integration

**File**: `internal/provider/native/pod.go` (modification)

Add node health check to `ExtractFinding`:

```go
func (p *podProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
    if domain.ShouldSkip(obj.GetAnnotations(), time.Now()) {
        return nil, nil
    }
    
    pod, ok := obj.(*corev1.Pod)
    if !ok {
        return nil, fmt.Errorf("podProvider: expected *corev1.Pod, got %T", obj)
    }

    // NEW: Check node health before proceeding
    if p.nodeFilter != nil && pod.Spec.NodeName != "" {
        ctx := context.Background()
        result, err := p.nodeFilter.CheckPod(ctx, pod.Name, pod.Namespace, pod.Spec.NodeName)
        if err == nil && result.ShouldSkip {
            // Log suppression for audit trail
            p.logger.Info("pod finding suppressed due to node health",
                "pod", pod.Name,
                "namespace", pod.Namespace,
                "node", pod.Spec.NodeName,
                "reason", result.Reason,
                "correlationGroup", result.CorrelationGroup)
            return nil, nil
        }
    }

    // Rest of existing ExtractFinding logic...
    // ...
    
    // NEW: Add node name to finding
    finding.NodeName = pod.Spec.NodeName
    
    return finding, nil
}
```

**File**: `internal/provider/native/deployment.go` (modification)

Similar modification for deployment provider.

### 4. Configuration

**File**: `internal/config/config.go`

Add node health configuration:

```go
type Config struct {
    // ... existing fields ...
    
    // Node health filtering
    SkipUnhealthyNodes bool          `env:"SKIP_UNHEALTHY_NODES" envDefault:"true"`
    NodeHealthCacheTTL time.Duration `env:"NODE_HEALTH_CACHE_TTL" envDefault:"30s"`
    
    // Node health filter instance (not from env)
    nodeFilter *nodehealth.Filter
}

// NodeFilter returns the node health filter, creating it if necessary.
func (c *Config) NodeFilter(client client.Client) *nodehealth.Filter {
    if c.nodeFilter == nil && c.SkipUnhealthyNodes {
        cache := nodehealth.NewCache(client, c.NodeHealthCacheTTL)
        c.nodeFilter = nodehealth.NewFilter(cache)
    }
    return c.nodeFilter
}
```

### 5. SourceProviderReconciler Integration

**File**: `internal/provider/provider.go` (modification)

Update the reconciler to use node health filter:

```go
func (r *SourceProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ... existing reconciliation logic ...
    
    // NEW: Check node health before creating finding
    if r.config.SkipUnhealthyNodes && r.config.NodeFilter(r.Client) != nil {
        // Create a temporary finding to check
        tempFinding := domain.Finding{
            Kind:      obj.GetObjectKind().GroupVersionKind().Kind,
            Name:      obj.GetName(),
            Namespace: obj.GetNamespace(),
        }
        
        shouldSkip, reason, err := r.config.NodeFilter(r.Client).ShouldSkipFinding(ctx, r.Client, tempFinding)
        if err != nil {
            r.logger.Error(err, "failed to check node health")
        } else if shouldSkip {
            r.logger.Info("finding suppressed due to node health",
                "kind", tempFinding.Kind,
                "name", tempFinding.Name,
                "namespace", tempFinding.Namespace,
                "reason", reason)
            
            // Emit Kubernetes event for audit trail
            r.recordEvent(obj, "FindingSuppressed", reason)
            return ctrl.Result{}, nil
        }
    }
    
    // Continue with existing finding extraction...
    // ...
}
```

## Unit Tests

**File**: `internal/filter/nodehealth/cache_test.go`

```go
package nodehealth

import (
    "context"
    "testing"
    "time"

    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNodeHealthCache(t *testing.T) {
    ctx := context.Background()
    
    // Create test node
    node := &corev1.Node{
        ObjectMeta: metav1.ObjectMeta{
            Name: "test-node",
        },
        Status: corev1.NodeStatus{
            Conditions: []corev1.NodeCondition{
                {
                    Type:   corev1.NodeReady,
                    Status: corev1.ConditionTrue,
                },
            },
        },
    }
    
    client := fake.NewClientBuilder().WithObjects(node).Build()
    cache := NewCache(client, 1*time.Minute)
    
    // Test GetNodeHealth
    nh, err := cache.GetNodeHealth(ctx, "test-node")
    if err != nil {
        t.Fatalf("GetNodeHealth error: %v", err)
    }
    
    if nh.Name != "test-node" {
        t.Errorf("NodeHealth.Name = %v, want test-node", nh.Name)
    }
    
    if nh.Status != NodeStatusReady {
        t.Errorf("NodeHealth.Status = %v, want Ready", nh.Status)
    }
    
    if !nh.IsHealthy() {
        t.Error("NodeHealth.IsHealthy() = false, want true")
    }
    
    // Test cache hit
    nh2, err := cache.GetNodeHealth(ctx, "test-node")
    if err != nil {
        t.Fatalf("Second GetNodeHealth error: %v", err)
    }
    
    if nh2.LastCheck != nh.LastCheck {
        t.Error("Cache should return same timestamp for cache hit")
    }
    
    // Test invalidate
    cache.Invalidate("test-node")
    nh3, err := cache.GetNodeHealth(ctx, "test-node")
    if err != nil {
        t.Fatalf("GetNodeHealth after invalidate error: %v", err)
    }
    
    if nh3.LastCheck == nh.LastCheck {
        t.Error("Cache should have fresh data after invalidate")
    }
}

func TestNodeHealthCache_NotReady(t *testing.T) {
    ctx := context.Background()
    
    node := &corev1.Node{
        ObjectMeta: metav1.ObjectMeta{
            Name: "not-ready-node",
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
    
    client := fake.NewClientBuilder().WithObjects(node).Build()
    cache := NewCache(client, 1*time.Minute)
    
    nh, err := cache.GetNodeHealth(ctx, "not-ready-node")
    if err != nil {
        t.Fatalf("GetNodeHealth error: %v", err)
    }
    
    if nh.Status != NodeStatusNotReady {
        t.Errorf("NodeHealth.Status = %v, want NotReady", nh.Status)
    }
    
    if nh.IsHealthy() {
        t.Error("NodeHealth.IsHealthy() = true, want false for NotReady node")
    }
}
```

**File**: `internal/filter/nodehealth/filter_test.go`

```go
package nodehealth

import (
    "context"
    "testing"

    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFilter_CheckPod(t *testing.T) {
    ctx := context.Background()
    
    // Create test nodes
    readyNode := &corev1.Node{
        ObjectMeta: metav1.ObjectMeta{Name: "ready-node"},
        Status: corev1.NodeStatus{
            Conditions: []corev1.NodeCondition{
                {Type: corev1.NodeReady, Status: corev1.ConditionTrue},
            },
        },
    }
    
    notReadyNode := &corev1.Node{
        ObjectMeta: metav1.ObjectMeta{Name: "not-ready-node"},
        Status: corev1.NodeStatus{
            Conditions: []corev1.NodeCondition{
                {Type: corev1.NodeReady, Status: corev1.ConditionFalse, Reason: "KubeletNotReady"},
            },
        },
    }
    
    client := fake.NewClientBuilder().WithObjects(readyNode, notReadyNode).Build()
    cache := NewCache(client, 1*time.Minute)
    filter := NewFilter(cache)
    
    // Test pod on ready node
    result, err := filter.CheckPod(ctx, "test-pod", "default", "ready-node")
    if err != nil {
        t.Fatalf("CheckPod error: %v", err)
    }
    
    if result.ShouldSkip {
        t.Error("CheckPod on ready node: ShouldSkip = true, want false")
    }
    
    // Test pod on not-ready node
    result, err = filter.CheckPod(ctx, "test-pod", "default", "not-ready-node")
    if err != nil {
        t.Fatalf("CheckPod error: %v", err)
    }
    
    if !result.ShouldSkip {
        t.Error("CheckPod on not-ready node: ShouldSkip = false, want true")
    }
    
    if result.Reason == "" {
        t.Error("CheckPod on not-ready node: Reason empty, want non-empty")
    }
    
    if result.CorrelationGroup != "node-failure-not-ready-node" {
        t.Errorf("CheckPod CorrelationGroup = %v, want node-failure-not-ready-node", result.CorrelationGroup)
    }
    
    // Test pod with empty node name (unscheduled)
    result, err = filter.CheckPod(ctx, "test-pod", "default", "")
    if err != nil {
        t.Fatalf("CheckPod error: %v", err)
    }
    
    if result.ShouldSkip {
        t.Error("CheckPod with empty node name: ShouldSkip = true, want false")
    }
}
```

## Integration Tests

**File**: `internal/provider/native/pod_integration_test.go`

Add integration tests for node health filtering in the pod provider.

## Configuration Updates

**File**: `charts/mendabot/values.yaml`

Add new configuration values:

```yaml
# Node health filtering
nodeHealth:
  # Skip investigations for resources on unhealthy nodes
  skipUnhealthyNodes: true
  # Node health cache TTL in seconds
  cacheTTL: 30
```

**File**: `charts/mendabot/templates/deployment-watcher.yaml`

Add environment variables:

```yaml
- name: SKIP_UNHEALTHY_NODES
  value: {{ .Values.nodeHealth.skipUnhealthyNodes | quote }}
- name: NODE_HEALTH_CACHE_TTL
  value: {{ .Values.nodeHealth.cacheTTL | quote }}
```

## Verification Steps

1. **Unit Tests**: Run `go test -timeout 30s ./internal/filter/nodehealth/...`
2. **Integration Tests**: Run `go test -timeout 30s ./internal/provider/native/...`
3. **Backward Compatibility**: Existing tests should still pass
4. **Configuration Test**: Deploy with `SKIP_UNHEALTHY_NODES=false` to verify old behavior
5. **Performance Test**: Verify cache reduces API calls

## Expected Outcomes

1. **Reduced Investigations**: Pods on `NotReady` nodes should not trigger investigations
2. **Correlation Groups**: Findings from same failed node should be grouped
3. **Audit Trail**: Suppression events should be logged and emit Kubernetes events
4. **Performance**: Node health cache should reduce API server load

## Metrics

New Prometheus metrics:
- `mendabot_findings_suppressed_total{reason="node_health"}`
- `mendabot_node_health_cache_hits_total`
- `mendabot_node_health_cache_misses_total`

## Next Steps

After this story is complete:
- STORY_02: Implement infrastructure cascade detection (builds on node health)
- STORY_03: Implement failure analysis CRD
- STORY_04: Implement smart PR management

This story addresses the most common source of wasted investigations: resources on unhealthy nodes.