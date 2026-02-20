# Story: Reconciler and Provider Skeletons

**Epic:** [Interfaces and Test Infrastructure](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 1 hour

---

## User Story

As a **developer**, I want the provider and reconciler structs declared (but unimplemented)
so that `cmd/watcher/main.go` can wire the full manager, and `go build ./...` passes
end-to-end before any reconcile logic is written.

---

## Acceptance Criteria

- [ ] `internal/provider/k8sgpt/reconciler.go` defines `ResultReconciler` with all fields
  from CONTROLLER_LLD.md §5.1, and stub `Reconcile` and `SetupWithManager` methods that
  `panic("not implemented")`:
  ```go
  type ResultReconciler struct {
      client.Client
      Scheme *runtime.Scheme
      Log    *zap.Logger
      Cfg    config.Config
  }
  ```

- [ ] `internal/provider/k8sgpt/provider.go` defines `K8sGPTSourceProvider` with the
  struct and `SetupWithManager` from CONTROLLER_LLD.md §5.0:
  ```go
  type K8sGPTSourceProvider struct {
      Cfg config.Config
      Log *zap.Logger
  }
  func NewProvider(cfg config.Config, log *zap.Logger) *K8sGPTSourceProvider
  func (p *K8sGPTSourceProvider) SetupWithManager(mgr ctrl.Manager) error
  ```
  Compile-time assertion: `var _ provider.SourceProvider = (*K8sGPTSourceProvider)(nil)`

- [ ] `internal/controller/remediationjob_controller.go` defines `RemediationJobReconciler`
  with all fields from CONTROLLER_LLD.md §6.1, and stub methods:
  ```go
  type RemediationJobReconciler struct {
      client.Client
      Scheme     *runtime.Scheme
      Log        *zap.Logger
      JobBuilder domain.JobBuilder
      Cfg        config.Config
  }
  ```

- [ ] `cmd/watcher/main.go` is updated from its empty stub to the full manager wiring
  from CONTROLLER_LLD.md §7:
  - Scheme registration (clientgo + batchv1 + v1alpha1)
  - `config.FromEnv()` with fatal on error
  - Logger construction
  - `RemediationJobReconciler` registered directly
  - Provider loop: `[]provider.SourceProvider{k8sgpt.NewProvider(cfg, logger)}`
  - Health probes
  - `mgr.Start(ctrl.SetupSignalHandler())`

- [ ] `go build ./...` compiles cleanly with stubs in place

---

## Note on `main.go`

The full wiring is included here because it references only types and interfaces — no
logic. It unblocks `go build` verification throughout all remaining epics and will not
need to change when the reconciler bodies are implemented in epic01.

---

## Tasks

- [ ] Create `internal/provider/interface.go` (if not created in STORY_02)
- [ ] Create `internal/provider/k8sgpt/provider.go` with struct + SetupWithManager stub
- [ ] Create `internal/provider/k8sgpt/reconciler.go` with struct + stub methods
- [ ] Create `internal/controller/remediationjob_controller.go` with struct + stub methods
- [ ] Rewrite `cmd/watcher/main.go` with full manager wiring
- [ ] Verify `go build ./...` compiles

---

## Dependencies

**Depends on:** STORY_01 (RemediationJob types + JobBuilderConfig)
**Depends on:** STORY_02 (JobBuilder and SourceProvider interfaces)
**Depends on:** epic00-foundation/STORY_03 (logging)
**Depends on:** epic00-foundation/STORY_04 (CRD types + AddToScheme)
**Blocks:** epic01-controller (fills in stub bodies)

---

## Definition of Done

- [ ] `go build ./...` clean
- [ ] `go vet ./...` clean
- [ ] `main.go` contains full manager wiring with provider loop, not just `func main() {}`
