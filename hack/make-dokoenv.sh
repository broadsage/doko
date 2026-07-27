#!/usr/bin/env bash
# hack/make-dokoenv.sh — Build and locally test the doko BuildKit frontend image.
#
# All commands run directly on the host (no dev container involved).
# Requires: docker, docker buildx
#
# Usage:
#   ./hack/make-dokoenv.sh             # Show help (default)
#   ./hack/make-dokoenv.sh check       # Verify docker + buildx are available
#   ./hack/make-dokoenv.sh build       # Build doko:local from the root Dockerfile
#   ./hack/make-dokoenv.sh test        # End-to-end test: nginx example → local image
#   ./hack/make-dokoenv.sh test redis  # End-to-end test: redis example → local image
#   ./hack/make-dokoenv.sh help        # Show this help message

set -euo pipefail

readonly LOCAL_IMAGE="doko:local"
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
readonly EXAMPLES_DIR="${REPO_ROOT}/examples"
readonly VALID_EXAMPLES=(nginx redis postgres python-api)


log_info()  { echo "[doko] $*"; }
log_error() { echo "[doko] ERROR: $*" >&2; }
die()       { log_error "$*"; exit 1; }


ARCH="$(uname -m)"
case "${ARCH}" in
    x86_64)        ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) die "Unsupported architecture: ${ARCH}" ;;
esac
readonly ARCH


check_repo() {
    [[ -f "${REPO_ROOT}/go.mod" ]] \
        || die "go.mod not found — run from the doko repository root."
}

check_tools() {
    local -i missing=0

    log_info "Checking required tools:"
    for tool in docker; do
        if command -v "${tool}" &>/dev/null; then
            local ver
            ver="$("${tool}" --version 2>/dev/null | head -1 || echo "unknown")"
            log_info "  ✓ ${tool}: ${ver}"
        else
            log_error "  ✗ ${tool}: not found"
            missing=1
        fi
    done

    # docker buildx can be a plugin (docker buildx) or a standalone binary
    if docker buildx version &>/dev/null 2>&1; then
        local bx_ver
        bx_ver="$(docker buildx version 2>/dev/null | head -1 || echo "unknown")"
        log_info "  ✓ docker buildx: ${bx_ver}"
    else
        log_error "  ✗ docker buildx: not found"
        log_error "    Install: https://docs.docker.com/build/install-buildx/"
        missing=1
    fi

    if [[ "${missing}" -ne 0 ]]; then
        echo
        die "Missing required tools. Install them and retry."
    fi

    log_info "All required tools are available."
}


# Build the doko BuildKit frontend image from the root Dockerfile.
# Tags it as doko:local for use in demo builds and local testing.
cmd_build() {
    log_info "Building doko frontend image (linux/${ARCH})..."
    docker build \
        --platform "linux/${ARCH}" \
        --tag "${LOCAL_IMAGE}" \
        --file "${REPO_ROOT}/Dockerfile" \
        "${REPO_ROOT}"
    log_info "Image built: ${LOCAL_IMAGE}"
    log_info "Use it in a doko.yaml with: # syntax=${LOCAL_IMAGE}"
}

# Ensure doko:local is available; build it if missing.
ensure_local_image() {
    if ! docker image inspect "${LOCAL_IMAGE}" &>/dev/null; then
        log_info "${LOCAL_IMAGE} not found — building first..."
        cmd_build
    fi
}

# Run an end-to-end build of an example spec using the locally built doko image.
# The # syntax= line is patched to point at doko:local so no registry pull occurs.
cmd_test() {
    local example="${1:-nginx}"

    # Validate example name
    local valid=0
    for e in "${VALID_EXAMPLES[@]}"; do
        [[ "${e}" == "${example}" ]] && valid=1 && break
    done
    if [[ "${valid}" -eq 0 ]]; then
        log_error "Unknown example: ${example}"
        log_error "Available: ${VALID_EXAMPLES[*]}"
        exit 1
    fi

    local yaml="${EXAMPLES_DIR}/${example}/doko.yaml"
    [[ -f "${yaml}" ]] || die "doko.yaml not found: ${yaml}"

    ensure_local_image

    # Patch the # syntax= line to use the local image (avoids registry pull).
    local tmpfile=""
    tmpfile="$(mktemp /tmp/doko-test-XXXXXX)"
    trap "rm -f '${tmpfile}'" EXIT
    sed "s|^# syntax=.*|# syntax=${LOCAL_IMAGE}|" "${yaml}" > "${tmpfile}"

    local out_tag="doko-test-${example}:local"
    log_info "Running test build: ${example} → ${out_tag}"
    log_info "  spec:     ${yaml}"
    log_info "  frontend: ${LOCAL_IMAGE}"

    docker buildx build \
        --network=host \
        --platform "linux/${ARCH}" \
        --file "${tmpfile}" \
        --load \
        --tag "${out_tag}" \
        "${EXAMPLES_DIR}/${example}"

    log_info "Test build complete. Image loaded: ${out_tag}"
    log_info "Run it with: docker run --rm ${out_tag}"
}


usage() {
    cat <<EOF
Usage: ./hack/make-dokoenv.sh [command] [args]

Commands:
  (none)        Show this help (default)
  check         Verify docker and docker buildx are available
  build         Build doko:local from the root Dockerfile
  test          End-to-end test using the nginx example (default)
  test <name>   End-to-end test using a specific example
  help          Show this message

Examples for test:
  ${VALID_EXAMPLES[*]}

How it works:
  build  → compiles the doko binary into a minimal Alpine image tagged doko:local
  test   → patches the example's # syntax= line to doko:local, then runs
           'docker buildx build --load' to produce a testable local image

Architecture: ${ARCH}
Local image:  ${LOCAL_IMAGE}
EOF
}


check_repo

case "${1:-}" in
    "")                      usage      ;;
    "check")                 check_tools ;;
    "build")                 cmd_build  ;;
    "test")                  cmd_test "${2:-nginx}" ;;
    "help"|"--help"|"-h")    usage      ;;
    *) log_error "Unknown command: $1"; echo; usage; exit 1 ;;
esac
