# STORY_05: Enhanced Metrics and Observability

## Purpose
Implement comprehensive business-level metrics and observability to track mechanic performance, value, and costs.

## Status: Not Started

## Dependencies
- STORY_00 (domain types) - for tier classification metrics
- STORY_01 (node health) - for filtering effectiveness metrics
- STORY_04 (PR management) - for PR outcome metrics

## Acceptance Criteria
- [ ] Business-level metrics implemented (detection-to-fix timeline, success rates)
- [ ] Cost metrics tracking (LLM tokens, compute time)
- [ ] Value metrics tracking (issues prevented, MTTR reduction)
- [ ] Enhanced Prometheus metrics with labels for tier, classification, outcome
- [ ] Grafana dashboard templates created
- [ ] All metrics documented
- [ ] All unit tests pass
- [ ] Worklog entry created

## Problem
Current metrics are technical but lack business context:
1. No end-to-end timeline tracking (detection → investigation → fix)
2. Can't measure what mechanic is good/bad at fixing
3. No cost tracking for LLM usage and compute resources
4. No value metrics to demonstrate ROI
5. Operators can't easily see performance trends

## Solution
Implement enhanced metrics across three categories:
1. **Performance Metrics**: Detection-to-fix timeline, success rates by issue type
2. **Cost Metrics**: LLM token usage, compute time, investigation cost
3. **Value Metrics**: Issues prevented, MTTR reduction, operational efficiency

### Technical Design

