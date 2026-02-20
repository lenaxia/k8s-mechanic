# Story: Reconciler Interface and Struct Skeleton

**Epic:** [Interfaces and Test Infrastructure](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 45 minutes

---

## User Story

As a **developer**, I want the `ResultReconciler` struct and its `SetupWithManager` method
declared (but unimplemented) so that `cmd/watcher/main.go` can wire the full manager
without the controller epic needing to be complete first.

---

## Acceptance Criteria

- [ ] `internal/controller/result_controller.go` defines the `ResultReconciler` struct with
  all fields it will need, but with stub method bodies that return `nil` or panic with
  `"not implemented"`:
  ```go
  type ResultReconciler struct {
      client.Client
      Scheme     *runtime.Scheme
      Log        *zap.Logger
      JobBuilder domain.JobBuilder
      cfg        domain.JobBuilderConfig
      mu         sync.Mutex
      processed  map[string]domain.ProcessedEntry
  }

  func (r *ResultReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
      panic("not implemented")
  }

  func (r *ResultReconciler) SetupWithManager(mgr ctrl.Manager) error {
      panic("not implemented")
  }
  ```
- [ ] `cmd/watcher/main.go` is updated from its empty stub to wire up the manager,
  config, logger, and reconciler skeleton — calling `mgr.Start()`. It does not need to
  work end-to-end yet; the reconciler will panic if a Result arrives, which is acceptable
  at this stage.
- [ ] `go build ./...` compiles cleanly with the stub in place
- [ ] No test is required for the stub itself — tests are written in epic01 when the
  body is implemented (TDD applies to the logic, not the skeleton declaration)

---

## Note on `main.go`

`main.go` wiring is included here rather than in epic01 because:
1. It only references the struct and interface — no logic
2. It unblocks local `go build` verification throughout epics 01 and 02
3. It will not change when epic01 implements the reconciler body

The full wiring spec is in CONTROLLER_LLD.md §7.

---

## Tasks

- [ ] Create `internal/controller/result_controller.go` with struct + stub methods
- [ ] Update `cmd/watcher/main.go` to full wiring (scheme, manager, reconciler, health probes,
  signal handler) per CONTROLLER_LLD.md §7
- [ ] Verify `go build ./...` compiles

---

## Dependencies

**Depends on:** STORY_01 (domain types)
**Depends on:** STORY_02 (JobBuilder interface)
**Depends on:** epic00-foundation/STORY_03 (logging — Logger field type)
**Depends on:** epic00-foundation/STORY_04 (CRD types — scheme registration)
**Blocks:** epic01-controller (fills in the stub body)

---

## Definition of Done

- [ ] `go build ./...` clean
- [ ] `go vet ./...` clean
- [ ] `main.go` contains full manager wiring, not just `func main() {}`
