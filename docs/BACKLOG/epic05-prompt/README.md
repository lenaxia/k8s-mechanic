# Epic: Prompt

## Purpose

Write the OpenCode agent prompt and wire it into the deployment as a ConfigMap. The prompt
is the instruction set given to OpenCode inside every agent Job — it defines the entire
investigation and PR-creation workflow.

## Status: Not Started

## Dependencies

- Deploy epic complete (ConfigMap resource exists in kustomization.yaml)

## Blocks

Nothing — this is the last functional epic before CI/CD.

## Success Criteria

- [ ] Prompt is stored in `deploy/kustomize/configmap-prompt.yaml`
- [ ] Prompt covers all 9 investigation steps from PROMPT_LLD.md
- [ ] PR deduplication check is the mandatory first step
- [ ] Hard rules section is present and complete
- [ ] Decision tree is present
- [ ] All `${VAR}` placeholders match the environment variables injected by the Job Builder
- [ ] `envsubst` substitution tested locally with a sample set of env vars
- [ ] Agent entrypoint script (`agent-entrypoint.sh`) present in the Docker image and wired
  into the container command

## Stories

| Story | File | Status |
|-------|------|--------|
| Prompt ConfigMap content | [STORY_01_prompt_content.md](STORY_01_prompt_content.md) | Not Started |
| Agent entrypoint script | [STORY_02_entrypoint_script.md](STORY_02_entrypoint_script.md) | Not Started |
| Placeholder substitution test | [STORY_03_substitution_test.md](STORY_03_substitution_test.md) | Not Started |

## Technical Overview

The prompt design is fully specified in
[`docs/DESIGN/lld/PROMPT_LLD.md`](../../DESIGN/lld/PROMPT_LLD.md). Read it in full before
starting any story in this epic.

The prompt is the most operationally sensitive component. Poor wording leads to:
- Duplicate PRs (if the dedup step is ambiguous)
- Overly aggressive changes (if confidence thresholds are not clear)
- Timeouts (if investigation steps are too broad)
- Silent failures (if hard rules are not enforced)

When tuning the prompt after deployment, always update PROMPT_LLD.md and write a worklog
entry explaining what changed and why.

## Definition of Done

- [ ] ConfigMap applies cleanly
- [ ] envsubst substitution verified manually
- [ ] Entrypoint script baked into agent image
- [ ] Prompt reviewed against all hard rules in PROMPT_LLD.md
