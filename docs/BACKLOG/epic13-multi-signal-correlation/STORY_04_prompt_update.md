# Story 04: Prompt Template Update for Correlated Context

**Epic:** [epic13-multi-signal-correlation](README.md)
**Priority:** Medium
**Status:** Not Started
**Estimated Effort:** 1 hour

---

## User Story

As a **mendabot operator**, I want the agent prompt to instruct the agent to use the
`FINDING_CORRELATED_FINDINGS` env var when investigating a correlated group, so that
the agent produces a single coherent PR covering all related findings rather than
investigating only the primary finding in isolation.

---

## Background

`FINDING_CORRELATED_FINDINGS` is only set when the primary `RemediationJob` is part of a
correlation group (STORY_03). When it is set, the agent must:

1. Acknowledge that this finding is part of a group
2. Investigate the root cause that explains all findings in the group
3. Propose a single fix that resolves the entire group
4. Reference all correlated findings in the PR body

This is a prompt-only change. Zero Go code.

---

## Acceptance Criteria

- [ ] `deploy/kustomize/configmap-prompt.yaml` has a new section in STEP 0 (Context
      Loading) that reads `FINDING_CORRELATED_FINDINGS` when non-empty
- [ ] The prompt instructs the agent that if `FINDING_CORRELATED_FINDINGS` is set, the
      investigation must explain all findings in the group, not just the primary
- [ ] PR body template includes a `## Correlated Findings` section (only rendered when
      the env var is non-empty)
- [ ] A new HARD RULE is added: when `FINDING_CORRELATED_FINDINGS` is set, the agent
      MUST NOT open separate PRs for the correlated findings — one PR covers the group
- [ ] Prompt change is reviewed for consistency with the existing STEP 0–10 structure

---

## Technical Implementation

### Addition to STEP 0 — Context Loading

```
CORRELATED_FINDINGS="${FINDING_CORRELATED_FINDINGS:-}"
CORRELATION_GROUP_ID="${FINDING_CORRELATION_GROUP_ID:-}"

if [ -n "$CORRELATED_FINDINGS" ]; then
  echo "=== CORRELATED GROUP: $CORRELATION_GROUP_ID ==="
  echo "This finding is part of a correlated group. All findings:"
  echo "$CORRELATED_FINDINGS" | jq -r '.[] | "  - \(.Kind)/\(.Name) in \(.Namespace): \(.Errors | join("; "))"'
  echo "Your investigation MUST explain the root cause common to ALL findings above."
  echo "Your PR MUST fix the root cause, not individual symptoms."
fi
```

### New HARD RULE

```
HARD RULE 10 — CORRELATED GROUP:
If FINDING_CORRELATED_FINDINGS is set and non-empty, your proposed fix MUST address
the shared root cause of all findings in the group. You MUST NOT open multiple PRs for
findings in the same group. The PR body MUST include a ## Correlated Findings section
listing all finding IDs in the group and explaining how the fix resolves each.
```

### PR body template addition

```markdown
## Correlated Findings
<!-- Only include when CORRELATION_GROUP_ID is set -->
This PR resolves a correlated group of findings (group: `${CORRELATION_GROUP_ID}`):
${FINDING_CORRELATED_FINDINGS_SUMMARY}
```

---

## Tasks

- [ ] Update `deploy/kustomize/configmap-prompt.yaml` with STEP 0 addition
- [ ] Add HARD RULE 10 to the prompt's HARD RULES section
- [ ] Add `## Correlated Findings` section to the PR body template in STEP 10
- [ ] Verify `kubectl apply -k deploy/kustomize/ --dry-run=client` passes (no YAML errors)

---

## Dependencies

**Depends on:** STORY_03 (env var names must match what `JobBuilder` injects)
**Blocks:** STORY_05 (integration tests validate the full path including the prompt)

---

## Definition of Done

- [ ] Prompt template updated with correlated context handling
- [ ] HARD RULE 10 added
- [ ] `kubectl apply -k deploy/kustomize/ --dry-run=client` passes
- [ ] No existing tests broken (prompt is in a ConfigMap; no Go tests cover its content directly)
