# Plan: Release Manifold with a Seamless Embedded PostgreSQL Runtime

## Goal

A novice user downloads one Manifold release artifact for their OS/architecture, runs the Manifold executable, and the application starts without requiring Docker, Homebrew, apt, Chocolatey, a separately installed PostgreSQL server, or manual extension setup.

For Manifold, embedded PostgreSQL must include these **required** extensions:

- `pgvector`
- `postgis`
- `pgrouting`

PostGIS and pgRouting are not optional. A release artifact that cannot initialize all three extensions must fail CI and must not be published as a stable release.

## Current repository starting point

Relevant existing files:

- `.github/workflows/ci.yml`
  - Already supports `workflow_dispatch`.
  - Currently builds Go-only artifacts and does not build or embed a complete PostgreSQL runtime.
- `internal/embeddedpg/embeddedpg.go`
  - Uses `github.com/fergusstrange/embedded-postgres` to download PostgreSQL binaries on first run.
  - Defaults empty embedded version to PostgreSQL 18.
- `internal/embeddedpg/extensions.go`
  - Installs `pgvector`, `postgis`, and `pgrouting` by downloading extension tarballs from a release URL, then falls back to local system discovery.
  - This is not sufficient for novice users because it depends on network availability and/or system packages.
- `scripts/package-pg-extensions.sh`
  - Packages extensions from already-installed system/Homebrew extension files.
  - Useful as a prototype, but not sufficient as the release source of truth because it does not build a full deterministic PostgreSQL runtime.

## Product decision

Use a **self-extracting native PostgreSQL runtime** bundled into each Manifold platform artifact.

Important nuance: PostgreSQL and its extensions are native executables/shared libraries. They cannot realistically run directly from Go memory. The Manifold binary can still feel like a single-binary product by embedding a compressed runtime payload with `go:embed`, then extracting it on first run into Manifold's cache directory.

User-visible behavior:

1. User downloads `manifold-<version>-<os>-<arch>`.
2. User runs it.
3. Manifold extracts its embedded PostgreSQL runtime to `~/.manifold/runtimes/postgres/<runtime-id>` or the platform equivalent.
4. Manifold initializes/starts PostgreSQL locally.
5. Manifold creates/verifies `vector`, `postgis`, and `pgrouting`.
6. Manifold starts the web app.

No system PostgreSQL installation is required.

## Runtime support matrix

Target release artifacts:

| OS | Arch | Artifact |
| --- | --- | --- |
| Linux | amd64 | `manifold-<version>-linux-amd64.tar.gz` |
| Linux | arm64 | `manifold-<version>-linux-arm64.tar.gz` |
| macOS | amd64 | `manifold-<version>-darwin-amd64.tar.gz` |
| macOS | arm64 | `manifold-<version>-darwin-arm64.tar.gz` |
| Windows | amd64 | `manifold-<version>-windows-amd64.zip` |
| Windows | arm64 | `manifold-<version>-windows-arm64.zip` |

If Windows arm64 cannot be completed immediately with the PostgreSQL/PostGIS/pgRouting dependency chain, it should be tracked as a release blocker for "every user" support or explicitly marked as a preview gap. Do not silently publish a Windows arm64 artifact that falls back to missing extensions.

## Pin versions

Do not default to the latest PostgreSQL major version.

Initial recommendation:

- PostgreSQL major: `17`
- Runtime patch: pinned by manifest, for example `17.x`
- `pgvector`: pinned, currently cataloged as `0.8.2`
- `postgis`: pinned, currently cataloged as `3.6.2`
- `pgrouting`: pinned, currently cataloged as `4.0.1`

The exact patch versions must be stored in a runtime manifest and treated as part of the release artifact identity.

Example runtime ID:

```text
postgres-17.5-pgvector-0.8.2-postgis-3.6.2-pgrouting-4.0.1-linux-amd64
```

## New artifact format

Create one compressed PostgreSQL runtime bundle per OS/architecture:

```text
postgres-runtime-<runtime-id>.tar.zst      # Linux/macOS
postgres-runtime-<runtime-id>.zip          # Windows
postgres-runtime-<runtime-id>.sha256
postgres-runtime-<runtime-id>.manifest.json
```

Each runtime bundle must contain:

```text
bin/                         # postgres, initdb, pg_ctl, psql, createdb, etc.
lib/                         # PostgreSQL libs and non-system runtime deps
lib/postgresql/              # extension shared libraries
share/postgresql/            # PostgreSQL share files
share/postgresql/extension/  # extension .control and .sql files
licenses/                    # third-party notices and licenses
manifest.json                # exact file list, versions, checksums
```

