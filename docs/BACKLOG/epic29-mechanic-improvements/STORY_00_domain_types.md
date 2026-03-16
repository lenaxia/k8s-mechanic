# STORY_00: Tiered Response Domain Types

## Status: Not Started

## Objective

Define the domain types for the tiered response system: tier classification, finding enhancement, and confidence scoring. This story establishes the foundation for all other stories in the epic.

## Acceptance Criteria

- [ ] `FindingTier` type with constants: `auto-fixable`, `infrastructure`, `transient`
- [ ] `EnhancedFinding` struct embedding `domain.Finding` with additional fields
- [ ] Classification constants for common issue types
- [ ] Confidence scoring type and helper functions
- [ ] Unit tests for all new types
- [ ] `go test -timeout 30s -race ./...` passes

## Tasks

### 1. Create Tier Classification Types

**File**: `internal/domain/tier.go`

```go
package domain

// FindingTier classifies findings based on fixability and impact.
type FindingTier string

const (
    // TierAutoFixable indicates issues that can be fixed via GitOps manifest changes.
    // Examples: missing health probes, incorrect resource requests, wrong image tags.
    TierAutoFixable FindingTier = "auto-fixable"

    // TierInfrastructure indicates issues caused by infrastructure problems.
    // Examples: node failures, network issues, storage problems, hardware failures.
    // These cannot be fixed via GitOps and should generate alerts, not PRs.
    TierInfrastructure FindingTier = "infrastructure"

    // TierTransient indicates self-healing or transient issues.
    // Examples: temporary network blips, scaling events, pod startup delays.
    // These should be suppressed with short TTL.
    TierTransient FindingTier = "transient"
)

// ValidTiers returns all valid tier values.
func ValidTiers() []FindingTier {
    return []FindingTier{TierAutoFixable, TierInfrastructure, TierTransient}
}

// IsValidTier checks if a string is a valid tier.
func IsValidTier(s string) bool {
    for _, t := range ValidTiers() {
        if string(t) == s {
            return true
        }
    }
    return false
}

// TierFromString converts a string to FindingTier, returns empty string if invalid.
func TierFromString(s string) FindingTier {
    if IsValidTier(s) {
        return FindingTier(s)
    }
    return ""
}
```

### 2. Create Classification Types

**File**: `internal/domain/classification.go`

```go
package domain

// FindingClassification categorizes the root cause of a finding.
type FindingClassification string

const (
    // Config errors that can be fixed via GitOps
    ClassificationConfigError     FindingClassification = "config-error"
    ClassificationMissingProbe    FindingClassification = "missing-probe"
    ClassificationResourceError   FindingClassification = "resource-error"
    ClassificationImageError      FindingClassification = "image-error"
    ClassificationSecurityError   FindingClassification = "security-error"

    // Infrastructure issues
    ClassificationNodeFailure     FindingClassification = "node-failure"
    ClassificationNetworkIssue    FindingClassification = "network-issue"
    ClassificationStorageIssue    FindingClassification = "storage-issue"
    ClassificationHardwareIssue   FindingClassification = "hardware-issue"
    ClassificationResourceExhaustion FindingClassification = "resource-exhaustion"

    // Transient issues
    ClassificationTransientNetwork FindingClassification = "transient-network"
    ClassificationScalingEvent     FindingClassification = "scaling-event"
    ClassificationStartupDelay     FindingClassification = "startup-delay"
    ClassificationMaintenance      FindingClassification = "maintenance"
)

// ClassificationMetadata provides additional context for a classification.
type ClassificationMetadata struct {
    // Confidence score 0.0-1.0
    Confidence float64 `json:"confidence"`

    // Supporting evidence for the classification
    Evidence []string `json:"evidence,omitempty"`

    // Suggested actions (if any)
    SuggestedActions []string `json:"suggestedActions,omitempty"`

    // Estimated time to resolution (if known)
    ETTR string `json:"ettr,omitempty"`
}

// ClassifyFinding analyzes a finding and returns classification with metadata.
func ClassifyFinding(f Finding) (FindingClassification, ClassificationMetadata) {
    // Default implementation - will be enhanced in later stories
    // For now, basic classification based on error patterns
    return classifyBasic(f)
}

func classifyBasic(f Finding) (FindingClassification, ClassificationMetadata) {
    // Simple rule-based classification
    // Enhanced in STORY_01 with node health correlation
    // Enhanced in STORY_02 with infrastructure cascade detection
    
    // Default to config error for backward compatibility
    return ClassificationConfigError, ClassificationMetadata{
        Confidence: 0.5,
        Evidence:   []string{"Default classification"},
    }
}
```

