# Story: Non-Root User and Agent Entrypoint

**Epic:** [epic03-agent-image](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 30 minutes

---

## User Story

As a **developer**, I want the agent to run as a non-root user and use a dedicated
entrypoint script that performs `envsubst` on the prompt template before handing off to
OpenCode so the image is secure and the prompt is correctly rendered at runtime.

---

## Acceptance Criteria

- [ ] `agent` user created with `uid=1000`
- [ ] All tool installs complete before `USER agent` instruction
- [ ] `agent-entrypoint.sh` script at `/usr/local/bin/agent-entrypoint.sh`:
  - Authenticates `gh` using `gh auth login --with-token < /workspace/github-token`
  - Runs `envsubst "$VARS"` with an explicit variable list (not bare `envsubst`) to
    prevent corruption of `$` patterns in `FINDING_ERRORS` / `FINDING_DETAILS`
  - Writes rendered prompt to `/tmp/rendered-prompt.txt`
  - Execs `opencode run --file /tmp/rendered-prompt.txt` (not `"$(cat ...)"`)
- [ ] `ENTRYPOINT ["/usr/local/bin/agent-entrypoint.sh"]`
- [ ] `WORKDIR /workspace`
- [ ] Container starts and runs as uid=1000

---

## Tasks

- [ ] Create `docker/scripts/agent-entrypoint.sh` matching AGENT_IMAGE_LLD.md §5
- [ ] Add `COPY`, `chmod`, `USER`, `WORKDIR`, `ENTRYPOINT` to Dockerfile
- [ ] Verify `docker run --rm <image> id` shows uid=1000

---

## Dependencies

**Depends on:** STORY_06 (opencode installed), STORY_07 (token script present)
**Blocks:** STORY_09 (multi-arch), STORY_10 (smoke test)

---

## Definition of Done

- [ ] Image runs as non-root confirmed
- [ ] Entrypoint script executable in image
- [ ] `opencode run --file` used (not shell expansion)
- [ ] `envsubst "$VARS"` used with explicit variable list
