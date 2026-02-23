# Worklog: Epic 10 Helm Chart — Implementation

**Date:** 2026-02-23
**Session:** Full chart implementation — all 13 stories complete
**Branch:** feature/epic10-helm-chart
**Status:** Complete

---

## Objective

Implement the `charts/mendabot/` Helm chart designed and planned in worklog 0036.
Implement all 13 stories in dependency order. Validate with `helm lint --strict` and
`helm template` dry-run before closing.

---

## Work Completed

### Branch

- Created `feature/epic10-helm-chart` from `main`

### STORY_01: Chart scaffold

- Created `charts/mendabot/` directory structure:
  `templates/`, `crds/`, `files/prompts/`
- Wrote `charts/mendabot/Chart.yaml` with `apiVersion: v2`, `name: mendabot`,
  `version: 0.1.0`, `appVersion: v0.3.0`, `kubeVersion: >=1.28.0-0`
- Copied `deploy/kustomize/crd-remediationjob.yaml` to `charts/mendabot/crds/remediationjob.yaml`
  (byte-for-byte identical)

### STORY_02: values.yaml + _helpers.tpl

- Wrote `charts/mendabot/values.yaml` with all keys from the approved schema,
  inline comments, and correct defaults
- Wrote `charts/mendabot/templates/_helpers.tpl` with named templates:
  - `mendabot.name` — `"mendabot"`
  - `mendabot.fullname` — release-name dedup, trunc 63
  - `mendabot.labels` — 5-label standard set
  - `mendabot.selectorLabels` — stable 2-label subset
  - `mendabot.watcherSAName` — `<fullname>-watcher`
  - `mendabot.agentSAName` — `<fullname>-agent`
  - `mendabot.watcherImage` — falls back to `Chart.AppVersion`
  - `mendabot.agentImage` — falls back to `Chart.AppVersion`

### STORY_03: Namespace template

- `templates/namespace.yaml` — guarded by `{{- if .Values.createNamespace }}`

### STORY_04: ServiceAccount templates

- `templates/serviceaccount-watcher.yaml`
- `templates/serviceaccount-agent.yaml`
- Neither sets `automountServiceAccountToken` (default `true` required for token mounts)

### STORY_05: RBAC templates (8 files)

Rules copied verbatim from `deploy/kustomize/`:

| Template | Source rule summary |
|----------|---------------------|
| `clusterrole-watcher` | core/apps/batch/remediationjobs full RBAC |
| `clusterrole-agent` | `*/*` get/list/watch |
| `clusterrolebinding-watcher` | binds to `watcherSAName` |
| `clusterrolebinding-agent` | binds to `agentSAName` |
| `role-watcher` | batch/jobs + core/pods in namespace |
| `rolebinding-watcher` | binds to `watcherSAName` |
| `role-agent` | remediationjobs/status get/patch |
| `rolebinding-agent` | binds to `agentSAName` |

All 8 gated by `{{- if .Values.rbac.create }}`

### STORY_06: Deployment template

- `templates/deployment-watcher.yaml` — all 13 env vars, security context matching
  Kustomize source, liveness/readiness probes, resource limits
- `GITOPS_REPO` and `GITOPS_MANIFEST_ROOT` use `required` — render errors if missing
- Image resolves to `Chart.AppVersion` when `image.tag` is empty

### STORY_07: Prompt ConfigMap + files

- Extracted prompt verbatim from `deploy/kustomize/configmap-prompt.yaml` into
  `charts/mendabot/files/prompts/default.txt`
- `templates/configmap-prompt.yaml` — uses `.Files.Get` with `fail` guard for missing
  prompt files; `prompt.override` takes precedence when set

### STORY_08: CRD install and upgrade hook

5 hook resources (all annotated `pre-upgrade,pre-install`, weight `-5`,
delete-policy `before-hook-creation,hook-succeeded`):

- `templates/configmap-crd-hook.yaml` — embeds CRD YAML via `.Files.Get`
- `templates/serviceaccount-crd-hook.yaml`
- `templates/clusterrole-crd-hook.yaml` — `apiextensions.k8s.io` CRD CRUD
- `templates/clusterrolebinding-crd-hook.yaml`
- `templates/job-crd-upgrade.yaml` — `registry.k8s.io/kubectl:v1.28.16`; mounts
  CRD ConfigMap at `/crds/`; `runAsUser: 65534`, full securityContext

### STORY_09: Metrics Service and ServiceMonitor

- `templates/service-metrics.yaml` — gated by `metrics.enabled`
- `templates/servicemonitor.yaml` — gated by `metrics.enabled AND serviceMonitor.enabled`;
  merges `metrics.serviceMonitor.labels` into metadata labels

### STORY_10: NOTES.txt

- `templates/NOTES.txt` — post-install instructions with exact `kubectl create secret`
  commands for both Secrets; conditional ServiceMonitor note

### STORY_11: CI workflow

- `.github/workflows/chart-test.yaml` — triggers on `charts/**`, runs
  `helm lint --strict` and `helm template` with required values

### STORY_12: README update

- Replaced the old `## Deployment` + `## Configuration` sections with:
  - `## Quick Start` — 3-step Helm install (create Secrets → helm install → verify)
  - Kustomize as alternative with a caveat note
  - `## Helm Configuration Reference` — full 24-row values table

### STORY_13: Agent token Secret

- `templates/secret-agent-token.yaml` — type `kubernetes.io/service-account-token`,
  annotation `kubernetes.io/service-account.name: <agentSAName>`, name
  `mendabot-agent-token` (hardcoded to match `job.go`)

---

## Validation Results

```
helm lint charts/mendabot/ --strict
→ 1 chart(s) linted, 0 chart(s) failed
   INFO: icon is recommended (cosmetic; no chart icon URL yet)
   WARN: gitops.repo/manifestRoot required — expected; required guard works correctly
```

