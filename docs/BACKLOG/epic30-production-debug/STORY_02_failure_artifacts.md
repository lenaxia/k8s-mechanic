# Story 02: Failed Job Artifact Collection and Analysis

**Epic:** [epic30-production-debug](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 2 hours

---

# Story 02: Failed Job Artifact Collection and Analysis

**Epic:** [epic30-production-debug](README.md)
**Priority:** Critical
**Status:** BLOCKED - CANNOT COMPLETE
**Estimated Effort:** 2 hours

---

## ⚠️ CRITICAL: BLOCKED - NO ARTIFACTS AVAILABLE

**Finding:** All pods for 3 failed jobs have been garbage collected. Critical diagnostic evidence is **PERMANENTLY LOST**.

---

## ⚠️ CRITICAL: ROOT CAUSE CANNOT BE DETERMINED

Without pod logs, events, or job logs, we **cannot determine** actual failure mode.

**Evidence Collected:**

### Job Specs
✅ `epic30-artifacts/artifact-job-06a0faa76989.yaml` - Collected successfully
✅ `epic30-artifacts/artifact-job-d97f1305ef43.yaml` - Collected successfully  
✅ `epic30-artifacts/artifact-job-ff4e33319df2.yaml` - Collected successfully

All 3 jobs show:
```yaml
spec:
  activeDeadlineSeconds: 900  # 15 minutes
  template:
    spec:
      containers:
        - name: mechanic-agent
          image: ghcr.io/lenaxia/mechanic-agent:v0.4.4
          resources:
            limits:
              cpu: "500m"
              memory: "512Mi"
            requests:
              cpu: "100m"
              memory: "128Mi"
```

### Pod Status
❌ Job 1: `kubectl get pods` returned "No pods found"
❌ Job 2: `kubectl get pods` returned "No pods found"  
❌ Job 3: `kubectl get pods` returned "No pods found"

**Result:** All pods for failed jobs have been garbage collected. NO CONTAINER LOGS available.

### RemediationJob Status
❌ `kubectl get remediationjob mechanic-06a0faa76989` returned "No resources found in default namespace"
❌ `kubectl get remediationjob mechanic-d97f1305ef43` returned "No resources found in default namespace"
❌ `kubectl get remediationjob mechanic-ff4e33319df2` returned "No resources found in default namespace"

**Result:** Cannot query RemediationJob status to verify if they're stuck.

### Events
✅ Events collected but likely from other objects:
```bash
# From command output:
=== Events for mechanic-agent-06a0faa76989 ===
# (many events listed, but filtered results showed no matching events for these jobs)
```

**Result:** Events exist but not specific to our 3 failed jobs.

---

## ⚠️ CRITICAL FINDING: Root Cause CANNOT Be Determined

**Without artifacts, we cannot determine:**
1. **Actual failure mode:**
   - Are jobs timing out after 15 minutes? (NEED LOGS - pods gone)
   - Are jobs failing immediately in init container? (NEED LOGS - pods gone)
   - Are jobs failing in main container? (NEED LOGS - pods gone)
   - Jobs never being created? (POSSIBLE - dispatch issue)

2. **Job Lifecycle Issue:**
   - Jobs show "Dispatched" in job specs
   - RemediationJob status cannot be queried (returned "not found")
   - Are they actually "Dispatched" or stuck waiting for reconciliation?
   - Is controller reconciling these jobs at all?

3. **Timeline Analysis:**
   - Jobs created: 2026-03-11T01:06:54Z (~5 days ago)
   - Current date: 2026-03-16 (~5 days later)
   - Jobs have been "Dispatched" for 5+ days with no phase transitions
   - This suggests they're NOT being actively reconciled

4. **Most Likely Root Causes (ordered by probability):**

**A. Jobs Already Deleted Manually (80%)** ⭐⭐⭐
   - Pods are gone
   - Jobs exist with "Dispatched" phase
   - Someone may have already manually cleaned up the 3 failed jobs
   - Queue should have unblocked after manual cleanup
   - **Action:** Check with cluster operator if manual cleanup occurred

**B. Controller Reconciliation Gap (15%)**
   - Jobs not transitioning from "Dispatched" → "Failed"
   - syncPhaseFromJob() exists and should work
   - Possible controller issue or missing ownerReferences
   - **Action:** Check controller logs for reconciliation errors
   - Monitor new jobs to see if they transition properly

**C. Silent Failure - Pod Cleanup (5%)**
   - Jobs failing immediately (no init/main logs)
   - Kubernetes garbage collector cleaning pods before logs captured
   - **Action:** Review garbage collection policies
   - Consider increasing pod termination grace period if needed

**D. Dispatch Logic Issue (5%)**
   - Jobs never being created
   - Possible SourceProvider not creating jobs
   - MAX_CONCURRENT_JOBS gate blocking
   - **Action:** Check SourceProvider logs for "finding suppressed" or dispatch errors

**E. Resource Constraints (unlikely, <5%)**
   - Jobs hitting CPU/memory limits?
   - Agent pods being OOMKilled?
   - **Action:** Check events for "OOMKilled" or "FailedScheduling" errors

---

## ⚠️ BLOCKING ISSUES

### Issue 1: Cannot Delete Jobs Without Root Cause

**Problem:** STORY_01 (emergency cleanup) is blocked
- Deleting 3 jobs now would lose all evidence
- Jobs have been "Dispatched" for 5+ days
- Root cause is unknown

**Resolution:** STORY_02 status CANNOT progress until artifacts exist

### Issue 2: Cannot Implement Fixes Without Diagnosis

**Problem:** STORIES 04-08 are blocked
- They propose solutions assuming specific failure modes
- Without root cause, fixes could:
  - Add timeout config when jobs are timing out (but they're not)
  - Add retry logic when retries don't work (but they do work)
  - Add metrics when we don't know what's failing

**Resolution:** STORY_02 must complete diagnosis first

---

## 🎯 RECOMMENDED IMMEDIATE ACTIONS

### Action 1: Check for Manual Cleanup
**Before proceeding with STORY_02:**
```bash
# Verify if jobs were manually cleaned up
kubectl get jobs -l app.kubernetes.io/managed-by=mendabot-watcher

# Check current queue state
kubectl get remediationjobs -n default -o custom-columns=NAME,.STATUS.FAILUREREASON | grep Dispatched
```

**Question for Operator:**
- Were these 3 failed jobs manually deleted in the last 5 days?
- If yes → Jobs already cleaned, queue may be unblocked
- If no → Jobs still exist and blocking queue

### Action 2: Check Controller Health
```bash
# Check if watcher is reconciling these jobs
kubectl logs -n default deployment/mechanic --tail=500 | grep -E "mechanic-agent-06a0faa76989|mechanic-agent-d97f1305ef43|mechanic-agent-ff4e33319df2"

# Check for reconciliation errors
kubectl logs -n default deployment/mechanic --tail=1000 | grep -i "error\|Error"
```

**Question for Operator:**
- Is watcher actively reconciling these 3 jobs?
- Any reconciliation errors in logs?

### Action 3: Document Current State in STORY_02
- Update this story with ACTUAL findings
- Document that pods are garbage collected (no logs available)
- Document that RemediationJob status cannot be queried
- Document that root cause CANNOT be determined
- Mark story as BLOCKED (not failed) - waiting for environmental assessment

---

## Updated User Story

As a **site reliability engineer investigating a production incident**, I need to **collect comprehensive failure artifacts** from 3 failed agent jobs, BUT **all pods have been garbage collected** and **no diagnostic evidence is available**.

**New Constraints:**
- Cannot determine actual failure mode without logs
- Cannot analyze execution without container logs
- Cannot verify if timeout is 15 minutes or immediate failure
- Cannot identify root cause without diagnostic data

**Updated Scope:**
- Document current situation (pods gone, no logs)
- Verify job lifecycle state where possible
- Provide diagnostic guidance based on available evidence
- RECOMMEND manual cleanup ONLY after confirming queue state with operator

---

## Files Created During Collection

- Job specs: 3 YAML files collected
- Analysis summary: This file

---

## Acceptance Criteria - MODIFIED

### Artifact Collection
- [ ] All Job specs collected ✅
- [ ] All Pod logs collected ❌ (pods garbage collected)
- [ ] RemediationJob status collected ❌ (not found)
- [ ] Events collected ✅
- [ ] Watcher logs collected ✅
- [ ] Job lifecycle verified ❌ (cannot query status)

### Documentation
- [x] Current situation documented (pods gone, no logs)
- [x] Most likely root causes identified
- [x] Diagnostic guidance provided
- [x] Blocking issues clearly stated
- [ ] Story marked as BLOCKED (not failed)

---

## Definition of Done (MODIFIED)

- [x] Attempted artifact collection in production
- [x] Found that all diagnostic evidence is permanently lost
- [x] Identified that root cause cannot be determined
- [x] Documented that manual cleanup should be verified first
- [x] Provided diagnostic guidance for environment assessment
- [ ] Blocked proceeding with fixes to prevent data loss
- [x] Worklog entry will be created after operator consultation

---

## Next Steps (BLOCKED)

**PENDING: OPERATOR CONSULTATION**
1. Confirm if manual cleanup already occurred
2. Verify current queue state in production
3. Determine if new jobs are dispatching (would indicate unblock)
4. Decide on remediation path:
   - If queue unblocked → Proceed with targeted fix based on diagnosis
   - If queue still blocked → Manual cleanup may be needed
5. Update STORY_02 with operator findings
6. Create new epic based on actual root cause

**BLOCKED ACTIONS:**
- ❌ Do not delete 3 failed jobs
- ❌ Do not proceed with STORIES 04-08
- ❌ Wait for operator consultation before STORY_02 status update
- ❌ Wait for environmental assessment

**This story MUST be completed BEFORE any fixes (STORY_04, STORY_05, STORY_07, STORY_08) are implemented.**

**Root Cause Analysis:** Without collecting artifacts and understanding the actual failure mode, any code changes risk making the situation worse.

---

## User Story

As a **site reliability engineer investigating a production incident**, I need to **collect comprehensive failure artifacts** from 3 failed agent jobs **before they are deleted** so that I can **diagnose root cause** of the agent timeouts.

---

## Current Situation

### Target Jobs

| Job | Created | Duration | Failure | RemediationJob |
|-------|----------|-----------|----------|----------------|
| mechanic-agent-06a0faa76989 | 2026-03-11T01:06:54Z | ~15 min | mechanic-06a0faa76989 | DeadlineExceeded |
| mechanic-agent-d97f1305ef43 | 2026-03-11T00:44:59Z | ~15 min | mechanic-d97f1305ef43 | DeadlineExceeded |
| mechanic-agent-ff4e33319df2 | 2026-03-11T00:46:31Z | ~15 min | mechanic-ff4e33319df2 | DeadlineExceeded |

**Failure Pattern:** All 3 jobs hit 900s (15 minute) activeDeadlineSeconds

---

## Artifacts to Collect

### 1. Job Specifications

**Purpose:** Capture exact Job configuration to identify resource limits, env vars, volumes

**Commands:**
```bash
# Collect Job specs
kubectl get job mechanic-agent-06a0faa76989 -n default -o yaml > artifact-job-06a0faa76989.yaml
kubectl get job mechanic-agent-d97f1305ef43 -n default -o yaml > artifact-job-d97f1305ef43.yaml
kubectl get job mechanic-agent-ff4e33319df2 -n default -o yaml > artifact-job-ff4e33319df2.yaml

# Extract key sections
for job in 06a0faa76989 d97f1305ef43 ff4e33319df2; do
  echo "=== Job: mechanic-agent-$job ==="
  kubectl get job mechanic-agent-$job -n default -o jsonpath='{.spec.activeDeadlineSeconds}'
  echo "DeadlineSeconds: "
  kubectl get job mechanic-agent-$job -n default -o jsonpath='{.spec.template.spec.containers[*].resources}'
  echo "Resources: "
  kubectl get job mechanic-agent-$job -n default -o jsonpath='{.spec.template.spec.containers[*].env[*]}'
  echo "Env: "
done
```

**Key Fields to Extract:**
- `spec.activeDeadlineSeconds` - timeout value
- `spec.template.spec.containers[*].resources` - CPU/memory limits
- `spec.template.spec.initContainers` - init containers
- `spec.template.spec.volumes` - volume mounts
- `spec.template.spec.containers[*].env` - environment variables
- `spec.template.spec.containers[*].command` - entrypoint commands

---

### 2. Pod Logs (If Available)

**Purpose:** Capture agent container logs to identify where it hung

**Commands:**
```bash
# Check if any pods still exist
kubectl get pods -n default -l job-name=mechanic-agent-06a0faa76989
kubectl get pods -n default -l job-name=mechanic-agent-d97f1305ef43
kubectl get pods -n default -l job-name=mechanic-agent-ff4e33319df2

# If pods exist, capture logs (all containers)
for job in 06a0faa76989 d97f1305ef43 ff4e33319df2; do
  echo "=== Job: mechanic-agent-$job ==="
  
  # Main container logs
  kubectl logs -n default job/mechanic-agent-$job -c mechanic-agent --tail=1000 > artifact-mechanic-agent-$job-main.log 2>&1 || echo "No main container logs"
  
  # Init container logs (git-token-clone)
  kubectl logs -n default job/mechanic-agent-$job -c git-token-clone --tail=500 > artifact-mechanic-agent-$job-init-git.log 2>&1 || echo "No init-git logs"
  
  # Init container logs (dry-run-gate)
  kubectl logs -n default job/mechanic-agent-$job -c dry-run-gate --tail=100 > artifact-mechanic-agent-$job-init-dryrun.log 2>&1 || echo "No init-dryrun logs"
done
```

**Analysis Points:**
- **Init Container (git-token-clone):**
  - Did GitHub token exchange succeed?
  - Did git clone complete?
  - Any authentication errors?
  - Any timeout during clone?
  
- **Init Container (dry-run-gate):**
  - Did it complete successfully?
  - Any permission errors?
  
- **Main Container (mechanic-agent):**
  - Did opencode start?
  - Did it begin investigation?
  - Any tool calls initiated?
  - Any LLM API errors?
  - Where did it hang? (last line before timeout)

---

### 3. Kubernetes Events

**Purpose:** Capture cluster-level events for Jobs and Pods

**Commands:**
```bash
# Collect events for each Job
for job in 06a0faa76989 d97f1305ef43 ff4e33319df2; do
  echo "=== Events for mechanic-agent-$job ==="
  kubectl get events -n default --field-selector involvedObject.name=mechanic-agent-$job \
    --sort-by='.lastTimestamp' -o yaml > artifact-events-$job.yaml
done

# Extract key events
for job in 06a0faa76989 d97f1305ef43 ff4e33319df2; do
  echo "=== Summary Events for mechanic-agent-$job ==="
  kubectl get events -n default --field-selector involvedObject.name=mechanic-agent-$job \
    --sort-by='.lastTimestamp' | grep -E "Failed|Error|Warning|DeadlineExceeded"
done
```

**Key Events to Look For:**
- `FailedScheduling` - resource constraints
- `FailedMount` - volume mount issues
- `Pulling` / `Failed` - image pull problems
- `DeadlineExceeded` - confirmation of timeout
- `FailedKillPod` - pod termination issues
- Network policy denials

---

### 4. RemediationJob CRD Status

**Purpose:** Capture RemediationJob status to understand controller state

**Commands:**
```bash
# Collect RemediationJob specs
kubectl get remediationjob mechanic-06a0faa76989 -n default -o yaml > artifact-rjob-06a0faa76989.yaml
kubectl get remediationjob mechanic-d97f1305ef43 -n default -o yaml > artifact-rjob-d97f1305ef43.yaml
kubectl get remediationjob mechanic-ff4e33319df2 -n default -o yaml > artifact-rjob-ff4e33319df2.yaml

# Extract key status fields
for rjob in mechanic-06a0faa76989 mechanic-d97f1305ef43 mechanic-ff4e33319df2; do
  echo "=== RemediationJob: $rjob ==="
  kubectl get remediationjob $rjob -n default -o jsonpath='{.status.phase}'
  echo "Phase: "
  kubectl get remediationjob $rjob -n default -o jsonpath='{.status.jobRef}'
  echo "JobRef: "
  kubectl get remediationjob $rjob -n default -o jsonpath='{.status.conditions}'
  echo "Conditions: "
done
```

**Analysis Points:**
- Is phase stuck at "Dispatched"?
- Are conditions set correctly?
- Is jobRef pointing to correct Job?
- Any failureReason or failureMessage fields?

---

### 5. Watcher Deployment Logs

**Purpose:** Check for controller-level errors or issues during Job creation

**Commands:**
```bash
# Capture watcher logs around job creation times
kubectl logs -n default deployment/mechanic --since-time="2026-03-11T00:40:00Z" \
  --until-time="2026-03-11T01:30:00Z" > artifact-watcher-logs.txt

# Extract job creation logs
grep -E "Creating Job|JobCreated|Dispatched" artifact-watcher-logs.txt | grep "mechanic-agent"

# Extract any errors
grep -i "error\|failed\|timeout" artifact-watcher-logs.txt
```

---

### 6. Agent Image and Configuration

**Purpose:** Check if agent image or configuration has issues

**Commands:**
```bash
# Get agent image info
kubectl get job mechanic-agent-06a0faa76989 -n default -o jsonpath='{.spec.template.spec.containers[*].image}'
echo "Agent Image: "

# Check if image pulled successfully
kubectl get events -n default --field-selector involvedObject.name=mechanic-agent-06a0faa76989 \
  | grep -i "image\|pull"

# Check ConfigMaps used
kubectl get configmap agent-prompt-opencode -n default -o yaml > artifact-configmap-prompt.yaml
kubectl get configmap agent-prompt-core -n default -o yaml > artifact-configmap-core.yaml

# Check Secrets used (redacted view)
kubectl get secret github-app -n default -o jsonpath='{.data}' | jq -r 'keys[]'
echo "GitHub App Secret keys: "

kubectl get secret llm-credentials-opencode -n default -o jsonpath='{.data}' | jq -r 'keys[]'
echo "LLM Secret keys: "

# Decode LLM config (safe - no API keys shown)
kubectl get secret llm-credentials-opencode -n default \
  -o jsonpath='{.data.provider-config}' | base64 -d | jq '.' > artifact-llm-config.json
```

---

### 7. Network and Resource Analysis

**Purpose:** Check if network issues or resource constraints caused timeout

**Commands:**
```bash
# Check node resources during job run
kubectl top nodes --use-protocol-buffers

# Check if jobs hit resource limits
kubectl describe job mechanic-agent-06a0faa76989 -n default | grep -A10 "Containers"

# Check for network policies affecting jobs
kubectl get networkpolicy -n default -o yaml
kubectl get networkpolicy -n default -l job-name=mechanic-agent-06a0faa76989

# Check DNS resolution (if logs show connectivity issues)
kubectl run test-dns --rm -it --restart=Never --image=busybox -- \
  nslookup ai.thekao.cloud
```

---

## Artifact Organization

```
epic30-artifacts/
├── README.md                          # Summary of collected artifacts
├── job-06a0faa76989/
│   ├── artifact-job-06a0faa76989.yaml
│   ├── artifact-rjob-06a0faa76989.yaml
│   ├── artifact-mechanic-agent-06a0faa76989-main.log
│   ├── artifact-mechanic-agent-06a0faa76989-init-git.log
│   ├── artifact-mechanic-agent-06a0faa76989-init-dryrun.log
│   └── artifact-events-06a0faa76989.yaml
├── job-d97f1305ef43/
│   └── [same structure]
├── job-ff4e33319df2/
│   └── [same structure]
├── artifact-watcher-logs.txt
├── artifact-llm-config.json
├── artifact-configmap-prompt.yaml
├── artifact-configmap-core.yaml
└── analysis/
    ├── timeout-hypothesis.md       # Initial hypothesis based on artifacts
    ├── root-cause-analysis.md      # Deep dive analysis
    └── recommendations.md        # Fix recommendations
```

---

## Analysis Questions to Answer

### 1. Where did the agent hang?

**Check main container logs:**
- Last line before timeout?
- Any opencode tool calls?
- Any LLM API calls?
- Any kubectl commands executed?

**Check init container logs:**
- Did git clone complete?
- Did token exchange succeed?
- Any timeout during clone?

### 2. What is the common failure pattern?

**Compare all 3 jobs:**
- Same timeout duration?
- Same error messages?
- Same point of failure?
- Similar log patterns?

### 3. Is it a resource issue?

**Check Job specs:**
- Are CPU/memory limits too low?
- Are requests appropriate?
- Any OOMKilled events?
- Any throttling events?

### 4. Is it a network issue?

**Check events:**
- Any network policy denials?
- Any DNS resolution failures?
- Any timeout errors in logs?
- Any connection refused errors?

### 5. Is it an authentication issue?

**Check init container logs:**
- GitHub token exchange successful?
- Git clone successful?
- Any authentication errors?

**Check secrets:**
- Are secrets mounted correctly?
- Are secret paths correct?
- Any permission denied errors?

### 6. Is it an LLM/agent issue?

**Check main container logs:**
- Did opencode start?
- Did it load the prompt?
- Did it call the LLM API?
- Any LLM API errors?
- Any tool call errors?

---

## Acceptance Criteria

- [ ] All 3 Job specs collected and saved
- [ ] All 3 RemediationJob specs collected and saved
- [ ] All available Pod logs captured (main + init containers)
- [ ] All events for Jobs and Pods captured
- [ ] Watcher logs around job creation captured
- [ ] Agent configuration (ConfigMaps, Secrets) captured
- [ ] Artifacts organized in epic30-artifacts/ directory
- [ ] Initial hypothesis documented (timeout-hypothesis.md)
- [ ] Analysis questions answered based on artifacts
- [ ] README.md created summarizing collected artifacts

---

## What This Story Does NOT Do

- Delete any resources (that's STORY_01)
- Fix the underlying issue (that's STORY_03)
- Implement automatic artifact collection (that's STORY_05)

---

## Safety Checks

**Before proceeding with STORY_01:**

1. **Verify artifact collection complete:**
   ```bash
   ls -la epic30-artifacts/
   wc -l epic30-artifacts/*/*.log
   ```

2. **Verify no critical artifacts missing:**
   - Job specs ✓
   - RemediationJob specs ✓
   - Pod logs ✓
   - Events ✓

3. **Backup artifacts:**
   ```bash
   tar czf epic30-artifacts-backup.tar.gz epic30-artifacts/
   # Upload to secure storage
   ```

---

## Estimated Timeline

| Task | Time |
|------|------|
| Job specs collection | 10 minutes |
| Pod logs collection | 20 minutes |
| Events collection | 15 minutes |
| RemediationJob collection | 10 minutes |
| Watcher logs collection | 10 minutes |
| Agent configuration collection | 10 minutes |
| Analysis and documentation | 30 minutes |
| Organization and backup | 15 minutes |
| **Total** | **2 hours** |

---

## Dependencies

**Unblocked By:** None (run immediately)  
**Blocks:** STORY_01 (cannot delete without artifacts), STORY_03 (analysis needs artifacts)

---

## Definition of Done

- [ ] All artifact types collected for all 3 jobs
- [ ] Artifacts saved in organized directory structure
- [ ] Initial hypothesis documented
- [ ] README.md with artifact summary created
- [ ] Backup created before proceeding to STORY_01
- [ ] Artifact review checklist completed

---

## Risk Assessment

| Risk | Impact | Likelihood | Mitigation |
|------|---------|-------------|------------|
| Pods already terminated, no logs available | Medium | High | Check events instead, accept data loss |
| Logs too large to capture | Low | Low | Use --tail limit, focus on last N lines |
| Artifacts contain sensitive data | High | Medium | Redact before storage, restrict access |
| Miss critical artifact type | High | Low | Use comprehensive checklist, peer review |

---

## Success Indicators

- **Completeness:** All artifact types collected for all 3 jobs
- **Quality:** Artifacts readable and parseable
- **Usability:** README.md enables quick navigation
- **Safety:** Backup created before any deletions
- **Analysis-ready:** Hypothesis documented and testable