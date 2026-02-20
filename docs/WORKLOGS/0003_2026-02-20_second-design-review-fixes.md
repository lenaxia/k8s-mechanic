# Worklog: Second Design Review and Fixes

**Date:** 2026-02-20
**Session:** Second adversarial design review by delegated skeptical architect agent, followed by triage and remediation of all confirmed findings
**Status:** Complete

---

## Objective

Run a second independent design review to catch anything the first review (0002) missed.
The reviewer found 48 findings. Each was triaged against the actual documents to determine
whether it was real, overblown, or dismissible. All confirmed findings were then fixed.

---

## Review Summary

The delegated reviewer produced 48 findings: 9 Critical, 20 Major, 19 Minor.

After triage against the actual documents, the confirmed breakdown is:

| Severity | Reviewer count | Confirmed after triage |
|---|---|---|
| CRITICAL | 9 | 5 |
| MAJOR | 20 | 16 |
| MINOR | 19 | 22 (some downgraded from higher, some upgraded) |
| Dismissed | 0 | 4 (F-036, F-039, F-045, and F-008 reclassified) |

Key dismissals / downgrades:
- F-008 (TOCTOU race): single-worker reconcile loop eliminates this; overblown
- F-009 (--state open): documented intentional behaviour; downgraded to MINOR
- F-012 (agent reads Secrets): explicitly accepted design decision matching k8sgpt-operator
- F-036 (hardcoded values): correct for target deployment; not a gap
- F-039 (kubectl logs on Deployment): guarded by "If the resource is a Pod" condition
- F-045 (PDB): PDB with minAvailable=0 adds no value for a stateless controller

---

## Work Completed

### 1. Critical fixes

| ID | Finding | Fix |
|---|---|---|
| F-001 | STORY_03 and STORY_04 say "requeue after 5 minutes"; CONTROLLER_LLD says "return nil" | Removed requeue language from STORY_03 and STORY_04; aligned to LLD |
| F-002 | STORY_04_init_container still references deleted `GitInitImage` and container name `git-clone` | Updated story: `git-token-clone`, uses `b.cfg.AgentImage` |
| F-003 | STORY_08_entrypoint acceptance criteria uses `opencode run "$(cat ...)"` | Updated story to use `opencode run --file /tmp/rendered-prompt.txt` and `envsubst "$VARS"` |
| F-007 | `GITHUB_APP_PRIVATE_KEY` injected into main container env — LLM has the private key | Removed `GITHUB_APP_PRIVATE_KEY`, `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID` from main container env in JOBBUILDER_LLD and HLD §12 |
| F-014 | JWT `iss` field uses `%s` format → JSON string; GitHub requires JSON number | Changed to `%d` format with integer conversion in AGENT_IMAGE_LLD §4 script |

### 2. Major fixes

| ID | Finding | Fix |
|---|---|---|
| F-004 | `opencode run --file` unverified against actual CLI | Added explicit verification requirement note to AGENT_IMAGE_LLD §5 |
| F-005 | HLD §4.3 and STORY_06 still reference install script; LLD uses binary download | Updated HLD §4.3 table and STORY_06 to match LLD binary download approach |
| F-006 / F-021 | Process substitution `<(printf ...)` for private key — `/dev/fd` portability risk | Script now writes key to a `mktemp` file, uses file path with openssl, removes temp file immediately |
| F-011 | No checksum verification on binary downloads | Added SHA256 verification step for every binary download in Dockerfile (AGENT_IMAGE_LLD §3) |
| F-013 | Prompt injection risk from FINDING_DETAILS not documented | Added prompt injection threat note to HLD §10 security constraints |
| F-015 | HLD §6 shows string-concatenation fingerprint; LLD §4 shows JSON marshal — different hashes | Replaced HLD §6 pseudocode with a note deferring to CONTROLLER_LLD §4 as authoritative |
| F-017 | No securityContext on agent Job pod spec | Added securityContext to JOBBUILDER_LLD §4 pod template: runAsNonRoot, runAsUser=1000, allowPrivilegeEscalation=false, drop ALL caps |
| F-018 | PROMPT_LLD §5 tuning guide says `--search`; Step 1 uses `--jq` | Updated tuning guide to reference the `--json --jq headRefName` filter |
| F-023 | STORY_04_crd_types acceptance criteria still lists `AutoRemediationStatus` | Removed from story; noted it is intentionally omitted per CONTROLLER_LLD |
| F-024 | Watcher Role has no `delete` verb for Jobs | Added `delete` to watcher Role in DEPLOY_LLD §4.3 and HLD §7 |
| F-026 | `flux logs --name=${FINDING_PARENT}` assumes HelmRelease name equals Deployment parent name | Step 6 reworded to instruct agent to search for HelmRelease name rather than assume equality |
| F-028 | Deleted+recreated Result silently skipped — fingerprint stays in map | Added eviction-on-delete to CONTROLLER_LLD §5 reconcile loop step 1 |
| F-029 | CI test timeout 30s kills envtest; STORY_07 says 120s | Updated EPIC06 README to use `-timeout 300s` for CI; separate unit/integration targets noted |
| F-041 | Watcher Dockerfile entirely missing from design | Added WATCHER_IMAGE_LLD.md with Dockerfile (scratch base, static CGO_DISABLED binary) |
| F-042 | `.gitignore` pattern `/deploy/kustomize/secret-*.yaml` excludes placeholder files | Fixed gitignore: renamed placeholder convention to `secret-*-placeholder.yaml`; updated kustomization.yaml in DEPLOY_LLD §9 |

