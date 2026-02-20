# High-Level Design

**Version:** 1.0
**Date:** 2026-02-19
**Status:** Authoritative Specification

---

## Document Control

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0 | 2026-02-19 | Initial HLD | LLM / Human |
| 1.1 | 2026-02-19 | Design review fixes: init image, token path, FINDING_NAMESPACE, fingerprint, AGENT_NAMESPACE constraint, config table, RBAC, data flow | LLM / Human |

---

## Table of Contents

1. [Problem Statement](#1-problem-statement)
2. [Goals and Non-Goals](#2-goals-and-non-goals)
3. [System Overview](#3-system-overview)
4. [Component Design](#4-component-design)
5. [Data Flow](#5-data-flow)
6. [Deduplication Strategy](#6-deduplication-strategy)
7. [RBAC Design](#7-rbac-design)
8. [GitHub Authentication](#8-github-authentication)
9. [Agent Investigation Strategy](#9-agent-investigation-strategy)
10. [Security Constraints](#10-security-constraints)
11. [Failure Modes](#11-failure-modes)
12. [Configuration Reference](#12-configuration-reference)
13. [Deployment Model](#13-deployment-model)
14. [Upstream Contribution Path](#14-upstream-contribution-path)
15. [v1 Scope](#15-v1-scope)
16. [Success Criteria](#16-success-criteria)

---

## 1. Problem Statement

The k8sgpt-operator already analyses Kubernetes clusters and writes `Result` CRDs describing
problems it finds. These results include an AI-generated explanation of the problem. However,
nothing acts on them — a human must still read the results, investigate the cluster, locate
the relevant GitOps manifests, determine a fix, and open a PR.

This project automates that entire loop: from Result CRD to a PR on the GitOps repository
containing a proposed fix, with the investigation and reasoning documented inline.

---

## 2. Goals and Non-Goals

### Goals

- Watch `Result` CRDs cluster-wide and react to new findings
- Deduplicate findings so multiple pods from the same bad Deployment produce one investigation
- Spawn an isolated, short-lived Kubernetes Job per unique finding
- Give the agent read-only cluster access via in-cluster ServiceAccount
- Have the agent clone the GitOps repo, investigate, and open a PR with a proposed fix
- Avoid creating duplicate PRs — detect existing open PRs for the same finding
- Be self-contained: deploy via Kustomize, no external state store required

### Non-Goals

- Automatically merging PRs (human review required)
- Remediating cluster state directly (no `kubectl apply` from the agent)
- Replacing the k8sgpt-operator (this project depends on it)
- Supporting GitOps repos other than Flux + Kustomize/Helm (out of v1 scope)
- Persisting deduplication state across watcher restarts (acceptable limitation in v1)

---

## 3. System Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│  Kubernetes Cluster                                                  │
│                                                                      │
│  ┌──────────────────┐  writes   ┌──────────────────────────────┐   │
│  │  k8sgpt-operator │ ────────▶ │  Result CRDs                 │   │
│  │  (pre-existing)  │           │  (results.core.k8sgpt.ai)    │   │
│  └──────────────────┘           └──────────────┬───────────────┘   │
│                                                 │ watch             │
│                                  ┌──────────────▼───────────────┐  │
│                                  │  mendabot-watcher             │  │
│                                  │  (Deployment, 1 replica)      │  │
│                                  │                               │  │
│                                  │  - controller-runtime         │  │
│                                  │  - in-memory fingerprint map  │  │
│                                  │  - creates batch/v1 Jobs      │  │
│                                  └──────────────┬───────────────┘  │
│                                                 │ creates           │
│                                  ┌──────────────▼───────────────┐  │
│                                  │  mendabot-agent Job           │  │
│                                  │  (one per unique fingerprint) │  │
│                                  │                               │  │
│                                  │  initContainer:               │  │
│                                  │    git-token-clone            │  │
│                                  │    (debian:bookworm-slim)     │  │
│                                  │  container:                   │  │
│                                  │    mendabot-agent image       │  │
│                                  │    (debian:bookworm-slim)     │  │
│                                  │    + opencode                 │  │
│                                  │    + kubectl (read-only SA)   │  │
│                                  │    + k8sgpt                   │  │
│                                  │    + helm + flux              │  │
│                                  │    + gh                       │  │
│                                  └───────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
                                          │
                                          ▼ opens PR (or comments)
                         ┌────────────────────────────────┐
                         │  lenaxia/talos-ops-prod         │
                         │  (Flux + Kustomize/Helm GitOps) │
                         └────────────────────────────────┘
```

---

## 4. Component Design

### 4.1 mendabot-watcher

A single-binary Go controller built on `controller-runtime`. It runs as a single-replica
Deployment in the `mendabot-watcher` namespace.

**Responsibilities:**
- Register an informer against `results.core.k8sgpt.ai` across all namespaces
- On each reconcile, compute the parent-resource fingerprint of the Result
- Check the in-memory processed map — skip if already seen with the same fingerprint
- Build a `batch/v1 Job` spec and create it via the Kubernetes API
- Record the fingerprint and Job name in the in-memory map

**State:** Entirely in-memory. On restart, all Result CRDs will re-trigger reconciliation.
This is safe because the agent checks for existing PRs before creating a new one.

**Concurrency:** controller-runtime's default single-worker reconcile loop is sufficient.
The watcher does not need parallelism — Job creation is fast and we deliberately want
sequential, predictable dispatch.

### 4.2 mendabot-agent Job

A `batch/v1 Job` created dynamically per unique finding. It is not stored as a static
manifest — the watcher generates it at runtime from the Result spec.

**Init container** (`ghcr.io/lenaxia/mendabot-agent` — same image as main container):
- Calls `get-github-app-token.sh` to exchange the GitHub App private key for a
  short-lived installation token
- Writes the token to a shared `emptyDir` volume at `/workspace/github-token`
- Clones the GitOps repo using the token into `/workspace/repo`

**Main container** (`ghcr.io/lenaxia/mendabot-agent`):
- Receives the finding as environment variables (kind, name, errors, details, fingerprint)
- Reads the rendered prompt from a mounted ConfigMap
- Runs `opencode run "<prompt>"` with in-cluster kubeconfig (automatic, via ServiceAccount)
- OpenCode uses `kubectl`, `k8sgpt`, `gh`, and Git to investigate and open a PR

**Job settings:**

| Setting | Value | Reason |
|---|---|---|
| `restartPolicy` | `Never` | Failed investigations should not silently retry |
| `backoffLimit` | `1` | Allow one retry on container crash only |
| `activeDeadlineSeconds` | `900` | 15 min hard cap; prevents runaway LLM sessions |
| `ttlSecondsAfterFinished` | `86400` | Clean up after 1 day |
| Name | `mendabot-agent-<12-char-fingerprint>` | Deterministic, collision-resistant |

### 4.3 mendabot-agent Docker image

Built on `debian:bookworm-slim`. Contains:

| Binary | Source |
|---|---|
| `opencode` | Official release binary (pinned via `ARG OPENCODE_VERSION`) |
| `kubectl` | Official release binary |
| `k8sgpt` | Official release binary |
| `helm` | Official release binary |
| `flux` | Official release binary |
| `gh` | GitHub CLI deb package |
| `git` | apt |
| `get-github-app-token.sh` | Custom script in this repo |

---

## 5. Data Flow

```
1. k8sgpt-operator writes Result CRD
      result.spec.kind         = "Pod"
      result.spec.parentObject = "my-deployment"
      result.spec.error[]      = [{text: "Back-off restarting failed container"}]
      result.spec.details      = "<LLM explanation>"

2. mendabot-watcher reconcile triggered
      fingerprint = sha256("Pod" + "my-deployment" + sorted(["Back-off..."]))
      check in-memory map → not seen → proceed

3. watcher creates Job "mendabot-agent-a3f9c2b14d8e"
      env: FINDING_KIND=Pod
           FINDING_NAME=my-deployment-abc12-xyz34
           FINDING_NAMESPACE=default
           FINDING_PARENT=my-deployment
           FINDING_ERRORS=[{"text":"Back-off restarting failed container"}]
           FINDING_DETAILS=<LLM text>
           FINDING_FINGERPRINT=a3f9c2b14d8e...
           GITOPS_REPO=lenaxia/talos-ops-prod
           GITOPS_MANIFEST_ROOT=kubernetes

4. Job init container
      → get-github-app-token.sh → writes /workspace/github-token
      → git clone https://x-access-token:<token>@github.com/lenaxia/talos-ops-prod /workspace/repo

5. Job main container
      → opencode run "<rendered prompt>"
      → OpenCode calls: kubectl describe pod, kubectl get events,
                        k8sgpt analyze, gh pr list, git diff, gh pr create

6. Outcome A — fix found:
      → branch: fix/k8sgpt-a3f9c2b14d8e
      → PR title: "fix(Pod/my-deployment): back-off restarting failed container"
      → PR body: investigation findings + proposed change + k8sgpt reference

   Outcome B — existing PR found:
      → gh issue comment on existing PR with updated findings

   Outcome C — no safe fix identified:
      → PR opened with investigation report only, labelled "needs-human-review"
```

---

## 6. Deduplication Strategy

### Fingerprint algorithm

The fingerprint is a deterministic SHA256 hash that incorporates the Result's namespace,
kind, parent object name, and sorted error texts. The authoritative algorithm is defined in
[CONTROLLER_LLD.md §4](docs/DESIGN/lld/CONTROLLER_LLD.md). The HLD does not duplicate it
to avoid divergence — the LLD is the single source of truth for the exact implementation.

**Key properties:**
- Includes `namespace` — prevents cross-namespace collisions between same-named parents
- Uses `parentObject` not the resource name — collapses multiple pods from one Deployment
- Error texts are sorted — ordering in the CRD is non-deterministic
- Full 64-char hex SHA256 used as branch name; first 12 chars used as Job name suffix

**Note:** Error text is taken verbatim from the k8sgpt Result CRD. Minor rewording by the
k8sgpt-operator between analyses of the same underlying problem will produce a different
fingerprint and trigger a new investigation. This is a known limitation documented in
§11 Failure Modes.

### Why parentObject, not resource name

The k8sgpt-operator writes one Result per failing resource. If a Deployment has 3 replicas
all CrashLooping, there will be 3 Result CRDs — one per pod. Without parent-based
deduplication, the watcher would dispatch 3 identical investigations.

By keying on `parentObject` (the Deployment name) + the error texts, all three collapse to
one fingerprint and one Job.

### When a new investigation is triggered despite an existing fingerprint

A new fingerprint — and therefore a new Job — is created when:
- The error texts change (the problem evolved or a new error appeared)
- The `parentObject` is different (genuinely different workload)
- The watcher restarted (in-memory state cleared; agent handles PR deduplication)

### Watcher restart safety

On restart, all existing Result CRDs re-reconcile. The in-memory map is empty, so the
watcher will attempt to create Jobs for all existing Results. Two mechanisms prevent this
from causing problems:

1. `errors.IsAlreadyExists` — if a Job with the same deterministic name already exists,
   the watcher records it as processed and moves on
2. The agent's PR-detection step — OpenCode searches for open PRs with the finding's
   branch name before creating a new one

---

## 7. RBAC Design

### mendabot-watcher ServiceAccount

Needs cluster-wide read access to Results and the ability to create Jobs in its own
namespace.

| Resource | Verbs | Scope |
|---|---|---|
| `results.core.k8sgpt.ai` | `get`, `list`, `watch` | ClusterRole (all namespaces) |
| `namespaces` | `get`, `list` | ClusterRole |
| `jobs.batch` | `get`, `list`, `create`, `watch`, `delete` | Role (own namespace only) |
| `pods` | `get`, `list` | Role (own namespace only, for job pod status) |

### mendabot-agent ServiceAccount

Needs read-only access across the entire cluster for investigation.

| Resource | Verbs | Scope |
|---|---|---|
| `*` (all resources) | `get`, `list`, `watch` | ClusterRole (all namespaces) |

This mirrors the permissions already granted to the k8sgpt deployment itself by the
k8sgpt Helm chart. The agent must never be granted `create`, `update`, `delete`, or `patch`
on any cluster resource.

---

## 8. GitHub Authentication

The agent uses a **GitHub App**, not a PAT. A GitHub App:
- Issues short-lived tokens (1 hour expiry) rather than long-lived credentials
- Has granular repository-level permissions
- Does not expose a personal account

**Required GitHub App permissions:**
- `Contents: Write` (create branches, push commits)
- `Pull requests: Write` (create PRs, add comments)
- `Issues: Write` (add comments to issues)

**Secret structure (Kubernetes Secret `github-app`):**

```yaml
data:
  app-id: <base64-encoded App ID>
  installation-id: <base64-encoded Installation ID>
  private-key: <base64-encoded PEM private key>
```

**Token exchange flow:**

```
init container
  → reads app-id, installation-id, private-key from mounted Secret
  → get-github-app-token.sh:
      1. Creates JWT signed with private key (RS256, 10 min expiry)
      2. POST /app/installations/<id>/access_tokens → installation token
      3. Writes token to /workspace/github-token
  → git clone https://x-access-token:<token>@github.com/... /workspace/repo
  → exits

main container
  → reads /workspace/github-token
  → gh auth login --with-token < /workspace/github-token
  → proceeds with investigation
```

---

## 9. Agent Investigation Strategy

The OpenCode agent receives the finding context via environment variables and a rendered
prompt. The prompt instructs OpenCode to follow this investigation sequence:

1. **Search for existing PRs** — `gh pr list --search "fix/k8sgpt-<fingerprint>"`. If found,
   add a comment and exit. Do not create a duplicate.

2. **Inspect the specific resource** — `kubectl describe <kind> <name> -n <namespace>`

3. **Check events** — `kubectl get events -n <namespace> --field-selector involvedObject.name=<name>`

4. **Check logs if Pod** — `kubectl logs <pod> -n <namespace> --previous`

5. **Run k8sgpt** — `k8sgpt analyze --filter <kind> --namespace <namespace> --explain`

6. **Locate GitOps manifests** — search `<GITOPS_MANIFEST_ROOT>/` in the cloned repo for
   the resource name and namespace

7. **Read related manifests** — HelmRelease, Kustomization, values files

8. **Determine root cause** — based on all gathered evidence

9. **Propose a fix** — targeted change to the GitOps manifests

10. **Open PR** — branch `fix/k8sgpt-<fingerprint>`, one PR with the minimum change needed.
    If no safe fix is determinable, open a PR with an investigation report only.

**Hard rules for the agent (enforced in the prompt):**
- Never commit to `main` directly
- Never create, modify, or reference Kubernetes Secrets
- One PR per invocation
- If uncertain, open an investigation-report PR rather than guessing

---

## 10. Security Constraints

| Constraint | Enforcement |
|---|---|
| Agent is read-only on the cluster | ClusterRole with only `get/list/watch` — no mutating verbs |
| Agent cannot write cluster Secrets | Follows from read-only RBAC |
| Agent can read cluster Secrets | `get/list/watch` on `["*"]["*"]` includes Secrets — this is a conscious accepted risk matching the permissions already granted to the k8sgpt-operator itself. Operators who consider this unacceptable must restrict the ClusterRole explicitly. |
| Agent cannot push to GitOps repo main directly | GitHub branch protection rules on target repo |
| GitHub token is short-lived | GitHub App installation token, 1 hour TTL |
| GitHub App private key never leaves the cluster | Mounted as K8s Secret into init container only; never injected into main container env |
| LLM API key never leaves the cluster | Mounted as K8s Secret, never logged or printed |
| Job has a hard deadline | `activeDeadlineSeconds: 900` |
| Job cleans itself up | `ttlSecondsAfterFinished: 86400` |
| Prompt injection risk | `FINDING_DETAILS` and `FINDING_ERRORS` originate from k8sgpt's LLM analysis of cluster state, which may be influenced by application log output or error messages an attacker can control. A crafted error message could attempt to override the agent's hard rules. Mitigations: hard rules are stated prominently in the prompt; GitHub branch protection prevents direct pushes to main; human review is required to merge any PR. |

---

## 11. Failure Modes

| Failure | Behaviour |
|---|---|
| Job creation fails (API error) | Watcher returns error, controller-runtime requeues with backoff |
| Job already exists | Watcher logs it, records as processed, moves on |
| Agent exceeds 15 min deadline | Job killed, marked Failed, watcher does not retry (fingerprint stays in map) |
| Agent crashes (container exit non-zero) | Job retries once (`backoffLimit: 1`), then marked Failed |
| OpenCode finds no fix | Agent opens an investigation-report PR labelled `needs-human-review` |
| GitHub token exchange fails | Init container exits non-zero, Job marked Failed, main container never runs |
| GitOps repo clone fails | Init container exits non-zero, same as above |
| Watcher restarts | In-memory map cleared; existing Jobs prevent duplicate dispatch via `IsAlreadyExists`; agent PR detection handles the rest |

---

## 12. Configuration Reference

### Watcher Deployment environment variables

| Variable | Required | Description |
|---|---|---|
| `GITOPS_REPO` | Yes | GitHub repo in `owner/repo` format, e.g. `lenaxia/talos-ops-prod` |
| `GITOPS_MANIFEST_ROOT` | Yes | Path within the cloned repo to the manifests root, e.g. `kubernetes` |
| `AGENT_IMAGE` | Yes | Full image ref for the agent, e.g. `ghcr.io/lenaxia/mendabot-agent:latest` |
| `AGENT_NAMESPACE` | Yes | Namespace where agent Jobs are created — **must equal the watcher's own namespace** |
| `AGENT_SA` | Yes | ServiceAccount name for agent Jobs |
| `LOG_LEVEL` | No | `debug`, `info` (default), `warn`, `error` |
| `MAX_CONCURRENT_JOBS` | No | Max agent Jobs running at once, default `3` — enforced by counting Jobs with `app.kubernetes.io/managed-by: mendabot-watcher` label |

### Agent Job environment variables (injected by watcher)

| Variable | Source |
|---|---|
| `FINDING_KIND` | `result.spec.kind` |
| `FINDING_NAME` | `result.spec.name` (plain name, no namespace prefix) |
| `FINDING_NAMESPACE` | `result.metadata.namespace` (ObjectMeta namespace of the Result) |
| `FINDING_PARENT` | `result.spec.parentObject` |
| `FINDING_ERRORS` | `json(result.spec.error)` — `Sensitive` fields redacted before injection |
| `FINDING_DETAILS` | `result.spec.details` |
| `FINDING_FINGERPRINT` | computed sha256 |
| `GITOPS_REPO` | from watcher env |
| `GITOPS_MANIFEST_ROOT` | from watcher env |
| `OPENAI_API_KEY` | from Secret `llm-credentials`, key `api-key` |
| `OPENAI_BASE_URL` | from Secret `llm-credentials`, key `base-url` (optional) |
| `OPENAI_MODEL` | from Secret `llm-credentials`, key `model` (optional) |

**Note:** `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID`, and `GITHUB_APP_PRIVATE_KEY` are
injected into the **init container only**. The main container reads the short-lived
installation token from `/workspace/github-token`. The private key must never be present
in the main container's environment.

---

## 13. Deployment Model

All Kubernetes resources are managed via Kustomize in `deploy/kustomize/`. The directory is
designed to be referenced directly from a Flux `Kustomization` resource in the GitOps repo.

Resources created:
- `Namespace: mendabot`
- `ServiceAccount: mendabot-watcher` (in `mendabot-watcher` namespace)
- `ServiceAccount: mendabot-agent` (in `mendabot-watcher` namespace)
- `ClusterRole: mendabot-watcher` (Result + Namespace read)
- `ClusterRole: mendabot-agent` (cluster-wide read-only)
- `ClusterRoleBinding: mendabot-watcher`
- `ClusterRoleBinding: mendabot-agent`
- `Role: mendabot-watcher` (Job + Pod read/create in own namespace)
- `RoleBinding: mendabot-watcher`
- `ConfigMap: opencode-prompt` (prompt template)
- `Secret: github-app` (placeholder — fill manually)
- `Secret: llm-credentials` (placeholder — fill manually)
- `Deployment: mendabot-watcher`

---

## 14. Upstream Contribution Path

Once v1 is stable and battle-tested:

1. Open a discussion in `k8sgpt-ai/k8sgpt-operator` proposing a `GitOpsRemediation`
   controller as an optional component
2. Refactor the watcher to be configurable (not hardcoded to OpenCode — pluggable agent
   command)
3. Generalise the prompt and GitOps assumptions (not Flux-specific)
4. Submit as a `contrib/` addition initially, with a path to first-class support

---

## 15. v1 Scope

**In scope:**
- Watcher controller watching all namespaces
- In-memory deduplication by parent fingerprint
- One agent Job per unique finding
- Debian-slim agent image with opencode + kubectl + k8sgpt + helm + flux + gh
- GitHub App authentication
- Kustomize manifests
- GitHub Actions CI for both images (ghcr.io)
- Full test coverage for watcher (TDD)

**Out of scope for v1:**
- Persistent deduplication state (Redis, ConfigMap)
- Auto-merging PRs
- Slack/webhook notifications
- Supporting non-Flux GitOps patterns
- Multi-cluster support
- Web UI or dashboard

---

## 16. Success Criteria

- [ ] Watcher starts, connects to cluster, and begins watching Result CRDs
- [ ] A new Result CRD triggers exactly one Job
- [ ] Multiple pods from the same Deployment produce one Job, not many
- [ ] Changed error text on an existing Result produces a new Job
- [ ] Agent Job completes within 15 minutes for a typical finding
- [ ] Agent opens a PR on the GitOps repo with a relevant proposed change
- [ ] Agent comments on an existing PR instead of opening a duplicate
- [ ] All watcher unit tests pass with race detector enabled
- [ ] Both images build and push successfully via GitHub Actions
- [ ] Kustomize manifests apply cleanly to a cluster (`--dry-run=client` passes)
