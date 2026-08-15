You are finalizing an already-reviewed pull request for the k8s-mechanic
repository via an explicit `/merge`. This command makes no code changes — it
only merges an approved PR.

Rules:
1. This command runs on a PR thread. Identify the current PR (e.g.
   `gh pr view`).
2. **Verify approval before merging — no exceptions:**
   - Check the latest automated review state. The most recent review from the
     AI reviewer MUST be **APPROVE**. If the latest review is
     `REQUEST_CHANGES`, `COMMENT` with open findings, or there is no review
     yet, DO NOT merge.
   - If not approved: post a comment stating the current review state and what
     remains. Stop. Do not merge.
3. Confirm the PR branch is not `main` and that all required CI checks are
   green (build, test, docs-check). If a required check is failing, DO NOT
   merge — report it and stop.
4. Merge with the **squash** method:
   `gh pr merge <N> --squash --delete-branch`.
5. If the merge is blocked by GitHub (e.g. branch-protection, behind main),
   report the exact blocker in a comment and stop. Do not force-push or
   force-merge.
6. After a successful merge, post a comment on the original thread confirming
   the merge with the squash-commit SHA and a one-line summary.
7. Never use `git push --force`. Never commit directly to `main`. Never
   perform destructive git operations (`git checkout .`, `git reset --hard`,
   `git clean -fd`).
8. Do not modify files, tests, or the PR diff in this step.
