# docs/DESIGN/

## Purpose

Design documents for the entire system. Read these before making any architectural or
implementation decisions.

## Rules

- The HLD in the **latest numbered folder** is the authoritative specification. If anything
  contradicts it, the HLD wins.
- LLDs refine the HLD for each component. Read the relevant LLD before implementing.
- When architecture changes, update the HLD first, then the affected LLDs, then code.
- Never update an LLD to match code that was written without reading the LLD first —
  fix the code instead.
- Design folders are numbered incrementally and dated so the evolution of the design is
  auditable. Do not edit documents in older folders — create a new folder instead.

## Design Generations

| Folder | Date | Description | Status |
|--------|------|-------------|--------|
| _(root)_ | 2026-02-20 | v1 — native K8s provider baseline, k8sgpt integration, RemediationJob CRD | Implemented |
| [`0002_2026-02-25_multi-source-signal-architecture/`](0002_2026-02-25_multi-source-signal-architecture/) | 2026-02-25 | v2 — multi-source signal layer, AlertSource CRD, resource-level fingerprinting, cross-source priority dedup | Proposed |
| [`0003_2026-02-26_proactive-engine/`](0003_2026-02-26_proactive-engine/) | 2026-02-26 | v3 — proactive audit engine, CheckBundle/AuditFinding/AuditSchedule/PersonaProfile CRDs, CEL check evaluation, maturity lifecycle, webhook delivery | Proposed |

---

## v1 Design (root — implemented)

- [`HLD.md`](HLD.md) — High-level design (v1.4)
- [`lld/CONTROLLER_LLD.md`](lld/CONTROLLER_LLD.md) — Result CRD watcher and reconciler
- [`lld/JOBBUILDER_LLD.md`](lld/JOBBUILDER_LLD.md) — Agent Job construction
- [`lld/AGENT_IMAGE_LLD.md`](lld/AGENT_IMAGE_LLD.md) — Docker image tool inventory and Dockerfile
- [`lld/DEPLOY_LLD.md`](lld/DEPLOY_LLD.md) — Kustomize manifests and RBAC
- [`lld/PROMPT_LLD.md`](lld/PROMPT_LLD.md) — OpenCode agent prompt design
- [`lld/PROVIDER_LLD.md`](lld/PROVIDER_LLD.md) — SourceProvider interface and native providers
- [`lld/REMEDIATIONJOB_LLD.md`](lld/REMEDIATIONJOB_LLD.md) — RemediationJob CRD full spec
- [`lld/SINK_PROVIDER_LLD.md`](lld/SINK_PROVIDER_LLD.md) — SinkProvider concept
- [`lld/WATCHER_IMAGE_LLD.md`](lld/WATCHER_IMAGE_LLD.md) — Watcher Docker image

---

## v2 Design (0002_2026-02-25 — proposed)

- [`0002_.../HLD.md`](0002_2026-02-25_multi-source-signal-architecture/HLD.md) — Multi-source signal architecture HLD

### LLD Index

| LLD | Component | Status |
|-----|-----------|--------|
| [ALERT_SOURCE_LLD.md](0002_2026-02-25_multi-source-signal-architecture/lld/ALERT_SOURCE_LLD.md) | AlertSource CRD, adapter interface, built-in adapters | Proposed |
| [FINGERPRINT_LLD.md](0002_2026-02-25_multi-source-signal-architecture/lld/FINGERPRINT_LLD.md) | Resource-level fingerprint redesign, cross-source dedup | Proposed |
| [ALERT_SOURCE_RECONCILER_LLD.md](0002_2026-02-25_multi-source-signal-architecture/lld/ALERT_SOURCE_RECONCILER_LLD.md) | AlertSourceReconciler, webhook server, polling, priority resolution, pending alert annotation | Proposed |
| [AGENT_CONTEXT_LLD.md](0002_2026-02-25_multi-source-signal-architecture/lld/AGENT_CONTEXT_LLD.md) | New agent env vars, FINDING_PREVIOUS_PR_URL handling, prompt changes | Proposed |
