You are implementing a feature or user story for the k8s-mechanic repository.

**Read README-LLM.md first** — it contains the critical rules (TDD, type
safety, zero tech debt, security-first, worklogs, multi-agent workflow).

Rules:
1. Read README-LLM.md in full before making any changes. Read the relevant
   story in `docs/BACKLOG/`, the HLD in `docs/DESIGN/HLD.md`, the relevant LLD
   in `docs/DESIGN/lld/`, and the last few worklogs (`ls docs/WORKLOGS/ |
   tail -5`) to understand current state and documented next steps.
2. Follow the established patterns — controller-runtime reconcilers,
   strongly-typed CRD types in `api/v1alpha1/`, providers in
   `internal/provider/`, the SourceProvider interface in
   `internal/domain/interfaces.go`. Check similar implementations before
   writing new code; do not invent novel patterns.
3. Test before code (TDD): write tests FIRST (happy + unhappy paths + edge
   cases), run them and confirm they fail, then implement. Use table-driven
   tests with `t.Run()` where appropriate. Always use `-timeout`.
4. For controller changes, follow the envtest rules:
   - Keep `testdata/crds/remediationjob_crd.yaml` in sync with any
     `RemediationJobSpec`/`RemediationJobStatus` field changes (the envtest
     API server silently strips unknown fields)
   - Pre-test cleanup: delete stale fixed-name objects at the start of the
     test body (ignore the error)
   - Job helper functions must use `Namespace: rjob.Namespace`, never a
     hardcoded namespace
5. Validate before pushing (zero tolerance):
   ```
   make build        # go build ./...
   make test         # go test -timeout 60s -race ./...
   make lint         # go vet ./...
   ```
   Fix pre-existing failures too.
6. If the feature touches CRD types, Helm chart values, or generated docs, run
   `make generate` / `make docs` and commit the regenerated artifacts.
7. If the feature touches `internal/redact/`, RBAC, CRD schema, NetworkPolicy,
   or secrets handling, read `docs/SECURITY/THREAT_MODEL.md` first and flag
   the change as security-sensitive in the PR description.
8. Security invariants must never be weakened: read-only cluster access for
   the agent, redaction before any LLM call, human-in-the-loop PR review.
9. Write a worklog entry in `docs/WORKLOGS/` for any substantive session
   (>30 min) and commit it with the change. Update the backlog story checklist
   in `docs/BACKLOG/`.
10. Leave the repo in a clean buildable state — no partial implementations,
    no TODO/FIXME placeholders.
