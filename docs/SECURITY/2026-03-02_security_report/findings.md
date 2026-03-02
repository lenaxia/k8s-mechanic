# All Findings

**Review date:** 2026-03-02 (partial code review — no live cluster)
**Git commit:** `8fc791a2a42d0cc500628edfe3d580da25d0f2df`
**Total new findings this review:** 4 (0 HIGH, 1 MEDIUM, 2 LOW, 1 INFO)
**Regressions from previous report:** 0

---

## New findings — this review

---

### 2026-03-02-001: `FINDING_CORRELATED_FINDINGS` not checked by `DetectInjection` before prompt injection

**Severity:** LOW
**Status:** Open
**Phase:** 2
**Attack Vector:** AV-04 (prompt injection variant)

#### Description

`FINDING_CORRELATED_FINDINGS` is constructed in `internal/controller/controller.go` by
serialising `rjob.Spec.Finding` values from peer `RemediationJob` objects that share the
same correlation group. Those peer findings entered the system through the provider
pipeline and passed through `domain.DetectInjection` at that point. However, when the
correlated findings are assembled into the `correlatedFindings` slice and injected into
the agent Job's environment (via `jobbuilder/job.go`), there is no second `DetectInjection`
call applied to the assembled slice or its component fields.

The primary `RemediationJob`'s own `Finding.Errors` and `Finding.Details` are checked by
`domain.DetectInjection` at `provider.go:190` and `provider.go:207` respectively, and
again at `controller.go:628`. But `correlatedFindings` values come from a separate path —
peer `rjob.Spec.Finding` fields are recovered from etcd without re-running the injection
gate.

The prompt template wraps `${FINDING_CORRELATED_FINDINGS}` with the
`<<<MECHANIC:UNTRUSTED_INPUT:...:BEGIN|END>>>` envelope (a compensating control), but
the pre-dispatch gate that would block job creation entirely on detection does not fire
for correlated finding content.

#### Evidence

`internal/controller/controller.go` — correlated findings construction (around line 628):
```go
// DetectInjection called on rjob.Spec.Finding.Errors / .Details for the PRIMARY job
// but the correlatedFindings slice is built from peer rjob.Spec.Finding values
// without a corresponding DetectInjection call.
```

`internal/jobbuilder/job.go` — `FINDING_CORRELATED_FINDINGS` env var set from
`correlatedFindings` parameter without injection check at this layer.

#### Exploitability

**Precondition:** Attacker must be able to create a `RemediationJob` object with
injected content in `Finding.Errors`. Under normal operation only the watcher SA can do
this. Requires watcher compromise or a ClusterRole misconfiguration.

**Path:**
1. Attacker creates a `RemediationJob` with a benign-looking `Finding.Errors` value that
   contains a prompt injection payload (potentially crafted to evade the 5 existing
   detection patterns)
2. The provider-path `DetectInjection` check at `provider.go:190` does not fire
3. A second legitimate `RemediationJob` fires in the same correlation group
4. The controller builds `correlatedFindings` from the attacker's RJob spec and injects
   it into the agent Job env without re-checking
5. The prompt envelope is a soft barrier only (compensating control, not a hard block)

#### Impact

Prompt injection payload delivered to the LLM via the correlated findings channel,
bypassing the hard-block gate that prevents job dispatch. Impact bounded by HARD RULE 8
and agent-level controls.

#### Recommendation

Add a `DetectInjection` call in `controller.go` when assembling `correlatedFindings`,
before passing to `job.go:Build()`. If injection is detected in any peer finding's fields,
apply the configured `INJECTION_DETECTION_ACTION` (suppress or log). Specifically:

```go
for _, peer := range correlatedPeers {
    if action, _ := domain.DetectInjection(peer.Spec.Finding.Errors); action == domain.ActionSuppress {
        // handle per INJECTION_DETECTION_ACTION config
    }
}
```

#### Resolution

Open — no remediation applied at commit `8fc791a`.

---

### 2026-03-02-002: `deploy/overlays/security/role-agent-ns.yaml` uses `resources: ["*"]` — wildcard grants Secret reads in namespace-scope mode

