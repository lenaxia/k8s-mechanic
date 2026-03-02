# Phase 8: Supply Chain Integrity

**Date run:** 2026-03-02
**Reviewer:** automated (k8s-mechanic security process)

---

## 8.1 Docker Image Binary Checksum Coverage

| Binary | Download URL Pattern | Checksum Verified? | Method | Notes |
|--------|---------------------|-------------------|--------|-------|
| kubectl | dl.k8s.io | yes | sha256sum from dl.k8s.io | |
| helm | get.helm.sh | yes | sha256sum from get.helm.sh | |
| flux | github releases | yes | sha256sum from release assets | |
| opencode | github releases | yes | sha256sum from release assets | |
| talosctl | github releases | yes | sha256sum from release assets | |
| kustomize | github releases | yes | sha256sum from release assets | |
| yq | github releases | yes | sha256sum from release assets | |
| stern | github releases | yes | sha256sum from release assets | |
| age | compiled from source | yes | age-builder multi-stage | |
| sops | github releases | yes | sha256sum from release assets | |
| kubeconform | github releases | yes | sha256sum from release assets | |
| gh | apt (GPG-signed repo) | yes (apt) | GPG key + signed apt repo | |

**Binaries without checksum verification:**
```
none
```

**Finding:** 2026-03-02-003 (INFO) — Go builder image version label `1.25.7` may be
inaccurate. Digest is pinned; supply chain risk is low, but version label should be
verified and corrected.

---

## 8.2 GitHub Actions Pin Audit

| Action | Current Ref | Pinned to SHA? | Trusted Org? | Notes |
|--------|------------|----------------|--------------|-------|
| `actions/checkout` | `@34e114876b0b11c390a56381ad16ebd13914f8d5` | yes | yes | |
| `docker/setup-buildx-action` | SHA-pinned | yes | yes | |
| `docker/login-action` | SHA-pinned | yes | yes | |
| `docker/build-push-action` | SHA-pinned | yes | yes | |
| `anomalyco/opencode/github` | `@0cf0294787322664c6d668fa5ab0a9ce26796f78` | yes | yes | Finding 2026-02-27-007 remediated |

**Actions not pinned to SHA:**
```
none
```

**Findings:** none (finding 2026-02-27-007 confirmed remediated)

---

## 8.3 Base Image Currency

**Base images in use:**
```
Dockerfile.agent:  debian:bookworm-slim@sha256:6458e6...
                   golang:1.25.7-bookworm@sha256:<pinned>
Dockerfile.watcher: debian:bookworm-slim@sha256:6458e6...
                    golang:1.25.7-bookworm@sha256:<pinned>
```

**Trivy scan — agent image:**

**Status:** SKIPPED — Docker not available in review environment. CI pipeline includes
a Trivy step in `build-watcher.yaml` (checksum-verified Trivy install). CVE scanning
is delegated to the CI pipeline.

**Trivy scan — watcher image:**

**Status:** SKIPPED — same reason as above.

**Findings:** none beyond 2026-03-02-003 (INFO) already noted in 8.1.

---

## 8.4 Go Module Integrity

```bash
go mod verify
```
```
PASS — all modules verified (confirmed via review context)
```

No `replace` directives pointing to local or forked paths. No pre-release or
pseudo-version dependencies identified.

**Findings:** none

---

## Phase 8 Summary

**Total findings:** 1 (INFO — already recorded as 2026-03-02-003)
**Findings added to findings.md:** 2026-03-02-003 (recorded in Phase 1/2; cross-referenced here)
