# STORY_02: Infrastructure Cascade Detection

## Status: Not Started

## Objective

Implement infrastructure cascade detection to group related findings from infrastructure failures (node failures, network issues, storage problems). This prevents mechanic from creating multiple investigations for symptoms of the same root cause.

## Acceptance Criteria

- [ ] Infrastructure cascade detection logic
- [ ] Namespace-wide failure detection (>50% pods failing)
- [ ] Storage system failure correlation
- [ ] Network issue correlation
- [ ] Single investigation for cascade root cause
- [ ] Configuration options for cascade detection
- [ ] Unit tests for cascade detection
- [ ] Integration tests with envtest
- [ ] `go test -timeout 30s -race ./...` passes

## Problem Analysis

### Current Issue
From operational analysis:
- Node failure causes multiple pods to fail
- Each failed pod triggers separate investigation
- All investigations converge on same root cause (node failure)
- Wasted LLM tokens and compute cycles
- PR noise from multiple "infrastructure issue" reports

### Desired Behavior
- Detect when multiple failures share common infrastructure cause
- Group related findings into single investigation
- Investigate root cause (node, storage, network) instead of symptoms
- Reduce investigation volume during infrastructure outages

## Technical Design

### 1. Cascade Detection Types

**File**: `internal/filter/cascade/types.go`

```go
package cascade

import (
    "time"

    "github.com/lenaxia/k8s-mendabot/internal/domain"
)

// CascadeType identifies the type of infrastructure cascade.
type CascadeType string

const (
    // NodeCascade indicates failures caused by node issues.
    NodeCascade CascadeType = "node"
    
    // StorageCascade indicates failures caused by storage system issues.
    StorageCascade CascadeType = "storage"
    
    // NetworkCascade indicates failures caused by network issues.
    NetworkCascade CascadeType = "network"
    
    // NamespaceCascade indicates widespread failures in a namespace.
    NamespaceCascade CascadeType = "namespace"
)

// CascadeDetection represents a detected infrastructure cascade.
type CascadeDetection struct {
    Type          CascadeType
    RootCause     string           // e.g., node name, storage system, network segment
    AffectedResources []string     // Resource names affected
    StartTime     time.Time
    Confidence    float64          // 0.0-1.0 confidence in detection
    Evidence      []string         // Supporting evidence
}

// CascadeChecker interface for detecting infrastructure cascades.
type CascadeChecker interface {
    // Check checks if a finding is part of an infrastructure cascade.
    Check(ctx context.Context, finding domain.Finding) (*CascadeDetection, error)
    
    // ShouldSuppress returns true if the finding should be suppressed due to cascade.
    ShouldSuppress(ctx context.Context, finding domain.Finding) (bool, string, error)
    
    // GetRootCauseFinding returns a finding for the cascade root cause.
    GetRootCauseFinding(ctx context.Context, detection CascadeDetection) (*domain.Finding, error)
}
```

### 2. Node Cascade Detector

**File**: `internal/filter/cascade/node.go`

