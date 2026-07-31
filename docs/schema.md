# Doko Schema Reference

**Version:** 1.0  
**Specification file:** `doko.yaml`

---

## Overview

A `doko.yaml` file is the single source of truth for building a hardened, policy-compliant OCI container image. It replaces traditional Dockerfiles with a fully declarative, auditable YAML schema.

Doko validates the schema at **build-time** and rejects any configuration that violates declared security policies — before a single layer is committed.

---

## Table of Contents

- [Top-Level Fields](#top-level-fields)
- [`vars` — Build-Time Variables](#vars--build-time-variables)
- [`environment` — Runtime Environment](#environment--runtime-environment)
- [`annotations` — OCI Annotations](#annotations--oci-annotations)
- [`os-release` — OS Identity Override](#os-release--os-identity-override)
- [`security` — Security Policy & Sandbox Profiles](#security--security-policy--sandbox-profiles)
- [`accounts` — Users & Groups](#accounts--users--groups)
- [`builds` — Multi-Stage Sub-Builds](#builds--multi-stage-sub-builds)
- [`contents` — Main Stage Contents](#contents--main-stage-contents)
  - [`contents.packages`](#contentspackages)
  - [`contents.repositories`](#contentsrepositories)
  - [`contents.keyring`](#contentskeyring)
  - [`contents.paths`](#contentspaths)
  - [`contents.pipeline`](#contentspipeline)
  - [Available Pipeline Templates](#available-pipeline-templates)
- [`artifacts` — External OCI Artifacts](#artifacts--external-oci-artifacts)
- [`runtime` — Container Runtime Configuration](#runtime--container-runtime-configuration)
- [`stop-signal` — Process Stop Signal](#stop-signal--process-stop-signal)
- [`timeout-seconds` — Network Request Timeout](#timeout-seconds--network-request-timeout)
- [Complete Example](#complete-example)

---

## Top-Level Fields

These fields are defined at the root level of `doko.yaml`.

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | `string` | ✅ Yes | Human-readable name for the image. Used in build logs and metadata. |
| `image` | `string` | No | Target OCI image reference (e.g. `ghcr.io/org/repo/name`). |
| `variant` | `string` | No | A label describing the image variant (e.g. `runtime`, `dev`, `production`). |
| `tags` | `[]string` | No | List of tags to apply to the built image (e.g. `latest`, `1.26.3`). |
| `platforms` | `[]string` | No | Target platforms (e.g. `linux/amd64`, `linux/arm64`). |
| `arch` | `string` | No | Primary target architecture (`amd64`, `arm64`). Defaults to `amd64`. |
| `dates` | `map[string]string` | No | Metadata dates (e.g. `release`, `end-of-life`). Arbitrary key-value pairs. |
| `vars` | `map[string]string` | No | Build-time variable substitution map. See [`vars`](#vars--build-time-variables). |
| `environment` | `map[string]string` | No | Runtime environment variables set in the final OCI image config. |
| `annotations` | `map[string]string` | No | OCI manifest annotations. |
| `os-release` | `object` | No | Override the `/etc/os-release` file inside the image. |
| `security` | `object` | No | Security policies and runtime sandbox profiles. |
| `accounts` | `object` | No | User and group definitions inside the image. |
| `builds` | `[]object` | No | Named multi-stage sub-builds (analogous to `FROM ... AS` in Dockerfiles). |
| `contents` | `object` | No | Packages, paths, and pipeline for the main (final) image stage. |
| `artifacts` | `[]object` | No | External OCI images from which to import files directly. |
| `runtime` | `object` | No | OCI runtime configuration (user, ports, env). |
| `stop-signal` | `string` | No | Override the default process stop signal (e.g. `SIGINT`, `SIGTERM`). |
| `timeout-seconds` | `int` | No | Network request timeout for package resolvers and scanners (defaults to `30`). |
| `work-dir` | `string` | No | Sets the working directory inside the container. |
| `entrypoint` | `[]string` | No | The process launched as PID 1 inside the container. |
| `cmd` | `[]string` | No | Default arguments passed to the entrypoint. |



---

## `vars` — Build-Time Variables

**Purpose:** Define build-time macro variables that are substituted into the rest of the YAML before parsing. Variables use the `${VAR_NAME}` syntax.

> **Advantage:** Keeps version numbers, paths, and configuration values DRY and in a single location. Changes to a version only require updating the `vars` block.

```yaml
vars:
  NGINX_VERSION: "1.26.3"
  NGINX_PKG: "nginx"
  WORKDIR: "/app"
```

Use variables anywhere in the file:

```yaml
environment:
  NGINX_VERSION: "${NGINX_VERSION}"   # expands to "1.26.3"

contents:
  packages:
    - "${NGINX_PKG}"                  # expands to "nginx"
```

> **Note:** `vars` are build-time only. They are NOT set as runtime environment variables in the final image. Use `environment:` for that.

---

## `environment` — Runtime Environment

**Purpose:** Define environment variables that are baked into the final OCI image configuration (`Config.Env`). These are set for every process that runs inside the container.

```yaml
environment:
  LANGUAGE: "en_US:en"
  NGINX_VERSION: "${NGINX_VERSION}"
  PATH: "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
```

> **Advantage over `runtime.env`:** `environment:` is the top-level canonical place for image-wide environment variables, consistent with DHI and OCI spec standards. `runtime.env` is also supported and merged with `environment:` into the final config.

---

## `annotations` — OCI Annotations

**Purpose:** Attach key-value metadata to the OCI image manifest, following the [OCI Image Spec annotation keys](https://github.com/opencontainers/image-spec/blob/main/annotations.md).

```yaml
annotations:
  org.opencontainers.image.title: "Nginx stable"
  org.opencontainers.image.description: "Hardened Nginx web server built with Doko"
  org.opencontainers.image.url: "https://github.com/your-org/your-repo"
  org.opencontainers.image.source: "https://github.com/your-org/your-repo"
  org.opencontainers.image.version: "${NGINX_VERSION}"
  org.opencontainers.image.authors: "Your Org <oss@your-org.com>"
```

**Standard OCI annotation keys:**

| Key | Description |
|---|---|
| `org.opencontainers.image.title` | Human-readable image title |
| `org.opencontainers.image.description` | Short description of the image |
| `org.opencontainers.image.version` | Version of the packaged software |
| `org.opencontainers.image.authors` | Contact details for the maintainer |
| `org.opencontainers.image.url` | URL to find more info about the image |
| `org.opencontainers.image.source` | Source code repository URL |
| `org.opencontainers.image.created` | Build timestamp (auto-set by Doko) |

> **Advantage:** Annotations are stored in the OCI manifest descriptor — not the image config or a layer — ensuring they are available for supply-chain tooling (Cosign, Notation, Rekor) without polluting the image filesystem.

---

## `os-release` — OS Identity Override

**Purpose:** Override the contents of `/etc/os-release` inside the final image. This allows enterprise teams to brand their hardened base images with a custom identity while retaining the underlying OS distribution.

```yaml
os-release:
  name: Broadsage Secured Images (Alpine)
  id: alpine
  version-id: "3.20"
  pretty-name: "Broadsage Secured Images / Alpine Linux v3.20"
  home-url: https://your-org.com/products/secured-images/
  bug-report-url: https://your-org.com/support/
```

| Field | Description |
|---|---|
| `name` | Full name of the OS (maps to `NAME=`) |
| `id` | OS identifier (maps to `ID=`). Should match the actual base (e.g. `alpine`). |
| `version-id` | Version string (maps to `VERSION_ID=`) |
| `version-codename` | Codename, if applicable (maps to `VERSION_CODENAME=`) |
| `pretty-name` | Human-readable name (maps to `PRETTY_NAME=`) |
| `home-url` | URL for OS home page (maps to `HOME_URL=`) |
| `bug-report-url` | URL for reporting issues (maps to `BUG_REPORT_URL=`) |

---

## `security` — Security Policy & Sandbox Profiles

**Purpose:** Define compile-time security gates and auto-generated runtime sandbox profiles. This is the core of Doko's hardening capability.

```yaml
security:
  policy:
    fail-on-cve: high
    allowed-licenses:
      - MIT
      - Apache-2.0
      - BSD-3-Clause
    custom-rego: |
      package doko
      deny[msg] {
        input.package == "telnet"
        msg := "telnet is not permitted in hardened images"
      }
    vex:
      format: openvex
      path: ./security/cve-exceptions.json
  sbom:
    formats:
      - spdx
      - cyclonedx
  hardening:
    remove-package-manager: true
    lock-shell-accounts: true
    sysctl:
      net.ipv4.ip_forward: "0"
    read-only-rootfs: true
  profiles:
    - seccomp
    - landlock
```

### `security.policy`

| Field | Type | Description |
|---|---|---|
| `fail-on-cve` | `string` | Minimum CVE severity that fails the build. Values: `critical`, `high`, `medium`, `low`, `none`. |
| `allowed-licenses` | `[]string` | Allowlist of SPDX license identifiers. Build fails if a package uses an unlisted license. |
| `custom-rego` | `string` | Inline [Open Policy Agent (OPA)](https://www.openpolicyagent.org/) Rego policy for advanced custom rules. |
| `vex` | `object` | VEX exception list configuration (requires `format` and `path`). |

### `security.sbom`

| Field | Type | Description |
|---|---|---|
| `formats` | `[]string` | Target SBOM formats to generate. Supported: `spdx`, `cyclonedx`. |

### `security.hardening`

| Field | Type | Description |
|---|---|---|
| `remove-package-manager` | `bool` | Purge package manager binaries in final stage. |
| `lock-shell-accounts` | `bool` | Set default shell to `/sbin/nologin` for non-root users. |
| `sysctl` | `map[string]string` | Key-value list of sysctl kernel parameters to configure. |
| `read-only-rootfs` | `bool` | Restrict dynamic write permissions via Landlock. |

**`fail-on-cve` Severity Levels:**

| Level | Description |
|---|---|
| `critical` | Only fail on CVSS >= 9.0 |
| `high` | Fail on CVSS >= 7.0 (recommended for production) |
| `medium` | Fail on CVSS >= 4.0 |
| `low` | Fail on any known CVE |
| `none` | Never fail on CVEs (not recommended) |

### `security.profiles`

Declares which runtime sandbox profiles to auto-generate and embed into the image as OCI annotations:

| Profile | Description |
|---|---|
| `seccomp` | Generates a minimal `seccomp` JSON profile allowing only the syscalls required by declared packages. |
| `landlock` | Generates a `landlock` policy restricting filesystem access to only declared paths. |

### `security.privileged`

Declares whether the final target container requires privileged execution mode at runtime.

When set to `true`, Doko attaches the OCI annotation `com.broadsage.bsi.privileged: "true"` to the OCI manifest metadata. Container runtimes and orchestrators (like Kubernetes Security Policies) can read this metadata to authorize and execute the container with elevated host privileges.

---

## `accounts` — Users & Groups

**Purpose:** Declaratively define all users and groups that exist inside the image. Doko writes these entries to `/etc/passwd`, `/etc/group`, and `/etc/shadow`.

```yaml
accounts:
  root: false          # Purge root user/group from the image (hardened default)
  run-as: postgres     # Set the default USER in the OCI config
  users:
    - name: postgres
      uid: 70
      gid: 70
  groups:
    - name: postgres
      gid: 70
      members:
        - postgres
```

| Field | Type | Description |
|---|---|---|
| `root` | `bool` | If `false` (default), root user and group are purged from `/etc/passwd` and `/etc/group`, enforcing a truly rootless image. If `true`, root entries are preserved. |
| `run-as` | `string` | The username to set as the default `USER` in the OCI config. Must exist in `users`. |
| `users` | `[]User` | List of users to create. Each requires `name`, `uid`, and `gid`. |
| `groups` | `[]Group` | List of groups to create. Each requires `name` and `gid`. Optional `members` list. |

> **Advantage:** Removing the `root` user from the image entirely (not just switching to a non-root user) prevents privilege escalation via UID 0 even if a container breakout occurs. Doko automates this hardening step.

---

## `builds` — Multi-Stage Sub-Builds

**Purpose:** Define named build stages that produce artifacts (binaries, configuration, compiled assets) without those build tools being present in the final image. Mirrors `FROM ... AS builder` in Dockerfiles but with a declarative, self-documenting schema.

Builds can also define **custom package compilation pipelines** using reusable template steps (e.g. `fetch`, `configure`, `make`, `install`). When a build spec includes `version` and `pipeline` fields, Doko compiles the package from source in an isolated BuildKit stage, assembles a local `.apk` archive, and injects it directly into the final image.

### Standard Sub-Build (artifact copy)

```yaml
builds:
  - name: app-builder
    base: golang:1.22-alpine
    provider: apk
    outputs:
      - source: /usr/local/bin/app
        target: /usr/local/bin/app
        uid: 65532
        gid: 65532
    contents:
      packages:
        - go
        - git
      paths:
        - type: directory
          path: /build
          uid: 0
          gid: 0
          mode: "0755"
      pipeline:
        - name: compile
          runs: |
            cd /build && go build -o /usr/local/bin/app ./cmd/app
```

### Custom Package Compilation Pipeline

```yaml
builds:
  - name: my-custom-lib
    version: "2.4.1"
    epoch: 0
    description: "Custom library compiled from source"
    url: https://example.com/my-custom-lib
    license: MIT
    dependencies:
      - libc-dev
      - gcc
    source-dir: /home/build
    pipeline:
      - uses: fetch
        with:
          uri: https://example.com/my-custom-lib-2.4.1.tar.gz
          expected-sha256: abc123...
      - uses: configure
        with:
          opts: --prefix=/usr --disable-static
      - uses: make
      - uses: install
```

| Field | Type | Description |
|---|---|---|
| `name` | `string` | Required. Unique identifier for this build stage. Referenced in build logs. |
| `version` | `string` | Package version string. Required for custom compilation pipelines. |
| `epoch` | `int` | Package epoch for versioning overrides (defaults to `0`). |
| `description` | `string` | Human-readable description embedded in the compiled package metadata. |
| `url` | `string` | Upstream project URL for the package. |
| `license` | `string` | SPDX license identifier for the compiled package. |
| `dependencies` | `[]string` | Build-time dependencies installed in the compilation stage. |
| `base` | `string` | Override the base image for this specific stage (e.g. `golang:1.22-alpine`). |
| `provider` | `string` | Override the package manager for this stage (`apk`). Auto-detected from `base` if omitted. |
| `work-dir` | `string` | Sets the working directory context inside the sub-build container for pipeline execution. |
| `source-dir` | `string` | Sets the source directory for compilation pipeline steps (defaults to `/home/build`). |
| `privileged` | `bool` | Runs the sub-build stage pipeline commands in privileged mode (enables `security.insecure` entitlement). |
| `outputs` | `[]Output` | **Producer-Push model.** Declares which paths this stage exports into the final image. |
| `contents` | `object` | Packages, paths, and pipeline steps for this stage. Supports all [`contents`](#contents--main-stage-contents) sub-fields. |
| `pipeline` | `[]PipelineStep` | Build-level pipeline steps for custom package compilation. Supports `uses` template references. |

### `builds[].outputs`

| Field | Type | Description |
|---|---|---|
| `source` | `string` | Required. Absolute path of the file or directory **inside the sub-build container** to export. |
| `target` | `string` | Required. Destination path **inside the final image**. |
| `uid` | `int` | UID to set on the copied artifact in the final image. |
| `gid` | `int` | GID to set on the copied artifact in the final image. |

> **Advantage over Dockerfile `COPY --from`:** Sub-builds declare their own `outputs:`, making them self-contained units. Reviewers can audit exactly what each stage produces without reading the final stage definition.

---

## `contents` — Main Stage Contents

**Purpose:** Define the packages, paths, and pipeline steps that make up the **final shipped image**.

### `contents.packages`

Install or remove packages using the configured package manager (`apk`).

```yaml
contents:
  packages:
    - nginx         # Install nginx
    - curl          # Install curl
    - "!telnet"     # Remove telnet from the base image
    - "!gawk"       # Remove gawk from the base image
```

**Negative packages (prefix `!`):** Prefixing a package name with `!` removes it from the base image. Doko runs the appropriate package manager removal command (`apk del`).

> **Advantage:** Directly reduces the attack surface by stripping unnecessary utilities from the base without requiring a custom base image.

### `contents.repositories`

Add extra package manager repository sources.

```yaml
contents:
  repositories:
    - https://dl-cdn.alpinelinux.org/alpine/edge/community
```

### `contents.keyring`

Add package signing keyring URLs for package verification.

```yaml
contents:
  keyring:
    - https://dl-cdn.alpinelinux.org/alpine/v3.20/main/x86_64/APKINDEX.tar.gz
```

### `contents.ca-certificates`

Add custom root CA certificate files or URLs to install in the trust store before package resolution.

```yaml
contents:
  ca-certificates:
    - ./certs/internal-root-ca.crt
    - https://pki.internal.corp/ca.crt
```

### `contents.paths`

**Purpose:** Declaratively create directories or copy local files with explicit ownership and permissions. **Replaces** ad-hoc `mkdir -p`, `chown`, and `chmod` shell commands in pipeline steps.

```yaml
contents:
  paths:
    # Create an empty directory with strict ownership
    - type: directory
      path: /var/lib/postgres/data
      uid: 70
      gid: 70
      mode: "0700"

    # Copy a local file/directory from the build context
    - type: file
      path: /app
      source: ./src
      uid: 65532
      gid: 65532
      mode: "0755"
```

| Field | Type | Description |
|---|---|---|
| `type` | `string` | `directory` — creates an empty directory. `file` — copies from the local build context. |
| `path` | `string` | Required. Absolute destination path inside the image. |
| `source` | `string` | Source path relative to build context. Required when `type: file`. |
| `uid` | `int` | User ID to set as owner. |
| `gid` | `int` | Group ID to set as owner. |
| `mode` | `string` | UNIX permission mode string (e.g. `"0700"`, `"0755"`). |

> **Advantage:** Declarative paths are auditable and verifiable in code review. Security teams can confirm exact directory permissions directly in the YAML without reading shell scripts.

### `contents.pipeline`

Execute custom shell commands as discrete, named build steps. Use this **only for logic that cannot be expressed declaratively** (e.g. writing generated config files, creating symlinks, running initialisation scripts).

Pipeline steps support two modes:
1. **Inline scripts** — Use `runs:` to execute arbitrary shell commands.
2. **Reusable templates** — Use `uses:` to reference a named pipeline template (e.g. `fetch`, `configure`, `make`, `install`). Templates are resolved from the provider's embedded step definitions.

```yaml
contents:
  pipeline:
    # Inline script step
    - name: write-nginx-config
      runs: |
        cat << 'EOF' > /etc/nginx/http.d/default.conf
        server {
          listen 8080 default_server;
        }
        EOF

    # Template-based step (for custom package compilation)
    - uses: fetch
      with:
        uri: https://example.com/src.tar.gz
        expected-sha256: abc123...
```

| Field | Type | Description |
|---|---|---|
| `name` | `string` | Human-readable label for this step. Shown in BuildKit build logs. |
| `runs` | `string` | Shell script executed via `/bin/sh -c`. Mutually exclusive with `uses`. |
| `uses` | `string` | Name of a reusable pipeline template to execute (e.g. `fetch`, `configure`, `make`, `install`). Mutually exclusive with `runs`. |
| `with` | `map[string]any` | Key-value parameters passed to a template step. Only used with `uses`. |
| `ssh` | `boolean` | Optional. Set to `true` to mount the host's forwarded SSH agent socket securely at `/run/ssh-agent.sock` and set the `SSH_AUTH_SOCK` environment variable. |
| `secrets` | `[]PipelineSecret` | Optional. List of BuildKit secret mounts to make available during this step. Each requires `id` and `target`. |
| `network` | `string` | Optional. Network mode for this step (e.g. `none`, `host`). Defaults to the BuildKit default. |

### Available Pipeline Templates

| Template | Description | Key `with` Parameters |
|---|---|---|
| `fetch` | Download and extract a source archive | `uri`, `expected-sha256`, `extract` |
| `configure` | Run `./configure` with options | `opts` |
| `make` | Run `make` with optional targets | `opts`, `install-dir` |
| `install` | Run `make install` | `prefix` |
| `patch` | Apply patches | `patches` |
| `cmake` | CMake build step | `opts` |
| `meson` | Meson build step | `opts` |
| `strip` | Strip debug symbols from binaries | `paths` |

> **Best Practice:** Do not use `pipeline` to create directories or set ownership — use `paths:` for those. Reserve `pipeline` for logic that genuinely requires shell execution or template-based compilation.

---

## `artifacts` — External OCI Artifacts

**Purpose:** Import specific files directly from external, pre-built OCI images without requiring a full sub-build stage. Ideal for pulling hardened, pre-compiled binaries from trusted registries.

```yaml
artifacts:
  - name: ghcr.io/your-org/gosu:1.17
    includes:
      - /usr/local/bin/gosu
    uid: 0
    gid: 0

  - name: ghcr.io/your-org/tini:0.19
    includes:
      - /tini
```

| Field | Type | Description |
|---|---|---|
| `name` | `string` | Required. Fully qualified OCI image reference to import from. |
| `includes` | `[]string` | Required. Absolute paths inside the source image to copy into the final image. |
| `uid` | `int` | UID to set on the imported files in the final image. |
| `gid` | `int` | GID to set on the imported files in the final image. |

> **Advantage over sub-builds:** When you only need a single binary from an external image, `artifacts:` is far cleaner than defining a full `builds:` stage. Registry authentication is handled transparently by the BuildKit daemon using the host's credential store.

---

## `runtime` — Container Runtime Configuration

**Purpose:** Define the OCI container runtime configuration — equivalent to Dockerfile's `USER`, `EXPOSE`, and `ENV`.

```yaml
runtime:
  user: nonroot
  ports:
    - 8080
  env:
    DEBUG: "false"
```

| Field | Type | Description |
|---|---|---|
| `user` | `string` | Username or UID to run the container process as. Overrides `accounts.run-as` if both are set. |
| `ports` | `[]int` | Ports to expose. Informational only (equivalent to Dockerfile `EXPOSE`). |
| `env` | `map[string]string` | Additional environment variables. Merged with the top-level `environment:` block. |



## `stop-signal` — Process Stop Signal

**Purpose:** Override the OS signal sent to PID 1 when the container runtime stops the container. Defaults to `SIGTERM` if not set.

```yaml
stop-signal: SIGINT
```

Accepted values: Any valid POSIX signal name — `SIGTERM`, `SIGINT`, `SIGQUIT`, `SIGUSR1`, `SIGUSR2`, etc.

> **Example use case:** PostgreSQL uses `SIGINT` for a fast shutdown and `SIGTERM` for a smart shutdown. Setting `stop-signal: SIGINT` ensures `docker stop` triggers the expected behaviour.

---

## `timeout-seconds` — Network Request Timeout

**Purpose:** Configure the network request timeout in seconds for fetching remote package indices, vulnerability scanning APIs, and CA certificates.

```yaml
timeout-seconds: 15
```

- **Default value:** `30`
- **Accepted values:** Any positive integer representing seconds.

---

## `work-dir` — Working Directory

**Purpose:** Set the working directory inside the container for runtime execution (equivalent to the `WORKDIR` instruction in a `Dockerfile`).

```yaml
work-dir: /app
```

Accepted values: Any valid absolute path.

> **Advantage:** Elevating `work-dir` to a top-level field matches enterprise schema standards like apko and Docker DHI, ensuring working directories are explicitly visible at the root of the definition rather than nested within runtime configuration.

---

## `entrypoint` — Entrypoint

**Purpose:** The process launched as PID 1 inside the container (equivalent to the `ENTRYPOINT` instruction in a `Dockerfile`).

```yaml
entrypoint: ["nginx", "-g", "daemon off;"]
```

---

## `cmd` — Command

**Purpose:** Default arguments passed to the entrypoint (equivalent to the `CMD` instruction in a `Dockerfile`).

```yaml
cmd: ["-c", "/etc/nginx/nginx.conf"]
```



---

## Complete Example

A production-grade, fully annotated `doko.yaml` for a hardened Nginx image:

```yaml
# syntax=ghcr.io/broadsage/doko:v1

name: Nginx stable
image: ghcr.io/your-org/nginx
variant: runtime
tags:
  - latest
  - "1.26.3"
  - 1.26.3-alpine3.20
platforms:
  - linux/amd64
  - linux/arm64
dates:
  release: "2026-07-16"
  end-of-life: "2027-07-16"

vars:
  NGINX_VERSION: "1.26.3"
  NGINX_PKG: "nginx"

environment:
  NGINX_VERSION: "${NGINX_VERSION}"
  LANGUAGE: "en_US:en"

annotations:
  org.opencontainers.image.title: "Nginx stable"
  org.opencontainers.image.description: "Hardened Nginx built with Doko"
  org.opencontainers.image.version: "${NGINX_VERSION}"
  org.opencontainers.image.authors: "Your Org <oss@your-org.com>"

os-release:
  name: "Your Org Secured Images (Alpine)"
  id: alpine
  version-id: "3.20"
  pretty-name: "Your Org Secured Images / Alpine Linux v3.20"
  home-url: https://your-org.com/products/secured-images/
  bug-report-url: https://your-org.com/support/

security:
  policy:
    fail-on-cve: high
    allowed-licenses:
      - MIT
      - Apache-2.0
      - BSD-2-Clause
  profiles:
    - seccomp
    - landlock
  privileged: false

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

builds:
  - name: config-builder
    outputs:
      - source: /etc/nginx/http.d/default.conf
        target: /etc/nginx/http.d/default.conf
    contents:
      paths:
        - type: directory
          path: /etc/nginx/http.d
          uid: 65532
          gid: 65532
          mode: "0755"
      pipeline:
        - name: write-vhost-config
          runs: |
            cat << 'EOF' > /etc/nginx/http.d/default.conf
            server {
              listen 8080 default_server;
              root /var/www/html;
              index index.html;
            }
            EOF

contents:
  packages:
    - "${NGINX_PKG}"
    - "!telnet"
    - "!gawk"
  paths:
    - type: directory
      path: /var/www/html
      uid: 65532
      gid: 65532
      mode: "0755"
    - type: directory
      path: /run/nginx
      uid: 65532
      gid: 65532
      mode: "0755"
    - type: directory
      path: /var/log/nginx
      uid: 65532
      gid: 65532
      mode: "0755"
  pipeline:
    - name: configure-log-symlinks
      runs: |
        ln -sf /dev/stdout /var/log/nginx/access.log
        ln -sf /dev/stderr /var/log/nginx/error.log

stop-signal: SIGTERM

timeout-seconds: 15

work-dir: /var/www/html

entrypoint: ["nginx", "-g", "daemon off;"]

runtime:
  ports:
    - 8080
```