**Severity:** MEDIUM
**Status:** Open
**Phase:** 2
**Attack Vector:** AV-08 (RBAC misconfiguration variant)

#### Description

`deploy/overlays/security/role-agent-ns.yaml` defines the namespace-scoped agent Role
used when `AGENT_RBAC_SCOPE=namespace`. The Role uses:

```yaml
apiGroups: ["*"]
resources: ["*"]
verbs: ["get", "list", "watch"]
```

The wildcard `resources: ["*"]` grant includes `secrets`, which is the exact resource
the namespace-scope mode is intended to restrict. An operator who applies this overlay
verbatim will deploy a namespace-scoped agent that can still read all Secrets in the
watched namespace — defeating the documented purpose of the scope restriction.

The Helm chart's `clusterrole-agent.yaml` is correctly defined with an explicit resource
allowlist (no `secrets`). The Kustomize overlay is a separately maintained file that
diverged from the Helm chart's intent.

#### Evidence

`deploy/overlays/security/role-agent-ns.yaml`:
```yaml
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["get", "list", "watch"]
```

The Helm ClusterRole for comparison (`deploy/helm/templates/clusterrole-agent.yaml`):
```yaml
rules:
- apiGroups: [""]
  resources: ["pods", "pods/log", "events", "services", "endpoints",
              "configmaps", "nodes", "persistentvolumeclaims",
              "replicationcontrollers", "serviceaccounts"]
  verbs: ["get", "list", "watch"]
```
(No `secrets` in the explicit allowlist.)

#### Exploitability

Any operator who applies `deploy/overlays/security/` believing it restricts the agent
to namespace scope without Secret access will still have an agent SA that can read all
Secrets in the watched namespace. The security control appears applied but is not
effective.

An agent (or a compromised agent process) running with this Role can call
`kubectl get secret -n <watched-ns>` and retrieve all Secret values in that namespace,
including application credentials and TLS keys.

#### Impact

Namespace-scope Secret read for all Secrets in the watched namespace. This is the
primary risk that `AGENT_RBAC_SCOPE=namespace` is intended to mitigate, so the
mitigation is rendered ineffective.

#### Recommendation

Replace `resources: ["*"]` in `deploy/overlays/security/role-agent-ns.yaml` with the
same explicit resource list used in the Helm ClusterRole:

```yaml
rules:
- apiGroups: [""]
  resources: ["pods", "pods/log", "events", "services", "endpoints",
              "configmaps", "nodes", "persistentvolumeclaims",
              "replicationcontrollers", "serviceaccounts"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["remediation.mechanic.io"]
  resources: ["remediationjobs"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["remediation.mechanic.io"]
  resources: ["remediationjobs/status"]
  verbs: ["get", "patch"]
```

Omit `secrets` from all groups. Add a comment explaining the intentional exclusion.

#### Resolution

Open — no remediation applied at commit `8fc791a`.

---

### 2026-03-02-003: Go builder image version label `1.25.7` in Dockerfiles may be inaccurate

**Severity:** INFO
**Status:** Open
**Phase:** 1 + 8
**Attack Vector:** N/A (supply chain hygiene)

#### Description

Both `Dockerfile.agent` and `Dockerfile.watcher` reference a Go builder image labelled
`golang:1.25.7-bookworm`. As of the review date (2026-03-02), Go 1.25.7 does not exist
in the public Go release cycle (latest stable is Go 1.24.x). The base image digest is
pinned (a supply-chain control), so this does not create an immediate security risk —
the exact image bytes used are fixed. However, the version label is inaccurate and
may confuse future maintainers attempting to reason about the Go version in use or
update it.

If the digest corresponds to an image that was pushed to Docker Hub with an incorrect
tag, the actual Go version inside the image is unknown from the Dockerfile alone. If
the digest does not correspond to a real image, builds would fail on CI.

#### Evidence

`Dockerfile.agent` and `Dockerfile.watcher`:
```dockerfile
ARG GO_VERSION=1.25.7
FROM golang:${GO_VERSION}-bookworm@sha256:<digest> AS builder
```

Go release history: 1.23.x, 1.24.x series exist; 1.25 has not been released as of
2026-03-02.

