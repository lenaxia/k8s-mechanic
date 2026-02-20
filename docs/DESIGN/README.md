# docs/DESIGN/

## Purpose

Design documents for the entire system. Read these before making any architectural or
implementation decisions.

## Rules

- `HLD.md` is the authoritative specification. If anything contradicts it, the HLD wins.
- LLDs refine the HLD for each component. Read the relevant LLD before implementing.
- When architecture changes, update the HLD first, then the affected LLDs, then code.
- Never update an LLD to match code that was written without reading the LLD first —
  fix the code instead.

## Structure

- [`HLD.md`](HLD.md) — High-level design. Start here.
- [`lld/CONTROLLER_LLD.md`](lld/CONTROLLER_LLD.md) — Result CRD watcher and reconciler
- [`lld/JOBBUILDER_LLD.md`](lld/JOBBUILDER_LLD.md) — Agent Job construction
- [`lld/AGENT_IMAGE_LLD.md`](lld/AGENT_IMAGE_LLD.md) — Docker image tool inventory and Dockerfile
- [`lld/DEPLOY_LLD.md`](lld/DEPLOY_LLD.md) — Kustomize manifests and RBAC
- [`lld/PROMPT_LLD.md`](lld/PROMPT_LLD.md) — OpenCode agent prompt design

## LLD Index

| LLD | Component | Status |
|-----|-----------|--------|
| [CONTROLLER_LLD.md](lld/CONTROLLER_LLD.md) | Result controller + fingerprint algorithm | Ready |
| [JOBBUILDER_LLD.md](lld/JOBBUILDER_LLD.md) | Job spec builder | Ready |
| [AGENT_IMAGE_LLD.md](lld/AGENT_IMAGE_LLD.md) | Agent Docker image | Ready |
| [DEPLOY_LLD.md](lld/DEPLOY_LLD.md) | RBAC and Kustomize manifests | Ready |
| [PROMPT_LLD.md](lld/PROMPT_LLD.md) | OpenCode investigation prompt | Ready |
