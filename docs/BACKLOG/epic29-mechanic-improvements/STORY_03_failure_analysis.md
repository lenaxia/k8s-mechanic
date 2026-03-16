# STORY_03: Failure Analysis CRD and Artifact Collection

## Purpose
Preserve investigation artifacts and create structured failure analysis for debugging failed remediation jobs.

## Status: Not Started

## Dependencies
- STORY_00 (domain types) - for classification fields
- epic11-self-remediation-cascade - for circuit breaker foundation

## Acceptance Criteria
- [ ] `FailureAnalysis` CRD created and deployed
- [ ] Failed remediation jobs automatically create FailureAnalysis resources
- [ ] Investigation artifacts (logs, k8s snapshots) preserved for failed jobs
- [ ] Failure classification (timeout, permission, LLM, etc.) implemented
- [ ] Artifact retention configurable (default: 24 hours)
- [ ] FailureAnalysis controller cleans up old artifacts
- [ ] All unit tests pass
- [ ] Worklog entry created

## Problem
When remediation jobs fail (3 failures observed in production), there's no post-mortem data:
1. Job logs are lost after pod termination
2. No structured analysis of failure root cause
3. Operators can't debug why investigations failed
4. No historical data for pattern analysis

## Solution
Create a `FailureAnalysis` CRD that captures structured failure data and preserves investigation artifacts.

### Technical Design

#### 1. FailureAnalysis CRD
```yaml
# api/v1alpha1/failureanalysis_types.go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FailureAnalysis is the Schema for the failureanalyses API
type FailureAnalysis struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FailureAnalysisSpec   `json:"spec,omitempty"`
	Status FailureAnalysisStatus `json:"status,omitempty"`
}

type FailureAnalysisSpec struct {
	// RemediationJobRef references the failed RemediationJob
	RemediationJobRef string `json:"remediationJobRef"`

	// FailureType classification
	FailureType FailureType `json:"failureType"`

	// OriginalFinding that triggered the investigation
	OriginalFinding FindingReference `json:"originalFinding,omitempty"`

	// ArtifactRefs to preserved investigation artifacts
	ArtifactRefs []ArtifactRef `json:"artifactRefs,omitempty"`
}

type FailureAnalysisStatus struct {
	// AnalysisComplete indicates if failure analysis is complete
	AnalysisComplete bool `json:"analysisComplete"`

	// RootCause identified by analysis
	RootCause string `json:"rootCause,omitempty"`

	// Recommendations for preventing similar failures
	Recommendations []string `json:"recommendations,omitempty"`

	// Created timestamp
	Created metav1.Time `json:"created"`

	// Expires timestamp for cleanup
	Expires metav1.Time `json:"expires"`
}

type FailureType string

const (
	FailureTypeTimeout    FailureType = "timeout"
	FailureTypePermission FailureType = "permission"
	FailureTypeLLM        FailureType = "llm"
	FailureTypeNetwork    FailureType = "network"
	FailureTypeUnknown    FailureType = "unknown"
)

type FindingReference struct {
	// Namespace of the original finding
	Namespace string `json:"namespace"`

	// Name of the finding resource
	Name string `json:"name"`

	// Tier classification
	Tier FindingTier `json:"tier,omitempty"`
}

type ArtifactRef struct {
	// Type of artifact (log, snapshot, config, etc.)
	Type string `json:"type"`

	// Location where artifact is stored
	Location string `json:"location"`

	// Size in bytes
	Size int64 `json:"size,omitempty"`
}
```

