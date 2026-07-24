# Security Policy

## Supported Versions

Only the latest release of Doko receives security fixes. We do not backport patches to older versions.

| Version | Supported |
| ------- | --------- |
| Latest  | ✅        |
| Older   | ❌        |

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

If you discover a security vulnerability, please report it privately using one of the following methods:

### GitHub Private Vulnerability Reporting (preferred)

Use GitHub's built-in private vulnerability reporting:

1. Go to the [Security tab](https://github.com/broadsage/doko/security) of this repository.
2. Click **"Report a vulnerability"**.
3. Fill in the details and submit.

We will receive a private notification and respond within **5 business days**.

### Email

If you prefer email, send details to the maintainers via the contact listed on the [Broadsage GitHub Organisation](https://github.com/broadsage).

Please include:

- A description of the vulnerability and its potential impact
- Steps to reproduce or a proof-of-concept
- The affected version(s)
- Any suggested mitigations (optional)

## Response Process

1. **Acknowledgement** — we will acknowledge receipt within 5 business days.
2. **Assessment** — we will assess severity and scope, and communicate a timeline.
3. **Fix & disclosure** — a patch will be prepared and released. We follow [coordinated disclosure](https://en.wikipedia.org/wiki/Coordinated_vulnerability_disclosure): we will notify you before publishing and credit you in the release notes unless you prefer to remain anonymous.
4. **CVE** — for significant vulnerabilities we will request a CVE via GitHub's advisory system.

## Scope

The following are **in scope**:

- The `doko` binary and its Go packages under `internal/` and `cmd/`
- The `Dockerfile` and `Dockerfile.goreleaser` build images
- The published OCI images at `ghcr.io/broadsage/doko`
- The CI/CD workflows (supply-chain attacks, secret leakage, etc.)

The following are **out of scope**:

- Vulnerabilities in upstream dependencies that are already publicly known — please report those to the upstream maintainer
- Issues in forks or unofficial distributions

## Security Best Practices for Users

- Always pin the `# syntax=` directive to a specific digest rather than a floating tag:
  ```dockerfile
  # syntax=ghcr.io/broadsage/doko@sha256:<digest>
  ```
- Verify the image signature before use with [cosign](https://github.com/sigstore/cosign):
  ```bash
  cosign verify ghcr.io/broadsage/doko:<version> \
    --certificate-identity-regexp "https://github.com/broadsage/doko/.github/workflows/release.yml" \
    --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
  ```
- Check the SBOM and provenance attestations attached to each release for supply-chain transparency.

## Dependency Updates

We use [Snyk](https://snyk.io) and GitHub's dependency graph to monitor for known vulnerabilities in dependencies. Critical CVEs are patched as soon as a fix is available upstream.
