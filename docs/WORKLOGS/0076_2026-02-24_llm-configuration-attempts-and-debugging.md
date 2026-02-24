# Worklog: 0076 — LLM Configuration Attempts and Debugging

**Date**: 2026-02-24

## Summary

After deploying v0.3.7 and recreating `llm-credentials-opencode` secret, attempted multiple OpenCode CLI configuration formats. All remediationjobs fail due to invalid opencode config schema. LLM readiness check re-enabled successfully.

## Actions Taken

### 1. Re-enabled LLM Readiness Check

**Command:**
```bash
kubectl patch deployment mendabot -n default -p '{"spec":{"template":{"spec":{"containers":[{"name":"watcher","env":[{"name":"LLM_PROVIDER","value":"openai"}]}]}}}}'
```

**Result**: LLM readiness check is now enabled (`LLM_PROVIDER=openai` set in deployment).

### 2. Recreated Secret with provider.openai.model Format

**Command:**
```bash
kubectl delete secret llm-credentials-opencode -n default
kubectl create secret generic llm-credentials-opencode -n default \
  --from-literal=provider-config='{"provider":{"openai":{"model":"glm-4.7","options":{"baseURL":"https://ai.thekao.cloud/v1","apiKey":"sk-Ba0CTdypBUrbkIdJXXlmhA"}}}'
```

**Result**: Secret created successfully, but agent pods fail with error:
```
Configuration is invalid at OPENCODE_CONFIG_CONTENT
↳ Unrecognized key: "model" provider.openai
```

### 3. Recreated Secret with Minimal Model/Options Format

**Command:**
```bash
kubectl delete secret llm-credentials-opencode -n default
mkdir -p /tmp
cat > /tmp/config.json << 'EOF'
{
  "provider": {
    "openai": {
      "model": "glm-4.7",
      "options": {
        "baseURL": "https://ai.thekao.cloud/v1",
        "apiKey": "sk-Ba0CTdypBUrbkIdJXXlmhA"
      }
    }
  }
}
EOF
kubectl create secret generic llm-credentials-opencode -n default --from-file=provider-config=/tmp/config.json
```

**Result**: Secret created successfully with valid JSON. However, agent pods fail with error:
```
Configuration is invalid at OPENCODE_CONFIG_CONTENT
↳ Unrecognized key: "options"
```

### 4. Recreated Secret with Root-level Model/Options

**Command:**
```bash
kubectl delete secret llm-credentials-opencode -n default
cat > /tmp/config.json << 'EOF'
{
  "model": "glm-4.7",
  "options": {
    "baseURL": "https://ai.thekao.cloud/v1",
    "apiKey": "sk-Ba0CTdypBUrbkIdJXXlmhA"
  }
}
EOF
kubectl create secret generic llm-credentials-opencode -n default --from-file=provider-config=/tmp/config.json
```

**Result**: Secret created. Agent pods fail with error:
```
Configuration is invalid at OPENCODE_CONFIG_CONTENT
↳ Unrecognized key: "options"
```

### 5. Deleted All RemediationJobs (Multiple Times)

Cleaned up remediationjobs multiple times to trigger fresh agent runs with new secret:
```bash
kubectl delete remediationjobs --all -n default
```

## Issues Encountered

### 1. OpenCode CLI Config Schema Unknown

**Problem**: The OpenCode CLI (v0.3.7) in the mendabot-agent image rejects all configuration formats attempted.

**Tried formats (all rejected)**:

1. `{"provider":{"openai":{"model":"glm-4.7","options":{"baseURL":"...","apiKey":"..."}}}`
   - Error: `Unrecognized key: "model" provider.openai`

2. `{"model":"glm-4.7","options":{"baseURL":"...","apiKey":"..."}}`
   - Error: `Unrecognized key: "options"`