```go
package cascade

import (
    "context"
    "fmt"
    "time"

    corev1 "k8s.io/api/core/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"

    "github.com/lenaxia/k8s-mendabot/internal/domain"
    "github.com/lenaxia/k8s-mendabot/internal/filter/nodehealth"
)

// NodeCascadeDetector detects cascades caused by node failures.
type NodeCascadeDetector struct {
    client     client.Client
    nodeFilter *nodehealth.Filter
    // Track recent node failures to avoid duplicate investigations
    recentNodeFailures map[string]time.Time
}

// NewNodeCascadeDetector creates a new node cascade detector.
func NewNodeCascadeDetector(client client.Client, nodeFilter *nodehealth.Filter) *NodeCascadeDetector {
    return &NodeCascadeDetector{
        client:             client,
        nodeFilter:         nodeFilter,
        recentNodeFailures: make(map[string]time.Time),
    }
}

// Check detects if a finding is caused by a node cascade.
func (d *NodeCascadeDetector) Check(ctx context.Context, finding domain.Finding) (*CascadeDetection, error) {
    // Skip if we already investigated this node recently
    if nodeName := finding.NodeName; nodeName != "" {
        if lastCheck, ok := d.recentNodeFailures[nodeName]; ok {
            if time.Since(lastCheck) < 30*time.Minute {
                // Already investigated this node recently
                return &CascadeDetection{
                    Type:       NodeCascade,
                    RootCause:  nodeName,
                    StartTime:  lastCheck,
                    Confidence: 0.9,
                    Evidence:   []string{"Node already investigated recently"},
                }, nil
            }
        }
    }

    // Check node health for pod findings
    if finding.Kind == "Pod" && finding.NodeName != "" {
        result, err := d.nodeFilter.CheckPod(ctx, finding.Name, finding.Namespace, finding.NodeName)
        if err != nil {
            return nil, err
        }
        
        if result.ShouldSkip && result.CorrelationGroup != "" {
            // This pod is on an unhealthy node
            detection := &CascadeDetection{
                Type:          NodeCascade,
                RootCause:     finding.NodeName,
                AffectedResources: []string{finding.Name},
                StartTime:     time.Now(),
                Confidence:    0.8,
                Evidence:      []string{result.Reason},
            }
            
            // Record this node failure
            d.recentNodeFailures[finding.NodeName] = time.Now()
            
            // Clean up old entries
            d.cleanupOldEntries()
            
            return detection, nil
        }
    }
    
    // Check for deployment/statefulset with pods on unhealthy nodes
    if finding.Kind == "Deployment" || finding.Kind == "StatefulSet" {
        // Get pods for this workload
        pods := &corev1.PodList{}
        selector := client.InNamespace(finding.Namespace)
        
        // Simplified - real implementation needs workload selector
        if err := d.client.List(ctx, pods, selector); err != nil {
            return nil, err
        }
        
        // Check node health for each pod
        var unhealthyNodes []string
        var affectedPods []string
        
        for _, pod := range pods.Items {
            if pod.Spec.NodeName == "" {
                continue
            }
            
            result, err := d.nodeFilter.CheckPod(ctx, pod.Name, pod.Namespace, pod.Spec.NodeName)
            if err != nil {
                continue
            }
            
            if result.ShouldSkip {
                unhealthyNodes = append(unhealthyNodes, pod.Spec.NodeName)
                affectedPods = append(affectedPods, pod.Name)
            }
        }
        
        if len(unhealthyNodes) > 0 {
            // Use the first unhealthy node as root cause
            rootNode := unhealthyNodes[0]
            
            detection := &CascadeDetection{
                Type:               NodeCascade,
                RootCause:          rootNode,
                AffectedResources:  affectedPods,
                StartTime:          time.Now(),
                Confidence:         0.7 + (0.1 * float64(len(unhealthyNodes))), // Higher confidence with more affected nodes
                Evidence:           []string{fmt.Sprintf("%d pods on unhealthy nodes", len(unhealthyNodes))},
            }
            
            // Record node failure
            d.recentNodeFailures[rootNode] = time.Now()
            d.cleanupOldEntries()
            
            return detection, nil
        }
    }
    
    return nil, nil
}

// ShouldSuppress returns true if the finding should be suppressed due to node cascade.
func (d *NodeCascadeDetector) ShouldSuppress(ctx context.Context, finding domain.Finding) (bool, string, error) {
    detection, err := d.Check(ctx, finding)
    if err != nil {
        return false, "", err
    }
    
    if detection != nil && detection.Confidence > 0.5 {
        return true, fmt.Sprintf("node cascade detected: %s", detection.RootCause), nil
    }
    
    return false, "", nil
}

// GetRootCauseFinding returns a finding for the node cascade root cause.
func (d *NodeCascadeDetector) GetRootCauseFinding(ctx context.Context, detection CascadeDetection) (*domain.Finding, error) {
    // Create a finding for the node itself
    errors := []map[string]string{
        {"text": fmt.Sprintf("node %s is unhealthy, affecting %d resources", 
            detection.RootCause, len(detection.AffectedResources))},
    }
    
    errorsJSON, err := json.Marshal(errors)
    if err != nil {
        return nil, err
    }
    
    return &domain.Finding{
        Kind:         "Node",
        Name:         detection.RootCause,
        Namespace:    "", // Nodes are cluster-scoped
        ParentObject: fmt.Sprintf("Node/%s", detection.RootCause),
        Errors:       string(errorsJSON),
        Details:      fmt.Sprintf("Infrastructure cascade detected. Affected resources: %v", detection.AffectedResources),
        Severity:     "critical",
        Tier:         domain.TierInfrastructure,
        Classification: domain.ClassificationNodeFailure,
        FixConfidence: 0.3, // Low confidence for infrastructure fixes
    }, nil
}

// cleanupOldEntries removes entries older than 1 hour.
func (d *NodeCascadeDetector) cleanupOldEntries() {
    cutoff := time.Now().Add(-1 * time.Hour)
    for node, lastCheck := range d.recentNodeFailures {
        if lastCheck.Before(cutoff) {
            delete(d.recentNodeFailures, node)
        }
    }
}
```

