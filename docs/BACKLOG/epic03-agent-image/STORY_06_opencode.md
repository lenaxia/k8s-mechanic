# Story: Install OpenCode

**Epic:** [epic03-agent-image](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 30 minutes

---

## User Story

As a **developer**, I want OpenCode installed in the agent image so it is the AI agent
driver that executes the investigation prompt.

---

## Acceptance Criteria

- [ ] OpenCode installed via direct pinned release binary download (not the install script)
- [ ] Version controlled by `ARG OPENCODE_VERSION` at the top of the Dockerfile
- [ ] Binary fetched from GitHub releases and placed directly at `/usr/local/bin/opencode`
- [ ] `opencode --version` responds without error
- [ ] Installation layer runs before the non-root user switch
- [ ] **Before first build:** verify the exact GitHub release URL and binary name for the
  version in use by running `opencode run --help` to confirm the `--file` flag exists

---

## Tasks

- [ ] Add `ARG OPENCODE_VERSION` to Dockerfile
- [ ] Add OpenCode binary download layer to Dockerfile (see AGENT_IMAGE_LLD.md §3)
- [ ] Verify `opencode run --file` flag exists in the installed version
- [ ] Verify in smoke test

---

## Notes

- Do NOT use `curl -fsSL https://opencode.ai/install | bash` — the install script is
  non-reproducible and installs to a user-local path (`~/.opencode/bin/`)
- Use the direct binary download approach specified in AGENT_IMAGE_LLD.md §3
- Verify the exact GitHub org/repo and binary naming convention for the target version
  before writing the Dockerfile layer

---

## Dependencies

**Depends on:** STORY_01 (base image)
**Blocks:** STORY_08 (entrypoint — opencode must be present before wiring the entrypoint)

---

## Definition of Done

- [ ] `opencode --version` callable in built image as non-root user
- [ ] `opencode run --file` flag confirmed to exist
