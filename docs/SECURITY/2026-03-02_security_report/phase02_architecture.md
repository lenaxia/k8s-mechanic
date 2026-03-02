# Phase 2: Architecture and Design Review

**Date run:** 2026-03-02
**Reviewer:** automated (k8s-mechanic security process)

---

## 2.1 Data Flow — Path 1: Error message → LLM prompt

### PodProvider (`internal/provider/native/pod.go`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| `State.Waiting.Message` truncation before `RedactSecrets` | pass | — | `buildWaitingText()` calls `redactor.Redact()` then `truncate()` |
| `State.Waiting.Message` — `RedactSecrets` called | pass | — | Confirmed via `p.redactor.Redact(msg)` |
| `State.Terminated.Message` — `RedactSecrets` called | pass | 91 | `p.redactor.Redact(msg)` |
| Condition messages — `RedactSecrets` called | pass | 105 | `p.redactor.Redact(cond.Message)` |
| No bypass path around truncation + redaction | pass | — | All text paths reviewed |

### DeploymentProvider (`internal/provider/native/deployment.go`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| All free-form text — truncation applied | pass | — | |
| All free-form text — `RedactSecrets` called | pass | 74 | `p.redactor.Redact(cond.Message)` on Available=False condition |

### StatefulSetProvider (`internal/provider/native/statefulset.go`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| All free-form text — truncation applied | pass | — | Pattern consistent with DeploymentProvider |
| All free-form text — `RedactSecrets` called | pass | — | |

### JobProvider (`internal/provider/native/job.go`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| All free-form text — truncation applied | pass | — | |
| All free-form text — `RedactSecrets` called | pass | — | |

### NodeProvider (`internal/provider/native/node.go`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| All free-form text — truncation applied | pass | — | |
| All free-form text — `RedactSecrets` called | pass | — | |

### PVCProvider (`internal/provider/native/pvc.go`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| All free-form text — truncation applied | pass | — | |
| All free-form text — `RedactSecrets` called | pass | — | |

### SourceProviderReconciler (`internal/provider/provider.go`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| `DetectInjection` called on `finding.Errors` | pass | 190 | |
| `DetectInjection` called on `finding.Details` | pass | 207 | |
| Detection fires before job creation | pass | — | Checked before `JobBuilder.Build()` at controller.go:628 |
| `INJECTION_DETECTION_ACTION=suppress` returns before creation | pass | — | Suppress path exits before dispatch |

### JobBuilder (`internal/jobbuilder/job.go`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| `FINDING_ERRORS` is the only env var with untrusted error text | pass | — | |
| `FINDING_DETAILS` assessed for untrusted content | assessed | — | Goes through DetectInjection; not RedactSecrets (see 2026-02-27-014, deferred) |
| All `Finding` fields injected as env vars reviewed | pass | — | `FINDING_CORRELATED_FINDINGS` gap noted (2026-03-02-001) |

### Agent entrypoint (`scripts/entrypoint-common.sh`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| `envsubst` restricts substitutions to known variable list | pass | 127 | Explicitly lists all 9 vars |
| Rendered prompt written to temp file (not inline) | pass | — | |
| Temp file path not attacker-controlled | pass | — | |
| No double-expansion of variables | pass | — | |

### Prompt template

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| Untrusted-data envelope around `${FINDING_ERRORS}` | pass | — | `<<<MECHANIC:UNTRUSTED_INPUT:...:BEGIN\|END>>>` present |
| HARD RULE 8 present and unambiguous | pass | — | |
| `${FINDING_CORRELATED_FINDINGS}` envelope assessed | pass | — | Envelope present; DetectInjection gap noted as 2026-03-02-001 |

**Findings:** 2026-03-02-001 (LOW) — `FINDING_CORRELATED_FINDINGS` not checked by `DetectInjection` before prompt injection

---

## 2.2 RBAC Audit

### ClusterRole: mechanic-agent

| Check | Result | Notes |
|-------|--------|-------|
| No write verbs | pass | Only `get/list/watch` and `remediationjobs/status: get/patch` |
| No `pods/exec` | pass | Explicit resource enumeration, no wildcards |
| No `nodes/proxy` | pass | |
| Namespace scope replaces (not supplements) ClusterRole | N/A | Covered by overlay audit below |

### ClusterRole: mechanic-watcher

| Check | Result | Notes |
|-------|--------|-------|
| ConfigMap write is namespace-scoped | pass | |
| No write outside mechanic ns (except RemediationJobs) | pass | |
| `delete` on remediationjobs — blast radius acceptable | pass | Scoped to RemediationJob resources only |

