# STORY_04: Smart PR Management and Auto-close

## Purpose
Implement intelligent PR handling with type classification, auto-close for infrastructure issues, and better templates to reduce alert fatigue.

## Status: Not Started

## Dependencies
- STORY_00 (domain types) - for tier classification
- STORY_01 (node health) - for infrastructure detection
- epic05-github-sink - for GitHub integration foundation

## Acceptance Criteria
- [ ] PR type classification implemented (fix, investigation, documentation)
- [ ] Auto-close logic for infrastructure issues (Tier 2)
- [ ] Enhanced PR templates per outcome type
- [ ] Auto-close webhook for resolved issues
- [ ] PR labels for classification (`needs-human-review`, `infrastructure`, `auto-fixable`)
- [ ] 50% reduction in PR noise for infrastructure issues
- [ ] All unit tests pass
- [ ] Worklog entry created

## Problem
Current PR management creates noise:
1. PR #1358: Created PR saying "no fix needed, it's infrastructure" (documentation-only)
2. Infrastructure issues generate PRs that can't be fixed via GitOps
3. No distinction between fix PRs and investigation reports
4. PRs remain open after underlying issues resolve
5. Operators get alert fatigue from low-value PRs

## Solution
Implement smart PR management with:
1. **PR type classification** based on investigation outcome
2. **Auto-close** for infrastructure issues with explanatory comments
3. **Enhanced templates** tailored to each PR type
4. **Auto-close webhook** to close PRs when issues resolve

### Technical Design

#### 1. PR Type Classification
```go
// internal/sink/github/types.go
package github

import "github.com/yourorg/mendabot/api/v1alpha1"

// PRType classification based on investigation outcome
type PRType string

const (
	// PRTypeFix - Actual manifest changes to fix the issue
	PRTypeFix PRType = "fix"

	// PRTypeInvestigation - Root cause analysis requiring human review
	PRTypeInvestigation PRType = "investigation"

	// PRTypeDocumentation - Infrastructure issue, no GitOps fix available
	PRTypeDocumentation PRType = "documentation"

	// PRTypeSuppressed - Transient issue, no PR created
	PRTypeSuppressed PRType = "suppressed"
)

// PRClassification determines PR type based on investigation results
type PRClassification struct {
	Type        PRType
	Labels      []string
	ShouldClose bool
	CloseReason string
}

// ClassifyPR determines PR type based on investigation outcome
func ClassifyPR(result *v1alpha1.RemediationJobResult, tier v1alpha1.FindingTier) PRClassification {
	// Tier-based classification first
	switch tier {
	case v1alpha1.TierInfrastructure:
		return PRClassification{
			Type:        PRTypeDocumentation,
			Labels:      []string{"infrastructure", "needs-human-review"},
			ShouldClose: true,
			CloseReason: "This is an infrastructure issue that cannot be fixed via GitOps changes.",
		}
	case v1alpha1.TierTransient:
		return PRClassification{
			Type:        PRTypeSuppressed,
			Labels:      []string{"transient", "suppressed"},
			ShouldClose: false,
		}
	}

	// Outcome-based classification for auto-fixable tier
	if result == nil || result.ProposedChanges == nil {
		return PRClassification{
			Type:        PRTypeInvestigation,
			Labels:      []string{"investigation", "needs-human-review"},
			ShouldClose: false,
		}
	}

	// Check if changes are actual fixes vs documentation
	if isDocumentationOnly(result.ProposedChanges) {
		return PRClassification{
			Type:        PRTypeDocumentation,
			Labels:      []string{"documentation", "needs-human-review"},
			ShouldClose: true,
			CloseReason: "No actionable changes identified - this appears to be a documentation update.",
		}
	}

	// Actual fix with changes
	return PRClassification{
		Type:        PRTypeFix,
		Labels:      []string{"fix", "auto-fixable"},
		ShouldClose: false,
	}
}

func isDocumentationOnly(changes []v1alpha1.ProposedChange) bool {
	for _, change := range changes {
		// Check if change is only comments or documentation
		if !containsActualChanges(change) {
			return true
		}
	}
	return false
}
```