### 3. Namespace Cascade Detector

**File**: `internal/filter/cascade/namespace.go`

```go
package cascade

import (
    "context"
    "fmt"
    "time"

    corev1 "k8s.io/api/core/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"

    "github.com/lenaxia/k8s-mendabot/internal/domain"
)

// NamespaceCascadeDetector detects widespread failures in a namespace.
type NamespaceCascadeDetector struct {
    client client.Client
    // Track namespace failure rates
    namespaceStats map[string]namespaceStat
}

type namespaceStat struct {
    totalPods    int
    failingPods  int
    lastUpdated  time.Time
}

// NewNamespaceCascadeDetector creates a new namespace cascade detector.
func NewNamespaceCascadeDetector(client client.Client) *NamespaceCascadeDetector {
    return &NamespaceCascadeDetector{
        client:         client,
        namespaceStats: make(map[string]namespaceStat),
    }
}

// Check detects if a finding is part of a namespace-wide cascade.
func (d *NamespaceCascadeDetector) Check(ctx context.Context, finding domain.Finding) (*CascadeDetection, error) {
    namespace := finding.Namespace
    if namespace == "" {
        return nil, nil
    }
    
    // Update namespace statistics
    stat, err := d.updateNamespaceStats(ctx, namespace)
    if err != nil {
        return nil, err
    }
    
    // Check if failure rate exceeds threshold (default: 50%)
    failureRate := float64(stat.failingPods) / float64(stat.totalPods)
    if stat.totalPods >= 5 && failureRate > 0.5 { // At least 5 pods, >50% failing
        return &CascadeDetection{
            Type:               NamespaceCascade,
            RootCause:          namespace,
            AffectedResources:  []string{finding.Name},
            StartTime:          time.Now(),
            Confidence:         failureRate, // Confidence proportional to failure rate
            Evidence:           []string{fmt.Sprintf("%.0f%% of pods failing in namespace %s", failureRate*100, namespace)},
        }, nil
    }
    
    return nil, nil
}

// updateNamespaceStats updates statistics for a namespace.
func (d *NamespaceCascadeDetector) updateNamespaceStats(ctx context.Context, namespace string) (namespaceStat, error) {
    // Check if stats are fresh (updated within last minute)
    if stat, ok := d.namespaceStats[namespace]; ok {
        if time.Since(stat.lastUpdated) < 1*time.Minute {
            return stat, nil
        }
    }
    
    // Get all pods in namespace
    pods := &corev1.PodList{}
    if err := d.client.List(ctx, pods, client.InNamespace(namespace)); err != nil {
        return namespaceStat{}, err
    }
    
    // Count failing pods
    failingPods := 0
    for _, pod := range pods.Items {
        if isPodFailing(pod) {
            failingPods++
        }
    }
    
    stat := namespaceStat{
        totalPods:   len(pods.Items),
        failingPods: failingPods,
        lastUpdated: time.Now(),
    }
    
    d.namespaceStats[namespace] = stat
    return stat, nil
}

// isPodFailing returns true if a pod is in a failing state.
func isPodFailing(pod corev1.Pod) bool {
    // Check pod phase
    if pod.Status.Phase == corev1.PodFailed {
        return true
    }
    
    // Check container statuses
    for _, cs := range pod.Status.ContainerStatuses {
        if cs.State.Waiting != nil {
            reason := cs.State.Waiting.Reason
            // Consider these as failing states
            if reason == "CrashLoopBackOff" || reason == "ImagePullBackOff" || 
               reason == "ErrImagePull" || reason == "CreateContainerError" {
                return true
            }
        }
        if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
            return true
        }
    }
    
    return false
}

// ShouldSuppress returns true if the finding should be suppressed due to namespace cascade.
func (d *NamespaceCascadeDetector) ShouldSuppress(ctx context.Context, finding domain.Finding) (bool, string, error) {
    detection, err := d.Check(ctx, finding)
    if err != nil {
        return false, "", err
    }
    
    if detection != nil && detection.Confidence > 0.5 {
        return true, fmt.Sprintf("namespace cascade detected: %s (%.0f%% failure rate)", 
            detection.RootCause, detection.Confidence*100), nil
    }
    
    return false, "", nil
}

// GetRootCauseFinding returns a finding for the namespace cascade.
func (d *NamespaceCascadeDetector) GetRootCauseFinding(ctx context.Context, detection CascadeDetection) (*domain.Finding, error) {
    errors := []map[string]string{
        {"text": fmt.Sprintf("namespace %s has widespread failures (%.0f%% of pods affected)", 
            detection.RootCause, detection.Confidence*100)},
    }
    
    errorsJSON, err := json.Marshal(errors)
    if err != nil {
        return nil, err
    }
    
    return &domain.Finding{
        Kind:         "Namespace",
        Name:         detection.RootCause,
        Namespace:    detection.RootCause,
        ParentObject: fmt.Sprintf("Namespace/%s", detection.RootCause),
        Errors:       string(errorsJSON),
        Details:      "Widespread failures detected across namespace. Likely infrastructure issue.",
        Severity:     "critical",
        Tier:         domain.TierInfrastructure,
        Classification: domain.ClassificationNodeFailure, // Usually node or network
        FixConfidence: 0.2, // Very low confidence for namespace-wide issues
    }, nil
}
```

