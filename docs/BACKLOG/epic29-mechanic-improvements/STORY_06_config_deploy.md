# STORY_06: Configuration, Deployment, and Helm Values

## Purpose
Implement configuration management, Helm chart updates, and deployment automation for all Epic 29 features.

## Status: Not Started

## Dependencies
- STORY_00 (domain types) - for tier configuration
- STORY_01 (node health) - for filtering configuration
- STORY_02 (cascade detection) - for cascade configuration
- STORY_03 (failure analysis) - for artifact configuration
- STORY_04 (PR management) - for PR configuration
- STORY_05 (metrics) - for metrics configuration

## Acceptance Criteria
- [ ] Comprehensive configuration structure implemented
- [ ] Helm chart updated with all new features
- [ ] Configuration validation and defaults
- [ ] Feature flags for progressive rollout
- [ ] Documentation for configuration options
- [ ] All unit tests pass
- [ ] Worklog entry created

## Problem
New features need proper configuration management:
1. No centralized configuration for Epic 29 features
2. Missing Helm values for new capabilities
3. No feature flags for progressive rollout
4. Configuration validation missing
5. Operators need clear documentation

## Solution
Implement comprehensive configuration system with:
1. **Structured configuration** in Go structs
2. **Helm chart updates** with new values
3. **Feature flags** for controlled rollout
4. **Configuration validation** with sensible defaults
5. **Documentation** for all options

### Technical Design

