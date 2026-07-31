#!/usr/bin/env bash
# hack/make-dev.sh — Bootstrap a consistent doko development environment.
#
# This script manages the Docker-based dev container. For day-to-day
# build/test/lint commands, use the Taskfile directly (e.g. `task build`).
#
# Usage:
#   ./hack/make-dev.sh             # Launch interactive dev container (default)
#   ./hack/make-dev.sh rebuild     # Force rebuild the dev container image
#   ./hack/make-dev.sh help        # Show this help message

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

cmd_rebuild() {
    log_info "Force rebuilding dev container image..."
    build_devenv_image
}

usage() {
    cat <<EOF
Usage: ./hack/make-dev.sh [command]

Commands:
  (none)    Launch an interactive dev container (default)
  rebuild   Force rebuild the dev container image
  help      Show this message

For build, test, lint, and tool checks use the Taskfile:
  task build          Compile the doko binary locally
  task test           Run the Go test suite
  task lint           Run golangci-lint
  task check          Verify all required tools

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
    "")                   cmd_run     ;;
    "rebuild")            cmd_rebuild ;;
    "help"|"--help"|"-h") usage      ;;
    *) log_error "Unknown command: $1"; echo; usage; exit 1 ;;
esac
