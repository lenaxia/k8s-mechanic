# Epic 29: Mechanic Operational Improvements

## Purpose

Based on operational analysis of mechanic performance in production, this epic addresses key weaknesses identified in the system:
1. **False positive handling**: Mechanic creates PR noise for infrastructure issues it can't fix
2. **Wasted investigation cycles**: Investigations launched for known transient patterns and infrastructure cascades
3. **Poor failure analysis**: Failed investigations leave no actionable post-mortem data
4. **PR noise**: Documentation-only "fixes" and infrastructure issues create alert fatigue

This epic implements a **tiered response system** with smart filtering, enhanced correlation, and improved PR management to make mechanic more effective, efficient, and operator-friendly.

## Status: Not Started

## Dependencies

- epic01-controller complete (SourceProviderReconciler foundation)
- epic09-native-provider complete (native provider detection logic)
- epic11-self-remediation-cascade complete (circuit breaker foundation)
- epic24-severity-tiers complete (severity classification)
- epic13-multi-signal-correlation complete (correlation engine)

## Blocks

- Improves accuracy and precision for all downstream features
- Reduces operational overhead for cluster operators

## Success Criteria

- [ ] **Tiered response system** implemented with 3-tier classification
- [ ] **Node health correlation** skips investigations on unhealthy nodes
- [ ] **Infrastructure cascade detection** groups findings from failed nodes
- [ ] **Failure artifact collection** preserves logs and snapshots for debugging
- [ ] **Smart PR management** with auto-close for infrastructure issues
- [ ] **Enhanced metrics** for detection-to-fix timeline and success rates
- [ ] **50% reduction in PR noise** for infrastructure issues
- [ ] **30% reduction in wasted investigations** via smart filtering
- [ ] All changes backward compatible with existing deployments
- [ ] `go test -timeout 30s -race ./...` passes
- [ ] Worklog entry created

## Problem Analysis

### Current Issues Identified

1. **False Positives**: PR #1358 (babybuddy) exposed fundamental flaw - mechanic opened PR saying "no fix needed, it's infrastructure"
2. **Infrastructure Blindness**: Can't distinguish GitOps-fixable vs infrastructure issues
3. **Wasted Cycles**: Investigations launched for pods on unhealthy nodes (worker-02 NotReady)
4. **Poor Failure Analysis**: 3 failed jobs with no logs or post-mortem data
5. **PR Noise**: Documentation-only "fixes" create alert fatigue

### Root Causes

1. **Single-tier response**: All findings treated equally regardless of fixability
2. **Lack of context**: Investigations launched without node health correlation
3. **No failure preservation**: Job artifacts cleaned up before analysis
4. **Binary outcome**: PR or nothing, no middle ground for infrastructure issues

## Solution Architecture

### 1. Tiered Response System

```
Tier 1: Auto-fixable (GitOps config issues) → Create PR
Tier 2: Infrastructure issues (node failures, hardware) → Alert only, no PR
Tier 3: Transient/self-healing → Suppress with short TTL
```

**Implementation**:
- Add `tier` field to `Finding` struct (`auto-fixable`, `infrastructure`, `transient`)
- Classify based on error patterns (node failures, network issues vs config errors)
- Tier 2 findings create Kubernetes Events or Prometheus alerts instead of PRs
- Simple rule-based classification (no ML initially)

### 2. Enhanced Filtering & Correlation

**Node Health Correlation**:
- Skip investigations for resources on `NotReady` nodes
- Group all findings from same failed node into single investigation
- Add node health check before investigation dispatch

**Infrastructure Cascade Detection**:
- Detect when >50% of pods in namespace are failing simultaneously
- Emit single `namespace-wide degradation` finding instead of individual pod findings
- Skip individual pod investigations during infrastructure cascades

**Known Pattern Suppression**:
- Configurable skip patterns (regex-based on error messages)
- Common transient patterns: `NodeNotReady`, `ImagePullBackOff` (transient), scaling events
- Cost-benefit analysis before investigation

### 3. Improved Failure Handling

**Structured Failure Analysis**:
- Add `FailureAnalysis` CRD to capture post-mortem data
- Preserve investigation artifacts: logs, k8s snapshots for failed jobs
- Failure classification: timeout, permission, LLM, etc.