#### 2. Enhanced GitHub Sink with Auto-close
```go
// internal/sink/github/enhanced_sink.go
package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v50/github"
	"github.com/yourorg/mendabot/api/v1alpha1"
	"github.com/yourorg/mendabot/internal/config"
	"go.uber.org/zap"
)

type EnhancedGitHubSink struct {
	*GitHubSink // Embed existing sink
	config      *config.Config
	logger      *zap.Logger
}

func NewEnhancedGitHubSink(baseSink *GitHubSink, cfg *config.Config, logger *zap.Logger) *EnhancedGitHubSink {
	return &EnhancedGitHubSink{
		GitHubSink: baseSink,
		config:     cfg,
		logger:     logger,
	}
}

func (s *EnhancedGitHubSink) CreatePR(ctx context.Context, result *v1alpha1.RemediationJobResult) error {
	// Classify PR type
	classification := ClassifyPR(result, result.FindingTier)

	// Skip suppressed PRs
	if classification.Type == PRTypeSuppressed {
		s.logger.Info("Skipping PR creation for suppressed issue",
			"finding", result.FindingRef,
			"tier", result.FindingTier)
		return nil
	}

	// Create PR with enhanced template
	pr, err := s.createEnhancedPR(ctx, result, classification)
	if err != nil {
		return fmt.Errorf("failed to create enhanced PR: %w", err)
	}

	// Auto-close if configured
	if classification.ShouldClose && s.config.PR.AutoCloseInfrastructure {
		if err := s.autoClosePR(ctx, pr, classification.CloseReason); err != nil {
			s.logger.Error("failed to auto-close PR", zap.Error(err))
			// Continue - PR was created, just couldn't close it
		}
	}

	return nil
}

func (s *EnhancedGitHubSink) createEnhancedPR(ctx context.Context, result *v1alpha1.RemediationJobResult, classification PRClassification) (*github.PullRequest, error) {
	// Get appropriate template
	template := s.getPRTemplate(classification.Type)

	// Format title based on type
	title := s.formatPRTitle(result, classification.Type)

	// Format body with template
	body := s.formatPRBody(template, result, classification)

	// Create PR
	pr, err := s.GitHubSink.client.CreatePullRequest(ctx, title, body)
	if err != nil {
		return nil, err
	}

	// Add labels
	if len(classification.Labels) > 0 {
		if err := s.GitHubSink.client.AddLabels(ctx, pr.GetNumber(), classification.Labels); err != nil {
			s.logger.Warn("failed to add labels to PR", zap.Error(err))
		}
	}

	// Add comment explaining classification if needed
	if classification.Type == PRTypeDocumentation || classification.Type == PRTypeInvestigation {
		comment := s.getClassificationComment(classification)
		if err := s.GitHubSink.client.CreateComment(ctx, pr.GetNumber(), comment); err != nil {
			s.logger.Warn("failed to add classification comment", zap.Error(err))
		}
	}

	return pr, nil
}

func (s *EnhancedGitHubSink) autoClosePR(ctx context.Context, pr *github.PullRequest, reason string) error {
	// Add closing comment
	comment := fmt.Sprintf("## Auto-closing PR\n\n%s\n\nThis PR has been automatically closed because it addresses an infrastructure issue that cannot be fixed via GitOps changes.", reason)
	if err := s.GitHubSink.client.CreateComment(ctx, pr.GetNumber(), comment); err != nil {
		return fmt.Errorf("failed to add closing comment: %w", err)
	}

	// Close the PR
	if err := s.GitHubSink.client.ClosePullRequest(ctx, pr.GetNumber()); err != nil {
		return fmt.Errorf("failed to close PR: %w", err)
	}

	s.logger.Info("Auto-closed infrastructure PR",
		"pr", pr.GetNumber(),
		"title", pr.GetTitle())

	return nil
}

func (s *EnhancedGitHubSink) getPRTemplate(prType PRType) string {
	switch prType {
	case PRTypeFix:
		return `## Fix Proposed by Mendabot

**Issue**: {{.IssueDescription}}
**Severity**: {{.Severity}}
**Confidence**: {{.Confidence}}%

### Changes
{{range .ProposedChanges}}
- **File**: {{.FilePath}}
  - **Change**: {{.Description}}
  - **Diff**:
{{.Diff}}
{{end}}

### Validation
- [ ] Automated tests pass
- [ ] Manual review recommended for production changes

### Notes
This fix was automatically generated based on cluster monitoring. Please review before merging.`
	
	case PRTypeInvestigation:
		return `## Investigation Report by Mendabot

**Issue**: {{.IssueDescription}}
**Severity**: {{.Severity}}
**Root Cause**: {{.RootCause}}

### Analysis
{{.Analysis}}

### Recommendations
{{range .Recommendations}}
- {{.}}
{{end}}

### Next Steps
This issue requires human intervention. The investigation identified the root cause but could not generate an automated fix.

**Labels**: needs-human-review`

	case PRTypeDocumentation:
		return `## Infrastructure Issue Detected by Mendabot

**Issue**: {{.IssueDescription}}
**Severity**: {{.Severity}}
**Classification**: Infrastructure (cannot be fixed via GitOps)

### Details
{{.Analysis}}

### Impact
{{.Impact}}

### Recommended Actions
{{range .Recommendations}}
- {{.}}
{{end}}