**Note on `secrets`:** The watcher ClusterRole retains `secrets: get/list/watch`. This is
AR-08 (accepted). The namespace Role has correct `get/list/watch` with `cache.ByObject`.
The ClusterRole entry is redundant pending live cluster verification (finding 2026-02-27-001).

### Role: mechanic-agent (namespace-scoped)

| Check | Result | Notes |
|-------|--------|-------|
| Status patch scoped to `remediationjobs/status` subresource only | pass | |
| Agent cannot update full remediationjobs spec | pass | |

### deploy/overlays/security/role-agent-ns.yaml (namespace-scope overlay)

| Check | Result | Notes |
|-------|--------|-------|
| Resources are explicitly enumerated (no wildcard) | **fail** | `resources: ["*"]` includes Secrets — see finding 2026-03-02-002 |

**Findings:** 2026-03-02-002 (MEDIUM) — `role-agent-ns.yaml` wildcard resources grant Secret reads

---

## 2.3 Secret Handling Audit

### GitHub App private key

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| Key read from volume mount, not env var (watcher) | pass | main.go:233 | `os.ReadFile()` — finding 2026-02-27-002 remediated |
| `GITHUB_APP_PRIVATE_KEY` absent from watcher Deployment env | pass | — | No env var in deployment-watcher.yaml |
| `GITHUB_APP_*` vars in init container Env only (agent Job) | pass | job.go:131-163 | Main container has no reference to github-app in Env or VolumeMounts |
| Main container has no `github-app-secret` reference | pass | — | |
| Shared emptyDir contains only the short-lived token | pass | — | |

### LLM API key

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| `OPENAI_API_KEY` sourced from Secret | pass | — | |
| Key not printed or logged in entrypoint | pass | — | |
| opencode config written to `/tmp/opencode-config.json` | pass | — | EX-009 remediated; `AGENT_PROVIDER_CONFIG` unset before launch |

### Token file

| Check | Result | Notes |
|-------|--------|-------|
| Token read from `/workspace/github-token` | pass | |
| Entrypoint does not log/echo token value | pass | |
| Token file path not attacker-influenced | pass | |

**Findings:** none (all checks pass)

---

## 2.4 Container Security Audit

### Dockerfile.agent

| Check | Result | Notes |
|-------|--------|-------|
| Non-root USER instruction | pass | `USER agent` (uid 1000) |
| All binary downloads have SHA256 checksum | pass | kubectl, helm, flux, talosctl, kustomize, yq, kubeconform, stern, sops, opencode all verified |
| `--no-install-recommends` on apt-get | pass | |
| Package lists cleaned after install | pass | |
| No secrets in ARG or ENV | pass | |
| Base image pinned to digest | pass | `debian:bookworm-slim@sha256:6458e6...` |
| Multi-stage build | pass | |
| `ReadOnlyRootFilesystem: true` | pass | Main container only (by design) |
| `age`/`age-keygen` | pass | Compiled from source via age-builder stage |
| `gh` CLI | pass | GPG-signed apt repo |

**Note:** Go builder image labelled `golang:1.25.7-bookworm` — version may be inaccurate (finding 2026-03-02-003, INFO).

### Dockerfile.watcher

| Check | Result | Notes |
|-------|--------|-------|
| Non-root USER instruction | pass | `USER watcher` (uid 1000) |
| Multi-stage build — build tools excluded from final image | pass | |
| No secrets in ARG or ENV | pass | |

**Findings:** 2026-03-02-003 (INFO) — Go builder image version label inaccurate

---

## 2.5 CI/CD Pipeline Audit

| Check | Workflow | Result | Notes |
|-------|---------|--------|-------|
| `permissions: contents: read` or equivalent | build-watcher.yaml | pass | `contents: read, packages: write` |
| `permissions: contents: read` or equivalent | build-agent.yaml | pass | |
| All third-party Actions pinned to commit SHA | all | pass | All 4 third-party actions pinned with SHA comments |
| `anomalyco/opencode/github` pinned | all | pass | `0cf0294787322664c6d668fa5ab0a9ce26796f78` — finding 2026-02-27-007 remediated |
| `ai-comment.yml` gated on `author_association` | ai-comment.yml | pass | `OWNER/MEMBER/COLLABORATOR` only — finding 2026-02-27-013 remediated |
| `renovate-analysis.yml` LLM merge authority | renovate-analysis.yml | pass (re-classified) | LLM posts comment only; does not merge — finding 2026-02-27-008 re-classified INFO/Accepted |
| Trivy scan step in CI | build-watcher.yaml | pass | Checksum-verified Trivy install |
| No fork PR secret exposure | all | pass | |

**Findings:** none

---

## Phase 2 Summary

**Total findings:** 3
**Findings added to findings.md:** 2026-03-02-001, 2026-03-02-002, 2026-03-02-003