#### 1. Configuration Structure
```go
// internal/config/epic29.go
package config

import "time"

// Epic29Config contains all configuration for Epic 29 features
type Epic29Config struct {
	// Tiered response system
	TieredResponse TieredResponseConfig `json:"tieredResponse" yaml:"tieredResponse"`
	
	// Node health correlation
	NodeHealth NodeHealthConfig `json:"nodeHealth" yaml:"nodeHealth"`
	
	// Infrastructure cascade detection
	CascadeDetection CascadeDetectionConfig `json:"cascadeDetection" yaml:"cascadeDetection"`
	
	// Failure analysis
	FailureAnalysis FailureAnalysisConfig `json:"failureAnalysis" yaml:"failureAnalysis"`
	
	// PR management
	PRManagement PRManagementConfig `json:"prManagement" yaml:"prManagement"`
	
	// Metrics
	Metrics MetricsConfig `json:"metrics" yaml:"metrics"`
	
	// Feature flags
	FeatureFlags FeatureFlags `json:"featureFlags" yaml:"featureFlags"`
}

// TieredResponseConfig configuration
type TieredResponseConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	// Minimum confidence for auto-fix attempts (0.0-1.0)
	AutoFixMinConfidence float64 `json:"autoFixMinConfidence" yaml:"autoFixMinConfidence"`
	
	// Classification rules
	ClassificationRules []ClassificationRule `json:"classificationRules" yaml:"classificationRules"`
	
	// Tier-specific actions
	TierActions map[string]TierAction `json:"tierActions" yaml:"tierActions"`
}

type ClassificationRule struct {
	Pattern       string `json:"pattern" yaml:"pattern"`
	Tier          string `json:"tier" yaml:"tier"`
	Classification string `json:"classification" yaml:"classification"`
	Confidence    float64 `json:"confidence" yaml:"confidence"`
}

type TierAction struct {
	CreatePR      bool   `json:"createPR" yaml:"createPR"`
	CreateEvent   bool   `json:"createEvent" yaml:"createEvent"`
	CreateAlert   bool   `json:"createAlert" yaml:"createAlert"`
	SuppressHours int    `json:"suppressHours" yaml:"suppressHours"`
	Comment       string `json:"comment" yaml:"comment"`
}

// NodeHealthConfig configuration
type NodeHealthConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	// Skip investigations on NotReady nodes
	SkipUnhealthyNodes bool `json:"skipUnhealthyNodes" yaml:"skipUnhealthyNodes"`
	
	// Node health cache TTL
	CacheTTL time.Duration `json:"cacheTTL" yaml:"cacheTTL"`
	
	// Node conditions to consider unhealthy
	UnhealthyConditions []string `json:"unhealthyConditions" yaml:"unhealthyConditions"`
	
	// Grace period for node transitions
	GracePeriodSeconds int `json:"gracePeriodSeconds" yaml:"gracePeriodSeconds"`
}

// CascadeDetectionConfig configuration
type CascadeDetectionConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	// Namespace failure threshold percentage
	NamespaceFailureThreshold int `json:"namespaceFailureThreshold" yaml:"namespaceFailureThreshold"`
	
	// Node failure detection
	NodeFailure NodeFailureConfig `json:"nodeFailure" yaml:"nodeFailure"`
	
	// Storage failure detection
	StorageFailure StorageFailureConfig `json:"storageFailure" yaml:"storageFailure"`
	
	// Network failure detection
	NetworkFailure NetworkFailureConfig `json:"networkFailure" yaml:"networkFailure"`
}

type NodeFailureConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
	MinPods int  `json:"minPods" yaml:"minPods"`
}

type StorageFailureConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
	MinPVs  int  `json:"minPVs" yaml:"minPVs"`
}

type NetworkFailureConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
	MinServices int `json:"minServices" yaml:"minServices"`
}

// FailureAnalysisConfig configuration
type FailureAnalysisConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	// Artifact preservation
	PreserveArtifacts bool `json:"preserveArtifacts" yaml:"preserveArtifacts"`
	
	// Artifact retention period
	RetentionHours int `json:"retentionHours" yaml:"retentionHours"`
	
	// Storage backend
	StorageBackend string `json:"storageBackend" yaml:"storageBackend"`
	
	// Storage configuration
	Storage StorageConfig `json:"storage" yaml:"storage"`
	
	// Failure classification rules
	ClassificationRules []FailureClassificationRule `json:"classificationRules" yaml:"classificationRules"`
}

type StorageConfig struct {
	LocalPath string `json:"localPath" yaml:"localPath"`
	
	S3 S3Config `json:"s3" yaml:"s3"`
	
	GCS GCSConfig `json:"gcs" yaml:"gcs"`
}

type S3Config struct {
	Enabled  bool   `json:"enabled" yaml:"enabled"`
	Bucket   string `json:"bucket" yaml:"bucket"`
	Region   string `json:"region" yaml:"region"`
	Prefix   string `json:"prefix" yaml:"prefix"`
	Endpoint string `json:"endpoint" yaml:"endpoint"`
}

type GCSConfig struct {
	Enabled    bool   `json:"enabled" yaml:"enabled"`
	Bucket     string `json:"bucket" yaml:"bucket"`
	Prefix     string `json:"prefix" yaml:"prefix"`
	Credential string `json:"credential" yaml:"credential"`
}

type FailureClassificationRule struct {
	Pattern     string `json:"pattern" yaml:"pattern"`
	FailureType string `json:"failureType" yaml:"failureType"`
	Confidence  float64 `json:"confidence" yaml:"confidence"`
}

// PRManagementConfig configuration
type PRManagementConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	// PR type classification
	Classification PRClassificationConfig `json:"classification" yaml:"classification"`
	
	// Auto-close settings
	AutoClose AutoCloseConfig `json:"autoClose" yaml:"autoClose"`
	
	// PR templates
	Templates PRTemplateConfig `json:"templates" yaml:"templates"`
	
	// Webhook for auto-close
	Webhook WebhookConfig `json:"webhook" yaml:"webhook"`
}

type PRClassificationConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	AutoCloseInfrastructure bool `json:"autoCloseInfrastructure" yaml:"autoCloseInfrastructure"`
	AutoCloseDocumentation  bool `json:"autoCloseDocumentation" yaml:"autoCloseDocumentation"`
	
	// Confidence thresholds
	FixConfidenceThreshold      float64 `json:"fixConfidenceThreshold" yaml:"fixConfidenceThreshold"`
	InvestigationConfidenceThreshold float64 `json:"investigationConfidenceThreshold" yaml:"investigationConfidenceThreshold"`
}

type AutoCloseConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	InfrastructureAfterDays int `json:"infrastructureAfterDays" yaml:"infrastructureAfterDays"`
	DocumentationAfterDays  int `json:"documentationAfterDays" yaml:"documentationAfterDays"`
	
	CommentOnClose bool   `json:"commentOnClose" yaml:"commentOnClose"`
	CloseReason    string `json:"closeReason" yaml:"closeReason"`
}

type PRTemplateConfig struct {
	Fix          string `json:"fix" yaml:"fix"`
	Investigation string `json:"investigation" yaml:"investigation"`
	Documentation string `json:"documentation" yaml:"documentation"`
	Suppressed   string `json:"suppressed" yaml:"suppressed"`
}

type WebhookConfig struct {
	Enabled    bool   `json:"enabled" yaml:"enabled"`
	SecretRef  string `json:"secretRef" yaml:"secretRef"`
	Port       int    `json:"port" yaml:"port"`
	Path       string `json:"path" yaml:"path"`
}

// MetricsConfig configuration
type MetricsConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	// Performance metrics
	Performance PerformanceMetricsConfig `json:"performance" yaml:"performance"`
	
	// Cost metrics
	Cost CostMetricsConfig `json:"cost" yaml:"cost"`
	
	// Value metrics
	Value ValueMetricsConfig `json:"value" yaml:"value"`
	
	// Filtering metrics
	Filtering FilteringMetricsConfig `json:"filtering" yaml:"filtering"`
}

type PerformanceMetricsConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	DetectionToFix DetectionToFixConfig `json:"detectionToFix" yaml:"detectionToFix"`
	SuccessRates   SuccessRatesConfig   `json:"successRates" yaml:"successRates"`
}

type DetectionToFixConfig struct {
	Enabled bool    `json:"enabled" yaml:"enabled"`
	Buckets []float64 `json:"buckets" yaml:"buckets"`
}

type SuccessRatesConfig struct {
	Enabled       bool          `json:"enabled" yaml:"enabled"`
	UpdateInterval time.Duration `json:"updateInterval" yaml:"updateInterval"`
	WindowSize    time.Duration `json:"windowSize" yaml:"windowSize"`
}

type CostMetricsConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	LLMCostPer1KTokens float64 `json:"llmCostPer1KTokens" yaml:"llmCostPer1KTokens"`
	ComputeCostPerSecond float64 `json:"computeCostPerSecond" yaml:"computeCostPerSecond"`
	
	// Model-specific costs
	ModelCosts map[string]float64 `json:"modelCosts" yaml:"modelCosts"`
}

type ValueMetricsConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	// Baseline MTTR values in seconds
	BaselineMTTR map[string]float64 `json:"baselineMTTR" yaml:"baselineMTTR"`
	
	// Severity weights for value calculation
	SeverityWeights map[string]float64 `json:"severityWeights" yaml:"severityWeights"`
}

type FilteringMetricsConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	FalsePositiveWindow time.Duration `json:"falsePositiveWindow" yaml:"falsePositiveWindow"`
	TrackingWindow      time.Duration `json:"trackingWindow" yaml:"trackingWindow"`
}

// FeatureFlags for progressive rollout
type FeatureFlags struct {
	// Enable all Epic 29 features
	Epic29Enabled bool `json:"epic29Enabled" yaml:"epic29Enabled"`
	
	// Individual feature flags
	TieredResponseEnabled   bool `json:"tieredResponseEnabled" yaml:"tieredResponseEnabled"`
	NodeHealthEnabled       bool `json:"nodeHealthEnabled" yaml:"nodeHealthEnabled"`
	CascadeDetectionEnabled bool `json:"cascadeDetectionEnabled" yaml:"cascadeDetectionEnabled"`
	FailureAnalysisEnabled  bool `json:"failureAnalysisEnabled" yaml:"failureAnalysisEnabled"`
	PRManagementEnabled     bool `json:"prManagementEnabled" yaml:"prManagementEnabled"`
	MetricsEnabled          bool `json:"metricsEnabled" yaml:"metricsEnabled"`
	
	// Rollout percentages (0-100)
	RolloutPercentage int `json:"rolloutPercentage" yaml:"rolloutPercentage"`
	
	// Namespace allowlist/denylist
	AllowedNamespaces   []string `json:"allowedNamespaces" yaml:"allowedNamespaces"`
	DeniedNamespaces    []string `json:"deniedNamespaces" yaml:"deniedNamespaces"`
}
```

