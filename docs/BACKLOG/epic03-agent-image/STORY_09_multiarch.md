# Story: Multi-Architecture Build Verification

**Epic:** [epic03-agent-image](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 30 minutes

---

## User Story

As a **developer**, I want the agent image to build for both `linux/amd64` and
`linux/arm64` via `docker buildx` so it runs on any node architecture in the Talos cluster.

---

## Acceptance Criteria

- [ ] `docker buildx build --platform linux/amd64,linux/arm64 -f docker/Dockerfile.agent .`
  succeeds for both platforms
- [ ] All binary downloads use `TARGETARCH` so the correct architecture binary is fetched
- [ ] No architecture-specific paths or assumptions outside of `TARGETARCH` substitution

---

## Tasks

- [ ] Audit Dockerfile for any hard-coded `amd64` references — replace with `${TARGETARCH}`
- [ ] Run multi-arch build locally with `docker buildx`
- [ ] Verify both platform images are present in the built manifest

---

## Dependencies

**Depends on:** STORY_08 (full Dockerfile complete)
**Blocks:** STORY_10 (smoke test should run against both arches in CI)

---

## Definition of Done

- [ ] Multi-arch build succeeds without errors
- [ ] `docker manifest inspect` shows both platforms
