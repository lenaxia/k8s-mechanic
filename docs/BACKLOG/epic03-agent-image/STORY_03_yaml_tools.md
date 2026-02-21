# Story: Install YAML/JSON Processing Tools

**Epic:** [epic03-agent-image](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 30 minutes

---

## User Story

As a **developer**, I want kustomize, yq, jq, and kubeconform installed so the agent can
render Kustomize overlays, parse and patch YAML, process JSON, and validate manifests
before opening a PR.

---

## Acceptance Criteria

- [ ] `kustomize` installed from GitHub release tarball at `KUSTOMIZE_VERSION`
- [ ] `yq` installed from GitHub release binary at `YQ_VERSION`
- [ ] `jq` installed via apt (already in base layer — verify it is available)
- [ ] `kubeconform` installed from GitHub release tarball at `KUBECONFORM_VERSION`
- [ ] All binaries use `TARGETARCH`
- [ ] All respond correctly when called with `--version` or `-v`

---

## Tasks

- [ ] Add kustomize install layer to Dockerfile
- [ ] Add yq install layer
- [ ] Confirm jq is available from base layer
- [ ] Add kubeconform install layer
- [ ] Verify in smoke test

---

## Dependencies

**Depends on:** STORY_01 (base image)

---

## Definition of Done

- [ ] All four tools callable in built image