Required manifest fields:

```json
{
  "schemaVersion": 1,
  "runtimeID": "postgres-17.x-pgvector-0.8.2-postgis-3.6.2-pgrouting-4.0.1-linux-amd64",
  "os": "linux",
  "arch": "amd64",
  "postgres": {
    "major": 17,
    "version": "17.x"
  },
  "extensions": {
    "vector": "0.8.2",
    "postgis": "3.6.2",
    "pgrouting": "4.0.1"
  },
  "dependencies": [],
  "files": [
    {
      "path": "bin/postgres",
      "mode": "0755",
      "sha256": "..."
    }
  ]
}
```

## Code changes required

### 1. Add embedded runtime asset package

Add a package such as:

```text
internal/embeddedpg/assets/
  assets.go
  manifest.go
  runtimes/
    .gitkeep
```

During release builds, GitHub Actions will place exactly one platform runtime payload under this package before compiling the platform-specific Manifold binary.

The Go package should expose:

```go
type RuntimeAsset struct {
    RuntimeID string
    OS        string
    Arch      string
    PGMajor   int
    Archive   []byte
    Manifest  Manifest
}

func Current() (RuntimeAsset, error)
```

### 2. Change embedded PostgreSQL startup order

Update `internal/embeddedpg` startup to use this order:

1. Look for an embedded runtime asset matching `runtime.GOOS/runtime.GOARCH`.
2. Extract it into Manifold's runtime cache if not already present.
3. Verify every file against the embedded manifest checksum.
4. Initialize the data directory if needed.
5. Start the bundled PostgreSQL binaries.
6. Run required extension verification.
7. Only if explicitly configured for development, allow remote download or system fallback.

The release path must not depend on:

- `apt`
- Homebrew
- Chocolatey
- system PostgreSQL
- an extension download URL
- existing files outside the Manifold cache/data directories

### 3. Make required extension verification fatal

After PostgreSQL starts, Manifold must run:

```sql
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pgrouting;
```

Then verify:

```sql
SELECT extname FROM pg_extension WHERE extname IN ('vector', 'postgis', 'pgrouting');
SELECT postgis_full_version();
SELECT pgr_version();
```

If any extension is unavailable, startup must fail with a clear novice-friendly error that includes:

- OS/architecture
- PostgreSQL runtime ID
- extension that failed
- path to the Manifold diagnostic log
- instruction to report the issue with the support bundle

Do not silently continue with missing PostGIS/pgRouting.

### 4. Pin the default embedded PostgreSQL major

Change the embedded default away from PostgreSQL 18 and pin it to the runtime-supported major, initially PostgreSQL 17.

Update:

- `internal/embeddedpg/embeddedpg.go`
- `config.yaml.example`
- docs that mention `embeddedVersion`

### 5. Keep remote/system extension install only as a development escape hatch

The current `EmbeddedExtensionURL` and system-copy fallback can remain only for development if useful, but release builds must prefer embedded assets and must not reach this path unless an explicit development flag is set, for example:

```yaml
databases:
  embeddedAllowExternalRuntimeResolution: false
```

Default for release artifacts must be false.

### 6. Add a doctor/smoke-test command

Add a command or subcommand that CI can call without guessing internal paths:

```bash
manifold embedded-postgres doctor --json
```

It should:

- extract/verify the embedded runtime
- initialize a temporary data directory
- start PostgreSQL on a random loopback port
- create `vector`, `postgis`, and `pgrouting`
- run extension version probes
- shut PostgreSQL down
- return machine-readable JSON

This command is the main devtest and release smoke-test gate.

## Build scripts to add

Add scripts under `scripts/release/`.

### `scripts/release/build-postgres-runtime.sh`

Purpose: build a deterministic runtime bundle for one OS/architecture.

Inputs:

```bash
--pg-version <17.x>
--pg-major 17
--pgvector-version 0.8.2
--postgis-version 3.6.2
--pgrouting-version 4.0.1
--target-os linux|darwin|windows
--target-arch amd64|arm64
--output-dir dist/postgres-runtime
```

Responsibilities:

- Build or assemble PostgreSQL.
- Build `pgvector`, `postgis`, and `pgrouting` against the exact PostgreSQL runtime.
- Bundle non-system shared library dependencies.
- Normalize paths and dynamic library search behavior.
- Generate `manifest.json`.
- Generate checksums.
- Generate license notices.
- Run an isolated smoke test before producing the final artifact.

