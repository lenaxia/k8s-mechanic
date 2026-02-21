#!/usr/bin/env bash
set -euo pipefail

IMAGE=${1:?Usage: smoke-test.sh <image-tag>}

check() {
    echo "Checking: $*"
    docker run --rm --entrypoint "" "$IMAGE" "$@"
}

echo "Checking: agent-entrypoint.sh executable"
docker run --rm --entrypoint /bin/sh "$IMAGE" -c "test -x /usr/local/bin/agent-entrypoint.sh"
echo "Checking: get-github-app-token.sh executable"
docker run --rm --entrypoint /bin/sh "$IMAGE" -c "test -x /usr/local/bin/get-github-app-token.sh"