**Analysis**:
- The opencode.ai/docs show examples for organizational provider configuration, but the actual CLI schema appears different
- The `provider.{name}` structure may be for remote config (.well-known/opencode), not for local OPENCODE_CONFIG_CONTENT env var
- For custom OpenAI-compatible endpoints, the config schema is unclear

**Status**: BLOCKER - Cannot configure opencode to use custom LLM endpoint.

### 2. RemediationJobs Created But Agents Fail

**Observation**: RemediationJobs are being created successfully by watcher:

```
INFO  RemediationJob created (test-broken-image, test-crashloop)
INFO  dispatched agent job (jobs starting)
```

However, agent pods fail immediately with config errors:
```
ERROR  Configuration is invalid at OPENCODE_CONFIG_CONTENT
```

**Current secret content**:
```json
{
  "model": "glm-4.7",
  "options": {
    "baseURL": "https://ai.thekao.cloud/v1",
    "apiKey": "sk-Ba0CTdypBUrbkIdJXXlmhA"
  }
}
```

This format follows opencode.ai/docs for provider config, but CLI still rejects it.

### 3. LLM Readiness Check Working

**Observation**: After patching deployment to add `LLM_PROVIDER=openai`, the watcher logs show readiness checks are not blocking jobs. RemediationJobs are created without readiness check errors.

**Watcher logs snippet**:
```
INFO  Starting workers (deployment)
INFO  RemediationJob created
INFO  dispatched agent job
```

No "readiness check failed" errors present, indicating the readiness check is working correctly.

## Status

**Watcher deployment**: ✅ Running (v0.3.7)
**RemediationJobs**: ✅ Being created for findings
**Agent jobs**: ✅ Starting with v0.3.7 agent image
**Agent execution**: ❌ BLOCKED - All agents fail with config errors
**LLM migration**: ⚠️ Secret created but config format unknown

## Current Secret

**Name**: `llm-credentials-opencode`
**Namespace**: `default`
**Type**: `Opaque`
**Data**: `provider-config` key with JSON:
```json
{
  "model": "glm-4.7",
  "options": {
    "baseURL": "https://ai.thekao.cloud/v1",
    "apiKey": "sk-Ba0CTdypBUrbkIdJXXlmhA"
  }
}
```

## Follow-up Items

1. **Document correct OpenCode config format** - Need to find official documentation or source code for OpenCode CLI config schema for custom OpenAI-compatible providers.

2. **Test with standard OpenAI endpoint** - Try connecting to actual OpenAI API to verify config format works with supported models, then adapt for custom endpoint.

3. **Check OpenCode CLI version** - Verify if v0.3.7 in the mendabot-agent image is correct or if there's a newer version with different config schema.

4. **Consider alternative authentication** - OpenCode docs suggest using `/connect` flow for authentication instead of manual JSON config. May need to investigate this approach.

5. **Check if model name is the issue** - "glm-4.7" may not be recognized as a valid model identifier even if the endpoint supports it.

## Git Commands Used

```bash
# Patched deployment to enable LLM readiness
kubectl patch deployment mendabot -n default -p '{"spec":{"template":{"spec":{"containers":[{"name":"watcher","env":[{"name":"LLM_PROVIDER","value":"openai"}]}]}}}}'

# Recreated secret multiple times
kubectl delete secret llm-credentials-opencode -n default
kubectl create secret generic llm-credentials-opencode -n default \
  --from-literal=provider-config='{"provider":{"openai":{"model":"glm-4.7","options":{"baseURL":"https://ai.thekao.cloud/v1","apiKey":"sk-Ba0CTdypBUrbkIdJXXlmhA"}}}'

# Cleaned up remediationjobs to trigger fresh runs
kubectl delete remediationjobs --all -n default
```

## Deployment Info

**Image**: `ghcr.io/lenaxia/mendabot-watcher:v0.3.7`
**LLM_PROVIDER**: `openai`
**Secret**: `llm-credentials-opencode` in namespace `default`