#### 2. Configuration Validation
```go
// internal/config/validator.go
package config

import (
	"fmt"
	"regexp"
)

func (c *Epic29Config) Validate() error {
	// Tiered response validation
	if err := c.TieredResponse.validate(); err != nil {
		return fmt.Errorf("tiered response config invalid: %w", err)
	}
	
	// Node health validation
	if err := c.NodeHealth.validate(); err != nil {
		return fmt.Errorf("node health config invalid: %w", err)
	}
	
	// Cascade detection validation
	if err := c.CascadeDetection.validate(); err != nil {
		return fmt.Errorf("cascade detection config invalid: %w", err)
	}
	
	// Failure analysis validation
	if err := c.FailureAnalysis.validate(); err != nil {
		return fmt.Errorf("failure analysis config invalid: %w", err)
	}
	
	// PR management validation
	if err := c.PRManagement.validate(); err != nil {
		return fmt.Errorf("PR management config invalid: %w", err)
	}
	
	// Metrics validation
	if err := c.Metrics.validate(); err != nil {
		return fmt.Errorf("metrics config invalid: %w", err)
	}
	
	// Feature flags validation
	if err := c.FeatureFlags.validate(); err != nil {
		return fmt.Errorf("feature flags invalid: %w", err)
	}
	
	return nil
}

func (c *TieredResponseConfig) validate() error {
	if !c.Enabled {
		return nil
	}
	
	if c.AutoFixMinConfidence < 0 || c.AutoFixMinConfidence > 1 {
		return fmt.Errorf("autoFixMinConfidence must be between 0 and 1")
	}
	
	for _, rule := range c.ClassificationRules {
		if err := rule.validate(); err != nil {
			return fmt.Errorf("classification rule invalid: %w", err)
		}
	}
	
	return nil
}

func (r *ClassificationRule) validate() error {
	if r.Pattern == "" {
		return fmt.Errorf("pattern cannot be empty")
	}
	
	// Try to compile as regex
	if _, err := regexp.Compile(r.Pattern); err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}
	
	if r.Tier != "auto-fixable" && r.Tier != "infrastructure" && r.Tier != "transient" {
		return fmt.Errorf("tier must be one of: auto-fixable, infrastructure, transient")
	}
	
	if r.Confidence < 0 || r.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1")
	}
	
	return nil
}

// ... similar validation methods for other config types
```

