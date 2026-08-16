# Custom GHCR Image Design

## Goal

Publish the admin redesign from `Jeor/mediastorm` as a Docker image that can
replace `godver3/mediastorm:latest` in the existing Compose configuration.

## Constraints

- The fork does not contain the `frontend/` source required by
  `backend/Dockerfile`, so it cannot perform the repository's full image build.
- The custom image must retain the upstream image's bundled frontend, FFmpeg,
  Iroh bridge, Python tools, health check, ports, paths, and startup command.
- The existing untracked design and reference files are outside this change.
- Publishing must not require a Docker Hub credential.

## Architecture

Add a derivative-image Dockerfile with two stages:

1. A Go builder compiles the fork's `backend/` tree for Linux with CGO enabled.
   Its Go version and Debian family match the repository's existing Dockerfile.
2. `godver3/mediastorm:latest` supplies the complete runtime. The newly compiled
   binary replaces `/app/mediastorm`; the runtime's existing `/root/mediastorm`
   symlink continues to resolve to it.

This boundary deliberately changes only the application binary. Runtime assets
that are unavailable in the fork continue to come from the published upstream
image.

## Publishing Workflow

Create a GitHub Actions workflow that:

- runs after pushes to `master` when backend, derivative Dockerfile, or workflow
  files change;
- also supports manual dispatch;
- logs in to GitHub Container Registry with the repository `GITHUB_TOKEN` and
  `packages: write` permission;
- builds Linux `amd64` and `arm64` images with Buildx;
- publishes a multi-platform manifest as `ghcr.io/jeor/mediastorm:latest`;
- publishes the immutable commit tag `ghcr.io/jeor/mediastorm:sha-<short-sha>`;
- uses GitHub Actions layer caching;
- does not publish images from pull-request events.

The package will inherit GitHub's package visibility rules. If GHCR creates it
as private, the owner must make it public once or authenticate Docker pulls.

## Data Flow

1. The UI/backend commit and image files reach `master`.
2. GitHub Actions checks out that exact commit.
3. Buildx compiles its backend for both target architectures.
4. Each binary is layered over the matching architecture of the upstream image.
5. GHCR receives architecture manifests and the combined multi-platform tags.
6. Docker Compose pulls `ghcr.io/jeor/mediastorm:latest` while keeping the
   existing PostgreSQL service, volumes, ports, user mapping, and environment.

## Failure Handling

- A compilation or architecture build failure prevents all publication for that
  run; the prior registry tags remain available.
- The immutable SHA tag provides a rollback target and is never intentionally
  overwritten.
- The workflow will verify the resulting image metadata and inspect the
  multi-platform manifest after publishing.
- Runtime validation will confirm that the image retains the expected entrypoint,
  exposed port, health check, and `/app/mediastorm` binary path.

## User-Facing Usage

The existing Compose file needs only this service image change:

```yaml
services:
  mediastorm:
    image: ghcr.io/jeor/mediastorm:latest
```

For a rollback, replace `latest` with the reported `sha-<short-sha>` tag and run
`docker compose pull && docker compose up -d`.

## Verification

- Validate workflow YAML and Dockerfile structure locally.
- Run the backend's relevant Go test suite before publication.
- Confirm the publishing workflow completes successfully on the merged commit.
- Inspect the GHCR manifest for both `linux/amd64` and `linux/arm64`.
- Report the exact published tags and Compose image reference.

## Scope

This change adds image packaging and publication only. It does not alter runtime
configuration, database schemas, APIs, application behavior, or the existing
Docker Hub image.