### 4. Storage Cascade Detector

**File**: `internal/filter/cascade/storage.go`

```go
package cascade

import (
    "context"
    "fmt"
    "strings"
    "time"

    corev1 "k8s.io/api/core/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"

    "github.com/lenaxia/k8s-mendabot/internal/domain"
)

// StorageCascadeDetector detects cascades caused by storage system failures.
type StorageCascadeDetector struct {
    client client.Client
}

// NewStorageCascadeDetector creates a new storage cascade detector.
func NewStorageCascadeDetector(client client.Client) *StorageCascadeDetector {
    return &StorageCascadeDetector{client: client}
}

// Check detects if a finding is caused by a storage cascade.
func (d *StorageCascadeDetector) Check(ctx context.Context, finding domain.Finding) (*CascadeDetection, error) {
    // Check for PVC-related errors
    if strings.Contains(finding.Errors, "PersistentVolumeClaim") || 
       strings.Contains(finding.Errors, "volume") ||
       strings.Contains(finding.Errors, "storage") {
        
        // Get the resource to check for PVC references
        switch finding.Kind {
        case "Pod":
            pod := &corev1.Pod{}
            if err := d.client.Get(ctx, client.ObjectKey{
                Name: finding.Name, Namespace: finding.Namespace}, pod); err != nil {
                return nil, err
            }
            
            // Check for PVC volumes
            var pvcNames []string
            for _, vol := range pod.Spec.Volumes {
                if vol.PersistentVolumeClaim != nil {
                    pvcNames = append(pvcNames, vol.PersistentVolumeClaim.ClaimName)
                }
            }
            
            if len(pvcNames) > 0 {
                // Check if any PVC is failing
                for _, pvcName := range pvcNames {
                    pvc := &corev1.PersistentVolumeClaim{}
                    if err := d.client.Get(ctx, client.ObjectKey{
                        Name: pvcName, Namespace: finding.Namespace}, pvc); err != nil {
                        continue
                    }
                    
                    if isPVCFailing(*pvc) {
                        // Check for other pods using same PVC
                        affectedPods, err := d.findPodsUsingPVC(ctx, finding.Namespace, pvcName)
                        if err != nil {
                            return nil, err
                        }
                        
                        if len(affectedPods) > 1 {
                            return &CascadeDetection{
                                Type:               StorageCascade,
                                RootCause:          pvcName,
                                AffectedResources:  affectedPods,
                                StartTime:          time.Now(),
                                Confidence:         0.7,
                                Evidence:           []string{fmt.Sprintf("PVC %s is failing, affecting %d pods", pvcName, len(affectedPods))},
                            }, nil
                        }
                    }
                }
            }
        }
    }
    
    return nil, nil
}

// isPVCFailing returns true if a PVC is in a failing state.
func isPVCFailing(pvc corev1.PersistentVolumeClaim) bool {
    if pvc.Status.Phase == corev1.ClaimPending {
        // Check for provisioning failures
        for _, condition := range pvc.Status.Conditions {
            if condition.Type == corev1.PersistentVolumeClaimResizing && 
               condition.Status == corev1.ConditionFalse {
                return true
            }
            if condition.Type == corev1.PersistentVolumeClaimFileSystemResizePending {
                return true
            }
        }
    }
    return false
}

// findPodsUsingPVC finds all pods using a specific PVC.
func (d *StorageCascadeDetector) findPodsUsingPVC(ctx context.Context, namespace, pvcName string) ([]string, error) {
    pods := &corev1.PodList{}
    if err := d.client.List(ctx, pods, client.InNamespace(namespace)); err != nil {
        return nil, err
    }
    
    var affectedPods []string
    for _, pod := range pods.Items {
        for _, vol := range pod.Spec.Volumes {
            if vol.PersistentVolumeClaim != nil && vol.PersistentVolumeClaim.ClaimName == pvcName {
                affectedPods = append(affectedPods, pod.Name)
                break
            }
        }
    }
    
    return affectedPods, nil
}

// ShouldSuppress returns true if the finding should be suppressed due to storage cascade.
func (d *StorageCascadeDetector) ShouldSuppress(ctx context.Context, finding domain.Finding) (bool, string, error) {
    detection, err := d.Check(ctx, finding)
    if err != nil {
        return false, "", err
    }
    
    if detection != nil && detection.Confidence > 0.5 {
        return true, fmt.Sprintf("storage cascade detected: %s", detection.RootCause), nil
    }
    
    return false, "", nil
}

// GetRootCauseFinding returns a finding for the storage cascade.
func (d *StorageCascadeDetector) GetRootCauseFinding(ctx context.Context, detection CascadeDetection) (*domain.Finding, error) {
    errors := []map[string]string{
        {"text": fmt.Sprintf("storage system issue affecting PVC %s, impacting %d pods", 
            detection.RootCause, len(detection.AffectedResources))},
    }
    
    errorsJSON, err := json.Marshal(errors)
    if err != nil {
        return nil, err
    }
    
    return &domain.Finding{
        Kind:         "PersistentVolumeClaim",
        Name:         detection.RootCause,
        Namespace:    "", // Will be set by caller
        ParentObject: fmt.Sprintf("PersistentVolumeClaim/%s", detection.RootCause),
        Errors:       string(errorsJSON),
        Details:      fmt.Sprintf("Storage cascade detected. Affected pods: %v", detection.AffectedResources),
        Severity:     "high",
        Tier:         domain.TierInfrastructure,
        Classification: domain.ClassificationStorageIssue,
        FixConfidence: 0.3,
    }, nil
}
```