### `scripts/release/test-postgres-runtime.sh`

Purpose: validate a runtime bundle without Manifold.

Required checks:

- `initdb` succeeds in a temporary directory.
- `postgres` starts on loopback with a random port.
- `CREATE EXTENSION vector` succeeds.
- `CREATE EXTENSION postgis` succeeds.
- `CREATE EXTENSION pgrouting` succeeds.
- `SELECT postgis_full_version()` succeeds.
- `SELECT pgr_version()` succeeds.
- PostgreSQL shuts down cleanly.

### `scripts/release/embed-runtime-asset.sh`

Purpose: copy one runtime bundle into the Go asset package before building the platform-specific Manifold binary.

Inputs:

```bash
--runtime-archive <path>
--runtime-manifest <path>
--target-os <os>
--target-arch <arch>
```

Responsibilities:

- Clean prior generated runtime assets.
- Copy the selected archive and manifest into `internal/embeddedpg/assets/runtimes/`.
- Generate `internal/embeddedpg/assets/generated_runtime.go` with build tags or constants for the selected runtime.

### `scripts/release/smoke-manifold-artifact.sh`

Purpose: validate the final release artifact as a user would run it.

Required checks:

- Use a temporary HOME/AppData equivalent so no developer machine state leaks in.
- Do not read `.env`.
- Start Manifold with embedded DB enabled.
- Wait for the web server to return HTTP 200 on loopback.
- Verify `manifold embedded-postgres doctor --json` passes.
- Verify logs do not show fallback to system PostgreSQL or missing extensions.

## GitHub Actions workflows

Create three new workflows and update the existing CI workflow.

### 1. `.github/workflows/ci.yml`

Keep the existing CI workflow, but make sure it also runs on pull requests and main branch pushes:

```yaml
on:
  pull_request:
  push:
    branches: [main]
  workflow_dispatch:
```

CI should remain fast and not build the full native PostgreSQL runtime on every PR unless explicitly requested.

Recommended jobs:

- Go tests/lint.
- Unit tests for runtime manifest/extraction logic using tiny test fixtures.
- No full PostGIS/pgRouting build in normal CI.

### 2. `.github/workflows/build-postgres-runtime.yml`

Purpose: build and test PostgreSQL runtime bundles.

Triggers:

```yaml
on:
  workflow_dispatch:
    inputs:
      pg_version:
        required: true
        default: "17.x"
      pgvector_version:
        required: true
        default: "0.8.2"
      postgis_version:
        required: true
        default: "3.6.2"
      pgrouting_version:
        required: true
        default: "4.0.1"
      upload_to_release:
        required: true
        default: "false"
        type: choice
        options: ["false", "true"]
      release_tag:
        required: false
  workflow_call:
    inputs:
      pg_version:
        required: true
        type: string
      pgvector_version:
        required: true
        type: string
      postgis_version:
        required: true
        type: string
      pgrouting_version:
        required: true
        type: string
```

Matrix:

```yaml
strategy:
  fail-fast: false
  matrix:
    include:
      - target_os: linux
        target_arch: amd64
        runner: ubuntu-22.04
      - target_os: linux
        target_arch: arm64
        runner: ubuntu-22.04-arm
      - target_os: darwin
        target_arch: amd64
        runner: macos-13
      - target_os: darwin
        target_arch: arm64
        runner: macos-14
      - target_os: windows
        target_arch: amd64
        runner: windows-2022
      - target_os: windows
        target_arch: arm64
        runner: windows-2022-arm
```

If a listed hosted runner is unavailable in the repository plan, replace it with a self-hosted runner label or a buildx/cross-build strategy. The workflow must still produce one runtime per target platform before stable release.

Important implementation notes:

- Linux should build on an old-enough glibc baseline and bundle non-glibc shared libraries needed by PostGIS/pgRouting.
- macOS should build/sign native binaries and fix `install_name`/`rpath` so dependencies resolve after extraction.
- Windows should bundle required `.dll` files and verify startup from a path containing spaces.
- Every matrix cell must run `scripts/release/test-postgres-runtime.sh` before uploading artifacts.

Artifacts uploaded by each matrix cell:

```text
postgres-runtime-*.tar.zst
postgres-runtime-*.zip
postgres-runtime-*.manifest.json
postgres-runtime-*.sha256
runtime-smoke-*.json
```