### 3. Create Enhanced Finding Type

**File**: `internal/domain/enhanced_finding.go`

```go
package domain

import (
    "encoding/json"
    "fmt"
)

// EnhancedFinding extends the base Finding with classification and confidence.
type EnhancedFinding struct {
    Finding

    // Tier classification
    Tier FindingTier `json:"tier,omitempty"`

    // Root cause classification
    Classification FindingClassification `json:"classification,omitempty"`

    // Classification metadata
    ClassificationMeta ClassificationMetadata `json:"classificationMeta,omitempty"`

    // Fix confidence score (0.0-1.0)
    FixConfidence float64 `json:"fixConfidence,omitempty"`

    // Node name (if applicable)
    NodeName string `json:"nodeName,omitempty"`

    // Infrastructure context (e.g., failed node name, storage system)
    InfrastructureContext string `json:"infrastructureContext,omitempty"`
}

// NewEnhancedFinding creates an EnhancedFinding from a base Finding.
func NewEnhancedFinding(f Finding) EnhancedFinding {
    classification, meta := ClassifyFinding(f)
    
    // Determine tier based on classification
    tier := determineTier(classification)
    
    // Calculate fix confidence
    confidence := calculateFixConfidence(classification, meta.Confidence)
    
    return EnhancedFinding{
        Finding:            f,
        Tier:              tier,
        Classification:    classification,
        ClassificationMeta: meta,
        FixConfidence:     confidence,
    }
}

func determineTier(c FindingClassification) FindingTier {
    switch c {
    case ClassificationConfigError, ClassificationMissingProbe,
         ClassificationResourceError, ClassificationImageError,
         ClassificationSecurityError:
        return TierAutoFixable
    
    case ClassificationNodeFailure, ClassificationNetworkIssue,
         ClassificationStorageIssue, ClassificationHardwareIssue,
         ClassificationResourceExhaustion:
        return TierInfrastructure
    
    case ClassificationTransientNetwork, ClassificationScalingEvent,
         ClassificationStartupDelay, ClassificationMaintenance:
        return TierTransient
    
    default:
        return TierAutoFixable // Default for backward compatibility
    }
}

func calculateFixConfidence(c FindingClassification, baseConfidence float64) float64 {
    // Auto-fixable issues get higher confidence
    switch c {
    case ClassificationConfigError, ClassificationMissingProbe:
        return baseConfidence * 0.9
    
    case ClassificationResourceError, ClassificationImageError:
        return baseConfidence * 0.8
    
    case ClassificationSecurityError:
        return baseConfidence * 0.7
    
    case ClassificationNodeFailure, ClassificationNetworkIssue:
        return baseConfidence * 0.3 // Low confidence for infrastructure fixes
    
    case ClassificationTransientNetwork, ClassificationScalingEvent:
        return baseConfidence * 0.2 // Very low for transient issues
    
    default:
        return baseConfidence * 0.5
    }
}

// ToJSON returns the enhanced finding as JSON.
func (ef EnhancedFinding) ToJSON() (string, error) {
    data, err := json.Marshal(ef)
    if err != nil {
        return "", fmt.Errorf("marshaling enhanced finding: %w", err)
    }
    return string(data), nil
}

// FromJSON parses JSON into an EnhancedFinding.
func FromJSON(data string) (EnhancedFinding, error) {
    var ef EnhancedFinding
    if err := json.Unmarshal([]byte(data), &ef); err != nil {
        return EnhancedFinding{}, fmt.Errorf("unmarshaling enhanced finding: %w", err)
    }
    return ef, nil
}

// ShouldDispatch returns true if this finding should trigger an investigation.
func (ef EnhancedFinding) ShouldDispatch(minConfidence float64) bool {
    // Don't dispatch transient issues
    if ef.Tier == TierTransient {
        return false
    }
    
    // Only dispatch auto-fixable issues with sufficient confidence
    if ef.Tier == TierAutoFixable {
        return ef.FixConfidence >= minConfidence
    }
    
    // Infrastructure issues - dispatch for investigation but not for fixes
    // This will be handled by the tiered response system
    return true
}
```