#### 3. Default Configuration
```go
// internal/config/defaults.go
package config

import "time"

func DefaultEpic29Config() *Epic29Config {
	return &Epic29Config{
		TieredResponse: DefaultTieredResponseConfig(),
		NodeHealth: DefaultNodeHealthConfig(),
		CascadeDetection: DefaultCascadeDetectionConfig(),
		FailureAnalysis: DefaultFailureAnalysisConfig(),
		PRManagement: DefaultPRManagementConfig(),
		Metrics: DefaultMetricsConfig(),
		FeatureFlags: DefaultFeatureFlags(),
	}
}

func DefaultTieredResponseConfig() TieredResponseConfig {
	return TieredResponseConfig{
		Enabled: false,
		AutoFixMinConfidence: 0.7,
		ClassificationRules: []ClassificationRule{
			{
				Pattern: "NodeNotReady",
				Tier: "infrastructure",
				Classification: "node-failure",
				Confidence: 0.9,
			},
			{
				Pattern: "ImagePullBackOff",
				Tier: "transient",
				Classification: "image-pull",
				Confidence: 0.7,
			},
			{
				Pattern: "CrashLoopBackOff",
				Tier: "auto-fixable",
				Classification: "config-error",
				Confidence: 0.8,
			},
		},
		TierActions: map[string]TierAction{
			"auto-fixable": {
				CreatePR: true,
				CreateEvent: true,
				CreateAlert: false,
				SuppressHours: 0,
			},
			"infrastructure": {
				CreatePR: false,
				CreateEvent: true,
				CreateAlert: true,
				SuppressHours: 24,
				Comment: "Infrastructure issue detected",
			},
			"transient": {
				CreatePR: false,
				CreateEvent: false,
				CreateAlert: false,
				SuppressHours: 1,
			},
		},
	}
}

func DefaultNodeHealthConfig() NodeHealthConfig {
	return NodeHealthConfig{
		Enabled: false,
		SkipUnhealthyNodes: true,
		CacheTTL: 30 * time.Second,
		UnhealthyConditions: []string{"Ready", "MemoryPressure", "DiskPressure", "PIDPressure"},
		GracePeriodSeconds: 60,
	}
}

// ... default configurations for other components
```

