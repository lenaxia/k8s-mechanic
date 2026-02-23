# Story: Upstream Contribution Routing for Mendabot Bugs

**Epic:** [epic11-self-remediation-cascade](README.md)
**Priority:** High
 **Status:** Removed
**Estimated Effort:** 2 hours

---

 ## User Story

**REMOVED**: This feature has been removed due to GitHub App permission complexity. Self-remediation cascade prevention now focuses on preventing infinite cascades without attempting upstream bug reporting.

**Reason for removal**: Upstream contributions required complex GitHub App permission handling (forking, cross-org access) that wasn't practical. The system now keeps only self-remediation cascade prevention (circuit breaker, max depth enforcement) without upstream routing.

All PRs are created against the user's configured `GITOPS_REPO` only.

---

 ## Acceptance Criteria

**REMOVED**: All upstream routing functionality has been removed.

 - [ ] Self-remediations at chain depth ≥ 2 target upstream repository - REMOVED
 - [ ] Upstream repository configurable via `MENDABOT_UPSTREAM_REPO` - REMOVED
 - [ ] Default upstream: `lenaxia/k8s-mendabot` - REMOVED
 - [ ] Can be disabled via `MENDABOT_DISABLE_UPSTREAM_CONTRIBUTIONS` - REMOVED
 - [ ] `TargetRepoOverride` field in `domain.Finding` and `RemediationJobSpec` - REMOVED
 - [ ] JobBuilder injects target repo into agent environment - REMOVED
 - [ ] Agent prompt handles upstream repository investigation - REMOVED
 - [ ] Unit tests verify routing logic at different depths - REMOVED
 - [ ] Integration tests simulate upstream contribution scenarios - REMOVED
 - [ ] Configuration validation and documentation - REMOVED

---

 ## Technical Implementation

**REMOVED**: All upstream routing code has been removed from:

1. **`internal/provider/native/job.go`**: Removed `TargetRepoOverride` assignment
2. **`internal/domain/provider.go`**: Removed `TargetRepoOverride` field from `Finding` struct
3. **`api/v1alpha1/remediationjob_types.go`**: Removed `TargetRepoOverride` field from `RemediationJobSpec`
4. **`internal/config/config.go`**: Removed `MendabotUpstreamRepo` and `DisableUpstreamContributions` fields
5. **`internal/jobbuilder/job.go`**: Removed `TARGET_REPO_OVERRIDE` environment variable injection
6. **`deploy/kustomize/configmap-prompt.yaml`**: Removed upstream guidance from agent prompt
7. **`deploy/kustomize/deployment-watcher.yaml`**: Removed upstream configuration environment variables

### Current Implementation

Self-remediation cascade prevention now focuses on:
1. **Chain depth tracking**: Prevents infinite recursion
2. **Circuit breaker**: Prevents rapid cascades
3. **Max depth enforcement**: Configurable limit (default: 2)

All PRs are created against the user's configured `GITOPS_REPO` only.

---

 ## Tasks

**REMOVED**: All upstream routing tasks have been removed.

 - [ ] Add TargetRepoOverride field to domain.Finding and RemediationJobSpec - REMOVED
 - [ ] Implement upstream routing logic in JobProvider (depth ≥ 2) - REMOVED
 - [ ] Add configuration for upstream repository and disable flag - REMOVED
 - [ ] Update JobBuilder to inject target repo environment variable - REMOVED
 - [ ] Update agent prompt to handle upstream repository investigation - REMOVED
 - [ ] Write unit tests for routing logic - REMOVED
 - [ ] Write integration tests for upstream contribution scenarios - REMOVED
 - [ ] Document configuration options - REMOVED

---

## Dependencies

**Depends on:** STORY_02_chain_depth_tracking
**Blocks:** STORY_07_configuration (configuration documentation)

---

 ## Definition of Done

**REMOVED**: Feature removed from codebase.

 - [ ] All tests pass with `-race` - N/A
 - [ ] `go vet` clean - N/A
 - [ ] Upstream routing works correctly at depth ≥ 2 - REMOVED
 - [ ] Configuration options validated and documented - REMOVED
 - [ ] Disable flag functions correctly - REMOVED
 - [ ] Agent receives correct target repository - REMOVED
 - [ ] Backward compatibility maintained (no routing for depth < 2) - REMOVED