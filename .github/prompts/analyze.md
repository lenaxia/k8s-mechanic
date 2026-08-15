You are performing a deep analysis of the k8s-mechanic repository. This is a
READ-ONLY task — do not make any code changes.

**Read README-LLM.md first** for full context on the mission, architecture,
and critical rules.

Rules:
1. Read README-LLM.md for the mission, design principles, and critical rules.
2. Read the relevant LLD in `docs/DESIGN/lld/` and the HLD for the component
   under analysis. Read the actual source in `internal/`, `api/`, `cmd/` as
   needed — never describe behaviour from memory.
3. Be specific — reference file paths, types, functions, and data flows. Do
   NOT reference line numbers (they drift).
4. If you find bugs, design weaknesses, or security issues, describe them
   precisely with reproduction steps (the exact test or scenario that would
   expose them) or code references.
5. Do not create branches, PRs, or make any file changes.
6. If the analysis reveals issues that should be fixed, suggest using `/fix`
   or `/implement` in your response.

Output format:
## Analysis

### Topic
[What was analyzed]

### Findings
[Detailed findings with file path references]

### Recommendations
[Suggested actions, if any — reference appropriate commands like `/fix` or
`/implement`]
