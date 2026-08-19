# Security Policy

We take the security of Doko seriously. If you believe you have found a security vulnerability, please report it to us using the instructions below.

---

## Supported Versions

Security fixes are actively applied to the latest release of Doko. We do not backport patches to older minor or major versions unless explicitly specified.

| Version | Supported |
| :--- | :---: |
| Latest Release | ✅ Yes |
| All Older Versions | ❌ No |

---

## Reporting a Vulnerability

> [!IMPORTANT]
> **Please do not report security vulnerabilities through public GitHub issues, pull requests, or public discussions.**

If you discover a security vulnerability, please report it privately using one of the following methods:

### 1. GitHub Private Vulnerability Reporting (Preferred)
GitHub provides a secure, private channel to report vulnerabilities directly to the maintainers:
1. Navigate to the [Doko Security Tab](https://github.com/broadsage/doko/security) on GitHub.
2. Click **"Report a vulnerability"**.
3. Complete the form with details, including reproduction steps or a proof-of-concept, and click submit.

This keeps the discussion entirely private between you and the project maintainers until a patch is released.

### 2. Email
If you prefer not to use GitHub's interface, you can report vulnerabilities via email. Send details to the security contact listed on the [Broadsage GitHub Organization Page](https://github.com/broadsage).

When reporting, please include:
- A detailed description of the vulnerability and its potential impact.
- Step-by-step instructions to reproduce the issue (including any config specs or commands).
- The exact version of Doko and the BuildKit environment you are running.
- Any proposed mitigations or fix implementations (optional).

---

## Response & Disclosure Process

We follow a coordinated disclosure model. Once a report is submitted, we will:

1. **Acknowledgement**: Acknowledge receipt of your report within **48 hours (2 business days)**.
2. **Validation & Assessment**: Investigate the report to verify the vulnerability and determine its severity. We will keep you updated on our progress.
3. **Remediation**: Prepare a security patch in a private fork.
4. **Coordinated Release**: Release the patch in a new version of Doko. We will coordinate the release date with you and credit you in the release notes unless you prefer to remain anonymous.
5. **CVE Assignment**: Request a CVE identifier through the GitHub Advisory Database for significant vulnerabilities.

---

## Scope

### In Scope
- The `doko` binary CLI and the Go packages under `internal/` and `cmd/`.
- The official BuildKit frontend images published at `ghcr.io/broadsage/doko`.
- The repository build configurations (`Dockerfile` and `Dockerfile.goreleaser`).
- The project CI/CD workflows (`.github/workflows`).

### Out of Scope
- Known public vulnerabilities in upstream dependencies (e.g., BuildKit or Go libraries) — please report these to the respective upstream maintainers.
- Unofficial third-party builds, forks, or packaging scripts.

---

## Security Best Practices for Users

To keep your builds secure and reproducible, we recommend implementing the following practices:

### 1. Pin the Frontend Syntax By Digest
Always pin the `# syntax=` header in your `doko.yaml` files to a specific, immutable SHA256 digest rather than a floating tag (like `:latest` or `:v1`):
```yaml
# syntax=ghcr.io/broadsage/doko@sha256:1a40477d945f9f61306eb254f664484cdb4354f5ab8ab1b9bcc8762fe64c816b
```

### 2. Verify Signatures Natively
Before running or building with Doko images in CI/CD or production, verify their cryptographic signatures using [Cosign](https://github.com/sigstore/cosign):
```bash
cosign verify ghcr.io/broadsage/doko:<version> \
  --certificate-identity-regexp "^https://github.com/broadsage/doko/.github/workflows/release.yml" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
```

### 3. Review Image Attestations
Inspect the Software Bill of Materials (SBOM) and SLSA provenance metadata attached to the OCI index registry entry of every release to verify supply-chain compliance.
