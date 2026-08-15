## AI Assistant Commands

The following commands are available on this issue/PR thread. Reply with one to
trigger the assistant — any text after a command tunes the request (e.g.
`/review focus on the redaction wrappers`).

| Command | Description |
|---|---|
| `/ai [text]` | Re-assess this issue/PR in full, or address a specific request (context-dependent). |
| `/review [text]` | Explicit review of the current PR. Append text to focus on specific areas (controller logic, redaction, RBAC, tests, security). |
| `/fix <description>` | Fix a bug: creates a branch, writes a failing regression test, fixes the Go code, opens a PR, iterates through review until approved, then merges. |
| `/implement <description>` | Implement a feature/story: reads the HLD/LLDs + backlog story, writes tests first (TDD), opens a PR, iterates through review until approved, then merges. |
| `/test <target>` | Write or improve tests for the specified package/component. Opens a PR, iterates through review until approved. |
| `/analyze [text]` | Deep read-only analysis of the codebase, design, or a component. Posts findings as a comment. No code changes. |
| `/explain <topic>` | Explain code, architecture, or data flow. Posts explanation as a comment. No code changes. |
| `/security [text]` | Security-focused review (redaction integrity, read-only RBAC, credential handling, prompt-injection guards, cascade prevention). |
| `/triage [text]` | Triage this issue — categorize, prioritize, assess impact, suggest labels. |
| `/design [text]` | Iterate on a design doc under `docs/DESIGN/` before implementing/fixing. Opens a PR, iterates through review, then **holds** (never auto-merges). |
| `/merge` | Explicitly merge an approved PR (squash). Use after `/design` or a `--no-merge` run. |
| `/help` | Show the full command reference. |

**All commands are available to repository owners, members, and collaborators.**
Code-change commands (`/fix`, `/implement`, `/test`, `/security`) auto-merge
after approval by default — append `--no-merge` to hold for an explicit
`/merge`. `/design` always holds. None of these ever commit to `main` directly.
