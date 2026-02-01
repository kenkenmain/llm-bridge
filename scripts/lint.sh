#!/usr/bin/env bash
set -euo pipefail

GOLANGCI_VERSION="1.62.2"

# Resolve workspace directory.
# BUILD_WORKSPACE_DIRECTORY is set by `bazel run`, not `bazel test`.
# For `bazel test --local`, derive it from the runfiles tree or script location.
if [[ -n "${BUILD_WORKSPACE_DIRECTORY:-}" ]]; then
    WORKSPACE="$BUILD_WORKSPACE_DIRECTORY"
elif [[ -n "${TEST_SRCDIR:-}" ]]; then
    # In bazel test, TEST_SRCDIR points to runfiles; the workspace is the execroot parent.
    WORKSPACE="$(cd "${TEST_SRCDIR}/../.." 2>/dev/null && pwd)" || true
    # Verify it looks like a Go workspace.
    if [[ ! -f "${WORKSPACE}/go.mod" ]]; then
        # Fall back: the execroot's `_main` dir may be a symlink to the workspace.
        WORKSPACE="$(readlink -f "${TEST_SRCDIR}/_main" 2>/dev/null || echo "")"
    fi
fi

# Final fallback: resolve from script's real location.
if [[ -z "${WORKSPACE:-}" ]] || [[ ! -f "${WORKSPACE}/go.mod" ]]; then
    WORKSPACE="$(cd "$(dirname "$(readlink -f "$0")")/.." && pwd)"
fi

if [[ ! -f "${WORKSPACE}/go.mod" ]]; then
    echo "error: cannot determine workspace directory (no go.mod found)." >&2
    echo "Try: BUILD_WORKSPACE_DIRECTORY=/path/to/llm-bridge scripts/lint.sh" >&2
    exit 1
fi

cd "$WORKSPACE"

# Set HOME and lint cache dir; Bazel may strip HOME from the environment.
export HOME="${HOME:-/tmp}"
export GOLANGCI_LINT_CACHE="${GOLANGCI_LINT_CACHE:-${HOME}/.cache/golangci-lint}"

# Add Bazel's Go SDK to PATH if go is not already available.
if ! command -v go &>/dev/null; then
    BAZEL_GO=$(find "${HOME}/.cache/bazel" -path "*go_sdk*bin/go" -type f 2>/dev/null | head -1) || true
    if [[ -n "${BAZEL_GO:-}" ]]; then
        export PATH="$(dirname "$BAZEL_GO"):$PATH"
        export GOROOT="$(dirname "$(dirname "$BAZEL_GO")")"
    fi
fi

if ! command -v go &>/dev/null; then
    echo "error: go not found. Install Go or run 'bazel build //...' first to download the SDK." >&2
    exit 1
fi

# golangci-lint binary: use GOLANGCI_LINT env if set, else look in PATH.
LINT="${GOLANGCI_LINT:-$(command -v golangci-lint 2>/dev/null || true)}"

if [[ -z "$LINT" ]]; then
    echo "golangci-lint not found — downloading v${GOLANGCI_VERSION}..." >&2
    DLDIR=$(mktemp -d)
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
    esac
    URL="https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_VERSION}/golangci-lint-${GOLANGCI_VERSION}-${OS}-${ARCH}.tar.gz"
    curl -fsSL "$URL" | tar xz -C "$DLDIR" --strip-components=1
    LINT="$DLDIR/golangci-lint"
    chmod +x "$LINT"
fi

exec "$LINT" run ./...
