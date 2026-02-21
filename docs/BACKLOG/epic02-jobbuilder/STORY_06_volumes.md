# Story: Volume Mounts

**Epic:** [epic02-jobbuilder](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 30 minutes

---

## User Story

As a **developer**, I want the Pod spec to declare all three volumes so the init and main
containers can share the workspace and access the prompt and secret.

---

## Acceptance Criteria

- [ ] Three volumes declared in PodSpec: `shared-workspace`, `prompt-configmap`,
  `github-app-secret`
- [ ] `shared-workspace` is an `emptyDir`
- [ ] `prompt-configmap` references ConfigMap `opencode-prompt`
- [ ] `github-app-secret` references Secret `github-app`
- [ ] Unit test verifies all three volumes by name and type

---

## Tasks

- [ ] Write tests first (TDD)
- [ ] Implement volumes list in `Build()`

---

## Dependencies

**Depends on:** STORY_04 (init container), STORY_05 (main container)
**Blocks:** STORY_07 (metadata — final assembly)

---

## Definition of Done

- [ ] Tests pass with `-race`
- [ ] `go vet` clean