### 5. Cascade Manager

**File**: `internal/filter/cascade/manager.go`

```go
package cascade

import (
    "context"
    "fmt"

    "sigs.k8s.io/controller-runtime/pkg/client"

    "github.com/lenaxia/k8s-mendabot/internal/domain"
    "github.com/lenaxia/k8s-mendabot/internal/filter/nodehealth"
)

// Manager coordinates multiple cascade detectors.
type Manager struct {
    detectors []CascadeChecker
    enabled   bool
}

// NewManager creates a new cascade manager.
func NewManager(client client.Client, nodeFilter *nodehealth.Filter, enabled bool) *Manager {
    if !enabled {
        return &Manager{enabled: false}
    }
    
    detectors := []CascadeChecker{
        NewNodeCascadeDetector(client, nodeFilter),
        NewNamespaceCascadeDetector(client),
        NewStorageCascadeDetector(client),
    }
    
    return &Manager{
        detectors: detectors,
        enabled:   enabled,
    }
}

// CheckAll checks all detectors for cascades.
func (m *Manager) CheckAll(ctx context.Context, finding domain.Finding) (*CascadeDetection, error) {
    if !m.enabled {
        return nil, nil
    }
    
    var bestDetection *CascadeDetection
    bestConfidence := 0.0
    
    for _, detector := range m.detectors {
        detection, err := detector.Check(ctx, finding)
        if err != nil {
            continue // Skip failed detectors
        }
        
        if detection != nil && detection.Confidence > bestConfidence {
            bestDetection = detection
            bestConfidence = detection.Confidence
        }
    }
    
    return bestDetection, nil
}

// ShouldSuppress checks if a finding should be suppressed due to any cascade.
func (m *Manager) ShouldSuppress(ctx context.Context, finding domain.Finding) (bool, string, error) {
    if !m.enabled {
        return false, "", nil
    }
    
    for _, detector := range m.detectors {
        suppress, reason, err := detector.ShouldSuppress(ctx, finding)
        if err != nil {
            continue
        }
        
        if suppress {
            return true, reason, nil
        }
    }
    
    return false, "", nil
}

// GetRootCauseFinding gets the root cause finding for a cascade.
func (m *Manager) GetRootCauseFinding(ctx context.Context, detection CascadeDetection) (*domain.Finding, error) {
    if !m.enabled {
        return nil, fmt.Errorf("cascade detection disabled")
    }
    
    // Find the appropriate detector
    for _, detector := range m.detectors {
        // Try to get root cause finding
        finding, err := detector.GetRootCauseFinding(ctx, detection)
        if err == nil && finding != nil {
            return finding, nil
        }
    }
    
    return nil, fmt.Errorf("no detector found for cascade type %s", detection.Type)
}
```