#### 2. Artifact Collection
```go
// internal/failure/artifact.go
package failure

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/yourorg/mendabot/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type ArtifactCollector struct {
	k8sClient kubernetes.Interface
	config    ArtifactConfig
}

type ArtifactConfig struct {
	// Enabled controls artifact collection
	Enabled bool

	// RetentionHours how long to keep artifacts
	RetentionHours int

	// StorageBackend where to store artifacts
	StorageBackend string // "local", "s3", "gcs"

	// StoragePath base path for artifacts
	StoragePath string
}

func (c *ArtifactCollector) CollectForJob(ctx context.Context, job *v1alpha1.RemediationJob) (*v1alpha1.FailureAnalysis, error) {
	if !c.config.Enabled || job.Status.Phase != v1alpha1.RemediationJobPhaseFailed {
		return nil, nil
	}

	// Create artifact directory
	artifactDir := filepath.Join(c.config.StoragePath, job.Namespace, job.Name)
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create artifact directory: %w", err)
	}

	// Collect pod logs
	podLogs, err := c.collectPodLogs(ctx, job)
	if err != nil {
		return nil, fmt.Errorf("failed to collect pod logs: %w", err)
	}

	// Collect k8s resource snapshots
	snapshots, err := c.collectResourceSnapshots(ctx, job)
	if err != nil {
		return nil, fmt.Errorf("failed to collect resource snapshots: %w", err)
	}

	// Determine failure type
	failureType := c.classifyFailure(job)

	// Create FailureAnalysis
	analysis := &v1alpha1.FailureAnalysis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-failure", job.Name),
			Namespace: job.Namespace,
			Labels: map[string]string{
				"remediation-job": job.Name,
				"failure-type":    string(failureType),
			},
		},
		Spec: v1alpha1.FailureAnalysisSpec{
			RemediationJobRef: fmt.Sprintf("%s/%s", job.Namespace, job.Name),
			FailureType:       failureType,
			OriginalFinding: v1alpha1.FindingReference{
				Namespace: job.Spec.FindingRef.Namespace,
				Name:      job.Spec.FindingRef.Name,
				Tier:      job.Spec.FindingTier,
			},
			ArtifactRefs: append(podLogs, snapshots...),
		},
		Status: v1alpha1.FailureAnalysisStatus{
			Created:          metav1.Now(),
			Expires:          metav1.NewTime(time.Now().Add(time.Duration(c.config.RetentionHours) * time.Hour)),
			AnalysisComplete: false,
		},
	}

	return analysis, nil
}

func (c *ArtifactCollector) classifyFailure(job *v1alpha1.RemediationJob) v1alpha1.FailureType {
	// Analyze job status and conditions to determine failure type
	if job.Status.Message != "" {
		switch {
		case strings.Contains(job.Status.Message, "timeout"):
			return v1alpha1.FailureTypeTimeout
		case strings.Contains(job.Status.Message, "permission denied"):
			return v1alpha1.FailureTypePermission
		case strings.Contains(job.Status.Message, "LLM"):
			return v1alpha1.FailureTypeLLM
		case strings.Contains(job.Status.Message, "network"):
			return v1alpha1.FailureTypeNetwork
		}
	}
	return v1alpha1.FailureTypeUnknown
}
```

#### 3. FailureAnalysis Controller
```go
// internal/failure/controller.go
package failure

import (
	"context"
	"fmt"
	"time"

	"github.com/yourorg/mendabot/api/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type FailureAnalysisReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	artifactCollector *ArtifactCollector
}

func (r *FailureAnalysisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("failureanalysis", req.NamespacedName)

	// Get the FailureAnalysis
	var analysis v1alpha1.FailureAnalysis
	if err := r.Get(ctx, req.NamespacedName, &analysis); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get FailureAnalysis: %w", err)
	}

	// Check if expired
	if analysis.Status.Expires.Time.Before(time.Now()) {
		log.Info("FailureAnalysis expired, cleaning up")
		if err := r.cleanupArtifacts(&analysis); err != nil {
			log.Error(err, "failed to cleanup artifacts")
			return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
		}
		if err := r.Delete(ctx, &analysis); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete expired FailureAnalysis: %w", err)
		}
		return ctrl.Result{}, nil
	}

	// Perform analysis if not complete
	if !analysis.Status.AnalysisComplete {
		if err := r.performAnalysis(ctx, &analysis); err != nil {
			log.Error(err, "failed to perform analysis")
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
		}
	}

	// Requeue for expiration check
	requeueAfter := time.Until(analysis.Status.Expires.Time)
	if requeueAfter < 0 {
		requeueAfter = 0
	}
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (r *FailureAnalysisReconciler) performAnalysis(ctx context.Context, analysis *v1alpha1.FailureAnalysis) error {
	// Analyze artifacts to determine root cause
	rootCause, recommendations := r.analyzeArtifacts(analysis)

	// Update status
	analysis.Status.RootCause = rootCause
	analysis.Status.Recommendations = recommendations
	analysis.Status.AnalysisComplete = true

	if err := r.Status().Update(ctx, analysis); err != nil {
		return fmt.Errorf("failed to update FailureAnalysis status: %w", err)
	}

	r.Log.Info("Failure analysis complete",
		"failureType", analysis.Spec.FailureType,
		"rootCause", rootCause)

	return nil
}

func (r *FailureAnalysisReconciler) analyzeArtifacts(analysis *v1alpha1.FailureAnalysis) (string, []string) {
	// Simple rule-based analysis based on failure type
	switch analysis.Spec.FailureType {
	case v1alpha1.FailureTypeTimeout:
		return "Investigation timed out", []string{
			"Consider increasing investigation timeout",
			"Check for resource constraints on investigation pods",
			"Review investigation complexity",
		}
	case v1alpha1.FailureTypePermission:
		return "Permission denied accessing resources", []string{
			"Review RBAC permissions for mechanic service account",
			"Check namespace-level restrictions",
			"Verify resource exists and is accessible",
		}
	case v1alpha1.FailureTypeLLM:
		return "LLM processing failed", []string{
			"Check LLM provider connectivity and API keys",
			"Review prompt complexity and token limits",
			"Consider implementing prompt optimization",
		}
	case v1alpha1.FailureTypeNetwork:
		return "Network connectivity issue", []string{
			"Check network policies and service mesh configuration",
			"Verify DNS resolution within cluster",
			"Review egress firewall rules",
		}
	default:
		return "Unknown failure type", []string{
			"Review investigation logs for patterns",
			"Check cluster events around failure time",
			"Consider enabling debug logging",
		}
	}
}
```

