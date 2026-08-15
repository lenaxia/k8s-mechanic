You are fixing a bug in the k8s-mechanic repository.

**Read README-LLM.md first** — it contains the critical rules (TDD, type
safety, zero tech debt, security-first, worklogs).

Rules:
1. Read README-LLM.md, the last few worklogs (`ls docs/WORKLOGS/ | tail -5`),
   and the relevant LLD in `docs/DESIGN/lld/` before making any changes.
2. Identify the root cause — do not fix symptoms. Trace the actual code path
   (watcher → provider → correlator → jobbuilder → sink) to confirm where the
   defect lives. Read the real code; never infer behaviour from memory.
3. Test before code (TDD): write a Go test that reproduces the failure, run it
   and confirm it fails, then implement the fix and confirm the test passes.
   Cover happy path, unhappy path, and edge cases.
4. **Regression tests are mandatory, not optional.** For every bug you fix,
   write a test that would FAIL against the old (broken) code and PASS against
   the fixed code. Name it after the bug. Commit it in the same PR, never as a
   follow-up. A `/fix` PR that ships a code change without a regression test
   for the bug it addresses is incomplete and must not be merged. If you
   discover additional bugs while fixing the reported one, write a test for
   each before fixing it.
5. Validate before pushing (zero tolerance):
   ```
   make build        # go build ./...
   make test         # go test -timeout 60s -race ./...
   make lint         # go vet ./...
   ```
   Fix pre-existing failures too — do not ship around them.
6. If the fix touches CRD types, Helm chart values, or generated docs, run
   `make generate` / `make docs` and commit the regenerated artifacts (CI's
   `docs-check` enforces this).
7. If the fix touches `internal/redact/`, RBAC, CRD schema, or secrets
   handling, read `docs/SECURITY/THREAT_MODEL.md` first and flag the change as
   security-sensitive in the PR description.
8. For a substantive session (>30 min), write a worklog entry in
   `docs/WORKLOGS/` following the README-LLM.md format.
