# Epic 26: Auto-Close Sinks on Remediation Resolution

## Purpose

When a finding clears — the deployment recovers, the PVC is provisioned, the node
returns to Ready — the `SourceProviderReconciler` already cancels `Pending` and
`Dispatched` `RemediationJob` objects. But if the agent opened a PR or a GitHub issue
before the finding cleared, those sinks remain open indefinitely. A human must manually
close them. For clusters with frequent transient failures this produces a backlog of
stale PRs and issues that obscures genuinely important ones.

This epic implements automatic sink closure when a `RemediationJob` reaches a terminal
state because its source finding has resolved.

## Status: Not Started

## Dependencies

- epic01-controller complete (`SourceProviderReconciler`, `RemediationJobReconciler`)
- epic04-deploy complete (watcher Deployment has access to GitHub App credentials)
- epic09-native-provider complete (native source providers that emit the findings)

## Blocks

- epic27-pr-feedback-iteration (feedback loop depends on the watcher being able to
  interact with open PRs — shares the GitHub client infrastructure built here)

## Success Criteria

- [ ] `SinkCloser` interface exists in `internal/domain/sink.go` with method
      `Close(ctx, remediationJob, reason string) error`
- [ ] `GitHubSinkCloser` implementation in `internal/sink/github/` that calls
      `gh pr close` and/or `gh issue close` based on `RemediationJob.status.sinkRef`
- [ ] `SourceProviderReconciler` calls `SinkCloser.Close` when cancelling a
      `RemediationJob` whose `status.prRef` or `status.issueRef` is non-empty
- [ ] Closure comment is human-readable and explains why the sink was closed:
      `"Closing automatically: the underlying issue (Pod/my-app CrashLoopBackOff) has
      resolved. No manual fix is required."`
- [ ] `PR_AUTO_CLOSE=false` env var disables auto-close (default: `true`)
- [ ] Watcher Deployment manifest mounts the GitHub App credentials Secret
- [ ] `go test -timeout 30s -race ./...` passes
- [ ] Worklog entry created

## Design

### SinkRef model

`RemediationJob.status` gains a `SinkRef` field (set by the agent via a status patch
after it opens a PR or issue):

```go
type RemediationJobStatus struct {
    // ... existing fields ...
    SinkRef SinkRef `json:"sinkRef,omitempty"`
}

type SinkRef struct {
    // Type is "pr" or "issue"
    Type    string `json:"type"`
    // URL is the full URL to the PR or issue (e.g. https://github.com/org/repo/pull/42)
    URL     string `json:"url"`
    // Number is the numeric ID for API calls
    Number  int    `json:"number"`
    // Repo is "owner/repo" format
    Repo    string `json:"repo"`
}
```

The existing `status.prRef` string is kept for backwards compatibility but the new
`SinkRef` struct is the canonical representation going forward.

### SinkCloser interface

```go
// SinkCloser closes an open sink (PR or issue) when the underlying finding resolves.
type SinkCloser interface {
    // Close closes the sink referenced by the RemediationJob's SinkRef.
    // reason is a human-readable explanation included in the closing comment.
    // Returns nil if the sink is already closed or the SinkRef is empty.
    Close(ctx context.Context, rjob *v1alpha1.RemediationJob, reason string) error
}
```

### GitHub App token in the watcher

The watcher currently does not mount the GitHub App credentials — only the agent Job
does (via `get-github-app-token.sh`). This epic adds a mounted Secret and a
`GitHubTokenProvider` that the watcher uses:

1. `internal/github/token.go` — exchanges App private key for installation token,
   caches it with a 55-minute TTL (refreshes before the 1-hour GitHub App expiry)
2. Token is injected into `GitHubSinkCloser` at construction time in `cmd/watcher/main.go`

The same token provider will be reused by epic27.

### Trigger condition in SourceProviderReconciler

In the `finding-cleared` path (object not found or no active error):

```go
if rjob.Status.Phase == v1alpha1.PhasePending || rjob.Status.Phase == v1alpha1.PhaseDispatched {
    if rjob.Status.SinkRef.URL != "" {
        reason := fmt.Sprintf("the underlying issue (%s/%s %s) has resolved",
            rjob.Spec.FindingKind, rjob.Spec.FindingName, rjob.Spec.FindingNamespace)
        if err := r.SinkCloser.Close(ctx, rjob, reason); err != nil {
            // log and continue — sink closure failure must not block RemediationJob cancellation
            log.Error(err, "failed to close sink", "sinkRef", rjob.Status.SinkRef.URL)
        }
    }
    // existing cancellation logic ...
}
```

Sink closure failure is logged and ignored — it must not block the `RemediationJob`
state transition. Idempotency: `gh pr close` and `gh issue close` are idempotent on
already-closed sinks.

### Configuration

```bash
# Disable automatic sink closure when a finding resolves (default: true)
PR_AUTO_CLOSE=true
```

## Stories

| Story | File | Status | Priority | Effort |
|-------|------|--------|----------|--------|
| SinkRef domain type + SinkCloser interface | [STORY_00_domain_types.md](STORY_00_domain_types.md) | Not Started | High | 1h |
| GitHub App token provider in watcher | [STORY_01_watcher_github_token.md](STORY_01_watcher_github_token.md) | Not Started | High | 2h |
| GitHubSinkCloser implementation | [STORY_02_github_sink_closer.md](STORY_02_github_sink_closer.md) | Not Started | High | 2h |
| Wire SinkCloser into SourceProviderReconciler | [STORY_03_wire_reconciler.md](STORY_03_wire_reconciler.md) | Not Started | Critical | 2h |
| Deploy manifest: Secret mount + PR_AUTO_CLOSE | [STORY_04_deploy.md](STORY_04_deploy.md) | Not Started | Medium | 1h |

## Technical Overview

### New files

| File | Purpose |
|------|---------|
| `internal/domain/sink.go` | `SinkRef` type, `SinkCloser` interface |
| `internal/domain/sink_test.go` | Unit tests |
| `internal/github/token.go` | GitHub App → installation token exchange + cache |
| `internal/github/token_test.go` | Token provider tests (mock HTTP) |
| `internal/sink/github/closer.go` | `GitHubSinkCloser` — calls `gh pr/issue close` |
| `internal/sink/github/closer_test.go` | Unit tests with `gh` command mock |

### Modified files

| File | Change |
|------|--------|
| `api/v1alpha1/remediationjob_types.go` | Add `SinkRef` struct + field to `RemediationJobStatus` |
| `internal/provider/provider.go` | Call `SinkCloser.Close` in finding-cleared path; accept `SinkCloser` dependency |
| `internal/config/config.go` | Add `PRAutoClose bool` |
| `deploy/kustomize/deployment-watcher.yaml` | Mount GitHub App Secret; add `PR_AUTO_CLOSE` env var |
| `charts/mendabot/templates/deployment-watcher.yaml` | Same as above for Helm chart |
| `testdata/crds/remediationjob_crd.yaml` | Add `sinkRef` to status schema |

## Definition of Done

- [ ] All unit tests pass: `go test -timeout 30s -race ./...`
- [ ] `go build ./...` succeeds
- [ ] `PR_AUTO_CLOSE=false` disables closure (verified by test)
- [ ] Closure comment appears on the GitHub PR/issue (manual verification in dev cluster)
- [ ] `SinkCloser` failure does not block `RemediationJob` cancellation (tested)
- [ ] Worklog entry created in `docs/WORKLOGS/`
