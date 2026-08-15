# Project Context: k8s-mechanic

## What this project is

k8s-mechanic (formerly k8s-mendabot) is a Kubernetes operator that watches
your cluster for failures, investigates them automatically, and opens pull
requests on your GitOps repository with proposed fixes — all without leaving
your cluster.

When a Pod is crash-looping, a Deployment is degraded, or a Node goes
NotReady, mechanic spawns an in-cluster OpenCode agent that inspects the live
cluster, locates the relevant manifests in the GitOps repo, determines the
root cause, and opens a PR. Security is a first-class citizen: read-only
RBAC, secrets redacted before they reach the LLM, human-in-the-loop PR
review. No external operators, no external databases, no persistent services
outside the cluster.

## What it does

1. **Detects failures** — watches Pods, Deployments, StatefulSets, PVCs,
   Nodes, and Jobs natively via the Kubernetes API
2. **Deduplicates by parent** — repeated pod restarts from the same
   Deployment produce one investigation, not one per pod restart (fingerprint
   = sha256 of namespace + kind + parentObject + sorted error texts)
3. **Stabilises before acting** — a configurable window (default: 120s)
   filters transient blips before dispatch
4. **Investigates in-cluster** — an agent Job runs with read-only RBAC,
   clones the GitOps repo, and inspects the live cluster
5. **Opens a PR** — with a structured body: summary, evidence, root cause,
   proposed fix, and confidence level

## Tech stack

- **Language**: Go (1.23+) — idiomatic, type-safe
- **Controller framework**: controller-runtime (Kubernetes controller pattern)
- **Logging**: go.uber.org/zap (structured)
- **Packaging**: Helm chart (`charts/mechanic/`) + Kustomize (`deploy/kustomize/`)
- **Images**: `mechanic-watcher` (controller binary) and `mechanic-agent`
  (opencode + kubectl + helm + gh + redact wrappers) built via Docker,
  published to ghcr.io
- **Agent driver**: OpenCode, run in-cluster as a per-finding Kubernetes Job
- **Metrics**: Prometheus via controller-runtime default registry (`:8080/metrics`)

## File structure

```
api/v1alpha1/          # RemediationJob CRD types (remediation.mechanic.io)
cmd/watcher/           # Watcher entrypoint (scheme, provider loop, manager)
internal/
  controller/          # RemediationJob controller (envtest suite)
  provider/            # SourceProviderReconciler + native providers (pod, deployment, ...)
  correlator/          # Multi-signal correlation engine
  jobbuilder/          # Builds the per-finding agent Job
  github/              # GitHub App installation token exchange
  sink/github/         # PR auto-close + merge detection
  redact/              # Secret-pattern redaction (wrappers + cmd/redact)
  readiness/           # Sink/LLM readiness checks
  metrics/             # Prometheus metrics
  domain/              # Types: annotations, correlation, severity, sink, interfaces
  circuitbreaker/      # Self-remediation cascade circuit breaker
  config/              # Config struct + FromEnv()
charts/mechanic/       # Helm chart (CRDs, RBAC, watcher deployment, agent templates)
docker/                # Dockerfiles + redact wrappers + entrypoint scripts
deploy/kustomize/      # Kustomize overlays
docs/
  DESIGN/              # HLD.md + lld/ (authoritative design docs)
  BACKLOG/             # Epics and user stories
  WORKLOGS/            # Mandatory session worklogs
  SECURITY/            # THREAT_MODEL.md, pentest/security reports, CHECKLIST.md
testdata/crds/         # envtest CRD schema (manually maintained copy)
```

## Key design constraints

- **Read-only cluster access** for the investigation agent — no writes to the
  cluster from the agent
- **PRs only** — never commit directly to the GitOps repo's default branch
- **Human-in-the-loop** — mechanic opens PRs; a human reviews and merges
- **Redact before LLM** — secret patterns are redacted from error text and
  tool-call output before anything reaches the model (`internal/redact/` +
  redact wrappers in `docker/scripts/redact-wrappers/`)
- **Durable dedup state** via RemediationJob CRDs (survives watcher restarts;
  no external store)
- **Self-remediation cascade prevention** — depth limit + circuit breaker so
  mechanic never remediates its own remediation
- **Worklogs are mandatory** — every meaningful session writes
  `docs/WORKLOGS/NNNN_YYYY-MM-DD_short-description.md`

## Authoritative documents

- `README-LLM.md` — the LLM implementation guide (critical rules, structure,
  workflow). Read it in full before making changes.
- `docs/DESIGN/HLD.md` + `docs/DESIGN/lld/` — architecture and component designs
- `docs/SECURITY/THREAT_MODEL.md` — threat model; read before touching
  redaction, RBAC, CRD schema, or secrets handling