### 3. Minor fixes

| ID | Fix |
|---|---|
| F-008/F-020 | Added explicit note to CONTROLLER_LLD §8 that single-worker loop eliminates TOCTOU race |
| F-009 | Added `--state open` limitation note to HLD §11 failure modes |
| F-010 | Added base image digest pinning note to AGENT_IMAGE_LLD §2 principles |
| F-012 | Added explicit note to HLD §10 that agent Secret read access is a known accepted risk matching k8sgpt-operator permissions |
| F-016/F-032 | Added namespace assumption note to HLD §12 and PROMPT_LLD §3 |
| F-019 | Added note to JOBBUILDER_LLD §4 that volume mount is for future file-based key reading |
| F-022 | Added note to AGENT_IMAGE_LLD §3 recommending noreply GitHub email format |
| F-025 | Changed CONTROLLER_LLD §4 and §9: `fingerprintFor` returns `(string, error)`; removed panic |
| F-027 | Added non-Flux graceful degradation note to PROMPT_LLD §2 ENVIRONMENT section |
| F-030 | Clarified backoffLimit wording in HLD §4.2 |
| F-031 | Moved §6.3 Required GitHub Label to its own §7 in DEPLOY_LLD |
| F-033 | Added `stern` usage note to PROMPT_LLD §2 Step 2 (multi-pod log collection) |
| F-034 | Clarified Config struct relationship in JOBBUILDER_LLD §3: jobbuilder.Config is a subset; full Config passed, unused fields ignored |
| F-035 | Fixed smoke test commands in AGENT_IMAGE_LLD §8 to use `--entrypoint /bin/bash` override |
| F-037 | Added `Owns(&batchv1.Job{})` to CONTROLLER_LLD §7 SetupWithManager |
| F-038 | Added `gettext-base` to STORY_01_base_image acceptance criteria package list |
| F-040 | Added fingerprint sensitivity note to HLD §11 failure modes |
| F-043 | Clarified Job count definition in CONTROLLER_LLD §5: use `job.Status.CompletionTime == nil` |
| F-044 | Added note on `dd` vs `install` for gh keyring in AGENT_IMAGE_LLD §3 |
| F-046 | Added label sanitisation note to JOBBUILDER_LLD §4 |
| F-047 | Clarified nil error slice → `[]` (not `null`) in JOBBUILDER_LLD §6 |
| F-048 | Updated HLD §8 token exchange flow to say env vars, not mounted Secret files |

---

## Key Decisions