### Notes
This PR will be automatically closed as it documents an infrastructure issue that requires manual intervention outside of GitOps.`
	
	default:
		return s.GitHubSink.defaultTemplate
	}
}
```

#### 3. Auto-close Webhook for Resolved Issues
```go
// internal/webhook/pr_autoclose.go
package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/v50/github"
	"github.com/yourorg/mendabot/internal/sink/github"
	"go.uber.org/zap"
)

type PRAutoCloseWebhook struct {
	githubClient *github.Client
	sink         *github.EnhancedGitHubSink
	logger       *zap.Logger
}

func (h *PRAutoCloseWebhook) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := github.ValidatePayload(r, []byte(h.sink.config.WebhookSecret))
	if err != nil {
		h.logger.Error("invalid webhook payload", zap.Error(err))
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	event, err := github.ParseWebHook(github.WebHookType(r), payload)
	if err != nil {
		h.logger.Error("failed to parse webhook", zap.Error(err))
		http.Error(w, "Failed to parse webhook", http.StatusBadRequest)
		return
	}

	switch event := event.(type) {
	case *github.IssuesEvent:
		h.handleIssuesEvent(event)
	case *github.PullRequestEvent:
		h.handlePullRequestEvent(event)
	case *github.StatusEvent:
		h.handleStatusEvent(event)
	}

	w.WriteHeader(http.StatusOK)
}

func (h *PRAutoCloseWebhook) handleIssuesEvent(event *github.IssuesEvent) {
	// Check if issue is related to a closed PR and should be auto-closed
	if event.GetAction() == "closed" {
		h.checkAndCloseRelatedPRs(event.GetIssue())
	}
}

func (h *PRAutoCloseWebhook) handlePullRequestEvent(event *github.PullRequestEvent) {
	// Auto-close PRs when the underlying issue is resolved
	if event.GetAction() == "synchronize" || event.GetAction() == "opened" {
		h.checkIfIssueResolved(event.GetPullRequest())
	}
}

func (h *PRAutoCloseWebhook) checkIfIssueResolved(pr *github.PullRequest) {
	// Check if the issue that triggered this PR is still present
	// This would query the cluster to see if the finding is still active
	
	// For now, implement simple rule: close PRs labeled "infrastructure" after 7 days
	labels := make([]string, len(pr.Labels))
	for i, label := range pr.Labels {
		labels[i] = label.GetName()
	}

	if contains(labels, "infrastructure") {
		// Check if PR is older than 7 days
		createdAt := pr.GetCreatedAt()
		// if time.Since(createdAt) > 7*24*time.Hour {
		//     h.sink.autoClosePR(context.Background(), pr, "Infrastructure issue PR auto-closed after 7 days.")
		// }
	}
}
```

#### 4. Configuration
```yaml
# charts/mendabot/values.yaml
prManagement:
  enabled: true
  
  # PR type classification
  classification:
    enabled: true
    autoCloseInfrastructure: true
    autoCloseDocumentation: true
    
  # PR templates
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
  
  # Auto-close settings
  autoClose:
    enabled: true
    infrastructureAfterDays: 7
    commentOnClose: true
    
  # Webhook for auto-close
  webhook:
    enabled: true
    secretRef: mendabot-webhook-secret
```

### New Files
| File | Purpose |
|------|---------|
| `internal/sink/github/types.go` | PR type classification types |
| `internal/sink/github/enhanced_sink.go` | Enhanced sink with auto-close |
| `internal/sink/github/templates.go` | PR template management |
| `internal/webhook/pr_autoclose.go` | Auto-close webhook handler |
| `config/pr-templates/` | Directory for PR template files |

### Modified Files
| File | Change |
|------|--------|
| `internal/sink/github/sink.go` | Integrate enhanced sink |
| `internal/controller/remediationjob_controller.go` | Use enhanced sink for PR creation |
| `charts/mendabot/templates/deployment.yaml` | Add webhook container |
| `charts/mendabot/values.yaml` | Add PR management configuration |
| `config/rbac/role.yaml` | Add webhook permissions |

### Testing Strategy
1. **Unit Tests**: Test PR classification logic
2. **Integration Tests**: Test GitHub API interactions
3. **E2E Tests**: Full workflow with auto-close
4. **Manual Tests**: Verify PR templates and labels

### Migration Notes
- Feature disabled by default (`prManagement.enabled: false`)
- Existing PRs unaffected
- Templates can be customized per deployment
- Auto-close can be disabled if too aggressive

### Success Metrics
- 50% reduction in PR noise (infrastructure issues)
- 100% of infrastructure issues auto-closed
- Appropriate labels applied to all PRs
- No regression in fix PR creation
- Operators report reduced alert fatigue