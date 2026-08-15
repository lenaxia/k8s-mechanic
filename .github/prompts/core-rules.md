## Core Rules

These rules apply to every response. They are non-negotiable. They are
summarized here for the AI workflow; the authoritative source is README-LLM.md
(read it in full before making changes).

### 1. Test Before You Ship (TDD — MANDATORY)

Write tests BEFORE writing functional code. Always.

```
1. Write test
2. Run test (must fail)
3. Write minimal code to pass
4. Run test (must pass)
5. Refactor if needed
```

Coverage requirements: multiple happy-path cases, multiple unhappy-path cases,
edge cases (empty fields, nil slices, very long strings), error conditions.
Use table-driven tests (`t.Run`) for functions with multiple input cases.
Always use `-timeout` when running tests.

### 2. Type Safety First

- Define strongly-typed structs for all data structures; create domain types
  for related fields (see `internal/domain/`)
- NEVER use `map[string]interface{}` for structured data, or `interface{}`
  when the type is known
- Maps are acceptable only when parsing external JSON/YAML with unknown
  structure — and even then, convert to a typed struct immediately

### 3. Idiomatic Go

- Follow Go conventions throughout; `(value, error)` return pattern
- Avoid global state; create custom error types for domain-specific errors
- Prefer minimal concurrency; add it only when there is clear, measurable
  benefit

### 4. Explicit Over Implicit

- Explicit error handling — no swallowed errors
- Explicit type declarations; no magic or hidden behaviour

### 5. Code Quality & Zero Technical Debt

- No comments unless strictly necessary and timeless; incorrect or outdated
  comments must be removed or corrected
- No TODOs, FIXMEs, or commented-out code
- No adapters for backwards compatibility — implement the final solution
- Never hack tests to pass — fix the root cause
- Pre-existing build failures are not acceptable — fix them when encountered

### 6. Uncertainty Protocol

If uncertain about correct behaviour: **ask the user**. Do not guess, assume,
or implement workarounds. If you cannot validate an assumption, do not rely on
it.

### 7. Understand the Architecture First

Before making any change, read `docs/DESIGN/HLD.md` and the relevant LLD in
`docs/DESIGN/lld/`. Understand how the change fits the overall data flow
(watcher → RemediationJob → agent Job → PR). Never modify code without knowing
why.

### 8. Security Is First-Class

- NEVER handle or create secrets. Never print, log, or pass credentials to an
  LLM
- This project redacts secret patterns before anything reaches a model
  (`internal/redact/` + wrappers in `docker/scripts/redact-wrappers/`) — do
  not weaken that boundary
- The agent runs with strictly read-only cluster access; do not introduce
  write paths
- Any change touching `internal/redact/`, RBAC (ClusterRole/ServiceAccount),
  CRD schema, NetworkPolicy, or secrets handling is security-sensitive —
  flag it for security review and read `docs/SECURITY/THREAT_MODEL.md` first

### 9. Worklogs Are Mandatory

Any meaningful session (>30 min), user story, bug discovery, architectural
decision, or blocker requires a worklog entry:
`docs/WORKLOGS/NNNN_YYYY-MM-DD_short-description.md` (zero-padded sequential
number; check the highest existing number first). Follow the exact format in
README-LLM.md. Worklogs are append-only — never rewrite history.

### 10. Verification Protocol

- Never state something exists without showing the file path
- Never state something doesn't exist without showing the search command and
  empty output
- Red flag words — stop and verify: "probably", "likely", "should be",
  "I believe", "I assume", "appears to", "seems like", "in theory"
- Read the actual code before describing it — never describe behaviour from
  memory, convention, or inference

### 11. No Destructive Git Operations

Never run `git checkout .`, `git reset --hard`, or `git clean -fd`. Multiple
agents may work simultaneously in this repository. Never commit directly to
`main` — always work on a branch and open a PR.

### 12. Conventional Commits

Commit messages follow the conventional format: `feat:`, `fix:`, `chore:`,
`docs:`, `test:`, `security:`, `refactor:` (lowercase prefix, imperative
mood).

### 13. Neutral, Factual Communication

Be a critical collaborator. No sycophancy, no sensationalism. State findings
directly with evidence. Validate claims before stating them.
