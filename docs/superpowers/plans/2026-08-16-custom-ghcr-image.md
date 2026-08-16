# Custom GHCR Image Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish the fork's backend and admin redesign as a multi-platform `ghcr.io/jeor/mediastorm` image that works with the user's existing Unraid `docker run` configuration.

**Architecture:** Build the Go backend for each target platform in a dedicated builder stage, then install that binary over every historical MediaStorm binary path in `godver3/mediastorm:latest`. A GitHub Actions workflow publishes `latest` and immutable SHA tags to GHCR and verifies the resulting multi-platform manifest.

**Tech Stack:** Docker BuildKit/Buildx, Go 1.26.5 with CGO, GitHub Actions, GitHub Container Registry

## Global Constraints

- Preserve the upstream image's frontend, FFmpeg, Iroh bridge, Python tools, health check, exposed port, volumes, environment variables, and startup behavior.
- Publish `linux/amd64` and `linux/arm64` under `ghcr.io/jeor/mediastorm`.
- Never include the repository's unrelated untracked design/reference files.
- Do not require Docker Hub credentials or modify `godver3/mediastorm`.
- Retain compatibility with both historical `/root/strmr` and current `/app/mediastorm` runtime commands.

---

### Task 1: Derivative Runtime Image

**Files:**
- Create: `Dockerfile.custom`

**Interfaces:**
- Consumes: the tracked `backend/` Go module and public `godver3/mediastorm:latest` image
- Produces: a Buildx-compatible image with the custom backend installed at `/app/mediastorm`, `/app/strmr`, and `/root/strmr`

- [ ] **Step 1: Add the derivative Dockerfile**

```dockerfile
# syntax=docker/dockerfile:1.7
FROM --platform=$TARGETPLATFORM golang:1.26.5-bookworm@sha256:1ecb7edf62a0408027bd5729dfd6b1b8766e578e8df93995b225dfd0944eb651 AS builder

WORKDIR /src
RUN apt-get update && apt-get install -y --no-install-recommends git build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY backend/go.mod backend/go.sum ./
ENV GOTOOLCHAIN=local
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked go mod download

COPY backend/ ./
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    test "$TARGETOS" = "linux" \
    && CGO_ENABLED=1 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -o /out/mediastorm .

FROM godver3/mediastorm:latest
USER root
COPY --from=builder --chmod=0755 /out/mediastorm /tmp/mediastorm-custom
RUN install -m 0755 /tmp/mediastorm-custom /app/mediastorm \
    && install -m 0755 /tmp/mediastorm-custom /app/strmr \
    && install -m 0755 /tmp/mediastorm-custom /root/strmr \
    && if [ ! -L /root/mediastorm ]; then install -m 0755 /tmp/mediastorm-custom /root/mediastorm; fi \
    && rm /tmp/mediastorm-custom
```

- [ ] **Step 2: Verify the Dockerfile contract statically**

Run:

```powershell
Select-String Dockerfile.custom -Pattern 'FROM --platform=\$TARGETPLATFORM','CGO_ENABLED=1','FROM godver3/mediastorm:latest','/app/mediastorm','/root/strmr'
git diff --check -- Dockerfile.custom
```

Expected: every pattern is reported and `git diff --check` exits 0.

- [ ] **Step 3: Run backend tests in Linux**

Run:

```powershell
wsl.exe -e sh -lc 'cd /mnt/c/Users/geoff/OneDrive/Documents/Mediastorm/backend && go test ./... -count=1'
```

Expected: `go test` exits 0 with no failing package.

### Task 2: GHCR Publishing Workflow

**Files:**
- Create: `.github/workflows/publish-custom-image.yml`

**Interfaces:**
- Consumes: `Dockerfile.custom`, repository `GITHUB_TOKEN`, and pushes to `master`
- Produces: `ghcr.io/jeor/mediastorm:latest` and `ghcr.io/jeor/mediastorm:sha-<short-sha>` manifests

- [ ] **Step 1: Add the workflow**

```yaml
name: Publish custom Mediastorm image

on:
  workflow_dispatch:
  push:
    branches: [master]
    paths:
      - backend/**
      - Dockerfile.custom
      - .github/workflows/publish-custom-image.yml

permissions:
  contents: read
  packages: write

concurrency:
  group: publish-custom-mediastorm
  cancel-in-progress: false

jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-qemu-action@v3
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/jeor/mediastorm
          tags: |
            type=raw,value=latest
            type=sha,prefix=sha-,format=short
      - uses: docker/build-push-action@v6
        with:
          context: .
          file: Dockerfile.custom
          platforms: linux/amd64,linux/arm64
          pull: true
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
      - name: Verify published manifest
        run: docker buildx imagetools inspect ghcr.io/jeor/mediastorm:sha-${GITHUB_SHA::7}
```

- [ ] **Step 2: Validate publication safeguards**

Run:

```powershell
Select-String .github\workflows\publish-custom-image.yml -Pattern 'packages: write','linux/amd64,linux/arm64','push: true','type=sha','imagetools inspect'
git diff --check -- .github/workflows/publish-custom-image.yml
```

Expected: every pattern is reported and `git diff --check` exits 0.

- [ ] **Step 3: Commit only the packaging files**

Run:

```powershell
git add Dockerfile.custom
git add -f .github/workflows/publish-custom-image.yml
git commit -m "ci: publish custom Mediastorm image"
```

Expected: one commit containing exactly the Dockerfile and workflow.

### Task 3: Publish and Validate

**Files:**
- No new files

**Interfaces:**
- Consumes: committed packaging files and the existing GitHub branch
- Produces: merged `master`, completed publication workflow, verified GHCR tags, and an Unraid image reference

- [ ] **Step 1: Push the current branch**

Run:

```powershell
git push -u origin ui/settings-basic-advanced-toggle
```

Expected: GitHub reports the branch updated to the packaging commit.

- [ ] **Step 2: Open and merge the pull request**

Create a PR from `ui/settings-basic-advanced-toggle` to `master` describing the derivative image, missing-private-frontend constraint, preserved runtime contract, and verification. Merge only if GitHub reports the PR mergeable.

Expected: `master` advances to a merge commit containing the admin redesign and packaging files.

- [ ] **Step 3: Monitor publication**

Use GitHub workflow-run status for the merged commit until it reaches a terminal state.

Expected: the `Publish custom Mediastorm image` run concludes `success`.

- [ ] **Step 4: Verify registry tags and platforms**

Inspect `ghcr.io/jeor/mediastorm:latest` and `ghcr.io/jeor/mediastorm:sha-<short-sha>` with the GitHub workflow output or Docker registry metadata.

Expected: both tags resolve and the manifest contains `linux/amd64` and `linux/arm64`.

- [ ] **Step 5: Provide the deployment command**

Tell the user to rerun their existing Unraid template with:

```text
ghcr.io/jeor/mediastorm:latest
```

For rollback, provide the exact immutable SHA tag published by the workflow.