#### 1. Enhanced Metrics Types
```go
// internal/metrics/business.go
package metrics

import (
	"time"

	"github.com/yourorg/mendabot/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type BusinessMetrics struct {
	// Performance Metrics
	detectionToFixDuration *prometheus.HistogramVec
	investigationDuration  *prometheus.HistogramVec
	successRateByTier      *prometheus.GaugeVec
	successRateByType      *prometheus.GaugeVec
	
	// Cost Metrics
	llmTokensUsed          *prometheus.CounterVec
	investigationCost      *prometheus.CounterVec
	computeTimeSeconds     *prometheus.CounterVec
	
	// Value Metrics
	issuesPrevented        *prometheus.CounterVec
	mttrReductionSeconds   *prometheus.GaugeVec
	operationalEfficiency  *prometheus.GaugeVec
	
	// Filtering Effectiveness
	investigationsSkipped  *prometheus.CounterVec
	falsePositiveRate      *prometheus.GaugeVec
}

func NewBusinessMetrics() *BusinessMetrics {
	reg := prometheus.DefaultRegisterer
	
	return &BusinessMetrics{
		// Performance Metrics
		detectionToFixDuration: promauto.With(reg).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "mendabot_detection_to_fix_duration_seconds",
				Help:    "Time from detection to fix completion",
				Buckets: prometheus.ExponentialBuckets(60, 2, 10), // 1min to ~8.5hrs
			},
			[]string{"tier", "classification", "outcome"},
		),
		
		investigationDuration: promauto.With(reg).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "mendabot_investigation_duration_seconds",
				Help:    "Time spent on investigation",
				Buckets: prometheus.ExponentialBuckets(30, 2, 8), // 30s to ~64min
			},
			[]string{"tier", "classification", "success"},
		),
		
		successRateByTier: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "mendabot_success_rate_by_tier",
				Help: "Success rate by finding tier (0.0-1.0)",
			},
			[]string{"tier"},
		),
		
		successRateByType: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "mendabot_success_rate_by_type",
				Help: "Success rate by issue classification",
			},
			[]string{"classification"},
		),
		
		// Cost Metrics
		llmTokensUsed: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "mendabot_llm_tokens_total",
				Help: "Total LLM tokens used across all investigations",
			},
			[]string{"model", "tier"},
		),
		
		investigationCost: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "mendabot_investigation_cost_usd",
				Help: "Estimated cost of investigations in USD",
			},
			[]string{"tier", "classification"},
		),
		
		computeTimeSeconds: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "mendabot_compute_time_seconds_total",
				Help: "Total compute time spent on investigations",
			},
			[]string{"tier"},
		),
		
		// Value Metrics
		issuesPrevented: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "mendabot_issues_prevented_total",
				Help: "Total number of issues prevented",
			},
			[]string{"severity", "tier"},
		),
		
		mttrReductionSeconds: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "mendabot_mttr_reduction_seconds",
				Help: "Mean Time To Resolution reduction in seconds",
			},
			[]string{"classification"},
		),
		
		operationalEfficiency: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "mendabot_operational_efficiency",
				Help: "Operational efficiency score (0.0-1.0)",
			},
			[]string{"namespace"},
		),
		
		// Filtering Effectiveness
		investigationsSkipped: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "mendabot_investigations_skipped_total",
				Help: "Total investigations skipped due to filtering",
			},
			[]string{"reason", "tier"},
		),
		
		falsePositiveRate: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "mendabot_false_positive_rate",
				Help: "False positive rate (0.0-1.0)",
			},
			[]string{"classification"},
		),
	}
}

// RecordDetectionToFix records the end-to-end timeline
func (m *BusinessMetrics) RecordDetectionToFix(
	finding *v1alpha1.Finding,
	result *v1alpha1.RemediationJobResult,
	detectionTime time.Time,
	completionTime time.Time,
) {
	duration := completionTime.Sub(detectionTime).Seconds()
	
	m.detectionToFixDuration.WithLabelValues(
		string(finding.Tier),
		finding.Classification,
		string(result.Outcome),
	).Observe(duration)
	
	// Update success rates
	m.updateSuccessRates(finding, result)
	
	// Record value metrics if successful
	if result.Outcome == v1alpha1.RemediationOutcomeFixed {
		m.recordValueMetrics(finding, result, duration)
	}
}

// RecordInvestigation records investigation metrics
func (m *BusinessMetrics) RecordInvestigation(
	job *v1alpha1.RemediationJob,
	startTime time.Time,
	endTime time.Time,
	success bool,
	tokensUsed int,
) {
	duration := endTime.Sub(startTime).Seconds()
	
	m.investigationDuration.WithLabelValues(
		string(job.Spec.FindingTier),
		job.Spec.Classification,
		boolToString(success),
	).Observe(duration)
	
	// Record cost metrics
	m.recordCostMetrics(job, duration, tokensUsed)
	
	// Record compute time
	m.computeTimeSeconds.WithLabelValues(
		string(job.Spec.FindingTier),
	).Add(duration)
}

// RecordFilteringEffectiveness records when investigations are skipped
func (m *BusinessMetrics) RecordFilteringEffectiveness(
	reason string,
	tier v1alpha1.FindingTier,
	classification string,
) {
	m.investigationsSkipped.WithLabelValues(reason, string(tier)).Inc()
	
	// Update false positive rate for certain skip reasons
	if reason == "node_unhealthy" || reason == "infrastructure_cascade" {
		m.updateFalsePositiveRate(classification)
	}
}

func (m *BusinessMetrics) recordCostMetrics(
	job *v1alpha1.RemediationJob,
	duration float64,
	tokensUsed int,
) {
	// Calculate estimated cost
	// LLM cost: $0.002 per 1K tokens (GPT-4 example)
	llmCost := float64(tokensUsed) * 0.002 / 1000
	
	// Compute cost: $0.0001 per second (example)
	computeCost := duration * 0.0001
	
	totalCost := llmCost + computeCost
	
	m.llmTokensUsed.WithLabelValues("gpt-4", string(job.Spec.FindingTier)).Add(float64(tokensUsed))
	m.investigationCost.WithLabelValues(
		string(job.Spec.FindingTier),
		job.Spec.Classification,
	).Add(totalCost)
}

func (m *BusinessMetrics) recordValueMetrics(
	finding *v1alpha1.Finding,
	result *v1alpha1.RemediationJobResult,
	duration float64,
) {
	// Record issue prevented
	m.issuesPrevented.WithLabelValues(
		string(finding.Severity),
		string(finding.Tier),
	).Inc()
	
	// Calculate MTTR reduction (baseline - actual)
	// Baseline MTTR for this issue type (could be configurable)
	baselineMTTR := map[string]float64{
		"config-error":    3600, // 1 hour baseline
		"resource-limit":  1800, // 30 minutes baseline
		"node-failure":    7200, // 2 hours baseline
	}
	
	if baseline, ok := baselineMTTR[finding.Classification]; ok {
		reduction := baseline - duration
		if reduction > 0 {
			m.mttrReductionSeconds.WithLabelValues(finding.Classification).Set(reduction)
		}
	}
	
	// Update operational efficiency
	// Simple formula: successful fixes / total investigations for namespace
	// This would be calculated periodically, not per event
}

func (m *BusinessMetrics) updateSuccessRates(
	finding *v1alpha1.Finding,
	result *v1alpha1.RemediationJobResult,
) {
	// This would track success rates over time
	// Implementation would maintain counters and calculate rates periodically
	// For now, placeholder implementation
	success := result.Outcome == v1alpha1.RemediationOutcomeFixed
	
	// Update tier success rate
	tierRate := m.calculateSuccessRate(finding.Tier, success)
	m.successRateByTier.WithLabelValues(string(finding.Tier)).Set(tierRate)
	
	// Update type success rate
	typeRate := m.calculateSuccessRate(finding.Classification, success)
	m.successRateByType.WithLabelValues(finding.Classification).Set(typeRate)
}

func (m *BusinessMetrics) updateFalsePositiveRate(classification string) {
	// Update false positive rate for this classification
	// Implementation would track false positives vs true positives
}
```