#### 4. Helm Chart Updates
```yaml
# charts/mendabot/values.yaml
# Epic 29 Configuration
epic29:
  enabled: false
  
  # Feature flags
  featureFlags:
    epic29Enabled: false
    tieredResponseEnabled: false
    nodeHealthEnabled: false
    cascadeDetectionEnabled: false
    failureAnalysisEnabled: false
    prManagementEnabled: false
    metricsEnabled: false
    
    rolloutPercentage: 0
    allowedNamespaces: []
    deniedNamespaces: []
  
  # Tiered response system
  tieredResponse:
    enabled: false
    autoFixMinConfidence: 0.7
    
    classificationRules:
      - pattern: "NodeNotReady"
        tier: "infrastructure"
        classification: "node-failure"
        confidence: 0.9
      - pattern: "ImagePullBackOff"
        tier: "transient"
        classification: "image-pull"
        confidence: 0.7
      - pattern: "CrashLoopBackOff"
        tier: "auto-fixable"
        classification: "config-error"
        confidence: 0.8
    
    tierActions:
      auto-fixable:
        createPR: true
        createEvent: true
        createAlert: false
        suppressHours: 0
      infrastructure:
        createPR: false
        createEvent: true
        createAlert: true
        suppressHours: 24
        comment: "Infrastructure issue detected"
      transient:
        createPR: false
        createEvent: false
        createAlert: false
        suppressHours: 1
  
  # Node health correlation
  nodeHealth:
    enabled: false
    skipUnhealthyNodes: true
    cacheTTL: 30
    unhealthyConditions:
      - Ready
      - MemoryPressure
      - DiskPressure
      - PIDPressure
    gracePeriodSeconds: 60
  
  # Infrastructure cascade detection
  cascadeDetection:
    enabled: false
    namespaceFailureThreshold: 50
    
    nodeFailure:
      enabled: true
      minPods: 3
    
    storageFailure:
      enabled: true
      minPVs: 2
    
    networkFailure:
      enabled: true
      minServices: 5
  
  # Failure analysis
  failureAnalysis:
    enabled: false
    preserveArtifacts: true
    retentionHours: 24
    storageBackend: "local"
    
    storage:
      localPath: "/var/run/mendabot/artifacts"
      
      s3:
        enabled: false
        bucket: ""
        region: ""
        prefix: "failures/"
        endpoint: ""
      
      gcs:
        enabled: false
        bucket: ""
        prefix: "failures/"
        credential: ""
    
    classificationRules:
      - pattern: "timeout"
        failureType: "timeout"
        confidence: 0.9
      - pattern: "permission denied"
        failureType: "permission"
        confidence: 0.8
  
  # PR management
  prManagement:
    enabled: false
    
    classification:
      enabled: true
      autoCloseInfrastructure: true
      autoCloseDocumentation: true
      fixConfidenceThreshold: 0.7
      investigationConfidenceThreshold: 0.5
    
    autoClose:
      enabled: true
      infrastructureAfterDays: 7
      documentationAfterDays: 3
      commentOnClose: true
      closeReason: "Auto-closed as infrastructure/documentation issue"
    
    templates:
      fix: |
        ## Fix Proposed by Mendabot
        **Issue**: {{.IssueDescription}}
        **Severity**: {{.Severity}}
        **Confidence**: {{.Confidence}}%
        
        ### Changes
        {{range .ProposedChanges}}
        - **File**: {{.FilePath}}
          - **Change**: {{.Description}}
        {{end}}
        
        ### Validation
        - [ ] Automated tests pass
        - [ ] Manual review recommended
        
      investigation: |
        ## Investigation Report
        **Issue**: {{.IssueDescription}}
        **Severity**: {{.Severity}}
        **Root Cause**: {{.RootCause}}
        
        ### Analysis
        {{.Analysis}}
        
        ### Recommendations
        {{range .Recommendations}}
        - {{.}}
        {{end}}
        
      documentation: |
        ## Infrastructure Issue
        **Issue**: {{.IssueDescription}}
        **Severity**: {{.Severity}}
        
        ### Details
        {{.Analysis}}
        
        ### Notes
        This documents an infrastructure issue.
      
      suppressed: |
        ## Transient Issue
        **Issue**: {{.IssueDescription}}
        **Severity**: {{.Severity}}
        
        ### Details
        {{.Analysis}}
        
        ### Notes
        This issue appears to be transient and has been suppressed.
    
    webhook:
      enabled: false
      secretRef: mendabot-webhook-secret
      port: 8081
      path: "/webhook/pr-autoclose"
  
  # Metrics
  metrics:
    enabled: false
    
    performance:
      enabled: true
      
      detectionToFix:
        enabled: true
        buckets: [60, 120, 300, 600, 1800, 3600, 7200, 14400]
      
      successRates:
        enabled: true
        updateInterval: "5m"
        windowSize: "24h"
    
    cost:
      enabled: true
      llmCostPer1KTokens: 0.002
      computeCostPerSecond: 0.0001
      
      modelCosts:
        gpt-4: 0.002
        gpt-3.5-turbo: 0.0005
    
    value:
      enabled: true
      
      baselineMTTR:
        config-error: 3600
        resource-limit: 1800
        node-failure: 7200
        network: 5400
        image-pull: 1800
      
      severityWeights:
        critical: 10.0
        high: 5.0
        medium: 2.0
        low: 1.0
    
    filtering:
      enabled: true
      falsePositiveWindow: "24h"
      trackingWindow: "7d"
```

