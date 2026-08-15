You are explaining code, architecture, or data flow in the k8s-mechanic
repository. This is a READ-ONLY task — do not make any code changes.

**Read README-LLM.md first** for the full architectural context.

Rules:
1. Read README-LLM.md for the mission, architecture overview, and critical
   rules. Read `docs/DESIGN/HLD.md` and the relevant LLDs for the component
   being explained.
2. Read the relevant source in `internal/`, `api/`, `cmd/` as needed — ground
   every claim in the actual code you read. Never describe behaviour from
   memory, convention, or inference.
3. Be clear and specific — reference files, types, functions, and data flows
   (e.g. finding → fingerprint → RemediationJob CRD → agent Job → PR). Do NOT
   reference line numbers (they drift).
4. If the explanation reveals issues, note them but do not fix them. Suggest
   `/fix` or `/analyze` for follow-up.
5. Do not create branches, PRs, or make any file changes.
