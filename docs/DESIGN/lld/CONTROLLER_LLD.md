# Domain: Controller — Low-Level Design

**Version:** 1.1
**Date:** 2026-02-19
**Status:** Implementation Ready
**HLD Reference:** [Sections 4.1, 5, 6, 11](../HLD.md)

---

## 1. Overview

### 1.1 Purpose

The controller watches `Result` CRDs produced by the k8sgpt-operator, computes a
parent-resource fingerprint for each, deduplicates against an in-memory map, and creates
a `batch/v1 Job` for each unique finding.

### 1.2 Responsibilities

- Register a controller-runtime informer on `results.core.k8sgpt.ai` (all namespaces)
- Compute deterministic fingerprints from Result specs
- Maintain an in-memory processed-fingerprint map (mutex-protected)
- Create agent Jobs via the Kubernetes API
- Handle transient errors with controller-runtime's built-in requeue backoff
- Filter out Results with no errors (skip-event predicate)

### 1.3 Design Principles

- **Single responsibility** — the controller only dispatches; the agent investigates
- **Safe under restart** — in-memory state loss on restart is tolerated; downstream dedup handles it
- **No external state** — no ConfigMap, no Redis; complexity belongs in the agent
- **Fail loud** — errors are logged and returned so controller-runtime requeues; never swallowed

---

## 2. Package Structure

```
api/
└── v1alpha1/
    └── result_types.go          # k8sgpt-operator CRD types (vendored subset)

internal/
└── controller/
    ├── result_controller.go     # ResultReconciler, fingerprintFor
    └── result_controller_test.go

cmd/
└── watcher/
    └── main.go                  # scheme registration, manager setup, entrypoint
```

---

## 3. Data Models

### 3.1 Vendored CRD types (`api/v1alpha1/result_types.go`)

We vendor a minimal subset of the k8sgpt-operator API types — just enough to register the
scheme and read the fields we care about. We do not import the full operator module.

```go
type ResultSpec struct {
    Backend      string    `json:"backend"`
    Kind         string    `json:"kind"`
    Name         string    `json:"name"`
    Error        []Failure `json:"error"`
    Details      string    `json:"details"`
    ParentObject string    `json:"parentObject"`
}

type Failure struct {
    Text      string      `json:"text,omitempty"`
    Sensitive []Sensitive `json:"sensitive,omitempty"`
}
```

`AutoRemediationStatus` is intentionally omitted — the watcher does not use it and it should
not be in the minimal vendored subset.

Both `Result` and `ResultList` must implement `runtime.Object` via `DeepCopyObject()` and
`DeepCopyInto()` — hand-written (no code generation dependency).

### 3.2 In-memory dedup entry

```go
type processedEntry struct {
    fingerprint  string
    dispatchedAt time.Time
    jobName      string
}
```

Stored in `map[string]processedEntry` keyed by fingerprint string. Protected by `sync.Mutex`.

---

## 4. Fingerprint Algorithm

```go
func fingerprintFor(namespace string, spec v1alpha1.ResultSpec) string {
    texts := make([]string, 0, len(spec.Error))
    for _, f := range spec.Error {
        texts = append(texts, f.Text)
    }
    sort.Strings(texts)

    payload := struct {
        Namespace    string   `json:"namespace"`
        Kind         string   `json:"kind"`
        ParentObject string   `json:"parentObject"`
        ErrorTexts   []string `json:"errorTexts"`
    }{
        Namespace:    namespace,
        Kind:         spec.Kind,
        ParentObject: spec.ParentObject,
        ErrorTexts:   texts,
    }

    b, err := json.Marshal(payload)
    if err != nil {
        // Should not happen for this fixed struct, but handle explicitly.
        panic(fmt.Sprintf("fingerprintFor: json.Marshal failed: %v", err))
    }
    return fmt.Sprintf("%x", sha256.Sum256(b))
}
```

