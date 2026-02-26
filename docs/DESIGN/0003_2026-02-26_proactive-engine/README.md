# docs/DESIGN/0003_2026-02-26_proactive-engine/

## Purpose

Design documents for the mendabot proactive engine — a scheduled and event-driven audit
system that evaluates Kubernetes cluster state against codified best-practice checks and
surfaces findings to users at the appropriate level of detail via their configured sinks.

This is a net-new capability alongside the existing reactive remediation path. The two
paths share a binary but are isolated by package boundary.

## Status

**Proposed** — design phase. No implementation has started.

## Rules

- Read the HLD before reading any LLD.
- Read the relevant LLD before implementing any component.
- If implementation reveals a design gap, update the LLD first, then the code.
- Do not edit documents in this folder once their dependent epic is marked Implemented —
  create a new design generation folder instead.

---

## Document Index

### HLD

| Document | Description | Status |
|----------|-------------|--------|
| [HLD.md](HLD.md) | Proactive engine high-level design | Proposed |

### LLDs

| Document | Component | Status |
|----------|-----------|--------|
| [CHECK_BUNDLE_LLD.md](lld/CHECK_BUNDLE_LLD.md) | `CheckBundle` CRD, CEL evaluator, trust model | Proposed |
| [CHECK_REGISTRY_LLD.md](lld/CHECK_REGISTRY_LLD.md) | `CheckRegistrySync` CRD, Git polling, signature verification | Proposed |
| [AUDIT_SCHEDULE_LLD.md](lld/AUDIT_SCHEDULE_LLD.md) | `AuditSchedule` CRD, `AuditController`, cron + event triggers, stabilisation | Proposed |
| [AUDIT_FINDING_LLD.md](lld/AUDIT_FINDING_LLD.md) | `AuditFinding` CRD, maturity lifecycle, deduplication | Proposed |
| [PERSONA_PROFILE_LLD.md](lld/PERSONA_PROFILE_LLD.md) | `PersonaProfile` CRD, persona-aware prompt variant | Proposed |
| [PROACTIVE_SINK_LLD.md](lld/PROACTIVE_SINK_LLD.md) | Webhook JSON delivery, structured report format, sink extension | Proposed |
| [AUTO_REMEDIATION_LLD.md](lld/AUTO_REMEDIATION_LLD.md) | `AuditFinding` → `RemediationJob` integration, per-check flags | Proposed |
| [PACKAGE_STRUCTURE_LLD.md](lld/PACKAGE_STRUCTURE_LLD.md) | `reactive/` `proactive/` `shared/` refactor, extraction contract | Proposed |

---

## Epic Map

| Epic | Title | Design docs |
|------|-------|-------------|
| `epic13.5` | Package structure refactor | [PACKAGE_STRUCTURE_LLD.md](lld/PACKAGE_STRUCTURE_LLD.md) |
| `epic14` | Check engine: `CheckBundle` CRD + CEL evaluator + `CheckRegistryController` | [CHECK_BUNDLE_LLD.md](lld/CHECK_BUNDLE_LLD.md), [CHECK_REGISTRY_LLD.md](lld/CHECK_REGISTRY_LLD.md) |
| `epic15` | `AuditFinding` CRD + maturity lifecycle + `AuditFindingReconciler` | [AUDIT_FINDING_LLD.md](lld/AUDIT_FINDING_LLD.md) |
| `epic16` | `AuditSchedule` CRD + `AuditController` (cron + event triggers) | [AUDIT_SCHEDULE_LLD.md](lld/AUDIT_SCHEDULE_LLD.md) |
| `epic17` | `PersonaProfile` CRD + persona-aware agent prompt variant | [PERSONA_PROFILE_LLD.md](lld/PERSONA_PROFILE_LLD.md) |
| `epic18` | Sink extension: webhook JSON + structured report format | [PROACTIVE_SINK_LLD.md](lld/PROACTIVE_SINK_LLD.md) |
| `epic19` | Auto-remediation: `AuditFinding` → `RemediationJob` (stages 3–5) | [AUTO_REMEDIATION_LLD.md](lld/AUTO_REMEDIATION_LLD.md) |
| `epic20` | Curated check bundles (separate `mendabot-checks` repo) | [CHECK_BUNDLE_LLD.md](lld/CHECK_BUNDLE_LLD.md) |

---

## Key Design Decisions (summary)

All decisions were made prior to writing these documents. Do not re-open them without a
documented reason.

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Service boundary | Same binary as reactive watcher; clean package split for future extraction | Avoids duplicating informer connections and RBAC; extraction path preserved |
| Check format | Declarative YAML + CEL (`google/cel-go`); `script` field is a no-op extension point in v1 | CEL is in-process, safe, type-checkable at validation time |
| Check registry | Separate Git repo (`lenaxia/mendabot-checks`); pulled on startup + scheduled refresh | Versioned, auditable, community-forkable; decoupled from binary release |
| Trust model | `curated` (cosign-signed) vs `custom` (restricted CEL environment) | Prevents untrusted checks from executing arbitrary expressions |
| Check maturity | 6-stage model (`maturityStage` field on every check) drives resolution behavior | Reflects real-world posture improvement progression |
| AuditFinding resolution | Automated by default (next passing CEL eval); stages 1–2 require manual close | Reduces toil without removing human control for early-stage checks |
| Event stabilisation | `0s` default (immediate), overridable per-check and per-`AuditSchedule` | Security checks should fire immediately; operational checks may need a window |
| Personas | `PersonaProfile` CRD; no external service in v1; microservice is a future upgrade path | CRD is the interface; extraction to a service requires no consumer code changes |
| Auto-remediation granularity | Per-check `remediationAllowed` + per-check-class constraint + `AuditSchedule` master switch | Prevents whole check classes (advisory, informational) from ever auto-remediating |
| Delivery | Structured JSON webhook + GitHub PR report; K8s Events as side-channel | Composable with Slack, PagerDuty, Grafana without mendabot owning the dashboard |
| Scope | K8s only; check framework interface designed for future cloud extension | Tight focus for v1; interface does not assume K8s-specific types in `shared/` |