### 4. Create Unit Tests

**File**: `internal/domain/tier_test.go`

```go
package domain

import (
    "testing"
)

func TestFindingTier(t *testing.T) {
    tests := []struct {
        name     string
        tier     FindingTier
        wantValid bool
    }{
        {"auto-fixable", TierAutoFixable, true},
        {"infrastructure", TierInfrastructure, true},
        {"transient", TierTransient, true},
        {"invalid", FindingTier("invalid"), false},
        {"empty", FindingTier(""), false},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got := IsValidTier(string(tt.tier)); got != tt.wantValid {
                t.Errorf("IsValidTier(%q) = %v, want %v", tt.tier, got, tt.wantValid)
            }
            
            if tt.wantValid {
                if got := TierFromString(string(tt.tier)); got != tt.tier {
                    t.Errorf("TierFromString(%q) = %v, want %v", tt.tier, got, tt.tier)
                }
            }
        })
    }
}

func TestValidTiers(t *testing.T) {
    tiers := ValidTiers()
    if len(tiers) != 3 {
        t.Errorf("ValidTiers() length = %d, want 3", len(tiers))
    }
    
    expected := []FindingTier{TierAutoFixable, TierInfrastructure, TierTransient}
    for i, tier := range tiers {
        if tier != expected[i] {
            t.Errorf("ValidTiers()[%d] = %v, want %v", i, tier, expected[i])
        }
    }
}
```

**File**: `internal/domain/enhanced_finding_test.go`

```go
package domain

import (
    "testing"
    "time"
)

func TestNewEnhancedFinding(t *testing.T) {
    baseFinding := Finding{
        Kind:         "Deployment",
        Name:         "test-deploy",
        Namespace:    "default",
        ParentObject: "Deployment/test-deploy",
        Errors:       `[{"text":"deployment test-deploy: 0/1 replicas ready"}]`,
        Details:      "",
        Severity:     "medium",
        ChainDepth:   0,
    }
    
    ef := NewEnhancedFinding(baseFinding)
    
    if ef.Kind != baseFinding.Kind {
        t.Errorf("EnhancedFinding.Kind = %v, want %v", ef.Kind, baseFinding.Kind)
    }
    
    if ef.Tier == "" {
        t.Error("EnhancedFinding.Tier should not be empty")
    }
    
    if ef.Classification == "" {
        t.Error("EnhancedFinding.Classification should not be empty")
    }
    
    if ef.FixConfidence < 0.0 || ef.FixConfidence > 1.0 {
        t.Errorf("EnhancedFinding.FixConfidence = %v, should be between 0.0 and 1.0", ef.FixConfidence)
    }
}

func TestEnhancedFindingJSON(t *testing.T) {
    baseFinding := Finding{
        Kind:         "Pod",
        Name:         "test-pod",
        Namespace:    "default",
        ParentObject: "Deployment/test-deploy",
        Errors:       `[{"text":"container test: OOMKilled"}]`,
        Details:      "",
        Severity:     "high",
        ChainDepth:   0,
    }
    
    ef := NewEnhancedFinding(baseFinding)
    
    // Test serialization
    jsonStr, err := ef.ToJSON()
    if err != nil {
        t.Fatalf("ToJSON() error = %v", err)
    }
    
    if jsonStr == "" {
        t.Error("ToJSON() returned empty string")
    }
    
    // Test deserialization
    ef2, err := FromJSON(jsonStr)
    if err != nil {
        t.Fatalf("FromJSON() error = %v", err)
    }
    
    if ef2.Kind != ef.Kind {
        t.Errorf("Deserialized Kind = %v, want %v", ef2.Kind, ef.Kind)
    }
    
    if ef2.Tier != ef.Tier {
        t.Errorf("Deserialized Tier = %v, want %v", ef2.Tier, ef.Tier)
    }
    
    if ef2.Classification != ef.Classification {
        t.Errorf("Deserialized Classification = %v, want %v", ef2.Classification, ef.Classification)
    }
}

func TestShouldDispatch(t *testing.T) {
    tests := []struct {
        name         string
        tier         FindingTier
        confidence   float64
        minConfidence float64
        wantDispatch bool
    }{
        {"auto-fixable high confidence", TierAutoFixable, 0.8, 0.7, true},
        {"auto-fixable low confidence", TierAutoFixable, 0.6, 0.7, false},
        {"infrastructure", TierInfrastructure, 0.3, 0.7, true}, // Infrastructure always dispatches
        {"transient", TierTransient, 0.5, 0.7, false},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ef := EnhancedFinding{
                Tier:          tt.tier,
                FixConfidence: tt.confidence,
            }
            
            if got := ef.ShouldDispatch(tt.minConfidence); got != tt.wantDispatch {
                t.Errorf("ShouldDispatch(%v) = %v, want %v", tt.minConfidence, got, tt.wantDispatch)
            }
        })
    }
}
```

