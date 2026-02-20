# Epic: Controller

## Purpose

Implement the `ResultReconciler` — the controller-runtime reconcile loop that watches
`Result` CRDs, computes parent-resource fingerprints for deduplication, and dispatches
agent Jobs for new findings.

## Status: Not Started

## Dependencies

- Foundation epic complete

## Blocks

- Job Builder epic (controller calls jobbuilder)
- Deploy epic (controller is what gets deployed)

## Success Criteria

- [ ] Controller registers against `results.core.k8sgpt.ai` across all namespaces
- [ ] `fingerprintFor()` produces stable, parent-aware hashes
- [ ] In-memory processed map prevents duplicate Job creation for the same fingerprint
- [ ] Results with zero errors are filtered out before entering the reconcile queue
- [ ] On watcher restart, `IsAlreadyExists` is handled gracefully
- [ ] All unit tests for `fingerprintFor` pass (see test table in CONTROLLER_LLD.md)
- [ ] All integration tests using envtest pass

## Stories

| Story | File | Status |
|-------|------|--------|
| Result CRD scheme registration | [STORY_01_scheme.md](STORY_01_scheme.md) | Not Started |
| fingerprintFor implementation + tests | [STORY_02_fingerprint.md](STORY_02_fingerprint.md) | Not Started |
| In-memory processed map | [STORY_03_dedup_map.md](STORY_03_dedup_map.md) | Not Started |
| Reconcile loop (dispatch path) | [STORY_04_reconcile.md](STORY_04_reconcile.md) | Not Started |
| Error-filter predicate | [STORY_05_predicate.md](STORY_05_predicate.md) | Not Started |
| Manager setup and wiring in main.go | [STORY_06_manager.md](STORY_06_manager.md) | Not Started |
| Integration tests (envtest) | [STORY_07_integration_tests.md](STORY_07_integration_tests.md) | Not Started |

## Technical Overview

The controller is the heart of the watcher. Its correctness is critical — bugs here
cause either missed investigations (false negatives) or duplicate Jobs (false positives).

The deduplication strategy is documented in detail in
[`docs/DESIGN/lld/CONTROLLER_LLD.md`](../../DESIGN/lld/CONTROLLER_LLD.md).

TDD is mandatory for this epic. Write `fingerprintFor` tests before writing
`fingerprintFor`. Write reconcile tests before wiring the reconciler.

## Definition of Done

- [ ] All unit tests pass with race detector
- [ ] All envtest integration tests pass
- [ ] `go vet` is clean
- [ ] Controller LLD test table fully covered
