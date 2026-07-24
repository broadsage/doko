#!/usr/bin/env bash
# hack/make-devenv.sh — Bootstrap a consistent doko development environment.
#
# Usage:
#   ./hack/make-devenv.sh             # Launch interactive dev container (default)
#   ./hack/make-devenv.sh check       # Verify all required local tools are installed
#   ./hack/make-devenv.sh build       # Build the doko binary inside Docker
#   ./hack/make-devenv.sh test        # Run the full test suite inside Docker
#   ./hack/make-devenv.sh lint        # Run golangci-lint inside Docker
#   ./hack/make-devenv.sh rebuild     # Force rebuild the dev container image
#   ./hack/make-devenv.sh help        # Show this help message

set -euo pipefail


readonly REPO_MODULE="github.com/broadsage/doko"
readonly DEVENV_IMAGE="doko-devenv"
readonly DEVENV_TAG="latest"
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Parse Go version from go.mod (e.g. "go 1.25.9" → "1.25")
GO_VERSION="$(grep -m1 '^go ' "${REPO_ROOT}/go.mod" | awk '{print $2}' | cut -d. -f1,2)"
readonly GO_VERSION

# Tool versions — keep in sync with .github/workflows/verify.yml
readonly GOLANGCI_LINT_VERSION="v2.12.2"
readonly GORELEASER_VERSION="v2.9.0"
readonly BUILDX_VERSION="v0.23.0"

# ── Logging ───────────────────────────────────────────────────────────────────
# Defined before arch detection so log_error is available in the case block.

log_info()  { echo "[doko-devenv] $*"; }
log_error() { echo "[doko-devenv] ERROR: $*" >&2; }
die()       { log_error "$*"; exit 1; }

# ── Architecture detection ────────────────────────────────────────────────────

ARCH="$(uname -m)"
case "${ARCH}" in
    x86_64)        ARCH="amd64"  ;;
    aarch64|arm64) ARCH="arm64"  ;;
    *) die "Unsupported architecture: ${ARCH}" ;;
esac
readonly ARCH


check_repo() {
    [[ -f "${REPO_ROOT}/go.mod" ]] \
        || die "go.mod not found — run from the doko repository root."
    grep -q "^module ${REPO_MODULE}$" "${REPO_ROOT}/go.mod" \
        || die "Not in the doko repository. Expected module: ${REPO_MODULE}"
}


check_tools() {
    # Required for build, test, lint
    local required_tools=(docker go golangci-lint)
    # Only needed for cutting releases
    local optional_tools=(goreleaser cosign)

    log_info "Required tools:"
    local -i missing=0
    for tool in "${required_tools[@]}"; do
        if command -v "${tool}" &>/dev/null; then
            local ver
            ver="$("${tool}" version 2>/dev/null | head -1 || echo "unknown")"
            log_info "  ✓ ${tool}: ${ver}"
        else
            log_error "  ✗ ${tool}: not found"
            missing=1
        fi
    done

    echo
    log_info "Optional tools (needed for releases only):"
    for tool in "${optional_tools[@]}"; do
        if command -v "${tool}" &>/dev/null; then
            local ver
            ver="$("${tool}" version 2>/dev/null | head -1 || echo "unknown")"
            log_info "  ✓ ${tool}: ${ver}"
        else
            log_info "  - ${tool}: not found (install only if cutting releases)"
        fi
    done

    if [[ "${missing}" -ne 0 ]]; then
        echo
        log_error "Missing required tools. Install them:"
        log_info "  go:            https://go.dev/dl/"
        log_info "  docker:        https://docs.docker.com/engine/install/"
        log_info "  golangci-lint: https://golangci-lint.run/usage/install/ (version ${GOLANGCI_LINT_VERSION})"
        exit 1
    fi

    log_info "All required tools are available."
}


image_tag() {
    echo "${DEVENV_IMAGE}:${DEVENV_TAG}-${ARCH}"
}

build_devenv_image() {
    log_info "Building dev container image (go${GO_VERSION}, linux/${ARCH})..."

    docker build \
        --platform "linux/${ARCH}" \
        --build-arg "GO_VERSION=${GO_VERSION}" \
        --build-arg "GOLANGCI_LINT_VERSION=${GOLANGCI_LINT_VERSION}" \
        --build-arg "GORELEASER_VERSION=${GORELEASER_VERSION}" \
        --build-arg "BUILDX_VERSION=${BUILDX_VERSION}" \
        --tag "$(image_tag)" \
        --file "${SCRIPT_DIR}/Dockerfile.devenv" \
        "${REPO_ROOT}"

    log_info "Dev container image built: $(image_tag)"
}

ensure_image() {
    if ! docker image inspect "$(image_tag)" &>/dev/null; then
        log_info "Dev container image not found — building..."
        build_devenv_image
    fi
}


# Run a command inside the dev container with the repo mounted.
run_in_devenv() {
    ensure_image
    docker run --rm \
        --platform "linux/${ARCH}" \
        --workdir /workspace \
        --volume "${REPO_ROOT}:/workspace" \
        --volume "/var/run/docker.sock:/var/run/docker.sock" \
        "$(image_tag)" \
        "$@"
}


cmd_run() {
    ensure_image
    log_info "Starting interactive dev environment (type 'exit' to leave)..."
    log_info "Source code is mounted at /workspace"
    echo

    docker run --rm -it \
        --platform "linux/${ARCH}" \
        --workdir /workspace \
        --volume "${REPO_ROOT}:/workspace" \
        --volume "/var/run/docker.sock:/var/run/docker.sock" \
        --env "GOFLAGS=-mod=mod" \
        "$(image_tag)" \
        /bin/bash -i
}

cmd_build() {
    log_info "Building doko binary (linux/${ARCH})..."
    run_in_devenv go build -o ./doko ./cmd/doko/
    log_info "Binary built: ./doko"
}

cmd_test() {
    log_info "Running tests..."
    run_in_devenv go test ./... -race -count=1
    log_info "All tests passed."
}

cmd_lint() {
    log_info "Running golangci-lint..."
    run_in_devenv golangci-lint run ./...
    log_info "Lint passed."
}

cmd_rebuild() {
    log_info "Force rebuilding dev container image..."
    build_devenv_image
}

usage() {
    cat <<EOF
Usage: ./hack/make-devenv.sh [command]

Commands:
  (none)    Launch an interactive dev container (default)
  check     Verify all required local tools are installed
  build     Build the doko binary inside Docker
  test      Run the full test suite inside Docker
  lint      Run golangci-lint inside Docker
  rebuild   Force rebuild the dev container image
  help      Show this message

Environment:
  Go version:         ${GO_VERSION} (from go.mod)
  golangci-lint:      ${GOLANGCI_LINT_VERSION}
  goreleaser:         ${GORELEASER_VERSION}
  Architecture:       ${ARCH}
  Dev image:          $(image_tag)
EOF
}


check_repo

case "${1:-}" in
    "")                cmd_run     ;;
    "check")           check_tools ;;
    "build")           cmd_build   ;;
    "test")            cmd_test    ;;
    "lint")            cmd_lint    ;;
    "rebuild")         cmd_rebuild ;;
    "help"|"--help"|"-h") usage   ;;
    *) log_error "Unknown command: $1"; echo; usage; exit 1 ;;
esac
