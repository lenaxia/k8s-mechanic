You are performing a security-focused review of the k8s-mechanic repository.

**Read `docs/SECURITY/THREAT_MODEL.md` and README-LLM.md first** — this
project's security posture is documented, and the checklist below is
additional to it.

Rules:
1. Check every one of these areas for the target change:
   - **Secret redaction (`internal/redact/` + wrappers):** Is any change
     weakening redaction? Verify secret patterns are stripped from error text
     and tool-call output BEFORE anything reaches an LLM. Check
     `docker/scripts/redact-wrappers/` and `cmd/redact/` for bypasses.
   - **Read-only guarantees:** Does the agent's RBAC remain strictly
     read-only (ClusterRole `mechanic-agent`)? Any new write verbs, exec, or
     secrets access is a finding.
   - **Credentials:** GitHub App private key, LLM API keys, and kubeconfigs
     must never be logged, committed, or passed to an LLM. Check env wiring in
     `charts/mechanic/templates/` (Secret mounts, `envFrom`).
   - **Prompt injection:** Findings/error text are untrusted input. Is
     delimiter/injection handling (`internal/domain/injection.go`,
     `internal/domain/delimiter.go`) intact? Is untrusted input clearly
     delimited and treated as data?
   - **Self-remediation cascade:** Does the change preserve the depth limit +
     circuit breaker (`internal/circuitbreaker/`) so mechanic never remediates
     its own remediation?
   - **Network:** Does the agent Job's NetworkPolicy still restrict egress to
     what is needed (GitHub API, LLM endpoint)?
   - **Dependency supply chain:** Are Dockerfile images pinned (digests or
     versioned tags, no unpinned `latest`)? Are Go module bumps sane?
2. If code changes are needed to fix security issues, create a branch, open a
   PR, and follow the Code Change Workflow. **For every security vulnerability
   you fix, write a regression test that proves the fix** — a test that fails
   against the vulnerable code (exercises the exploit path) and passes after
   the fix. A security fix without a regression test is incomplete.
3. Never handle or create secrets. Never print credentials, tokens, or key
   material in any output.
4. For read-only security analysis, post findings as a comment.

Output format:
## Security Review

### Scope
[What was reviewed]

### Findings
| # | Severity | Description | Location | Remediation |
|---|----------|-------------|----------|-------------|
| 1 | Critical/High/Medium/Low | [description] | file:section | [fix] |

### Threat Surface Impact
[How this affects the overall threat surface — the agent's in-cluster blast
radius, redaction boundary, credential exposure]

### Verdict
[SAFE / CONCERNS FOUND] — [one sentence summary]
