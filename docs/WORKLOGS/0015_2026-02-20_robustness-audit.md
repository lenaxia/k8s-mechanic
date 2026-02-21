# Worklog: Robustness Audit

**Date:** 2026-02-20
**Session:** Full robustness audit of all Go code; 2 review passes, all gaps resolved
**Status:** Complete

---

## Objective

Harden all existing Go code before proceeding to epic04. A robustness audit identified 21
findings (2 critical, 7 high). All critical and high findings were fixed; medium and low
findings were addressed where actionable.

---

## Work Completed

### Critical findings fixed

| Finding | File | Issue | Fix |
|---------|------|-------|-----|
| 1 | `internal/provider/k8sgpt/provider.go` | `Fingerprint()` panic on hash write error | Changed interface to `(string, error)`; replaced panic with `return "", fmt.Errorf(...)` |
| 2 | `internal/provider/k8sgpt/reconciler.go` | `fingerprintFor()` dead code with same panic risk | Deleted file entirely |

### High findings fixed

| Finding | File | Issue | Fix |
|---------|------|-------|-----|
| 3 | `internal/provider/provider.go` | `ExtractFinding` errors were already propagated — confirmed clean | No change needed |
| 9 | `internal/provider/provider.go` | Result deletion only cancelled Pending/Dispatched jobs; Running jobs orphaned | Added `PhaseCancelled` to CRD; expanded filter to `Pending\|Dispatched\|Running\|""`; patch phase before delete; collect errors across loop |
| MaxConcurrentJobs | `internal/controller/remediationjob_controller.go` | `activeCount >= 0` always true when `MaxConcurrentJobs==0` | Fixed guard to `MaxConcurrentJobs > 0 && activeCount >= MaxConcurrentJobs` |
| SinkType | `internal/provider/provider.go` | `SinkType` hardcoded as `"github"` | Changed to `r.Cfg.SinkType` |
| fp length | `internal/provider/provider.go` | No `len(fp) < 12` guard before slice | Added guard returning error |

### Medium findings fixed

| Finding | File | Issue | Fix |
|---------|------|-------|-----|
| 17+18 | `internal/provider/k8sgpt/reconciler_test.go` | 7 tests exercising dead `fingerprintFor` | Deleted file |
| 19 | `internal/controller/remediationjob_controller.go` | Missing RBAC markers for `batch/jobs watch+list` and `remediationjobs/finalizers` | Added all three markers |
| 20 | `cmd/watcher/main.go` | No `GracefulShutdownTimeout` | Added `30 * time.Second` |
| 14 | `internal/controller/remediationjob_controller.go` | `MaxConcurrentJobs` undocumented | Added godoc comment |
| DeepCopy | `api/v1alpha1/remediationjob_types.go` | Shallow `out.Spec = in.Spec` unexplained | Added comment explaining value-type safety |

### New tests added

| Test | File | Purpose |
|------|------|---------|
| `TestRemediationJobReconciler_Cancelled_ReturnsNil` | `controller/remediationjob_controller_test.go` | PhaseCancelled stops requeue |
| `TestSourceProviderReconciler_NotFound_DeletesRunningRJobs` | `provider/provider_test.go` | PhaseRunning is now cancelled |
| `TestSourceProviderReconciler_FingerprintError_ReturnsError` | `provider/provider_test.go` | Fingerprint error propagates |
| Updated `TestK8sGPTProvider_Fingerprint_Deterministic` | `provider/k8sgpt/provider_test.go` | New `(string, error)` signature |
| Updated `TestK8sGPTProvider_Fingerprint_OrderIndependent` | `provider/k8sgpt/provider_test.go` | New `(string, error)` signature |

---

## Key Design Decisions

- **Fingerprint interface**: Changed to `(string, error)` throughout. The only implementation
  (`K8sGPTProvider`) encodes a simple struct — `json.Encode` cannot fail for it — but the
  interface must not constrain future providers.

- **PhaseCancelled**: Added as a terminal phase distinct from `Failed`. Semantics: job was
  deleted because its source Result was deleted, not because the agent failed. The controller
  reconciler short-circuits on `PhaseCancelled` identically to `PhaseFailed`.

- **Cancel loop error handling**: Changed from fail-fast to collect-and-continue. A status
  patch `Conflict` on one job no longer prevents the remaining jobs from being cancelled.
  All errors are joined and returned at the end so the reconciler requeues.

- **fingerprintFor deleted**: The function operated on `v1alpha1.ResultSpec` directly,
  bypassing the `domain.Finding` abstraction. The `SourceProvider.Fingerprint(f *Finding)`
  interface method is the correct reuse point for all future providers.

---

## Tests Run

```
go mod tidy                       → clean
go build ./...                    → clean
go test -timeout 30s -race ./...  → all 9 packages pass
```

---

## Files Created/Modified

| File | Change |
|------|--------|
| `internal/domain/provider.go` | `Fingerprint` signature: `string` → `(string, error)` |
| `internal/provider/k8sgpt/provider.go` | `Fingerprint` returns error instead of panic |
| `internal/provider/k8sgpt/provider_test.go` | Updated Fingerprint call sites |
| `internal/provider/k8sgpt/reconciler.go` | **Deleted** — dead `fingerprintFor` |
| `internal/provider/k8sgpt/reconciler_test.go` | **Deleted** — tests for dead code |
| `internal/provider/provider.go` | Fingerprint error propagation; fp length guard; SinkType from cfg; PhaseCancelled cancel loop; collect-and-continue error handling |
| `internal/provider/provider_test.go` | `fpErr` field on fake; 3 new tests |
| `api/v1alpha1/remediationjob_types.go` | `PhaseCancelled` constant; enum marker; DeepCopy comment |
| `internal/controller/remediationjob_controller.go` | RBAC markers; MaxConcurrentJobs guard fix; PhaseCancelled case; Cfg godoc |
| `internal/controller/remediationjob_controller_test.go` | `TestRemediationJobReconciler_Cancelled_ReturnsNil` |
| `cmd/watcher/main.go` | `GracefulShutdownTimeout`; `time` import |

---

## Next Steps

Robustness audit complete. Next: **epic04 — Kustomize deploy manifests**.
Read `docs/DEPLOY_LLD.md` before starting.