### 5. Update Existing Types

**File**: `internal/domain/finding.go`

Add the new fields to the existing `Finding` struct (backward compatible):

```go
type Finding struct {
    Kind         string `json:"kind"`
    Name         string `json:"name"`
    Namespace    string `json:"namespace"`
    ParentObject string `json:"parentObject"`
    Errors       string `json:"errors"` // JSON array of errorEntry
    Details      string `json:"details,omitempty"`
    Severity     string `json:"severity,omitempty"`
    ChainDepth   int    `json:"chainDepth,omitempty"`
    NodeName     string `json:"nodeName,omitempty"` // NEW: Node where resource is running
    
    // NEW: Classification fields (optional for backward compatibility)
    Tier              FindingTier           `json:"tier,omitempty"`
    Classification    FindingClassification `json:"classification,omitempty"`
    FixConfidence     float64               `json:"fixConfidence,omitempty"`
}
```

## Implementation Notes

1. **Backward Compatibility**: All new fields are optional (`omitempty` JSON tag)
2. **Default Behavior**: Existing code continues to work without changes
3. **Gradual Adoption**: New features can be enabled via configuration
4. **Testing Strategy**: Unit tests for new types, integration tests in later stories

## Files Modified

| File | Change |
|------|--------|
| `internal/domain/tier.go` | New file with tier classification types |
| `internal/domain/classification.go` | New file with classification types and logic |
| `internal/domain/enhanced_finding.go` | New file with EnhancedFinding type |
| `internal/domain/finding.go` | Add new optional fields to Finding struct |
| `internal/domain/tier_test.go` | Unit tests for tier types |
| `internal/domain/enhanced_finding_test.go` | Unit tests for EnhancedFinding |
| `go.mod` | No changes needed |
| `go.sum` | No changes needed |

## Verification

1. Run unit tests: `go test -timeout 30s ./internal/domain/...`
2. Verify backward compatibility: existing tests should still pass
3. Check JSON serialization: `EnhancedFinding.ToJSON()` should produce valid JSON
4. Verify type safety: `FindingTier` should only accept valid values

## Next Steps

After this story is complete:
- STORY_01: Implement node health correlation using the new types
- STORY_02: Implement infrastructure cascade detection
- STORY_03: Implement failure analysis CRD
- STORY_04: Implement smart PR management

This story establishes the foundation for the entire tiered response system.