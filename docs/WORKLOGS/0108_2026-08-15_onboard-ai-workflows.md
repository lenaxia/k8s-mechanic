# Worklog: Onboard to ai-workflows Reusable Workflows

**Date:** 2026-08-15
**Session:** Onboarded k8s-mechanic as a plumbing-only consumer of lenaxia/ai-workflows: replaced the three bespoke OpenCode workflows with thin callers delegating to the pinned reusable workflows, added 16 k8s-mechanic-specific forked prompt files, and registered the repo as a consumer.
**Status:** Complete

---

## Objective

Replace the three hand-maintained OpenCode GitHub Action workflows
(`ai-comment.yml`, `issue-opened.yml`, `pr-review.yml`) with thin caller
workflows delegating to the pinned reusable workflows in
`lenaxia/ai-workflows`, and register k8s-mechanic as a consumer so future
ai-workflows releases propagate prompt/pin updates automatically.

## Work Completed

### 1. Consumer registration (ai-workflows repo, PR #32)
- Created `consumers/k8s-mechanic.yaml` — plumbing-only consumer config
  (all 15 templated prompts forked; `context.md` consumer-owned)
- Added `k8s-mechanic` to the `propagate.yml` consumer matrix
- Documented the consumer in the ai-workflows README consumers table
- Merged via ai-workflows PR #32 (AI-reviewed, approved)

### 2. Caller workflows (this repo, PR #48)
- `.github/workflows/ai-comment.yml` — slash-command caller
  (`/ai`, `/fix`, `/implement`, `/review`, `/analyze`, `/test`,
  `/security`, `/explain`, `/triage`, `/design`, `/merge`, `/help`)
  delegating to `ai-workflows/.github/workflows/ai-comment.yml@v0.2.9`;
  command-token + author_association filter kept in the caller per
  ai-workflows Lessons Learned #4
- `.github/workflows/issue-opened.yml` — issue-responder caller
  (`issue-opened.yml@v0.2.9`)
- `.github/workflows/pr-review.yml` — formal blocking AI review caller
  (`pr-review.yml@v0.2.9`); now also re-reviews on `synchronize`;
  automation-bot PRs are skipped at the reusable-workflow job level
  (replaces the old inline `if: renovate[bot]` filter)

### 3. Forked prompt files (this repo)
- Created 16 k8s-mechanic-specific prompts under `.github/prompts/`:
  context, core-rules, code-change-workflow, pr-review, fix, implement,
  test, security, analyze, explain, triage, design, issue-responder,
  merge, help, commands-footer
- All prompts are forked (never rendered by ai-sync) — content reflects
  the operator's domain: controller-runtime reconciliation, RemediationJob
  CRDs, Helm chart, redact wrappers, THREAT_MODEL security posture,
  envtest rules, worklog discipline

### 4. opencode.json
- Added `go*` and `make*` to the bash allowlist so the AI can run the Go
  build/test cycle (`make build`, `make test`, `make lint`)

## Key Decisions

- **Plumbing-only consumer (all prompts forked):** the shared ai-workflows
  templates are goKore-derived and wrong for this repo; forking every prompt
  matches the established pattern for non-gokore consumers (containers,
  talos-ops-prod, synology-to-immich, ai-or-not) and keeps sync from ever
  overwriting project-specific content.
- **Pin to v0.2.9:** the current ai-workflows tag; `uses:` refs must be
  hardcoded literals (no `vars` context allowed in `uses:` — ai-workflows
  Lessons Learned #1). Future bumps are handled by `propagate.yml`.
- **No `if:` bot filter in pr-review caller:** the reusable workflow at
  v0.2.9+ skips automation bots at the job level; Renovate PRs are
  separately handled by `renovate-analysis.yml`.

## Blockers

None.

## Tests Run

- `route-command.sh` (ai-workflows, v0.2.9) exercised locally against the new
  prompts for all 12 slash commands — all assemble correctly, including
  `--no-merge` hold detection
- Workflow YAML parsed cleanly (pyyaml)
- ai-workflows `go test ./...` green after consumer registration
  (incl. `TestPropagateMatrixConsumersHaveConfigFiles`)
- End-to-end validation: the new PR Review caller fired on this PR and the
  reusable workflow ran successfully (review posted via `gh pr review`)

## Next Steps

- Monitor the first AI-review cycle on a real code PR to confirm prompt
  quality in production
- Future ai-workflows releases will propagate pin bumps + prompt renders
  automatically via `propagate.yml` (requires `AI_WORKFLOWS_PAT` for
  cross-repo PRs)

## Files Modified

- `.github/workflows/ai-comment.yml` (rewritten as caller)
- `.github/workflows/issue-opened.yml` (rewritten as caller)
- `.github/workflows/pr-review.yml` (rewritten as caller)
- `opencode.json` (bash allowlist: +go*, +make*)
- `.github/prompts/` (16 new files)
- `docs/WORKLOGS/0108_2026-08-15_onboard-ai-workflows.md` (this entry)
