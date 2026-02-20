# Story: Install Miscellaneous Investigation Tools

**Epic:** [epic03-agent-image](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 30 minutes

---

## User Story

As a **developer**, I want stern, age, and sops installed so the agent can tail multi-pod
logs and decrypt SOPS-encrypted files in the GitOps repo for read-only inspection.

---

## Acceptance Criteria

- [ ] `stern` installed from GitHub release tarball at `STERN_VERSION`
- [ ] `age` and `age-keygen` installed from GitHub release tarball at `AGE_VERSION`
- [ ] `sops` installed from GitHub release binary at `SOPS_VERSION`
- [ ] All use `TARGETARCH`
- [ ] All callable in smoke test

---

## Tasks

- [ ] Add stern install layer
- [ ] Add age install layer
- [ ] Add sops install layer
- [ ] Verify in smoke test

---

## Dependencies

**Depends on:** STORY_01 (base image)

---

## Definition of Done

- [ ] All three tools callable in built image
