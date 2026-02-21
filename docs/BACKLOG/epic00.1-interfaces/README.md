# Epic: Interfaces, Data Structures, and Test Infrastructure

## Purpose

Define all shared interfaces, data structures, and test infrastructure before any
functional epic begins. This epic produces no observable runtime behaviour — it produces
the skeleton that epics 01 and 02 fill in. Getting these right first prevents costly
refactoring later.

## Status: Complete

## Dependencies

- epic00-foundation complete (module compiles, config works, logging works, CRD types exist)

## Blocks

- epic01-controller (uses SourceProvider interface, fingerprintFor, RemediationJob CRD)
- epic02-jobbuilder (uses JobBuilder interface, jobbuilder.Config)

## Stories

| Story | File | Status |
|-------|------|--------|
| Core domain types | [STORY_01_domain_types.md](STORY_01_domain_types.md) | Complete |
| Builder interface | [STORY_02_builder_interface.md](STORY_02_builder_interface.md) | Complete |
| Reconciler interface and struct skeleton | [STORY_03_reconciler_skeleton.md](STORY_03_reconciler_skeleton.md) | Complete |
| envtest suite setup | [STORY_04_envtest_suite.md](STORY_04_envtest_suite.md) | Complete |
| Fake/stub implementations | [STORY_05_fakes.md](STORY_05_fakes.md) | Complete |

## Success Criteria

- [x] All interfaces compile and are reachable from the packages that will use them
- [x] `SourceProvider`, `Finding`, `SourceRef` defined in `internal/domain/provider.go`
- [x] `JobBuilder` interface defined in `internal/domain/interfaces.go`
- [x] envtest suite bootstraps a real API server and tears it down cleanly
- [x] Fake `JobBuilder` exists and is usable in controller unit tests without a cluster
- [x] `go build ./...` and `go vet ./...` are clean
- [x] No functional logic implemented — only types, interfaces, and test plumbing

## Definition of Done

- [x] All tests in this epic pass with `-race`
- [x] `go vet ./...` clean
- [x] `go build ./...` clean
- [x] Downstream epics can reference these types without circular imports