**Key properties:**
- Includes `namespace` — prevents cross-namespace collisions between same-named parents
- Uses `parentObject` not the resource name — collapses multiple pods from the same Deployment
- Error texts are sorted — ordering in the CRD is non-deterministic
- JSON marshalling with a fixed struct layout — deterministic across runs
- Returns the full 64-char hex SHA256
- `json.Marshal` error is handled explicitly (not discarded) with a panic; the fixed struct
  will never trigger it in practice, but the discard pattern is not used

---

## 5. Reconcile Loop

```
Reconcile(ctx, req):
  1. Fetch Result CRD
     if NotFound:
       - The Result was deleted. Evict any matching fingerprint from the processed map:
         iterate processed map, find entries whose result name/namespace match req.NamespacedName,
         delete them. This allows a re-created Result with the same fingerprint to be dispatched.
       - return nil
  2. fingerprintFor(result.Namespace, result.Spec) → fp
  3. Lock mutex
     check processed[fp] → if exists, unlock, return nil (no requeue — already settled)
     unlock
  4. Check MAX_CONCURRENT_JOBS limit:
     list Jobs in AgentNamespace with label app.kubernetes.io/managed-by=mendabot-watcher
     count Jobs where job.Status.CompletionTime == nil (not yet finished)
     if count >= cfg.MaxConcurrentJobs: requeue after 30s, return
  5. jobBuilder.Build(result, fp) → job
  6. client.Create(ctx, job)
     if AlreadyExists: log, fall through to step 7
     if other error: return error (controller-runtime requeues)
  7. Lock mutex
     processed[fp] = processedEntry{fp, time.Now(), job.Name}
     unlock
  8. Log dispatch, return nil
```

**Already-processed results:** Return `nil` with no requeue. A Result that has been
dispatched is in a settled state — there is nothing more for the controller to do until
the watcher restarts (clearing the map) or the Result changes (triggering a new reconcile
via the informer).

**Result deletion:** When a Result is deleted (NotFound), its fingerprint entry is evicted
from the processed map. The eviction uses the Result's `NamespacedName` stored in
`processedEntry.resultRef` (add this field to `processedEntry`). This allows a re-created
Result with the same fingerprint to trigger a new Job, preventing silent suppression.

**MAX_CONCURRENT_JOBS enforcement:** Before creating a Job, list all Jobs in
`AgentNamespace` with label `app.kubernetes.io/managed-by: mendabot-watcher` and count
those where `job.Status.CompletionTime == nil` (the Job has not yet finished). This is
simpler and more correct than counting Active/Pending states, which have race windows
between Job creation and pod scheduling. If count ≥ `MaxConcurrentJobs`, requeue after
30 seconds without creating a new Job.

---

## 6. Event Filtering

A `predicate.NewPredicateFuncs` filter is applied so the controller only processes events
where the object is a `Result` with at least one error:

```go
WithEventFilter(predicate.NewPredicateFuncs(func(obj client.Object) bool {
    result, ok := obj.(*v1alpha1.Result)
    if !ok {
        return true // pass through non-Result events (e.g. owned Jobs)
    }
    return len(result.Spec.Error) > 0
}))
```

Results with zero errors are never queued for reconciliation.

---

## 7. Manager Setup (`cmd/watcher/main.go`)

```go
// Scheme registration
scheme := runtime.NewScheme()
_ = clientgoscheme.AddToScheme(scheme)
_ = batchv1.AddToScheme(scheme)
_ = v1alpha1.AddToScheme(scheme)

// Manager
mgr := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
    Scheme:                 scheme,
    LeaderElection:         false, // single replica, no election needed
    Metrics: server.Options{BindAddress: ":8080"},
    HealthProbeBindAddress: ":8081",
})

// Reconciler
reconciler := &controller.ResultReconciler{
    Client:     mgr.GetClient(),
    Scheme:     mgr.GetScheme(),
    Log:        logger,
    JobBuilder: jobbuilder.New(cfg),
}
reconciler.SetupWithManager(mgr)

// Health checks
mgr.AddHealthzCheck("healthz", healthz.Ping)
mgr.AddReadyzCheck("readyz", healthz.Ping)

mgr.Start(ctrl.SetupSignalHandler())
```