#### 2. Metrics Collection Integration
```go
// internal/controller/metrics_integration.go
package controller

import (
	"context"
	"time"

	"github.com/yourorg/mendabot/api/v1alpha1"
	"github.com/yourorg/mendabot/internal/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type MetricsIntegration struct {
	metrics *metrics.BusinessMetrics
}

func (m *MetricsIntegration) WrapReconciler(reconciler reconcile.Reconciler) reconcile.Reconciler {
	return &metricsReconciler{
		Reconciler: reconciler,
		metrics:    m.metrics,
	}
}

type metricsReconciler struct {
	reconcile.Reconciler
	metrics *metrics.BusinessMetrics
}

func (r *metricsReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	startTime := time.Now()
	
	// Call wrapped reconciler
	result, err := r.Reconciler.Reconcile(ctx, req)
	
	// Record metrics
	duration := time.Since(startTime).Seconds()
	
	// This would need context about what was reconciled
	// For now, placeholder implementation
	
	return result, err
}

// Integration with RemediationJob controller
func (r *RemediationJobReconciler) recordJobMetrics(
	ctx context.Context,
	job *v1alpha1.RemediationJob,
	result *v1alpha1.RemediationJobResult,
) {
	if r.metrics == nil {
		return
	}
	
	// Get detection time from finding
	// This would require storing detection timestamp in finding
	detectionTime := job.CreationTimestamp.Time
	
	completionTime := time.Now()
	
	r.metrics.RecordDetectionToFix(
		// Need to get the original finding
		// finding,
		// result,
		detectionTime,
		completionTime,
	)
}
```

