# Story 03: JobBuilder Multi-Finding Support

**Epic:** [epic13-multi-signal-correlation](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 2 hours

---

## User Story

As a **mendabot developer**, I want `JobBuilder.Build()` to accept a slice of correlated
findings and inject them as a `FINDING_CORRELATED_FINDINGS` env var, so that the agent
Job for a correlated group receives the full group context rather than a single finding.

---

## Background

Today `JobBuilder.Build()` builds a Job spec from a single `RemediationJob`. After
correlation, the primary `RemediationJob` may represent a group of related findings. The
agent must know about all of them to produce a coherent investigation and PR.

The change is additive: when no correlated findings are present (the common case), the
env var is absent or empty and existing agent behaviour is unchanged.

---

## Acceptance Criteria

- [ ] `JobBuilder.Build()` signature changes to accept an additional
      `correlatedFindings []v1alpha1.FindingSpec` parameter (nil or empty = single-finding mode)
- [ ] When `len(correlatedFindings) > 1`, a `FINDING_CORRELATED_FINDINGS` env var is
      injected into the main container as a JSON-encoded array of `FindingSpec` objects
- [ ] When `len(correlatedFindings) <= 1`, `FINDING_CORRELATED_FINDINGS` is not set
      (no empty env var pollution)
- [ ] `FINDING_CORRELATION_GROUP_ID` env var is injected when the primary
      `RemediationJob` carries a `mendabot.io/correlation-group-id` label
- [ ] `internal/jobbuilder/job_test.go` covers:
  - Single finding (no correlated env var set)
  - Two correlated findings (env var set, valid JSON)
  - JSON encodes all `FindingSpec` fields correctly
- [ ] `go test -timeout 30s -race ./internal/jobbuilder/...` passes

---

## Technical Implementation

### Signature change

```go
// Before
func (b *Builder) Build(rjob *v1alpha1.RemediationJob) (*batchv1.Job, error)

// After
func (b *Builder) Build(rjob *v1alpha1.RemediationJob, correlatedFindings []v1alpha1.FindingSpec) (*batchv1.Job, error)
```

All existing call sites pass `nil` for `correlatedFindings` until STORY_02 wires the
full group context. The reconciler (STORY_02) passes the full findings slice.

### Env var injection

```go
if len(correlatedFindings) > 1 {
    raw, err := json.Marshal(correlatedFindings)
    if err != nil {
        return nil, fmt.Errorf("jobbuilder: marshal correlated findings: %w", err)
    }
    env = append(env, corev1.EnvVar{
        Name:  "FINDING_CORRELATED_FINDINGS",
        Value: string(raw),
    })
}

if groupID, ok := rjob.Labels[domain.CorrelationGroupIDLabel]; ok && groupID != "" {
    env = append(env, corev1.EnvVar{
        Name:  "FINDING_CORRELATION_GROUP_ID",
        Value: groupID,
    })
}
```

### Callers to update

- `internal/controller/remediationjob_controller.go` — primary call site
- Any test helpers that call `Build()` directly

---

## Tasks

- [ ] Write new `job_test.go` cases for multi-finding injection (TDD — must fail first)
- [ ] Update `Builder.Build()` signature and inject `FINDING_CORRELATED_FINDINGS`
- [ ] Update all call sites of `Build()` to pass `nil` (existing) or the findings slice (STORY_02 path)
- [ ] Run `go test -timeout 30s -race ./...` — must pass

---

## Dependencies

**Depends on:** STORY_00 (domain types, `CorrelationGroupIDLabel` constant)
**Blocks:** STORY_04 (prompt must reference the env var that this story injects)

---

## Definition of Done

- [ ] `Build()` signature updated; all call sites compile
- [ ] Env vars injected correctly for both single and multi-finding cases
- [ ] All tests pass
