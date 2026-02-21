# Epic: Job Builder

## Purpose

Implement `internal/jobbuilder` — the pure function that constructs a fully-specified
`batch/v1 Job` from a `*RemediationJob`. This is the single source of truth for what
every agent Job looks like.

## Status: Complete

## Dependencies

- epic00-foundation complete
- epic00.1-interfaces complete (JobBuilder interface, RemediationJob types)

## Blocks

- epic01-controller/STORY_04 (RemediationJobReconciler calls `jobBuilder.Build(rjob)`)
- epic04-deploy (Job spec must be finalised before manifests can be written)

## Success Criteria

- [x] `Build(*RemediationJob)` produces a deterministic Job name from the fingerprint
- [x] All FINDING_* and GITOPS_* environment variables are present in the Job spec
- [x] Init container and main container are both present with correct images
- [x] All three volumes are mounted correctly
- [x] Job has correct ownerReference pointing at the RemediationJob
- [x] Job lifecycle settings match the HLD (backoff=1, deadline=900s, TTL=86400s)
- [x] All unit tests from JOBBUILDER_LLD.md §7 test table pass
- [x] Compile-time assertion `var _ domain.JobBuilder = (*Builder)(nil)` is present

## Stories

| Story | File | Status |
|-------|------|--------|
| Builder struct and Config | [STORY_01_builder_struct.md](STORY_01_builder_struct.md) | Complete |
| Job name generation | [STORY_02_job_name.md](STORY_02_job_name.md) | Complete |
| Environment variable injection | [STORY_03_env_vars.md](STORY_03_env_vars.md) | Complete |
| Init container spec | [STORY_04_init_container.md](STORY_04_init_container.md) | Complete |
| Main container spec | [STORY_05_main_container.md](STORY_05_main_container.md) | Complete |
| Volume mounts | [STORY_06_volumes.md](STORY_06_volumes.md) | Complete |
| Job metadata (labels, annotations, ownerReference) | [STORY_07_metadata.md](STORY_07_metadata.md) | Complete |

## Technical Overview

`Build(*RemediationJob)` is a pure function — no side effects, no API calls. The full
spec is in [`docs/DESIGN/lld/JOBBUILDER_LLD.md`](../../DESIGN/lld/JOBBUILDER_LLD.md).

The `FINDING_ERRORS` field is read directly from `rjob.Spec.Finding.Errors` — the
`SourceProviderReconciler` already redacted Sensitive fields when it created the `RemediationJob`.
The builder does not perform redaction.

## Definition of Done

- [x] All unit tests pass with race detector
- [x] JOBBUILDER_LLD.md test table fully covered
- [x] `go vet` clean
- [x] Compile-time `domain.JobBuilder` assertion present