```
helm template mendabot charts/mendabot/ \
  --set gitops.repo=org/repo \
  --set gitops.manifestRoot=kubernetes \
  --namespace mendabot
→ 20 resources rendered without error
```

Spot checks passed:
- `required` fires with clear error when `gitops.repo` missing
- `metrics.enabled=true` renders Service only
- `metrics.enabled=true --set metrics.serviceMonitor.enabled=true` renders both
- `createNamespace=true` renders Namespace
- Image tag defaults to `v0.3.0` (Chart.AppVersion) when `image.tag` is empty
- Prompt content renders verbatim with `${FINDING_KIND}` etc. preserved
- CRD YAML embedded correctly in hook ConfigMap
- All 5 hook resources carry correct annotations and delete-policy
- Hook infra resources (SA, ClusterRole, ClusterRoleBinding, ConfigMap) weight `-10`
- Hook Job weight `-5` (runs after infra)
- Agent token Secret has correct type and annotation
- CRD schema contains `isSelfRemediation`, `chainDepth`, `targetRepoOverride`

---

## File Layout Created

```
charts/mendabot/
├── Chart.yaml
├── values.yaml
├── crds/
│   └── remediationjob.yaml
├── files/
│   └── prompts/
│       └── default.txt
└── templates/
    ├── _helpers.tpl
    ├── namespace.yaml
    ├── serviceaccount-watcher.yaml
    ├── serviceaccount-agent.yaml
    ├── secret-agent-token.yaml
    ├── configmap-prompt.yaml
    ├── clusterrole-watcher.yaml
    ├── clusterrole-agent.yaml
    ├── clusterrolebinding-watcher.yaml
    ├── clusterrolebinding-agent.yaml
    ├── role-watcher.yaml
    ├── role-agent.yaml
    ├── rolebinding-watcher.yaml
    ├── rolebinding-agent.yaml
    ├── deployment-watcher.yaml
    ├── configmap-crd-hook.yaml
    ├── serviceaccount-crd-hook.yaml
    ├── clusterrole-crd-hook.yaml
    ├── clusterrolebinding-crd-hook.yaml
    ├── job-crd-upgrade.yaml
    ├── service-metrics.yaml
    ├── servicemonitor.yaml
    └── NOTES.txt

.github/workflows/chart-test.yaml   ← new CI workflow
README.md                            ← Quick Start + Configuration Reference updated
```

---

## Post-Implementation Bug Fixes (Session 2)

A skeptical deep-dive review identified four issues fixed in the same session:

### CRITICAL-1 — CRD schema missing self-remediation fields

**File:** `charts/mendabot/crds/remediationjob.yaml`

The chart's CRD copy was missing `isSelfRemediation`, `chainDepth`, and `targetRepoOverride`
from its openAPIV3Schema. These fields are present in the canonical
`deploy/kustomize/crd-remediationjob.yaml` but were not copied over. Kubernetes
structural schemas prune unknown fields silently — agent Jobs would always see
`IS_SELF_REMEDIATION=false` and `CHAIN_DEPTH=0`, breaking the cascade prevention logic
added in v0.3.0.

**Fix:** Added all three fields to `charts/mendabot/crds/remediationjob.yaml` under
`spec.properties`, matching the canonical file exactly.

### CRITICAL-2 — `secrets.*.name` values are documentation-only

**File:** `charts/mendabot/values.yaml`

The `secrets.githubApp.name` and `secrets.llm.name` keys gave false confidence that
renaming the Secrets would work. `internal/controller/job.go` hardcodes `"github-app"`
and `"llm-credentials"` at compile time. Changing the values without rebuilding the
image causes `CreateContainerConfigError` on every agent Job.

**Fix:** Replaced the mild "Default matches..." comments with an explicit `WARNING` block
explaining the compile-time constraint and that the names must match unless the image is
rebuilt.

### CRITICAL-3 — Hook infra resources had same weight as hook Job

**Files:** `serviceaccount-crd-hook.yaml`, `clusterrole-crd-hook.yaml`,
`clusterrolebinding-crd-hook.yaml`, `configmap-crd-hook.yaml`

All hook resources shared weight `-5`. Helm executes same-weight resources in parallel;
the hook Job could start before its ServiceAccount or ConfigMap was created, causing a
race condition on fresh installs.

**Fix:** Changed hook infra weights to `-10`. Job weight remains `-5`. Helm now
guarantees infra is applied before the Job runs.

### HIGH-2 — Hardcoded ConfigMap name gap undocumented

**File:** `charts/mendabot/templates/configmap-prompt.yaml`

The ConfigMap is named `opencode-prompt` because `job.go` mounts it by that exact name.
This was not documented — a user with two Helm releases in the same namespace would get
a silent collision. Added a prominent comment.

---

## Known Gaps / Follow-up

- **No `values.schema.json`** — schema validation not in scope for this epic.
  Would prevent misconfigured installs from reaching the cluster.
- **No OCI push** — `helm install oci://ghcr.io/...` not yet wired; requires
  Chart Releaser Action setup (future epic).
- **No `helm test` hook** — a connectivity test pod would strengthen CI.
  `helm/chart-testing` (ct) can be added when OCI publishing is configured.
- **Kustomize gap still exists** — `mendabot-agent-token` Secret is still absent
  from `deploy/kustomize/`. Fix is out of scope for this epic but documented in
  STORY_13.

---

## Next Steps

1. Commit and push `feature/epic10-helm-chart` to remote
2. Open PR against `main`
3. Verify CI (`Chart Lint` workflow) passes on the PR
4. Merge after review
5. Tag `charts/mendabot-0.1.0` (or configure Chart Releaser Action for automation)
