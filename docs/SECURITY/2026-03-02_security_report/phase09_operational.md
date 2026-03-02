# Phase 9: Operational Security

**Date run:** 2026-03-02
**Reviewer:** automated (k8s-mechanic security process)

---

## 9.1 Secret Placeholder Audit

| Check | Result | Notes |
|-------|--------|-------|
| Both Secrets contain only placeholder values | pass | `<PLACEHOLDER>` values only in deploy/kustomize/ |
| Neither placeholder is applied by default kustomization | pass | Operator must apply explicitly |
| Documentation instructs operators to replace before deployment | pass | |

**Findings:** none

---

## 9.2 Configuration Security

| Check | Result | Notes |
|-------|--------|-------|
| `FromEnv()` validates `AGENT_RBAC_SCOPE` — error on invalid value | pass | Returns error on unrecognised scope value |
| `AGENT_WATCH_NAMESPACES` required when scope=namespace | pass | Validated at startup |
| Default `INJECTION_DETECTION_ACTION` is `log` — weaker than `suppress` | documented | Acceptable trade-off for day-1 operators; documented in CHECKLIST.md |
| No config values from Secrets are logged | pass | |
| `EXTRA_REDACT_PATTERNS` validated at startup | pass | Invalid regex causes startup failure |

**Findings:** none

---

## 9.3 Error Message Information Disclosure

Manual review of error paths in `internal/` — no error messages observed that expose
internal file paths beyond what is necessary, stack traces, or credential values derived
from Secret or Finding fields.

**Result:** No disclosure issues identified.

**Findings:** none

---

## 9.4 Job Security Settings

| Setting | Present? | Value | Adequate? |
|---------|---------|-------|----------|
| `activeDeadlineSeconds` | yes | 900 (15 min) | yes |
| `ttlSecondsAfterFinished` | yes | 86400 (1 day) | yes |
| `backoffLimit` | yes | 1 | yes |
| `restartPolicy: Never` | yes | Never | yes |

**Findings:** none

---

## 9.5 Git Wrapper Dry-Run Blocklist

Finding 2026-02-27-003 was partially remediated. Current blocklist:

| Subcommand | Blocked in dry-run? |
|------------|---------------------|
| `push` | yes |
| `commit` | yes |
| `reset` | yes |
| `rm` | yes |
| `clean` | yes |
| `rebase` | yes |
| `config --global/--system` | yes |
| `remote set-url` | yes |
| `tag -a/-s` | yes |
| `checkout -b/--orphan` | **no** — finding 2026-03-02-004 |

**Finding:** 2026-03-02-004 (LOW) — `checkout -b` and `--orphan` not blocked in dry-run
(cross-referenced from Phase 3).

---

## 9.6 gh Wrapper Dry-Run Allowlist

Finding 2026-02-27-004 confirmed remediated. The `gh` wrapper uses an allowlist approach:

| Check | Result |
|-------|--------|
| `gh api` with GET method allowed | yes |
| `gh api` with POST/PUT/PATCH/DELETE blocked | yes |
| `gh pr create`, `gh issue create`, etc. blocked | yes |
| Fail-closed on unrecognised patterns | yes |

**Findings:** none

---

## 9.7 emit_dry_run_report Redaction

Finding 2026-02-27-009 confirmed remediated:

| Check | Result |
|-------|--------|
| `investigation-report.txt` piped through `redact` before ConfigMap write | pass |
| `git diff HEAD` output piped through `redact` | pass |
| Hard-fail if `redact` not in PATH | pass |

**Findings:** none

---

## 9.8 AGENT_PROVIDER_CONFIG (EX-009)

EX-009 confirmed remediated:

| Check | Result |
|-------|--------|
| `AGENT_PROVIDER_CONFIG` unset before launching opencode | pass |
| opencode config written to `/tmp/opencode-config.json` | pass |
| `AGENT_PROVIDER_CONFIG` absent from agent process environment | pass |

**Findings:** none

---

## Phase 9 Summary

**Total findings:** 1 (LOW — 2026-03-02-004, cross-referenced from Phase 3)
**Findings added to findings.md:** 2026-03-02-004