**Configuration** is read from environment variables at startup into a typed `Config` struct:

```go
type Config struct {
    GitOpsRepo          string // GITOPS_REPO
    GitOpsManifestRoot  string // GITOPS_MANIFEST_ROOT
    AgentImage          string // AGENT_IMAGE
    AgentNamespace      string // AGENT_NAMESPACE — must equal watcher's own namespace
    AgentSA             string // AGENT_SA
    LogLevel            string // LOG_LEVEL
    MaxConcurrentJobs   int    // MAX_CONCURRENT_JOBS
}
```

---

## 8. Concurrency

The controller uses controller-runtime's default single-worker reconcile loop. No additional
goroutines are needed. The `sync.Mutex` on the processed map is defensive — it protects
against controller-runtime's internal goroutines even in single-worker mode.

---

## 9. Error Handling

| Error | Handling |
|---|---|
| Result not found | Return nil — resource was deleted, no action needed |
| Job AlreadyExists | Log at Info level, record as processed, return nil |
| Job creation fails (other) | Return wrapped error — controller-runtime requeues with exponential backoff |
| fingerprintFor panics | Can only happen if json.Marshal fails, which requires a type change to the fixed struct. Will surface as a controller panic — caught by controller-runtime and logged. |
| Config env var missing | Fatal at startup — fail fast before any reconciliation |
| MAX_CONCURRENT_JOBS reached | Requeue after 30s — not an error |

---

## 10. Logging

Uses `go.uber.org/zap` via controller-runtime's `zapr` adapter. Structured fields on every
log line:

```go
log.Info("dispatching agent job",
    zap.String("fingerprint", fp[:12]),
    zap.String("kind", result.Spec.Kind),
    zap.String("parentObject", result.Spec.ParentObject),
    zap.String("job", job.Name),
    zap.String("namespace", job.Namespace),
)
```

Log level controlled by `LOG_LEVEL` env var. Default is `info`.

---

## 11. Testing Strategy

All tests are in `internal/controller/result_controller_test.go`.

### Unit tests (no cluster required)

| Test | Description |
|---|---|
| `TestFingerprintFor_SameParentDifferentPods` | Same namespace + parent + errors → same fingerprint |
| `TestFingerprintFor_DifferentErrors` | Different error texts → different fingerprint |
| `TestFingerprintFor_ErrorOrderIndependent` | Reversed error slice → same fingerprint |
| `TestFingerprintFor_DifferentParents` | Same errors, different parent → different fingerprint |
| `TestFingerprintFor_DifferentNamespaces` | Same parent + errors, different namespace → different fingerprint |
| `TestFingerprintFor_EmptyErrors` | nil vs empty slice → same fingerprint |
| `TestFingerprintFor_DifferentKinds` | Same parent, different kind → different fingerprint |
| `TestFingerprintFor_Deterministic` | Same input twice → same output |

### Integration tests (uses envtest)

| Test | Description |
|---|---|
| `TestReconcile_NewResult_CreatesJob` | New Result with errors → Job created |
| `TestReconcile_AlreadyProcessed_SkipsJob` | Same fingerprint twice → second reconcile skips |
| `TestReconcile_NoErrors_Skipped` | Result with no errors → no Job created |
| `TestReconcile_JobAlreadyExists_RecordsAsProcessed` | Job exists → recorded, no error |
| `TestReconcile_ResultDeleted_NoOp` | Reconcile for deleted Result → nil return |
| `TestReconcile_DifferentParents_TwoJobs` | Two Results with different parents → two Jobs |
| `TestReconcile_ErrorTextChanges_NewJob` | Same parent, new error text → new fingerprint, new Job |