#### Recommendation

1. Run `docker manifest inspect golang:1.25.7-bookworm` to verify whether this tag
   exists and what Go version the image actually contains.
2. If the tag does not exist or the version is wrong, update `ARG GO_VERSION` to the
   correct value matching the pinned digest (e.g., `1.24.1`).
3. Add a comment beside the digest confirming the Go version, e.g.:
   ```dockerfile
   # golang:1.24.1-bookworm
   FROM golang:1.24.1-bookworm@sha256:<digest> AS builder
   ```

#### Resolution

Open — requires verification of the pinned image digest.

---

### 2026-03-02-004: `git` wrapper does not block `checkout -b` or `--orphan` in dry-run mode

**Severity:** LOW
**Status:** Open
**Phase:** 3 + 9
**Attack Vector:** AV-13 (dry-run bypass variant)

#### Description

Finding 2026-02-27-003 (partially remediated) extended the `git` wrapper blocklist in
dry-run mode to cover `push`, `commit`, `reset`, `rm`, `clean`, `rebase`,
`config --global/--system`, `remote set-url`, and annotated/signed `tag`.

The wrapper does not block `git checkout -b <branch>` or `git checkout --orphan <branch>`,
which create new local branches. While a new local branch cannot be pushed (the `push`
block prevents that), this is a defence-in-depth gap:

1. If the `push` block is ever bypassed in a future regression, `checkout -b` would
   allow the agent to create and push an arbitrary branch.
2. `git checkout --orphan` creates a branch with no history, which combined with
   `git commit` (blocked) is currently inert — but the combination of controls is
   fragile.
3. In dry-run mode, the expected behaviour is that the agent makes no persistent changes
   to the repository. Creating local branches violates this intent even if they are never
   pushed.

#### Evidence

`scripts/git-wrapper.sh` (current dry-run blocklist):
```bash
case "${1:-}" in
    push|commit)     echo "[DRY_RUN] blocked" >&2; exit 0 ;;
    reset|rm|clean)  echo "[DRY_RUN] blocked" >&2; exit 0 ;;
    rebase)          echo "[DRY_RUN] blocked" >&2; exit 0 ;;
    tag)             # blocks -a/-s flags only
    ...
esac
# checkout not in the case statement — passes through unconditionally
```

#### Exploitability

Not independently exploitable given the `push` block is in place. This is a
defence-in-depth gap only. Exploitability requires a future regression in the `push`
block.

#### Impact

If `push` block is bypassed: agent in dry-run mode could push a new branch to the
remote repository, potentially triggering CI workflows.

#### Recommendation

Add `checkout` with `-b` or `--orphan` flags to the dry-run blocklist:

```bash
checkout)
    # Block branch creation in dry-run; read-only checkout of existing branches is OK
    for arg in "$@"; do
        case "$arg" in
            -b|--orphan|-B)
                echo "[DRY_RUN] git $* blocked — branch creation disabled" >&2
                exit 0
                ;;
        esac
    done
    ;;
```

This allows `git checkout <existing-branch>` (read-only) while blocking
`git checkout -b <new-branch>`.

#### Resolution

Open — no remediation applied at commit `8fc791a`.

---

## Regression check — findings from 2026-02-27

The following entries record the regression verification outcome for all 14 findings
from the previous report. Full regression detail is in
[phase10_regression.md](phase10_regression.md).

---

### 2026-02-27-001: Watcher ClusterRole `secrets` — redundant pending live verification

**Status as of this review:** Accepted (AR-08) — unchanged
**Regression:** Not a regression — status unchanged from 2026-02-27.

---

### 2026-02-27-002: GitHub App private key exposed as plain-text environment variable in watcher pod

**Status as of this review:** Remediated — confirmed at commit `8fc791a`
**Evidence:** `cmd/watcher/main.go:233` uses `os.ReadFile()` to read the private key
from `/var/run/secrets/mechanic/github-app-private-key/private-key`. No `GITHUB_APP_PRIVATE_KEY`
env var present in `deploy/helm/templates/deployment-watcher.yaml`.

---