### 3. `.github/workflows/build-manifold-release.yml`

Purpose: build final Manifold artifacts that contain the matching embedded PostgreSQL runtime.

Triggers:

```yaml
on:
  workflow_dispatch:
    inputs:
      version:
        required: true
      runtime_artifact_run_id:
        required: false
      publish_release:
        required: true
        default: "false"
        type: choice
        options: ["false", "true"]
  workflow_call:
    inputs:
      version:
        required: true
        type: string
```

Matrix should match the runtime matrix.

Per matrix cell:

1. Checkout repository.
2. Set up Go.
3. Set up frontend toolchain.
4. Download the matching PostgreSQL runtime artifact.
5. Run `scripts/release/embed-runtime-asset.sh`.
6. Build frontend assets.
7. Build Manifold with the release tags.
8. Package the artifact.
9. Generate checksums.
10. Upload matrix artifact.

Build command should be based on the existing target:

```bash
make build-manifold
```

or the equivalent explicit command:

```bash
go build -tags "forge,embedded_pg_runtime" -o dist/manifold ./cmd/agentd
```

Release package contents:

```text
manifold                  # or manifold.exe
README-embedded-postgres.txt
THIRD_PARTY_NOTICES.txt
checksums.txt             # optional per archive, also uploaded separately
```

The extracted package should not require users to see or manage the PostgreSQL runtime files manually.

### 4. `.github/workflows/smoke-release-artifacts.yml`

Purpose: test the final user-facing artifacts on clean runners.

Triggers:

```yaml
on:
  workflow_dispatch:
    inputs:
      version:
        required: true
      artifact_run_id:
        required: false
  workflow_call:
    inputs:
      version:
        required: true
        type: string
```

Matrix should match release artifacts.

Per matrix cell:

1. Download the final Manifold artifact.
2. Extract it into a temporary directory with spaces in the path.
3. Set HOME/AppData to a fresh temporary directory.
4. Run `manifold embedded-postgres doctor --json`.
5. Start Manifold with embedded DB enabled.
6. Wait for HTTP 200 from the local web server.
7. Assert logs do not contain:
   - `extension not available`
   - `system discovery`
   - `apt`
   - `homebrew`
   - `fallback`
8. Stop Manifold.
9. Upload logs and doctor JSON as artifacts.

### 5. `.github/workflows/release.yml`

Purpose: orchestrate the full release.

Triggers:

```yaml
on:
  push:
    tags:
      - "v*"
  workflow_dispatch:
    inputs:
      version:
        description: "Version/tag to build, for example v0.8.0-devtest.1"
        required: true
      publish:
        description: "Create/update a GitHub Release"
        required: true
        default: "false"
        type: choice
        options: ["false", "true"]
      prerelease:
        required: true
        default: "true"
        type: choice
        options: ["true", "false"]
```

Release orchestration jobs:

```text
validate-version
build-postgres-runtimes
smoke-postgres-runtimes
build-manifold-artifacts
smoke-manifold-artifacts
create-or-update-github-release
```

Rules:

- `publish=false` should run the entire pipeline and upload workflow artifacts only. This is the devtest path.
- `publish=true` should create or update a draft/prerelease/stable GitHub Release after all smoke tests pass.
- Tag pushes should publish only after all required jobs pass.
- Stable releases must not be published if any matrix cell fails.
- Devtest prereleases may be published only if clearly marked prerelease and all selected matrix cells pass.

## Manual devtest flow

During development, maintainers should be able to manually test the entire pipeline without creating a stable release.

Recommended devtest sequence:

1. Run `Build PostgreSQL Runtime` manually.
   - Use default pinned versions.
   - Confirm all runtime matrix artifacts pass smoke tests.
2. Run `Build Manifold Release` manually.
   - Point it at the runtime build artifacts or let the orchestrator pass them through.
   - Set `publish_release=false`.
3. Run `Smoke Release Artifacts` manually.
   - Confirm all Manifold artifacts pass on clean runners.
4. Run `Release` manually with:
   - `version=vX.Y.Z-devtest.N`
   - `publish=false` for dry run, or `publish=true` with `prerelease=true` for a GitHub prerelease.
5. Only after devtest succeeds, create/push the real release tag `vX.Y.Z` or run `Release` with `publish=true` and `prerelease=false`.

## Release gates

A release is blocked if any of these fail:

