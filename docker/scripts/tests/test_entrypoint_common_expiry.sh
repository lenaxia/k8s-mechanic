#!/usr/bin/env bats
# Tests for the pre-flight token expiry block in entrypoint-common.sh.
#
# Run with:  bats docker/scripts/tests/test_entrypoint_common_expiry.sh
#
# Strategy: rather than executing the full entrypoint-common.sh (which requires
# kubectl, gh, kubeconfig, prompt files, and env vars), extract only the pre-flight
# block and run it as a standalone bash snippet in a temp file.  Each test sets up
# a controlled EXPIRY_FILE and a fixed NOW value via a wrapper that overrides
# `date +%s` output.

# ---------------------------------------------------------------------------
# setup / teardown
# ---------------------------------------------------------------------------
setup() {
    TMPDIR_TEST="$(mktemp -d)"
    EXPIRY_FILE="${TMPDIR_TEST}/github-token-expiry"

    # Snippet under test: the pre-flight block verbatim from entrypoint-common.sh.
    # We inject a FAKE_NOW variable and override `date` so the snippet uses it.
    SNIPPET="${TMPDIR_TEST}/snippet.sh"
    cat > "$SNIPPET" <<'SNIPPET_EOF'
#!/usr/bin/env bash
set -euo pipefail

# `date` override: if FAKE_NOW is set use it, otherwise call real date.
date() {
    if [ "${FAKE_NOW:-}" != "" ] && [ "$*" = "+%s" ]; then
        printf '%s\n' "$FAKE_NOW"
    else
        command date "$@"
    fi
}

EXPIRY_FILE=/workspace/github-token-expiry
if [ -f "$EXPIRY_FILE" ]; then
    EXPIRY=$(cat "$EXPIRY_FILE")
    NOW=$(date +%s)
    if [ "$NOW" -ge "$((EXPIRY - 60))" ]; then
        echo "ERROR: GitHub App token is expired or expiring imminently." >&2
        echo "  EXPIRY=${EXPIRY}  NOW=${NOW}  (threshold: EXPIRY-60=$((EXPIRY - 60)))" >&2
        echo "  Re-queue the RemediationJob to obtain a fresh token." >&2
        exit 1
    fi
else
    echo "WARNING: /workspace/github-token-expiry not found — skipping expiry pre-flight check." >&2
fi
SNIPPET_EOF
    chmod +x "$SNIPPET"
}

teardown() {
    rm -rf "$TMPDIR_TEST"
}

# ---------------------------------------------------------------------------
# Test 1: expiry file absent — warning printed to stderr, exit 0 (continues)
# ---------------------------------------------------------------------------
@test "expiry file absent: warning to stderr, exits 0" {
    # EXPIRY_FILE does not exist — pass a path that is guaranteed absent.
    local absent="${TMPDIR_TEST}/nonexistent-expiry-file"
    local now
    now=$(date +%s)

    run bash -c "FAKE_NOW=${now} bash <(sed \"s|/workspace/github-token-expiry|${absent}|g\" \"${SNIPPET}\") 2>&1"

    [ "$status" -eq 0 ]
    # The warning contains the substituted path and the invariant text "not found".
    [[ "$output" == *"WARNING:"* ]]
    [[ "$output" == *"not found"* ]]
    [[ "$output" != *"ERROR"* ]]
}

# ---------------------------------------------------------------------------
# Test 2: token valid (EXPIRY = NOW + 3600) — no error, exit 0
# ---------------------------------------------------------------------------
@test "token valid (EXPIRY=NOW+3600): no error, exits 0" {
    local now
    now=$(date +%s)
    local expiry=$(( now + 3600 ))
    printf '%d' "$expiry" > "$EXPIRY_FILE"

    run bash -c "FAKE_NOW=${now} bash <(sed \"s|/workspace/github-token-expiry|${EXPIRY_FILE}|g\" \"${SNIPPET}\") 2>&1"

    [ "$status" -eq 0 ]
    [[ "$output" != *"ERROR"* ]]
    [[ "$output" != *"WARNING"* ]]
}

# ---------------------------------------------------------------------------
# Test 3: token expired (EXPIRY = NOW - 100) — error to stderr, exit 1
# ---------------------------------------------------------------------------
@test "token expired (EXPIRY=NOW-100): error to stderr, exits 1" {
    local now
    now=$(date +%s)
    local expiry=$(( now - 100 ))
    printf '%d' "$expiry" > "$EXPIRY_FILE"

    run bash -c "FAKE_NOW=${now} bash <(sed \"s|/workspace/github-token-expiry|${EXPIRY_FILE}|g\" \"${SNIPPET}\") 2>&1"

    [ "$status" -eq 1 ]
    [[ "$output" == *"ERROR: GitHub App token is expired or expiring imminently."* ]]
    [[ "$output" == *"EXPIRY="* ]]
    [[ "$output" == *"NOW="* ]]
    [[ "$output" == *"Re-queue the RemediationJob"* ]]
}

# ---------------------------------------------------------------------------
# Test 4: token within 60s threshold (EXPIRY = NOW + 30) — error, exit 1
# ---------------------------------------------------------------------------
@test "token within 60s threshold (EXPIRY=NOW+30): error to stderr, exits 1" {
    local now
    now=$(date +%s)
    local expiry=$(( now + 30 ))
    printf '%d' "$expiry" > "$EXPIRY_FILE"

    run bash -c "FAKE_NOW=${now} bash <(sed \"s|/workspace/github-token-expiry|${EXPIRY_FILE}|g\" \"${SNIPPET}\") 2>&1"

    [ "$status" -eq 1 ]
    [[ "$output" == *"ERROR: GitHub App token is expired or expiring imminently."* ]]
}

# ---------------------------------------------------------------------------
# Test 5: token exactly at threshold (EXPIRY = NOW + 60) — exit 1
# When EXPIRY = NOW + 60: EXPIRY - 60 = NOW, so NOW >= EXPIRY - 60 is true.
# ---------------------------------------------------------------------------
@test "token exactly at threshold (EXPIRY=NOW+60): exits 1" {
    local now
    now=$(date +%s)
    local expiry=$(( now + 60 ))
    printf '%d' "$expiry" > "$EXPIRY_FILE"

    run bash -c "FAKE_NOW=${now} bash <(sed \"s|/workspace/github-token-expiry|${EXPIRY_FILE}|g\" \"${SNIPPET}\") 2>&1"

    [ "$status" -eq 1 ]
    [[ "$output" == *"ERROR: GitHub App token is expired or expiring imminently."* ]]
}
