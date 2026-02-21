# Story: Install Kubernetes and GitOps Tools

**Epic:** [epic03-agent-image](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 1 hour

---

## User Story

As a **developer**, I want kubectl, k8sgpt, helm, flux, and talosctl installed in the
agent image so the OpenCode agent can interact with the cluster and GitOps toolchain.

---

## Acceptance Criteria

- [ ] `kubectl` installed from official release binary at `KUBECTL_VERSION`
- [ ] `k8sgpt` installed from GitHub release binary at `K8SGPT_VERSION`
- [ ] `helm` installed from official tarball at `HELM_VERSION`
- [ ] `flux` installed from GitHub release tarball at `FLUX_VERSION`
- [ ] `talosctl` installed from GitHub release binary at `TALOSCTL_VERSION`
- [ ] All binaries executable and respond to `--version` or `version` subcommand
- [ ] Each install uses `TARGETARCH` for multi-arch correctness

---

## Tasks

- [ ] Add kubectl install layer to Dockerfile
- [ ] Add k8sgpt install layer
- [ ] Add helm install layer
- [ ] Add flux install layer
- [ ] Add talosctl install layer
- [ ] Verify all five binaries in a `docker run` smoke test

---

## Dependencies

**Depends on:** STORY_01 (base image)
**Blocks:** STORY_10 (smoke test)

---

## Definition of Done

- [ ] All five binaries callable in built image
- [ ] Multi-arch build succeeds (`linux/amd64` and `linux/arm64`)