#### 3. Grafana Dashboard Templates
```json
// config/grafana/dashboards/mendabot-business.json
{
  "title": "Mendabot Business Metrics",
  "panels": [
    {
      "title": "Detection to Fix Timeline",
      "targets": [
        {
          "expr": "histogram_quantile(0.95, rate(mendabot_detection_to_fix_duration_seconds_bucket[5m]))",
          "legendFormat": "{{tier}} - {{classification}}"
        }
      ],
      "type": "graph"
    },
    {
      "title": "Success Rate by Tier",
      "targets": [
        {
          "expr": "mendabot_success_rate_by_tier",
          "legendFormat": "{{tier}}"
        }
      ],
      "type": "stat"
    },
    {
      "title": "LLM Token Usage",
      "targets": [
        {
          "expr": "rate(mendabot_llm_tokens_total[5m])",
          "legendFormat": "{{model}} - {{tier}}"
        }
      ],
      "type": "graph"
    },
    {
      "title": "Investigation Cost (USD)",
      "targets": [
        {
          "expr": "rate(mendabot_investigation_cost_usd[1h])",
          "legendFormat": "{{tier}} - {{classification}}"
        }
      ],
      "type": "graph"
    },
    {
      "title": "Issues Prevented",
      "targets": [
        {
          "expr": "increase(mendabot_issues_prevented_total[1h])",
          "legendFormat": "{{severity}} - {{tier}}"
        }
      ],
      "type": "graph"
    },
    {
      "title": "Investigations Skipped",
      "targets": [
        {
          "expr": "increase(mendabot_investigations_skipped_total[1h])",
          "legendFormat": "{{reason}}"
        }
      ],
      "type": "graph"
    },
    {
      "title": "Operational Efficiency",
      "targets": [
        {
          "expr": "mendabot_operational_efficiency",
          "legendFormat": "{{namespace}}"
        }
      ],
      "type": "gauge"
    }
  ]
}
```

#### 4. Configuration
```yaml
# charts/mendabot/values.yaml
metrics:
  business:
    enabled: true
    
    # Performance metrics
    detectionToFix:
      enabled: true
      buckets: [60, 120, 300, 600, 1800, 3600, 7200, 14400]
    
    successRates:
      enabled: true
      updateInterval: "5m"
    
    # Cost metrics
    costTracking:
      enabled: true
      llmCostPer1KTokens: 0.002
      computeCostPerSecond: 0.0001
    
    # Value metrics
    valueTracking:
      enabled: true
      baselineMTTR:
        config-error: 3600
        resource-limit: 1800
        node-failure: 7200
        network: 5400
    
    # Filtering metrics
    filtering:
      enabled: true
      falsePositiveWindow: "24h"
  
  # Prometheus configuration
  prometheus:
    enabled: true
    port: 9090
    path: "/metrics"
    
  # Grafana dashboards
  grafana:
    enabled: true
    dashboards:
      - name: "mendabot-business"
        file: "config/grafana/dashboards/mendabot-business.json"
      - name: "mendabot-technical"
        file: "config/grafana/dashboards/mendabot-technical.json"
```

### New Files
| File | Purpose |
|------|---------|
| `internal/metrics/business.go` | Business-level metrics implementation |
| `internal/metrics/integration.go` | Metrics integration with controllers |
| `config/grafana/dashboards/mendabot-business.json` | Business metrics dashboard |
| `config/grafana/dashboards/mendabot-technical.json` | Technical metrics dashboard |
| `docs/metrics/README.md` | Metrics documentation |

### Modified Files
| File | Change |
|------|--------|
| `internal/controller/remediationjob_controller.go` | Integrate metrics recording |
| `internal/provider/provider.go` | Record investigation metrics |
| `internal/filter/*.go` | Record filtering effectiveness |
| `charts/mendabot/templates/service.yaml` | Add metrics service |
| `charts/mendabot/values.yaml` | Add metrics configuration |
| `charts/mendabot/templates/serviceaccount.yaml` | Add metrics permissions |

### Testing Strategy
1. **Unit Tests**: Test metrics calculation and recording
2. **Integration Tests**: Test Prometheus metric exposure
3. **E2E Tests**: Verify metrics in test environment
4. **Performance Tests**: Ensure metrics don't impact performance

### Migration Notes
- Metrics disabled by default (`metrics.business.enabled: false`)
- No impact on existing deployments
- Cost tracking requires configuration of actual costs
- Baseline MTTR values should be customized per environment

### Success Metrics
- All business metrics exposed via Prometheus
- Grafana dashboards working
- No performance degradation from metrics collection
- Operators can track ROI and effectiveness
- Metrics used to make operational decisions