| Decision | Rationale |
|---|---|
| Private key read from mounted file in init container (not env var) | Eliminates PEM newline risk, eliminates `/dev/fd` dependency, removes the key from the process environment of the main container entirely |
| Main container env stripped of all GitHub App credentials | The main container only needs the installation token at `/workspace/github-token`; the App ID and private key serve no purpose there and are a security liability |
| `fingerprintFor` returns `(string, error)` instead of panicking | Idiomatic Go; makes error handling explicit; no meaningful performance cost |
| Watcher Dockerfile uses `scratch` base | Static CGO_DISABLED Go binary needs no runtime dependencies; scratch is smallest possible and eliminates OS-level CVEs in the watcher image |
| Kustomize placeholder files use `-placeholder.yaml` suffix | Makes the gitignore pattern unambiguous: `secret-*.yaml` (real, ignored) vs `secret-*-placeholder.yaml` (template, committed) |
| CI test timeout set to 300s | envtest downloads and starts a real API server; 30s is reliably insufficient on cold CI runners |

---

## Blockers

None. All 44 confirmed findings resolved.

---

## Tests Run

No tests run — this session was documentation-only. Implementation has not begun.

---

## Next Steps

Design is now ready for implementation. Begin with **epic00-foundation**:

1. `STORY_01_module_setup.md` — directory structure and stubs (already partially done this session; needs commit)
2. `STORY_02_config.md` — `internal/config/config.go` with `FromEnv()`, tests first
3. `STORY_03_logging.md` — `internal/logging/logging.go`, tests first
4. `STORY_04_crd_types.md` — `api/v1alpha1/result_types.go`, tests first

Each story follows TDD. Read the relevant LLD before starting each story.

---

## Files Modified

| File | Action |
|---|---|
| `docs/BACKLOG/epic01-controller/STORY_03_dedup_map.md` | Updated — removed requeue-5min language |
| `docs/BACKLOG/epic01-controller/STORY_04_reconcile.md` | Updated — removed requeue-5min language |
| `docs/BACKLOG/epic02-jobbuilder/STORY_04_init_container.md` | Updated — GitInitImage→AgentImage, git-clone→git-token-clone, /bin/sh→/bin/bash |
| `docs/BACKLOG/epic03-agent-image/STORY_06_opencode.md` | Updated — install script→binary download |
| `docs/BACKLOG/epic03-agent-image/STORY_08_entrypoint.md` | Updated — opencode run --file, envsubst "$VARS" |
| `docs/BACKLOG/epic00-foundation/STORY_04_crd_types.md` | Updated — removed AutoRemediationStatus |
| `docs/BACKLOG/epic03-agent-image/STORY_01_base_image.md` | Updated — added gettext-base |
| `docs/BACKLOG/epic06-ci-cd/README.md` | Updated — test timeout 300s |
| `docs/DESIGN/HLD.md` | Updated — §4.3 binary download, §6 fingerprint algo, §7 delete verb, §8 env vars, §10 prompt injection + Secret read note, §11 --state open + fingerprint sensitivity, §12 removed private key from main container env, namespace note |
| `docs/DESIGN/lld/CONTROLLER_LLD.md` | Updated — §4 returns (string,error), §5 evict on delete + CompletionTime nil, §7 Owns Job, §8 single-worker note, §9 no panic |
| `docs/DESIGN/lld/JOBBUILDER_LLD.md` | Updated — §3 Config note, §4 removed private key from main env + securityContext + label sanitisation note + volume mount note, §6 nil→[] |
| `docs/DESIGN/lld/AGENT_IMAGE_LLD.md` | Updated — §2 digest pin note, §3 checksum verification + gh keyring note + noreply email note, §4 JWT iss %d + mktemp key file, §5 verification requirement note, §8 smoke test --entrypoint fix |
| `docs/DESIGN/lld/PROMPT_LLD.md` | Updated — §2 stern step + non-Flux note, §3 namespace assumption note, §5 --jq tuning guide |
| `docs/DESIGN/lld/DEPLOY_LLD.md` | Updated — §4.3 delete verb, §6.3→§7 renumbered, §9 placeholder filenames |
| `docs/DESIGN/lld/WATCHER_IMAGE_LLD.md` | Created — watcher Dockerfile design |
| `.gitignore` | Updated — placeholder file naming convention |
| `docs/WORKLOGS/0003_2026-02-20_second-design-review-fixes.md` | Created |
| `docs/WORKLOGS/README.md` | Updated — added 0003 entry |
| `docs/STATUS.md` | Updated |
