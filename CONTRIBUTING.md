# Contributing

Thank you for your interest in Doko! You are welcome here.

Doko is an open-source BuildKit frontend for building minimal, hardened, and policy-compliant OCI container images. We welcome contributions of all kinds — bug reports, feature ideas, documentation improvements, and code.

If you are looking for a place to start, take a look at the [open issues](https://github.com/broadsage/doko/issues), especially those marked with [good first issue](https://github.com/broadsage/doko/issues?q=is%3Aopen+is%3Aissue+label%3A%22good+first+issue%22).

---

## Prerequisites

Before contributing code, make sure you have the following tools installed:

| Tool | Version | Purpose |
|---|---|---|
| [Go](https://go.dev/dl/) | 1.25+ | Build and test |
| [Docker](https://docs.docker.com/engine/install/) | 24+ | Run builds and dev environment |
| [golangci-lint](https://golangci-lint.run/usage/install/) | v2.12.2 | Code linting |
| [goreleaser](https://goreleaser.com/install/) | v2 | Release snapshot builds |
| [cosign](https://docs.sigstore.dev/cosign/system_config/installation/) | v2 | Image signing |

Verify your setup at any time:

```bash
./hack/make-devenv.sh check
```

---

## Setting Up a Development Environment

To make development easier across all platforms, we provide a script that bootstraps a fully configured development container using Docker.

Run the following from the root of the repository:

```bash
./hack/make-devenv.sh
```

This will:
1. Build a dev container image with all required tools pre-installed
2. Mount the doko repository at `/workspace` inside the container
3. Drop you into an interactive shell

Once inside the container, you can build and run doko:

```
[doko] ❯ go build -o ./doko ./cmd/doko/
[doko] ❯ ./doko version
Doko - BuildKit Image Orchestrator v1.0.0-dev
Commit: unknown
BuildTime: unknown
Go version: go1.25
OS/Arch: linux/amd64
```

When you are done, type `exit` to leave the container. Your local repository changes are preserved — the directory is mounted, not copied.

---

## Local Development

You can also develop directly on your machine without Docker.

### Build

```bash
go build -o ./doko ./cmd/doko/
```

With version metadata injected:

```bash
CGO_ENABLED=0 go build \
  -ldflags "-X main.version=v1.0.0-dev -X main.commit=$(git rev-parse HEAD) -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o doko ./cmd/doko/
```

### Test

```bash
# Run all tests
go test ./... -race -count=1

# Run a specific package
go test ./internal/vulnerability/... -v

# Run example config tests
go test ./examples/... -v -count=1
```

Or inside Docker:

```bash
./hack/make-devenv.sh test
```

### Lint

```bash
golangci-lint run ./...
```

Or inside Docker:

```bash
./hack/make-devenv.sh lint
```

### Build and test the doko frontend image locally

The `make-devenv.sh` commands work on the Go source. To build the actual doko
OCI frontend image and run it against an example spec, use `make-dokoenv.sh`:

```bash
# Build the doko:local image from the root Dockerfile
./hack/make-dokoenv.sh build

# End-to-end test against an example (nginx by default)
./hack/make-dokoenv.sh test

# Test a specific example (nginx / redis / postgres / python-api)
./hack/make-dokoenv.sh test redis
```

The `test` command patches the `# syntax=` line in the example's `doko.yaml` to
point at `doko:local` (no registry pull), runs `docker buildx build --load`, and
loads the resulting image into your local Docker image store as
`doko-test-<example>:local`.

```bash
goreleaser build --snapshot --clean
```

---

## Commit Messages

Doko uses **[Conventional Commits](https://www.conventionalcommits.org/)** for all commit messages. The automated release workflow reads commit history to determine the next version and generate release notes — so well-formed commit messages directly drive the changelog.

### Format

```
<type>(<scope>): <short summary>
```

### Types

| Type | When to use |
|---|---|
| `feat` | A new feature |
| `fix` | A bug fix |
| `chore` | Build process, tooling, or dependency updates |
| `docs` | Documentation changes only |
| `test` | Adding or updating tests |
| `refactor` | Code change that neither fixes a bug nor adds a feature |
| `perf` | Performance improvements |
| `ci` | CI configuration changes |

### Examples

```
feat(resolver): add secure mirror fallback for APKINDEX downloads
fix(policy): correctly evaluate medium severity CVEs against threshold
chore(deps): update github.com/moby/buildkit to v0.32.0
docs(schema): add work-dir field documentation
test(vulnerability): add scanner batch size edge case test
```

### Breaking changes

Add `!` after the type or include `BREAKING CHANGE:` in the footer:

```
feat(config)!: rename provider field to package-manager

BREAKING CHANGE: the `provider` field in doko.yaml has been renamed to
`package-manager`. Update all existing doko.yaml files accordingly.
```

---

## Pull Request Process

1. **Fork** the repository and create a branch from `main`
2. **Make your changes** — keep them focused on a single concern
3. **Add or update tests** for any changed behaviour
4. **Run lint and tests** locally before pushing:
   ```bash
   go test ./... -race -count=1
   golangci-lint run ./...
   ```
5. **Open a pull request** against `main` with a clear description of what changed and why
6. **Address review feedback** — we aim to review PRs within a few days

All PRs must pass the following CI checks before merging:
- `Build` — binary compiles cleanly
- `Verify` (lint + tests + `go mod tidy`)
- `Test Examples` — all example configs resolve correctly
- `Snyk` — no new high/critical vulnerabilities

---

## Code Style

- Follow standard Go formatting: run `gofmt -w .` or `gofumpt -w .`
- Imports are organized by `goimports` with the local prefix `github.com/broadsage/doko`
- All exported types, functions, and packages must have doc comments
- Errors from external packages must be wrapped with `%w`
- HTTP requests must use context-aware methods — pass `ctx` through call chains
- Do not use `print`, `println`, or `fmt.Print*` for debug output — use structured logging

---

## Project Structure

```
cmd/doko/          — BuildKit frontend entrypoint and version info
internal/
  config/          — doko.yaml schema, types, and parsing
  llb/             — BuildKit LLB translation engine
  netutil/         — Shared HTTP client utilities
  pipeline/        — Reusable pipeline template engine (vars, steps)
  policy/          — Compile-time CVE and license policy gates
  provenance/      — SLSA provenance attestation generation
  providers/       — Unified provider registry (resolvers + builders)
    apk/           — Alpine APKINDEX resolver and LLB builder
      builder/     — APK package compilation engine and pipeline templates
  sbom/            — CycloneDX and SPDX SBOM generation
  security/        — Seccomp and Landlock profile generators
  vulnerability/   — OSV.dev CVE scanner and VEX matcher
docs/              — Project documentation
examples/          — Example doko.yaml configs with integration tests
hack/              — Developer tooling scripts
```

---

## Release Process

Releases are automated — see [docs/release.md](docs/release.md) for full details.

In short:
- Releases run automatically every Monday via GitHub Actions
- The release workflow reads conventional commits to determine the next patch version
- Minor version bumps are triggered manually via the [Release workflow](https://github.com/broadsage/doko/actions/workflows/release.yml)
- Artifacts are built with GoReleaser and signed with Cosign

---

## Reporting Issues

When reporting a bug, please include:
- The `doko.yaml` that triggered the issue (redact sensitive values)
- The full error output
- Your platform and architecture (`uname -a`)
- The doko version (`./doko version`)

---

## License

By contributing to Doko, you agree that your contributions will be licensed under the [Apache-2.0 License](LICENSE).
