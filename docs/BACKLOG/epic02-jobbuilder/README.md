# Epic: Job Builder

## Purpose

Implement `internal/jobbuilder` — the pure function that constructs a fully-specified
`batch/v1 Job` from a `Result` CRD and a fingerprint. This is the single source of truth
for what every agent Job looks like.

## Status: Not Started

## Dependencies

- Foundation epic complete
- Controller epic complete (controller calls `jobbuilder.Build()`)

## Blocks

- Deploy epic (Job spec must be finalised before manifests can be written)

## Success Criteria

- [ ] `Build()` produces a deterministic Job name from the fingerprint
- [ ] All FINDING_* and GITOPS_REPO environment variables are present in the Job spec
- [ ] FINDING_ERRORS is valid JSON
- [ ] Init container and main container are both present with correct images
- [ ] All three volumes are mounted correctly
- [ ] Job lifecycle settings match the HLD (backoff=1, deadline=900s, TTL=86400s)
- [ ] All unit tests from JOBBUILDER_LLD.md test table pass

## Stories

| Story | File | Status |
|-------|------|--------|
| Builder struct and Config | [STORY_01_builder_struct.md](STORY_01_builder_struct.md) | Not Started |
| Job name generation | [STORY_02_job_name.md](STORY_02_job_name.md) | Not Started |
| Environment variable injection | [STORY_03_env_vars.md](STORY_03_env_vars.md) | Not Started |
| Init container spec | [STORY_04_init_container.md](STORY_04_init_container.md) | Not Started |
| Main container spec | [STORY_05_main_container.md](STORY_05_main_container.md) | Not Started |
| Volume mounts | [STORY_06_volumes.md](STORY_06_volumes.md) | Not Started |
| Job metadata (labels, annotations) | [STORY_07_metadata.md](STORY_07_metadata.md) | Not Started |

## Technical Overview

`Build()` is a pure function — no side effects, no API calls. It takes a `*Result` and a
fingerprint string and returns a `*batchv1.Job`. This makes it trivially testable without
a cluster or envtest.

See [`docs/DESIGN/lld/JOBBUILDER_LLD.md`](../../DESIGN/lld/JOBBUILDER_LLD.md) for the
complete spec including the init container shell script design.

## Definition of Done

- [ ] All unit tests pass with race detector
- [ ] JOBBUILDER_LLD.md test table fully covered
- [ ] `go vet` clean
