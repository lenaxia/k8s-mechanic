# Phase 6: GitHub App Private Key Isolation

**Date run:** 2026-03-02
**Reviewer:** automated (k8s-mechanic security process)

---

## 6.1 Code Verification — Watcher

Finding 2026-02-27-002 was remediated. The watcher no longer reads the private key from
an environment variable.

**Watcher deployment — volume mount review (`deploy/helm/templates/deployment-watcher.yaml`):**
```
Volume: github-app-private-key (Secret: github-app)
  mounted at: /var/run/secrets/mechanic/github-app-private-key
  readOnly: true
  conditional: only when watcher.prAutoClose=true
```

**Watcher main.go (cmd/watcher/main.go:233):**
```go
keyBytes, err := os.ReadFile("/var/run/secrets/mechanic/github-app-private-key/private-key")
privKey, err := jwt.ParseRSAPrivateKeyFromPEM(keyBytes)
```

No `os.Getenv("GITHUB_APP_PRIVATE_KEY")` call remains in the watcher.

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| Private key read from volume mount (watcher) | pass | main.go:233 | `os.ReadFile()` |
| `GITHUB_APP_PRIVATE_KEY` absent from watcher Deployment env | pass | — | No env var in deployment-watcher.yaml |
| Secret only mounted when `prAutoClose=true` | pass | — | Helm conditional |

---

## 6.2 Code Verification — Agent Job

**`internal/domain/job.go` — init container (git-token-clone) verification:**

```
GITHUB_APP_PRIVATE_KEY  — set only in init container (git-token-clone) Env block
GITHUB_APP_ID           — set only in init container Env block
GITHUB_APP_INSTALLATION_ID — set only in init container Env block
github-app-secret volume — mounted only in init container VolumeMounts
```

The main agent container (`mechanic-agent`) has no reference to `github-app-secret`
in `Env` or `VolumeMounts`. The shared `emptyDir` at `/workspace` carries only the
short-lived GitHub installation token (`github-token`).

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| `github-app-secret` volume in init container VolumeMounts only | pass | job.go:131-163 | |
| `GITHUB_APP_PRIVATE_KEY` in init container Env only | pass | | |
| `GITHUB_APP_ID` in init container Env only | pass | | |
| `GITHUB_APP_INSTALLATION_ID` in init container Env only | pass | | |
| Main container has no `github-app-secret` reference | pass | | |
| Shared emptyDir carries only the short-lived token | pass | | |

---

## 6.3 Live Verification

**Status:** SKIPPED — no live cluster available

**Notes:** Live verification (confirming `GITHUB_APP_PRIVATE_KEY` absent from main
container env at runtime, and `/secrets/github-app` not mounted) deferred to next
review with cluster access.

---

## Phase 6 Summary

**Total findings:** 0 (finding 2026-02-27-002 confirmed remediated)
**Findings added to findings.md:** none