#### 4. Integration with RemediationJob Controller
```go
// internal/controller/remediationjob_controller.go (add to existing)
func (r *RemediationJobReconciler) handleFailedJob(ctx context.Context, job *v1alpha1.RemediationJob) error {
	if r.artifactCollector != nil && r.artifactCollector.config.Enabled {
		analysis, err := r.artifactCollector.CollectForJob(ctx, job)
		if err != nil {
			r.Log.Error(err, "failed to collect artifacts for failed job")
		} else if analysis != nil {
			if err := r.Create(ctx, analysis); err != nil {
				r.Log.Error(err, "failed to create FailureAnalysis")
			} else {
				r.Log.Info("created FailureAnalysis for failed job",
					"failureAnalysis", analysis.Name,
					"failureType", analysis.Spec.FailureType)
			}
		}
	}
	return nil
}
```

### Configuration
```yaml
# charts/mendabot/values.yaml
failureAnalysis:
  enabled: true
  retentionHours: 24
  storageBackend: "local"
  storagePath: "/var/run/mendabot/artifacts"
  
  # S3 configuration (optional)
  s3:
    enabled: false
    bucket: "mendabot-artifacts"
    region: "us-east-1"
    prefix: "failures/"
```

### New Files
| File | Purpose |
|------|---------|
| `api/v1alpha1/failureanalysis_types.go` | FailureAnalysis CRD types |
| `config/crd/bases/failureanalysis.yaml` | CRD manifest |
| `internal/failure/artifact.go` | Artifact collection logic |
| `internal/failure/controller.go` | FailureAnalysis controller |
| `internal/failure/analysis.go` | Analysis logic |
| `internal/failure/cleanup.go` | Artifact cleanup |

### Modified Files
| File | Change |
|------|--------|
| `api/v1alpha1/groupversion_info.go` | Add FailureAnalysis to scheme |
| `internal/controller/remediationjob_controller.go` | Create FailureAnalysis on job failure |
| `config/rbac/role.yaml` | Add permissions for FailureAnalysis |
| `charts/mendabot/templates/crds.yaml` | Add FailureAnalysis CRD |
| `charts/mendabot/values.yaml` | Add failure analysis configuration |

### Testing Strategy
1. **Unit Tests**: Test artifact collection and failure classification
2. **Integration Tests**: Test FailureAnalysis CRD lifecycle
3. **E2E Tests**: Full workflow from failed job to analysis

### Migration Notes
- Feature disabled by default (`failureAnalysis.enabled: false`)
- No impact on existing deployments
- Artifacts stored locally by default (configurable)

### Success Metrics
- 100% of failed jobs have FailureAnalysis created
- Artifacts preserved for configured retention period
- Root cause analysis completed within 5 minutes
- No storage leaks (artifacts cleaned up after expiration)