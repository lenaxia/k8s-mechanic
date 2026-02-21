#!/usr/bin/env bash
set -euo pipefail

# Authenticate gh CLI using the token written by the init container
gh auth login --with-token < /workspace/github-token

# Substitute environment variables into the prompt template.
# envsubst only replaces ${VAR} patterns it knows about. To avoid corrupting
# content in FINDING_ERRORS or FINDING_DETAILS that may contain literal $ signs
# (e.g. from Helm templates or shell variables in log output), we restrict
# envsubst to only the known variable names.
VARS='${FINDING_KIND}${FINDING_NAME}${FINDING_NAMESPACE}${FINDING_PARENT}${FINDING_FINGERPRINT}${FINDING_ERRORS}${FINDING_DETAILS}${GITOPS_REPO}${GITOPS_MANIFEST_ROOT}'
envsubst "$VARS" < /prompt/prompt.txt > /tmp/rendered-prompt.txt

# Run opencode with the rendered prompt passed via a temp file to avoid
# shell word-splitting on the prompt content.
exec opencode run --file /tmp/rendered-prompt.txt