**Smart Retry Logic**:
- Exponential backoff with classification-based decisions
- Different retry strategies for different failure types
- Permanent failure marking after classification-specific retry limits

### 4. Smarter PR Management

**Outcome-based PR Handling**:
- **Fix PRs**: Actual manifest changes (merge automatically if confidence high)
- **Investigation PRs**: Root cause analysis (label: `needs-human-review`)
- **Documentation PRs**: Infrastructure issues (auto-close with comment)
- **Auto-close logic**: Close PRs when underlying issue resolves

**PR Type Classification**:
- Add PR type field in agent output
- Implement auto-close webhook for resolved issues
- Add PR templates for each outcome type

### 5. Enhanced Metrics & Observability

**Comprehensive Metrics Dashboard**:
- **Detection-to-fix timeline**: Measure end-to-end remediation time
- **Success rate by issue type**: Track what mechanic is good/bad at fixing
- **Cost metrics**: LLM token usage, compute time per investigation
- **Value metrics**: Issues prevented, MTTR reduction

**Implementation**:
- Extend Prometheus metrics with business-level indicators
- Add Grafana dashboard templates
- Implement cost tracking for LLM usage

## Technical Design

### New Types and Interfaces

```go
// Tier classification for findings
type FindingTier string

const (
    TierAutoFixable    FindingTier = "auto-fixable"
    TierInfrastructure FindingTier = "infrastructure"
    TierTransient      FindingTier = "transient"
)

// Extended Finding with tier and classification
type EnhancedFinding struct {
    domain.Finding
    Tier            FindingTier
    Classification  string // e.g., "node-failure", "config-error", "resource-exhaustion"
    FixConfidence   float64 // 0.0-1.0 confidence in auto-fixability
}

// Failure analysis for post-mortem
type FailureAnalysis struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    
    Spec FailureAnalysisSpec `json:"spec,omitempty"`
}

type FailureAnalysisSpec struct {
    RemediationJobRef string `json:"remediationJobRef"`
    FailureType       string `json:"failureType"` // timeout, permission, llm, etc.
    ArtifactRefs      []ArtifactRef `json:"artifactRefs,omitempty"`
    RootCause         string `json:"rootCause,omitempty"`
    Recommendations   []string `json:"recommendations,omitempty"`
}
```

### Modified Components

1. **SourceProvider Enhancements**:
   - Add tier classification to `ExtractFinding`
   - Add node health correlation check
   - Add infrastructure cascade detection

2. **SourceProviderReconciler**:
   - Skip dispatch for Tier 2/3 findings
   - Create Kubernetes Events for infrastructure issues
   - Enhanced correlation with node health

3. **Agent Prompt Updates**:
   - Include tier and classification in prompt
   - Different instructions per tier
   - Infrastructure issues → investigation report only

4. **Sink (GitHub) Enhancements**:
   - PR type classification
   - Auto-close logic for infrastructure issues
   - Better PR templates per outcome

## Configuration

```bash
# --- Tiered Response System ---
# Enable tiered response system (default: true)
TIERED_RESPONSE_ENABLED=true

# Minimum confidence for auto-fix attempts (0.0-1.0)
AUTO_FIX_MIN_CONFIDENCE=0.7

# --- Node Health Correlation ---
# Skip investigations on NotReady nodes (default: true)
SKIP_UNHEALTHY_NODES=true

# Node health cache TTL in seconds (default: 30)
NODE_HEALTH_CACHE_TTL=30

# --- Infrastructure Cascade Detection ---
# Enable cascade detection (default: true)
INFRASTRUCTURE_CASCADE_DETECTION=true

# Namespace failure threshold percentage (default: 50)
NAMESPACE_FAILURE_THRESHOLD=50

# --- Failure Analysis ---
# Preserve failure artifacts (default: true)
PRESERVE_FAILURE_ARTIFACTS=true

# Artifact retention period in hours (default: 24)
ARTIFACT_RETENTION_HOURS=24

# --- PR Management ---
# Auto-close infrastructure issue PRs (default: true)
AUTO_CLOSE_INFRASTRUCTURE_PR=true

# PR type classification (default: true)
PR_TYPE_CLASSIFICATION=true
```

## Stories

