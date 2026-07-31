# Doko — Declarative OCI Kit Orchestrator

> Declarative, hardened OCI container images — without writing a single Dockerfile.
>
> *"Where is my container configured?" -> doko (In the simple YAML spec).*

[![Build](https://github.com/broadsage/doko/actions/workflows/build.yml/badge.svg)](https://github.com/broadsage/doko/actions/workflows/build.yml)
[![Verify](https://github.com/broadsage/doko/actions/workflows/verify.yml/badge.svg)](https://github.com/broadsage/doko/actions/workflows/verify.yml)
[![Test Examples](https://github.com/broadsage/doko/actions/workflows/test-examples.yml/badge.svg)](https://github.com/broadsage/doko/actions/workflows/test-examples.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Go version](https://img.shields.io/github/go-mod/go-version/broadsage/doko)](go.mod)
[![Latest Release](https://img.shields.io/github/v/release/broadsage/doko)](https://github.com/broadsage/doko/releases/latest)

**Doko** is an open-source [BuildKit](https://github.com/moby/buildkit) frontend that compiles a simple `doko.yaml` spec into a minimal, hardened, policy-compliant OCI container image — all from a single declarative file.

---

## Table of Contents

- [Why Doko?](#why-doko)
- [Quick Start](#quick-start)
- [Key Features](#key-features)
- [Documentation](#documentation)
- [Project Structure](#project-structure)
- [Development](#development)
- [Security](#security)
- [License](#license)

---

## Why Doko?

Traditional Dockerfiles are imperative, error-prone, and difficult to audit. Existing alternatives each have significant limitations:

| Feature | Docker DHI | Chainguard (apko) | **Doko** |
| :--- | :--- | :--- | :--- |
| Target OS | Alpine, Debian | Wolfi/Alpine only | **Alpine** |
| Multi-Stage Builds | Yes | No | **Yes (with `outputs:`)** |
| Negative Packages | Yes (`!pkg`) | No | **Yes (`!pkg`)** |
| Open Source | Partial | Yes | **100% Open Source** |

---

## Quick Start

### 1. Create a `doko.yaml`

```yaml
# syntax=ghcr.io/broadsage/doko:v1

name: secure-web-app

accounts:
  root: false
  run-as: nonroot
  users:
    - name: nonroot
      uid: 65532
      gid: 65532
  groups:
    - name: nonroot
      gid: 65532
      members:
        - nonroot

contents:
  packages:
    - nginx
    - "!telnet"
  paths:
    - type: directory
      path: /var/www/html
      uid: 65532
      gid: 65532
      mode: "0755"

work-dir: /var/www/html
entrypoint: ["nginx", "-g", "daemon off;"]

runtime:
  ports:
    - 8080
```

### 2. Build with Docker Buildx

```bash
docker buildx build -f doko.yaml --tag my-secure-app:latest --load .
```

No Dockerfile. No shell scripts. No post-build scanning.

---

## Key Features

**Declarative YAML Syntax**
Define packages, directories, users, ports, and entrypoint in a single auditable file. Every security decision is visible and reviewable in version control.

**APK Package Provider**
Use `apk` for Alpine.

**Negative Package Removal**
Prefix any package with `!` to strip it from the base image:
```yaml
contents:
  packages:
    - nginx
    - "!telnet"
    - "!gawk"
```

**Multi-Stage Sub-Builds**
Compile artifacts in an isolated build stage and copy them into the final image declaratively:
```yaml
builds:
  - name: app-builder
    outputs:
      - source: /usr/local/bin/app
        target: /usr/local/bin/app
    contents:
      packages:
        - go
      pipeline:
        - name: compile
          runs: go build -o /usr/local/bin/app ./cmd/app
```

**Custom Package Compilation Pipelines**
Build packages from source using reusable pipeline templates (e.g. `fetch`, `configure`, `make`, `install`) and inject the resulting `.apk` files directly into the final image:
```yaml
builds:
  - name: my-custom-pkg
    version: "1.2.3"
    license: MIT
    pipeline:
      - uses: fetch
        with:
          uri: https://example.com/src.tar.gz
      - uses: configure
      - uses: make
      - uses: install
```

**External OCI Artifact Imports**
Pull individual files from any OCI image without defining a full build stage:
```yaml
artifacts:
  - name: ghcr.io/your-org/gosu:1.17
    includes:
      - /usr/local/bin/gosu
```

**Root Purging**
`accounts.root: false` (the default) removes root entirely from `/etc/passwd` and `/etc/group`, eliminating UID 0 privilege escalation.

**OCI-Native Layout**
All annotations follow the [OCI Image Spec](https://github.com/opencontainers/image-spec/blob/main/annotations.md).

---

## Documentation

| Document | Description |
|---|---|
| [Schema Reference](docs/schema.md) | Complete reference for all `doko.yaml` fields, types, and examples |
| [Release Process](docs/release.md) | Automated and manual release workflow, versioning, and GoReleaser |
| [Contributing](CONTRIBUTING.md) | Dev environment setup, testing, linting, and PR guidelines |
| [Examples](examples/) | Ready-to-use `doko.yaml` configs for Nginx, PostgreSQL, Redis, and Python API |

---

## Project Structure

```
cmd/doko/           — BuildKit frontend entrypoint
docs/               — Project documentation
examples/           — Example doko.yaml configs with integration tests
hack/               — Developer tooling scripts
internal/
  builder/          — BuildKit frontend gateway builder orchestrator
  config/           — doko.yaml schema, types, and validation
  llb/              — BuildKit LLB translation engine
  pipeline/         — Reusable pipeline template engine (vars, steps)
  providers/        — Unified provider registry (resolvers + builders)
    apk/            — Alpine APKINDEX resolver and LLB builder
      builder/      — APK package compilation engine and pipeline templates
  utils/            — Unified utility helpers (strings, BuildKit)
```

---

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for full setup instructions. Quick reference using Taskfile:

```bash
# Bootstrap containerised development environment
task devenv

# Build, test, and lint locally
task generate                       # regenerate schema.json when structs change
task build
task test
task lint

# Run end-to-end test against nginx example
task test-example pkg=nginx

# GoReleaser snapshot build
goreleaser build --snapshot --clean
```

---

## Security

Doko is a security-focused project. If you discover a vulnerability, please **do not open a public issue**. Report it privately via [GitHub Security Advisories](https://github.com/broadsage/doko/security/advisories/new).

---

## License

By contributing to Doko, you agree that your contributions will be licensed under the [Apache-2.0 License](LICENSE).

