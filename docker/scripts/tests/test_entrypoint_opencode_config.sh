#!/usr/bin/env bats
# Tests for the AGENT_PROVIDER_CONFIG → file handling in entrypoint-opencode.sh.
#
# Run with:  bats docker/scripts/tests/test_entrypoint_opencode_config.sh
#
# Strategy: extract only the config-writing block from entrypoint-opencode.sh
# and run it in a controlled environment. Verify that:
#   1. The config JSON is written to /tmp/opencode-config.json
#   2. OPENCODE_CONFIG is set to that path
#   3. AGENT_PROVIDER_CONFIG is unset (absent from env of subsequent processes)

# ---------------------------------------------------------------------------
# setup / teardown
# ---------------------------------------------------------------------------
setup() {
    TMPDIR_TEST="$(mktemp -d)"

    # Simulate /tmp as the emptyDir writable volume.
    FAKE_TMP="${TMPDIR_TEST}/tmp"
    mkdir -p "$FAKE_TMP"

    # Snippet under test: the config-writing block from entrypoint-opencode.sh,
    # with /tmp replaced by our fake tmp dir.
    SNIPPET="${TMPDIR_TEST}/snippet.sh"
    cat > "$SNIPPET" <<SNIPPET_EOF
#!/usr/bin/env bash
set -euo pipefail
FAKE_TMP="${FAKE_TMP}"
printf '%s' "\$AGENT_PROVIDER_CONFIG" > "\${FAKE_TMP}/opencode-config.json"
export OPENCODE_CONFIG="\${FAKE_TMP}/opencode-config.json"
unset AGENT_PROVIDER_CONFIG
# Emit the resulting env for test inspection.
echo "OPENCODE_CONFIG=\${OPENCODE_CONFIG}"
env | grep '^AGENT_PROVIDER_CONFIG=' && echo "STILL_SET=1" || echo "STILL_SET=0"
SNIPPET_EOF
    chmod +x "$SNIPPET"
}

teardown() {
    rm -rf "$TMPDIR_TEST"
}

# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------

run_snippet() {
    local provider_config="$1"
    FAKE_TMP="$FAKE_TMP" \
    AGENT_PROVIDER_CONFIG="$provider_config" \
    bash "$SNIPPET"
}

# ---------------------------------------------------------------------------
# tests
# ---------------------------------------------------------------------------

@test "config JSON is written to the tmp file" {
    local config='{"provider":{"test":{"options":{"apiKey":"sk-test123"}}}}'
    run run_snippet "$config"
    [ "$status" -eq 0 ]
    written=$(cat "${FAKE_TMP}/opencode-config.json")
    [ "$written" = "$config" ]
}

@test "OPENCODE_CONFIG points to the written file" {
    local config='{"model":"test/model"}'
    run run_snippet "$config"
    [ "$status" -eq 0 ]
    [[ "$output" == *"OPENCODE_CONFIG=${FAKE_TMP}/opencode-config.json"* ]]
}

@test "AGENT_PROVIDER_CONFIG is unset after the block runs" {
    local config='{"provider":{"test":{"options":{"apiKey":"sk-secret-key"}}}}'
    run run_snippet "$config"
    [ "$status" -eq 0 ]
    [[ "$output" == *"STILL_SET=0"* ]]
}

@test "API key does not appear in env output after block runs" {
    local api_key="sk-supersecret-llm-api-key-99999"
    local config="{\"provider\":{\"test\":{\"options\":{\"apiKey\":\"${api_key}\"}}}}"
    run run_snippet "$config"
    [ "$status" -eq 0 ]
    # The API key must not appear in the env output line
    [[ "$output" != *"$api_key"* ]]
}

@test "written file contains the full config including API key" {
    local api_key="sk-supersecret-llm-api-key-99999"
    local config="{\"provider\":{\"test\":{\"options\":{\"apiKey\":\"${api_key}\"}}}}"
    run run_snippet "$config"
    [ "$status" -eq 0 ]
    written=$(cat "${FAKE_TMP}/opencode-config.json")
    [[ "$written" == *"$api_key"* ]]
}

@test "empty AGENT_PROVIDER_CONFIG writes empty file without error" {
    run run_snippet ""
    [ "$status" -eq 0 ]
    written=$(cat "${FAKE_TMP}/opencode-config.json")
    [ "$written" = "" ]
}
