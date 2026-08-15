## Code Change Workflow (MANDATORY)

Every code change MUST follow this review-iterate-approve cycle without
exception:

1. **Read the docs first** — `README-LLM.md` in full (all critical rules
   apply), the last 3–5 worklogs (`ls docs/WORKLOGS/ | tail -5`), and the
   relevant LLD in `docs/DESIGN/lld/` for the component you are touching.
2. **Branch:** Create a feature branch (`feat/`, `fix/`, `test/`,
   `security/`, `docs/`, or `design/` prefix). Never commit to main.
3. **Test before code (TDD):** Write the failing test first. Run it — it must
   fail. Then implement the minimal code to pass. Run it — it must pass.
   Happy paths, unhappy paths, and edge cases.
4. **Validate before pushing (zero tolerance):**
   ```
   make build        # go build ./... — ALL packages must build
   make test         # go test -timeout 60s -race ./... — ALL tests must pass
   make lint         # go vet ./...
   ```
   Fix pre-existing failures too — do not ship around them.
5. **Generated artifacts:** If you changed CRD types
   (`api/v1alpha1/remediationjob_types.go`), run `make generate` to
   regenerate `charts/mechanic/crds/` and `testdata/crds/`. If you changed
   `charts/mechanic/values.yaml`, `Chart.yaml`, or the CRDs, run
   `make docs` (helm-docs + crd-ref-docs) and commit the regenerated docs.
   The CI `docs-check` job fails if generated files are out of date.
6. **envtest rules** (integration tests in `internal/controller/`):
   - New `RemediationJobSpec`/`RemediationJobStatus` fields MUST also be added
     to `testdata/crds/remediationjob_crd.yaml` (the envtest schema copy) —
     the real API server strips unknown fields, so a missing entry passes unit
     tests but fails integration tests
   - Tests creating objects with fixed names add a pre-test delete (ignore the
     error) before creating, so stale objects from previous runs cannot
     pollute the run
   - Job helper functions must use `Namespace: rjob.Namespace`, never a
     hardcoded namespace
7. **PR:** Open a pull request with a clear description (what, why, how
   tested). Reference the triggering issue or comment ("Closes #N").
8. **Wait for review:** The automated PR review triggers on every PR open and
   push. Wait for it to complete before proceeding.
9. **Address feedback:** Read every finding. Fix ALL real issues. Push to the
   same branch — this triggers automatic re-review.
10. **Iterate:** Repeat steps 8–9 until the automated reviewer posts APPROVE.
11. **Merge:** After approval only — merge with squash method, **unless this
    run was invoked with `--no-merge`** (see Hold below) or it is a `/design`
    run (which always holds).
12. **Worklog:** For any substantive session (>30 min), write a worklog entry
    under `docs/WORKLOGS/` following the README-LLM.md format and commit it
    with the change.
13. **Report:** Post a comment on the original issue/PR confirming completion
    with a summary of changes.

**Merge control (`--no-merge` and `/merge`):**
- By default `/fix`, `/implement`, `/test`, and `/security` auto-merge after
  approval (step 11).
- Append `--no-merge` to any of those commands to hold the merge: the run
  iterates to approval but does NOT merge — it stops and waits for an explicit
  `/merge`.
- `/design` **always** holds — design docs never auto-merge.
- `/merge` is the explicit finalize command: it verifies the latest review is
  APPROVE and required CI is green, then squash-merges and deletes the branch.

**Hard rules:**
- NEVER merge before the automated review approves — no exceptions
- NEVER dismiss review findings — fix them or document with evidence why they
  are false alarms
- NEVER commit directly to main
- `make build` must exit 0 and `make test` must show zero failures — fix
  pre-existing failures too
- NEVER weaken redaction, RBAC, or read-only guarantees for the agent
- If the review cycle exceeds 3 iterations, step back and reassess the
  approach — something is wrong