### 6. Provider Integration

**File**: `internal/provider/provider.go` (modification)

Update the reconciler to use cascade detection:

```go
func (r *SourceProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ... existing reconciliation logic ...
    
    // NEW: Check for infrastructure cascades
    if r.cascadeManager != nil {
        suppress, reason, err := r.cascadeManager.ShouldSuppress(ctx, *finding)
        if err != nil {
            r.logger.Error(err, "failed to check cascade detection")
        } else if suppress {
            r.logger.Info("finding suppressed due to infrastructure cascade",
                "kind", finding.Kind,
                "name", finding.Name,
                "namespace", finding.Namespace,
                "reason", reason)
            
            // Emit event for audit trail
            r.recordEvent(obj, "FindingSuppressed", reason)
            
            // Check if we should create a root cause finding instead
            detection, err := r.cascadeManager.CheckAll(ctx, *finding)
            if err == nil && detection != nil && detection.Confidence > 0.7 {
                // Create root cause finding instead
                rootFinding, err := r.cascadeManager.GetRootCauseFinding(ctx, *detection)
                if err == nil && rootFinding != nil {
                    // Process the root cause finding instead
                    finding = rootFinding
                    // Continue with normal processing for root cause
                }
            } else {
                return ctrl.Result{}, nil
            }
        }
    }
    
    // Continue with existing processing...
    // ...
}
```

## Unit Tests

**File**: `internal/filter/cascade/node_test.go`

