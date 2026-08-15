You are writing or improving tests for the k8s-mechanic repository.

**Read README-LLM.md first** — TDD and test coverage requirements are
mandatory.

Rules:
1. Follow the project's testing requirements exactly (README-LLM.md "Testing
   Requirements"):
   - Multiple happy-path cases, multiple unhappy-path cases, edge cases
     (empty fields, nil slices, very long strings), error conditions
   - Table-driven tests with `t.Run()` for functions with multiple input cases
   - Always use `-timeout` when running tests
2. Cover the meaningful surface of the changed code, not trivial checks:
   - Fingerprinting and dedup logic (parent + error set → fingerprint)
   - Provider classification and severity mapping
   - Correlator rules and grouping
   - JobBuilder output (image, env, RBAC-relevant fields, namespaces,
     `restartPolicy: Never`, `activeDeadlineSeconds`)
   - Redaction (see `cmd/redact/` and `internal/redact/` tests) — verify no
     secret patterns survive
   - Error paths — every `(value, error)` return
3. envtest integration tests (`internal/controller/`) — three rules:
   - New `RemediationJobSpec`/`RemediationJobStatus` fields MUST be mirrored in
     `testdata/crds/remediationjob_crd.yaml`
   - Add a pre-test delete (ignoring the error) for fixed-name objects before
     creating them
   - Job helper functions must use `Namespace: rjob.Namespace`, never a
     hardcoded namespace
4. Check existing tests in the target package for patterns and conventions
   before writing new ones. Do not invent novel assertion styles.
5. Validate before pushing:
   ```
   make test    # go test -timeout 60s -race ./...
   make build   # go build ./...
   ```
6. Never hack tests to pass — if a test fails, fix the root cause (the code),
   not the test.