#### 5. Configuration Loading
```go
// internal/config/loader.go
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
	"k8s.io/client-go/kubernetes"
)

type ConfigLoader struct {
	k8sClient kubernetes.Interface
	namespace string
}

func (l *ConfigLoader) Load() (*Epic29Config, error) {
	// Try loading from ConfigMap first
	configMapConfig, err := l.loadFromConfigMap()
	if err == nil && configMapConfig != nil {
		return configMapConfig, nil
	}
	
	// Fall back to file
	fileConfig, err := l.loadFromFile()
	if err != nil {
		return nil, fmt.Errorf("failed to load config from any source: %w", err)
	}
	
	return fileConfig, nil
}

func (l *ConfigLoader) loadFromConfigMap() (*Epic29Config, error) {
	if l.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not available")
	}
	
	configMap, err := l.k8sClient.CoreV1().ConfigMaps(l.namespace).Get(
		context.Background(),
		"mendabot-epic29-config",
		metav1.GetOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get configmap: %w", err)
	}
	
	data, ok := configMap.Data["epic29.yaml"]
	if !ok {
		return nil, fmt.Errorf("configmap missing epic29.yaml key")
	}
	
	var config Epic29Config
	if err := yaml.Unmarshal([]byte(data), &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal configmap data: %w", err)
	}
	
	return &config, nil
}

func (l *ConfigLoader) loadFromFile() (*Epic29Config, error) {
	// Try multiple possible file locations
	paths := []string{
		"/etc/mendabot/epic29.yaml",
		"./config/epic29.yaml",
		"./epic29.yaml",
	}
	
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			
			var config Epic29Config
			if err := yaml.Unmarshal(data, &config); err != nil {
				return nil, fmt.Errorf("failed to unmarshal config file %s: %w", path, err)
			}
			
			return &config, nil
		}
	}
	
	// Return defaults if no config file found
	return DefaultEpic29Config(), nil
}
```

### New Files
| File | Purpose |
|------|---------|
| `internal/config/epic29.go` | Epic 29 configuration types |
| `internal/config/validator.go` | Configuration validation |
| `internal/config/defaults.go` | Default configuration values |
| `internal/config/loader.go` | Configuration loading |
| `config/epic29.yaml` | Example configuration file |
| `charts/mendabot/templates/configmap-epic29.yaml` | ConfigMap template |

### Modified Files
| File | Change |
|------|--------|
| `charts/mendabot/values.yaml` | Add Epic 29 configuration section |
| `charts/mendabot/templates/deployment.yaml` | Mount ConfigMap and env vars |
| `internal/controller/*.go` | Load and use Epic 29 config |
| `internal/provider/*.go` | Use tiered response config |
| `internal/filter/*.go` | Use filtering config |
| `internal/failure/*.go` | Use failure analysis config |
| `internal/sink/github/*.go` | Use PR management config |
| `internal/metrics/*.go` | Use metrics config |

### Testing Strategy
1. **Unit Tests**: Test configuration validation and defaults
2. **Integration Tests**: Test ConfigMap loading and env var parsing
3. **E2E Tests**: Test full configuration lifecycle
4. **Helm Tests**: Test Helm chart deployment with config

### Migration Notes
- All features disabled by default (`epic29.enabled: false`)
- Configuration can be loaded from ConfigMap or file
- Sensible defaults provided for all options
- Feature flags allow progressive rollout
- Backward compatible - existing deployments unaffected

### Success Metrics
- Configuration loaded successfully from all sources
- Validation catches invalid configurations
- Defaults provide safe out-of-box experience
- Feature flags work as expected
- Operators can customize all aspects of Epic 29 features