```go
package cascade

import (
    "context"
    "testing"
    "time"

    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"

    "github.com/lenaxia/k8s-mendabot/internal/domain"
    "github.com/lenaxia/k8s-mendabot/internal/filter/nodehealth"
)

func TestNodeCascadeDetector(t *testing.T) {
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
    cache := nodehealth.NewCache(client, 1*time.Minute)
    nodeFilter := nodehealth.NewFilter(cache)
    detector := NewNodeCascadeDetector(client, nodeFilter)
    
    // Test pod on not-ready node
    finding := domain.Finding{
        Kind:      "Pod",
        Name:      "test-pod",
        Namespace: "default",
        NodeName:  "not-ready-node",
    }
    
    detection, err := detector.Check(ctx, finding)
    if err != nil {
        t.Fatalf("Check error: %v", err)
    }
    
    if detection == nil {
        t.Fatal("Check returned nil detection for pod on not-ready node")
    }
    
    if detection.Type != NodeCascade {
        t.Errorf("Detection.Type = %v, want node", detection.Type)
    }
    
    if detection.RootCause != "not-ready-node" {
        t.Errorf("Detection.RootCause = %v, want not-ready-node", detection.RootCause)
    }
    
    if detection.Confidence < 0.5 {
        t.Errorf("Detection.Confidence = %v, want >= 0.5", detection.Confidence)
    }
    
    // Test ShouldSuppress
    suppress, reason, err := detector.ShouldSuppress(ctx, finding)
    if err != nil {
        t.Fatalf("ShouldSuppress error: %v", err)
    }
    
    if !suppress {
        t.Error("ShouldSuppress = false, want true for pod on not-ready node")
    }
    
    if reason == "" {
        t.Error("ShouldSuppress reason empty, want non-empty")
    }
    
    // Test GetRootCauseFinding
    rootFinding, err := detector.GetRootCauseFinding(ctx, *detection)
    if err != nil {
        t.Fatalf("GetRootCauseFinding error: %v", err)
    }
    
    if rootFinding.Kind != "Node" {
        t.Errorf("RootFinding.Kind = %v, want Node", rootFinding.Kind)
    }
    
    if rootFinding.Name != "not-ready-node" {
        t.Errorf("RootFinding.Name = %v, want not-ready-node", rootFinding.Name)
    }
    
    if rootFinding.Tier != domain.TierInfrastructure {
        t.Errorf("RootFinding.Tier = %v, want infrastructure", rootFinding.Tier)
    }
}
```

## Configuration

**File**: `internal/config/config.go`

Add cascade detection configuration:

```go
type Config struct {
    // ... existing fields ...
    
    // Cascade detection
    EnableCascadeDetection bool `env:"ENABLE_CASCADE_DETECTION" envDefault:"true"`
    
    // Namespace failure threshold (percentage)
    NamespaceFailureThreshold int `env:"NAMESPACE_FAILURE_THRESHOLD" envDefault:"50"`
    
    // Cascade manager instance (not from env)
    cascadeManager *cascade.Manager
}

// CascadeManager returns the cascade manager, creating it if necessary.
func (c *Config) CascadeManager(client client.Client, nodeFilter *nodehealth.Filter) *cascade.Manager {
    if c.cascadeManager == nil && c.EnableCascadeDetection {
        c.cascadeManager = cascade.NewManager(client, nodeFilter, true)
    }
    return c.cascadeManager
}
```

**File**: `charts/mendabot/values.yaml`

Add configuration values:

```yaml
# Cascade detection
cascade:
  # Enable infrastructure cascade detection
  enabled: true
  # Namespace failure threshold percentage
  namespaceFailureThreshold: 50
```

## Verification Steps

1. **Unit Tests**: Run `go test -timeout 30s ./internal/filter/cascade/...`
2. **Integration Tests**: Test with simulated node failure
3. **Performance Test**: Verify cascade detection doesn't add significant latency
4. **Backward Compatibility**: Existing tests should still pass

## Expected Outcomes

1. **Reduced Investigations**: Multiple symptoms of same root cause trigger single investigation
2. **Root Cause Focus**: Investigations target infrastructure issues instead of symptoms
3. **Correlation**: Related findings grouped appropriately
4. **Audit Trail**: Cascade detection events logged

## Metrics

New Prometheus metrics:
- `mendabot_cascade_detections_total{type="node|namespace|storage"}`
- `mendabot_findings_suppressed_total{reason="cascade"}`
- `mendabot_namespace_failure_rate{namespace}`

## Next Steps

After this story is complete:
- STORY_03: Implement failure analysis CRD
- STORY_04: Implement smart PR management
- STORY_05: Implement enhanced metrics

This story significantly reduces investigation volume during infrastructure outages.