| Story | File | Status | Priority | Effort |
|-------|------|--------|----------|--------|
| Tiered response domain types and classification | [STORY_00_domain_types.md](STORY_00_domain_types.md) | Not Started | High | 2h |
| Node health correlation and filtering | [STORY_01_node_health_correlation.md](STORY_01_node_health_correlation.md) | Not Started | High | 3h |
| Infrastructure cascade detection | [STORY_02_infrastructure_cascade.md](STORY_02_infrastructure_cascade.md) | Not Started | High | 3h |
| Failure analysis CRD and artifact collection | [STORY_03_failure_analysis.md](STORY_03_failure_analysis.md) | Not Started | Medium | 4h |
| Smart PR management and auto-close | [STORY_04_pr_management.md](STORY_04_pr_management.md) | Not Started | High | 3h |
| Enhanced metrics and observability | [STORY_05_metrics_observability.md](STORY_05_metrics_observability.md) | Not Started | Medium | 2h |
| Configuration, deployment, and Helm values | [STORY_06_config_deploy.md](STORY_06_config_deploy.md) | Not Started | Medium | 2h |

## Story Execution Order

STORY_00 must run first (domain foundation). STORY_01 and STORY_02 can run in parallel. STORY_03 depends on STORY_00. STORY_04 depends on STORY_00 and STORY_01. STORY_05 can run anytime after STORY_00. STORY_06 closes the epic.

```
STORY_00 (domain types)
    ├──> STORY_01 (node health)
    ├──> STORY_02 (cascade detection)
    ├──> STORY_03 (failure analysis)
    └──> STORY_04 (PR management)
               └──> STORY_05 (metrics)
                         └──> STORY_06 (config + deploy)
```

## Technical Overview

### New Files

| File | Purpose |
|------|---------|
| `internal/domain/tier.go` | Tier classification types and constants |
| `internal/domain/classification.go` | Finding classification logic |
| `internal/filter/nodehealth.go` | Node health correlation filter |
| `internal/filter/cascade.go` | Infrastructure cascade detection |
| `internal/failure/analysis.go` | Failure analysis types and controller |
| `internal/failure/artifact.go` | Artifact collection and preservation |
| `internal/sink/enhanced.go` | Enhanced sink with PR type classification |
| `internal/metrics/business.go` | Business-level metrics collection |

### Modified Files

| File | Change |
|------|--------|
| `api/v1alpha1/remediationjob_types.go` | Add tier, classification, confidence fields |
| `api/v1alpha1/failureanalysis_types.go` | New CRD for failure analysis |
| `internal/domain/finding.go` | Add tier and classification fields |
| `internal/provider/native/*.go` | Enhanced classification in ExtractFinding |
| `internal/provider/provider.go` | Tier-based dispatch logic |
| `internal/sink/github/sink.go` | PR type classification and auto-close |
| `internal/config/config.go` | Add new configuration fields |
| `charts/mendabot/templates/*.yaml` | Add new env vars and CRDs |
| `charts/mendabot/values.yaml` | New configuration section |

## Definition of Done

- [ ] All unit tests pass: `go test -timeout 30s -race ./...`
- [ ] `go build ./...` succeeds
- [ ] Tiered response system reduces PR noise by 50% in test environment
- [ ] Node health correlation skips investigations on NotReady nodes
- [ ] Infrastructure cascade detection groups related findings
- [ ] Failure artifacts preserved for debugging
- [ ] PR auto-close works for infrastructure issues
- [ ] Enhanced metrics visible in Prometheus
- [ ] All changes backward compatible (existing deployments continue working)
- [ ] Documentation updated for new features
- [ ] Worklog entry created in `docs/WORKLOGS/`

## Expected Outcomes

| Improvement | Impact | Complexity | ROI |
|------------|--------|------------|-----|
| Tiered Response | 50% reduction in PR noise | Low | High |
| Node Correlation | 30% fewer wasted investigations | Low | High |
| Smart PR Management | Better signal-to-noise ratio | Medium | High |
| Failure Analysis | Faster debugging of failures | Medium | Medium |
| Enhanced Metrics | Better operational visibility | Low | Medium |

## Migration Path

1. **Phase 1 (Backward Compatible)**: New features disabled by default
2. **Phase 2 (Opt-in)**: Operators enable features via configuration
3. **Phase 3 (Default-on)**: Features enabled by default in next major version
4. **Phase 4 (Deprecation)**: Old behavior deprecated with migration guide

All changes are designed to be **progressive enhancements** that don't break existing workflows.