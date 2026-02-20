# Story: Install gh CLI and git

**Epic:** [epic03-agent-image](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 30 minutes

---

## User Story

As a **developer**, I want the GitHub CLI (`gh`) and `git` installed so the agent can
clone the GitOps repo, create branches, push commits, and open pull requests.

---

## Acceptance Criteria

- [ ] `git` installed via apt (already in base layer — verify available)
- [ ] `gh` installed from GitHub's official apt repository
- [ ] `gh` responds to `gh --version`
- [ ] `git` responds to `git --version`
- [ ] `gh auth login --with-token` works in a container with a valid token injected

---

## Tasks

- [ ] Confirm `git` available from base layer
- [ ] Add GitHub CLI apt repo and install `gh`
- [ ] Verify both in smoke test

---

## Dependencies

**Depends on:** STORY_01 (base image)

---

## Definition of Done

- [ ] `git` and `gh` callable in built image