- Runtime manifest checksum verification.
- `initdb` with bundled runtime.
- PostgreSQL start on clean runner.
- `CREATE EXTENSION vector`.
- `CREATE EXTENSION postgis`.
- `CREATE EXTENSION pgrouting`.
- `SELECT postgis_full_version()`.
- `SELECT pgr_version()`.
- Final Manifold artifact starts with a fresh user home.
- Web server returns HTTP 200.
- Logs show remote/system PostgreSQL fallback in release mode.
- macOS signing/notarization fails for stable release.
- Windows signing fails for stable release, if signing is configured as required.

## Platform-specific implementation notes

### Linux

- Build against a conservative glibc baseline.
- Bundle GEOS, PROJ, GDAL, Boost, pgRouting, PostGIS, and other non-glibc dependencies as needed.
- Set runtime library paths so PostgreSQL can load extensions after extraction.
- Validate on at least Ubuntu LTS and one non-Ubuntu Linux runner/container if possible.

### macOS

- Build separately for `amd64` and `arm64`.
- Use `install_name_tool` to rewrite dylib references to extraction-relative paths.
- Sign the final Manifold binary and bundled native runtime files.
- Notarize stable release artifacts.
- Smoke test after signing/notarization, not before only.

### Windows

- Bundle `postgres.exe`, `initdb.exe`, `pg_ctl.exe`, required PostgreSQL DLLs, PostGIS DLLs, pgRouting DLLs, GEOS/PROJ/GDAL/Boost DLLs, and extension SQL/control files.
- Test from a path containing spaces.
- Ensure Manifold starts PostgreSQL as a child process, not as a Windows service.
- Bind only to loopback.
- Handle antivirus-sensitive extraction by using a stable cache directory and checksummed files.

## Data directory and upgrade policy

- Runtime directory and data directory must be separate.
- Store metadata in the data directory indicating PostgreSQL major version and runtime ID.
- Patch-level runtime upgrades may reuse an existing data directory after compatibility checks.
- Major PostgreSQL upgrades require an explicit migration path using `pg_dump`/restore or `pg_upgrade` with both old and new runtimes available.
- Avoid changing PostgreSQL major versions frequently.

## Security requirements

- Bind embedded PostgreSQL only to loopback.
- Use a generated local password or local socket authentication where supported.
- Do not expose the embedded database externally.
- Restrict runtime/data directory permissions.
- Verify embedded runtime checksums before execution.
- Never execute files from a mutable cache directory if checksums fail.

## Observability and support bundle

Add a support bundle command or endpoint that collects:

- Manifold version.
- OS/architecture.
- Embedded runtime ID.
- PostgreSQL version.
- Extension versions.
- Startup logs.
- Last runtime verification error.

Do not include secrets.

## Migration path from current implementation

Phase 1: Runtime manifest and doctor command

- Add manifest types and extraction verification with test fixtures.
- Add `manifold embedded-postgres doctor --json`.
- Make required extension verification fatal.
- Pin default embedded PostgreSQL major to 17.

Phase 2: Runtime bundle builder

- Add `scripts/release/build-postgres-runtime.sh`.
- Add `scripts/release/test-postgres-runtime.sh`.
- Build Linux amd64 first until green.
- Add Linux arm64, macOS amd64/arm64, Windows amd64/arm64.

Phase 3: Embedded asset build

- Add `internal/embeddedpg/assets`.
- Add `scripts/release/embed-runtime-asset.sh`.
- Build one platform-specific Manifold binary embedding one matching runtime.

Phase 4: GitHub Actions

- Add `build-postgres-runtime.yml`.
- Add `build-manifold-release.yml`.
- Add `smoke-release-artifacts.yml`.
- Add orchestration `release.yml`.
- Update `ci.yml` triggers.

Phase 5: Release hardening

- Add macOS signing/notarization.
- Add Windows signing.
- Add third-party notices.
- Add upgrade tests.
- Add clean-machine smoke tests for all supported platforms.

## Definition of done

The work is complete when:

1. A maintainer can manually run the release workflow from GitHub Actions with `publish=false` and get all platform artifacts as workflow artifacts.
2. A maintainer can manually run the release workflow with `publish=true` and create a prerelease.
3. A tag push can create a stable release only after all runtime and final-artifact smoke tests pass.
4. Each release artifact contains a Manifold executable that can start on a clean machine without Docker, system PostgreSQL, apt, Homebrew, Chocolatey, or internet access.
5. Manifold verifies `pgvector`, `postgis`, and `pgrouting` before completing startup.
6. Failure messages are clear enough for a novice user to report the issue without understanding PostgreSQL internals.