### 2026-02-27-003: `git` dry-run wrapper — incomplete blocklist

**Status as of this review:** Partially Remediated
**Evidence:** `scripts/git-wrapper.sh` now blocks `push`, `commit`, `reset`, `rm`,
`clean`, `rebase`, `config --global/--system`, `remote set-url`, and annotated/signed
`tag`. `checkout -b` and `--orphan` are not blocked. See new finding 2026-03-02-004.

---

### 2026-02-27-004: `gh api` write bypass in dry-run mode

**Status as of this review:** Remediated — confirmed at commit `8fc791a`
**Evidence:** `scripts/gh-wrapper.sh` uses an allowlist approach. `gh api` with GET
method is allowed; POST/PUT/PATCH/DELETE methods are blocked fail-closed.

---

### 2026-02-27-005: `agentImage` CRD field has no validation

**Status as of this review:** Open
**Evidence:** `deploy/helm/templates/crds/` — no CEL validation rule restricting
`agentImage` to an allowed registry prefix in the CRD schema at this commit.
The Kyverno `restrict-agent-image` policy path was noted in the 2026-02-27 fix field
but was not verified present at this commit.

---

### 2026-02-27-006: `agentSA` CRD field has no validation

**Status as of this review:** Open
**Evidence:** Same as 005. `agentSA` has no `enum` or CEL allowlist constraint in
the CRD. Immutability rule noted in 2026-02-27 fix field but not re-verified present
at this commit (requires live cluster for full verification).

---

### 2026-02-27-007: `anomalyco/opencode/github@latest` unpinned

**Status as of this review:** Remediated — confirmed at commit `8fc791a`
**Evidence:** All four OpenCode action uses pinned to SHA
`0cf0294787322664c6d668fa5ab0a9ce26796f78`.

---

### 2026-02-27-008: `renovate-analysis.yml` LLM auto-merge

**Status as of this review:** Accepted/INFO (re-classified from HIGH)
**Evidence:** Review of `.github/workflows/renovate-analysis.yml` at commit `8fc791a`
confirms the workflow posts a PR comment only. The LLM does not have autonomous merge
authority. The original HIGH classification overstated the impact. Re-classified to
Accepted/INFO.

---

### 2026-02-27-009: `emit_dry_run_report` writes unredacted output to ConfigMap

**Status as of this review:** Remediated — confirmed at commit `8fc791a`
**Evidence:** `scripts/entrypoint-common.sh` lines 159/172 pipe output through the
`redact` binary before writing to the ConfigMap. Hard-fail if `redact` is absent from PATH.

---

### 2026-02-27-010: Circuit breaker state is in-memory only

**Status as of this review:** Open — unchanged
**Evidence:** `internal/circuitbreaker/circuitbreaker.go` — `lastAllowed time.Time`
is still a struct field with no persistence. Watcher restart resets cooldown to zero.

---

### 2026-02-27-011: Agent Job containers have no resource limits

**Status as of this review:** Remediated — confirmed at commit `8fc791a`
**Evidence:** `internal/domain/job.go` — `containerResources()` function applies
CPU and memory requests/limits to all three Job containers.

---

### 2026-02-27-012: `sinkRef.url` has no format validation

**Status as of this review:** Open
**Evidence:** CRD schema for `sinkRef.url` field not verified to have a `pattern`
constraint at this commit. Noted as fixed in the 2026-02-27 fix field — requires
live CRD apply to confirm the generated CRD YAML includes the pattern.

---

### 2026-02-27-013: `ai-comment.yml` — any authenticated user can trigger LLM

**Status as of this review:** Remediated — confirmed at commit `8fc791a`
**Evidence:** `.github/workflows/ai-comment.yml` `if:` condition gates on
`github.event.comment.author_association` being `OWNER`, `MEMBER`, or `COLLABORATOR`.

---

### 2026-02-27-014: `finding.Details` is never passed through `RedactSecrets`

**Status as of this review:** Deferred — unchanged
**Evidence:** `internal/provider/provider.go` — `finding.Details` goes through
`DetectInjection` but not `RedactSecrets`. No native provider currently sets the
`Details` field. Risk is latent.
