# Story 03: Agent Timeout Deep Dive Investigation

**Epic:** [epic30-production-debug](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 4 hours

---

## User Story

As a **developer debugging the production incident**, I need to **perform a deep dive analysis** of the agent timeout root cause so that I can **implement a permanent fix** to prevent future deadlocks.

---

## Investigation Context

**Summary of Available Data (from STORY_02):**

| Job | Created | Duration | Finding | Parent |
|------|---------|-----------|----------|---------|
| mechanic-agent-06a0faa76989 | 2026-03-11T01:06:54Z | 15 min | Deployment/grafana |
| mechanic-agent-d97f1305ef43 | 2026-03-11T00:44:59Z | 15 min | StatefulSet/sonarr |
| mechanic-agent-ff4e33319df2 | 2026-03-11T00:46:31Z | 15 min | StatefulSet/outline |

**Common Pattern:**
- All 3 jobs hit the 900s (15 minute) activeDeadlineSeconds
- All 3 jobs have phase "Dispatched" (controller never updated to "Failed")
- All 3 jobs have finding.type = "Deployment" or "StatefulSet"
- All 3 jobs use agent image: ghcr.io/lenaxia/mechanic-agent:v0.4.4

---

## Hypothesis Development

### Hypothesis 1: Init Container Timeout (Git Clone)

**Description:** The `git-token-clone` init container hangs during git clone operations.

**Evidence to Collect:**
```bash
# Check init container logs
cat artifact-mechanic-agent-06a0faa76989-init-git.log

# Look for:
# - GitHub token exchange: "get-github-app-token.sh" output
# - Git clone: "Cloning repository" message
# - Timeout: Any hanging without completion
# - Large repo warnings
```

**Testable Prediction:**
- If init container hangs → no main container logs exist
- If init container completes → main container logs exist

**Investigation Steps:**
1. Check if git clone URL is correct: `lenaxia/talos-ops-prod`
2. Check if GitHub App credentials are valid
3. Manually test git clone in same namespace:
   ```bash
   kubectl run git-test --rm -it --restart=Never \
     --image=ghcr.io/lenaxia/mechanic-agent:v0.4.4 \
     --overrides='{
       "spec": {
         "containers": [{
           "name": "git-test",
           "image": "ghcr.io/lenaxia/mechanic-agent:v0.4.4",
           "env": [
             {"name": "GITHUB_APP_ID", "valueFrom": {"secretKeyRef": {"name": "github-app", "key": "app-id"}}},
             {"name": "GITHUB_APP_INSTALLATION_ID", "valueFrom": {"secretKeyRef": {"name": "github-app", "key": "installation-id"}}},
             {"name": "GITHUB_APP_PRIVATE_KEY", "valueFrom": {"secretKeyRef": {"name": "github-app", "key": "private-key"}}}
           ],
           "command": ["/bin/bash", "-c"],
           "args": ["TOKEN=$(get-github-app-token.sh); echo Token acquired; git clone https://x-access-token:${TOKEN}@github.com/lenaxia/talos-ops-prod.git /tmp/repo; echo Clone complete"]
         }]
       }
     }'
   ```

**If Hypothesis Confirmed:**
- Root cause: git clone taking > 15 minutes
- Fix: Increase init container timeout, use shallow clone, or pre-pull image with repo

---

### Hypothesis 2: Init Container Timeout (GitHub App Token Exchange)

**Description:** The `git-token-clone` init container hangs during GitHub App JWT token generation.

**Evidence to Collect:**
```bash
# Check init container logs for token exchange
cat artifact-mechanic-agent-06a0faa76989-init-git.log | grep -E "TOKEN|token|JWT|exchange|GitHub"

# Look for:
# - JWT generation: "Generating JWT token"
# - App exchange: "Exchanging for installation token"
# - Success: "Token acquired"
# - Failure: Error messages
```

**Testable Prediction:**
- If token exchange fails → no "Token acquired" message
- If token exchange succeeds → token printed to logs (should be redacted)

**Investigation Steps:**
1. Verify GitHub App credentials in secret:
   ```bash
   # Decode app-id (base64)
   kubectl get secret github-app -n default -o jsonpath='{.data.app-id}' | base64 -d
   
   # Decode installation-id (base64)
   kubectl get secret github-app -n default -o jsonpath='{.data.installation-id}' | base64 -d
   
   # Verify private key format (PEM header/footer present)
   kubectl get secret github-app -n default -o jsonpath='{.data.private-key}' | base64 -d | head -2
   ```

2. Manually test token exchange:
   ```bash
   kubectl run token-test --rm -it --restart=Never \
     --image=ghcr.io/lenaxia/mechanic-agent:v0.4.4 \
     --overrides='{
       "spec": {
         "containers": [{
           "name": "token-test",
           "image": "ghcr.io/lenaxia/mechanic-agent:v0.4.4",
           "env": [
             {"name": "GITHUB_APP_ID", "valueFrom": {"secretKeyRef": {"name": "github-app", "key": "app-id"}}},
             {"name": "GITHUB_APP_INSTALLATION_ID", "valueFrom": {"secretKeyRef": {"name": "github-app", "key": "installation-id"}}},
             {"name": "GITHUB_APP_PRIVATE_KEY", "valueFrom": {"secretKeyRef": {"name": "github-app", "key": "private-key"}}}
           ],
           "command": ["/bin/bash", "-c"],
           "args": ["get-github-app-token.sh"]
         }]
       }
     }'
   ```

3. Check GitHub App installation status:
   - Verify app-id: 2917483 matches installation
   - Verify installation-id: 111603560 is valid
   - Check if app has permissions to read `lenaxia/talos-ops-prod`

**If Hypothesis Confirmed:**
- Root cause: Token exchange hanging or failing
- Fix: Validate credentials during deployment, add timeout to script

---

### Hypothesis 3: Main Container Timeout (OpenCode Startup)

**Description:** The `mechanic-agent` main container hangs during OpenCode initialization or LLM API calls.

**Evidence to Collect:**
```bash
# Check main container logs
cat artifact-mechanic-agent-06a0faa76989-main.log

# Look for:
# - OpenCode startup: "Starting opencode agent" or similar
# - Prompt loading: "Loading prompt from /prompt"
# - LLM provider initialization: "Connecting to thekao-cloud"
# - Tool initialization: "Tools available: kubectl, helm, gh, git"
# - First LLM call: "Calling LLM API"
# - First tool call: "Executing tool: kubectl get deployment grafana"
# - Where it hangs: last line before timeout
```

**Testable Prediction:**
- If OpenCode never starts → no OpenCode logs
- If OpenCode starts but hangs during LLM call → API connection issue
- If OpenCode starts but hangs during tool execution → tool issue

**Investigation Steps:**
1. Check if OpenCode binary exists and is executable:
   ```bash
   # From Job spec, check entrypoint
   kubectl get job mechanic-agent-06a0faa76989 -n default \
     -o jsonpath='{.spec.template.spec.containers[0].command}'
   
   # Should be something like: ["/bin/bash", "-c", "/entrypoint.sh"]
   ```

2. Check LLM configuration:
   ```bash
   # From artifact-llm-config.json (collected in STORY_02)
   cat artifact-llm-config.json | jq '.'
   
   # Verify:
   # - Provider: "thekao-cloud"
   # - Model: "glm-4.7"
   # - Base URL: "https://ai.thekao.cloud/v1"
   # - API key: masked/hidden (should not be visible)
   ```

3. Manually test LLM API connectivity:
   ```bash
   kubectl run llm-test --rm -it --restart=Never \
     --image=ghcr.io/lenaxia/mechanic-agent:v0.4.4 \
     --overrides='{
       "spec": {
         "containers": [{
           "name": "llm-test",
           "image": "ghcr.io/lenaxia/mechanic-agent:v0.4.4",
           "env": [{"name": "OPENCODE_CONFIG_CONTENT", "valueFrom": {"secretKeyRef": {"name": "llm-credentials-opencode", "key": "provider-config"}}}],
           "command": ["/bin/bash", "-c"],
           "args": ["echo $OPENCODE_CONFIG_CONTENT | jq -c '.' > /tmp/config.json && opencode run --config /tmp/config.json --prompt 'Say hello'"]
         }]
       }
     }'
   ```

4. Test kubectl read-only access:
   ```bash
   kubectl run kubectl-test --rm -it --restart=Never \
     --image=ghcr.io/lenaxia/mechanic-agent:v0.4.4 \
     --overrides='{
       "spec": {
         "serviceAccountName": "mechanic-agent",
         "containers": [{
           "name": "kubectl-test",
           "image": "ghcr.io/lenaxia/mechanic-agent:v0.4.4",
           "command": ["kubectl", "get", "deployment", "grafana", "-n", "monitoring"]
         }]
       }
     }'
   ```

**If Hypothesis Confirmed:**
- Root cause: OpenCode startup or LLM API hanging
- Fix: Add health checks, increase timeout, fix API configuration

---

### Hypothesis 4: Resource Exhaustion

**Description:** The agent pod hits CPU or memory limits, causing throttling or OOM kills that appear as timeouts.

**Evidence to Collect:**
```bash
# Check Job resource limits
cat artifact-job-06a0faa76989.yaml | grep -A5 "resources:"

# From STORY_02 artifacts:
# cpu: 500m limit, 100m request
# memory: 512Mi limit, 128Mi request

# Check for OOMKilled events
cat artifact-events-06a0faa76989.yaml | grep -i "oomkilled\|memory\|throttling"

# Check if pod was killed
kubectl describe job mechanic-agent-06a0faa76989 -n default | grep -A20 "State:"
```

**Testable Prediction:**
- If OOMKilled → memory limit too low
- If throttling → CPU limit too low
- If resource healthy → not a resource issue

**Investigation Steps:**
1. Calculate resource needs:
   - OpenCode binary size: ~100MB
   - kubectl binary: ~100MB
   - Git repo clone: talos-ops-prod size?
   - LLM response buffer: potentially large
   - Total memory needed: > 512Mi?

2. Check node resource availability:
   ```bash
   # Check if pods are scheduled on same node
   kubectl get pods -n default -o wide | grep mechanic-agent
   
   # Check node resources
   kubectl describe node <node-name> | grep -A10 "Allocated resources"
   ```

3. Monitor resource usage during new job run:
   ```bash
   # Create test job with monitoring
   kubectl create -f - <<EOF
   apiVersion: batch/v1
   kind: Job
   metadata:
     name: mechanic-agent-resource-test
   spec:
     template:
       spec:
         containers:
         - name: mechanic-agent
           image: ghcr.io/lenaxia/mechanic-agent:v0.4.4
           resources:
             limits:
               cpu: 500m
               memory: 512Mi
             requests:
               cpu: 100m
               memory: 128Mi
           command: ["sleep", "300"]
   EOF
   
   # Watch resource usage
   kubectl top pod mechanic-agent-resource-test-<pod-id> -n default --use-protocol-buffers
   ```

**If Hypothesis Confirmed:**
- Root cause: Resource limits too low
- Fix: Increase limits, add monitoring, add resource checks

---

### Hypothesis 5: Network Connectivity Issues

**Description:** The agent pod cannot reach external services (GitHub, LLM API) due to network policies or DNS issues.

**Evidence to Collect:**
```bash
# Check for network policy events
cat artifact-events-06a0faa76989.yaml | grep -i "network\|policy\|denied"

# Check logs for connectivity errors
cat artifact-mechanic-agent-06a0faa76989-main.log | grep -i "timeout|connection|refused|dns|network"

# Check for DNS resolution issues
cat artifact-mechanic-agent-06a0faa76989-init-git.log | grep -i "dns|lookup|address"
```

**Testable Prediction:**
- If network issue → connection refused, timeout errors in logs
- If DNS issue → lookup failures in logs

**Investigation Steps:**
1. Check network policies:
   ```bash
   kubectl get networkpolicy -n default -o yaml | grep -A50 "mechanic-agent"
   
   # Verify agent pod can egress to:
   # - github.com (443/tcp)
   # - ai.thekao.cloud (443/tcp)
   # - kubernetes.default.svc (443/tcp for API)
   ```

2. Test DNS resolution:
   ```bash
   kubectl run dns-test --rm -it --restart=Never --image=busybox -- \
     nslookup github.com && \
     nslookup ai.thekao.cloud && \
     nslookup kubernetes.default.svc
   ```

3. Test network connectivity:
   ```bash
   kubectl run net-test --rm -it --restart=Never --image=busybox -- \
     wget -O /dev/null --timeout=10 https://github.com && \
     wget -O /dev/null --timeout=10 https://ai.thekao.cloud/v1/models
   ```

**If Hypothesis Confirmed:**
- Root cause: Network policy blocking or DNS issues
- Fix: Update network policies, add DNS troubleshooting

---

### Hypothesis 6: Agent Image or Entrypoint Issues

**Description:** The agent container's entrypoint script or binary has a bug that causes it to hang indefinitely.

**Evidence to Collect:**
```bash
# Check Job command/args
cat artifact-job-06a0faa76989.yaml | grep -A10 "command:"

# Should show:
# command:
# - /bin/bash
# - -c
# - /agent-entrypoint.sh

# Check if script exists and is executable
kubectl run image-test --rm -it --restart=Never \
  --image=ghcr.io/lenaxia/mechanic-agent:v0.4.4 \
  --command=["ls", "-la", "/agent-entrypoint.sh"]

# Check entrypoint script content
kubectl run script-test --rm -it --restart=Never \
  --image=ghcr.io/lenaxia/mechanic-agent:v0.4.4 \
  --command=["cat", "/agent-entrypoint.sh"]
```

**Testable Prediction:**
- If script missing/broken → container exits immediately or hangs
- If script has infinite loop → hangs forever

**Investigation Steps:**
1. Verify agent image version:
   ```bash
   # Image tag: v0.4.4
   # Is this the correct/latest version?
   kubectl get deployment mechanic -n default -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="AGENT_IMAGE")].value}'
   ```

2. Check if image was updated recently:
   ```bash
   # Check image creation date
   docker manifest inspect ghcr.io/lenaxia/mechanic-agent:v0.4.4
   ```

3. Manually run agent entrypoint:
   ```bash
   kubectl run entrypoint-test --rm -it --restart=Never \
     --image=ghcr.io/lenaxia/mechanic-agent:v0.4.4 \
     --overrides='{
       "spec": {
         "containers": [{
           "name": "entrypoint-test",
           "image": "ghcr.io/lenaxia/mechanic-agent:v0.4.4",
           "env": [{"name": "FINDING_NAME", "value": "test"}],
           "command": ["/bin/bash", "-c"],
           "args": ["/agent-entrypoint.sh && echo Entrypoint completed"]
         }]
       }
     }'
   ```

**If Hypothesis Confirmed:**
- Root cause: Agent image bug or entrypoint issue
- Fix: Patch image, update entrypoint script

---

## Investigation Workflow

### Phase 1: Evidence Review (1 hour)

1. **Review all artifacts collected in STORY_02:**
   - Job specs for all 3 jobs
   - Pod logs (main + init)
   - Events
   - RemediationJob status
   - Watcher logs
   - Configuration (ConfigMaps, Secrets)

2. **Create evidence matrix:**
   ```
   | Job | Init Git Logs | Main Logs | Events | Resource Errors | Network Errors |
   |------|---------------|------------|---------|----------------|----------------|
   | 06a0 |               |            |         |                |                |
   | d97f |               |            |         |                |                |
   | ff4e |               |            |         |                |                |
   ```

3. **Identify patterns:**
   - Common failure points?
   - Common error messages?
   - Resource constraints?
   - Network issues?

### Phase 2: Hypothesis Testing (2 hours)

1. **Rank hypotheses by likelihood** based on evidence
2. **Test most likely hypothesis first:**
   - Create test Job
   - Monitor logs in real-time
   - Capture events
3. **If hypothesis confirmed:**
   - Document evidence
   - Move to fix design (STORY_04)
   - Stop testing other hypotheses
4. **If hypothesis rejected:**
   - Document why
   - Move to next hypothesis

### Phase 3: Root Cause Documentation (1 hour)

1. **Write root cause analysis:**
   ```
   ## Root Cause

   **Issue:** [Summary of what failed]

   **Evidence:**
   - [Evidence 1]
   - [Evidence 2]
   - [Evidence 3]

   **Root Cause:** [Detailed explanation of why it failed]

   **Why It Weren't Detected:**
   - [Explanation of monitoring gap]
   - [Explanation of why tests didn't catch it]

   **Fix Required:**
   - [Immediate fix]
   - [Long-term prevention]
   ```

2. **Write recommendations:**
   ```
   ## Recommendations

   ### Immediate (Emergency Fix)
   - [Action 1]
   - [Action 2]

   ### Short-term (Prevention)
   - [Improvement 1]
   - [Improvement 2]

   ### Long-term (Architecture)
   - [Change 1]
   - [Change 2]
   ```

---

## Acceptance Criteria

### Evidence Review
- [ ] All artifacts from STORY_02 reviewed
- [ ] Evidence matrix created
- [ ] Common patterns identified

### Hypothesis Testing
- [ ] At least 2 hypotheses tested with reproduction Jobs
- [ ] Most likely hypothesis confirmed or rejected with evidence
- [ ] Test artifacts collected (logs, events, specs)

### Root Cause Documentation
- [ ] Root cause analysis document created
- [ ] Root cause identified with 90%+ confidence
- [ ] Evidence for root cause documented
- [ ] Why it wasn't detected documented

### Recommendations
- [ ] Immediate fix documented
- [ ] Short-term prevention documented
- [ ] Long-term architecture improvements documented
- [ ] All recommendations actionable and prioritized

---

## What This Story Does NOT Do

- Implement the fix (that's STORY_04 and STORY_05)
- Deploy the fix to production
- Add monitoring or alerts (that's STORY_06)

---

## Success Indicators

- **Confidence:** Root cause identified with 90%+ confidence
- **Evidence:** Multiple independent data points support conclusion
- **Testability:** Root cause can be reproduced on demand
- **Clarity:** Fix requirements are clear and actionable

---

## Risk Assessment

| Risk | Impact | Likelihood | Mitigation |
|------|---------|-------------|------------|
| Insufficient evidence to identify root cause | High | Medium | Accept partial certainty, proceed with most likely fix |
| Multiple root causes (not single issue) | Medium | Low | Fix most common cause, monitor for others |
| Hypothesis testing takes too long | Low | Medium | Timebox to 2 hours, proceed with best evidence |
| Fix causes new issues | High | Low | Test fix thoroughly before deployment |

---

## Definition of Done

- [ ] All artifacts reviewed and analyzed
- [ ] Evidence matrix created
- [ ] At least 2 hypotheses tested
- [ ] Root cause identified with supporting evidence
- [ ] Root cause analysis document created
- [ ] Recommendations document created
- [ ] Fix requirements clear and actionable
- [ ] Worklog entry created with investigation summary

---

## Deliverables

1. **evidence-matrix.md** - Summary of evidence from all artifacts
2. **root-cause-analysis.md** - Deep dive analysis of root cause
3. **recommendations.md** - Prioritized list of fixes and improvements
4. **test-jobs/** - YAML manifests for test Jobs created during investigation
5. **Worklog entry** - Summary of investigation process and findings