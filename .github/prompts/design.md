You are iterating on a **design document** for the k8s-mechanic repository —
the step that comes *before* `/implement` or `/fix`. The goal is a reviewed,
approved design, not code.

Output target: a markdown design in the PR description, or a new design
document under `docs/DESIGN/lld/` (for component designs), or an update to an
existing design document. Follow the repository's design-doc conventions —
read an existing LLD first for the format.

Rules:
1. Read README-LLM.md first — especially the mission, architecture overview,
   and critical rules. Read `docs/DESIGN/HLD.md`, the relevant LLDs, and any
   related backlog stories in `docs/BACKLOG/` before writing.
2. Decide where the design lives:
   - Small / single-component scope → the PR description itself (markdown).
   - Larger / cross-cutting → a new LLD in `docs/DESIGN/lld/` following the
     existing LLD format.
   - Updating an existing design → edit it in place; do not silently
     duplicate.
3. Scope the design to the request text from the collaborator. If the request
   is ambiguous, state the ambiguity explicitly and pick the narrowest
   reasonable scope.
4. A design doc must cover at minimum: problem statement, goals/non-goals,
   proposed design (types, interfaces, data flow through watcher →
   RemediationJob → agent Job → PR), alternatives considered, and open
   questions. Trace every claim to source (file:type:function) where the
   codebase is referenced — do not describe behavior from memory.
5. State assumptions up front and validate each one against the actual code
   before relying on it.
6. Security invariants are non-negotiable in any design: read-only agent
   RBAC, redaction before LLM, human-in-the-loop, self-remediation cascade
   prevention. A design that weakens any of these must justify it explicitly
   or be rejected.
7. Workflow — follow the Code Change Workflow: feature branch (`design/` or
   `docs/` prefix), open a PR, iterate through the automated review until it
   posts APPROVE.
8. **MERGE HOLD — this command never auto-merges.** After the automated review
   posts APPROVE, STOP. Do not merge. Post a comment on the PR summarising the
   design and stating it is approved and awaiting an explicit `/merge`.
9. Do not write production code in this step — only the design document.
