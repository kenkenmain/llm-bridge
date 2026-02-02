#!/usr/bin/env bash
set -euo pipefail

# check-coverage.sh — Parse LCOV coverage output and enforce a minimum threshold.
#
# Usage:
#   scripts/check-coverage.sh [--threshold N] [--exclude PATTERN] [--lcov-file PATH]
#   scripts/check-coverage.sh --self-test
#   scripts/check-coverage.sh --help

DEFAULT_THRESHOLD=90
DEFAULT_LCOV_FILE="bazel-out/_coverage/_coverage_report.dat"

# ─── Argument parsing ───────────────────────────────────────────────────────

THRESHOLD="$DEFAULT_THRESHOLD"
LCOV_FILE=""
EXCLUDES=()
SELF_TEST=false

usage() {
    cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Parse LCOV coverage output and enforce a minimum coverage threshold.

Options:
  --threshold N        Minimum coverage percentage (integer 0-100, default: $DEFAULT_THRESHOLD)
  --exclude PATTERN    Grep pattern for packages to exclude (repeatable, default: cmd/)
  --lcov-file PATH     Path to LCOV file (default: $DEFAULT_LCOV_FILE)
  --self-test          Run self-validation with synthetic LCOV data
  --help               Show this help and exit

Examples:
  $(basename "$0")                                  # defaults: 90% threshold, exclude cmd/
  $(basename "$0") --threshold 80 --exclude cmd/    # 80% threshold, exclude cmd/
  $(basename "$0") --lcov-file coverage.dat         # custom LCOV file
  $(basename "$0") --self-test                      # run built-in self-tests
EOF
    exit 0
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --threshold)
            if [[ -z "${2:-}" ]]; then
                echo "error: --threshold requires a value" >&2
                exit 1
            fi
            THRESHOLD="$2"
            shift 2
            ;;
        --exclude)
            if [[ -z "${2:-}" ]]; then
                echo "error: --exclude requires a value" >&2
                exit 1
            fi
            EXCLUDES+=("$2")
            shift 2
            ;;
        --lcov-file)
            if [[ -z "${2:-}" ]]; then
                echo "error: --lcov-file requires a value" >&2
                exit 1
            fi
            LCOV_FILE="$2"
            shift 2
            ;;
        --self-test)
            SELF_TEST=true
            shift
            ;;
        --help)
            usage
            ;;
        *)
            echo "error: unknown option: $1" >&2
            echo "Run '$(basename "$0") --help' for usage." >&2
            exit 1
            ;;
    esac
done

# Apply default excludes if none specified.
if [[ ${#EXCLUDES[@]} -eq 0 ]]; then
    EXCLUDES=("^cmd/")
fi

# Always exclude external dependencies (Bazel sandbox paths for third-party code).
EXCLUDES+=("^external/")

# ─── Threshold validation ───────────────────────────────────────────────────

if ! [[ "$THRESHOLD" =~ ^[0-9]+$ ]]; then
    echo "error: threshold must be an integer 0-100, got: $THRESHOLD" >&2
    exit 1
fi

if [[ "$THRESHOLD" -lt 0 ]] || [[ "$THRESHOLD" -gt 100 ]]; then
    echo "error: threshold must be an integer 0-100, got: $THRESHOLD" >&2
    exit 1
fi

# ─── LCOV parsing function ──────────────────────────────────────────────────

# parse_lcov FILE
#   Reads an LCOV file and outputs per-package aggregated coverage.
#   Output format: PACKAGE\tLINES_HIT\tLINES_FOUND (one line per package).
parse_lcov() {
    local lcov_file="$1"

    awk '
    BEGIN {
        pkg = ""
        lh = 0
        lf = 0
    }
    /^SF:/ {
        # Extract source file path.
        path = substr($0, 4)

        # Strip Bazel sandbox prefix: remove everything up to and including _main/
        if (match(path, /_main\//)) {
            path = substr(path, RSTART + RLENGTH)
        }

        # Strip module path prefix.
        gsub(/^github\.com\/anthropics\/llm-bridge\//, "", path)

        # Package = directory of the source file.
        n = split(path, parts, "/")
        pkg = ""
        for (i = 1; i < n; i++) {
            if (i > 1) pkg = pkg "/"
            pkg = pkg parts[i]
        }
        if (pkg == "") pkg = "."

        lh = 0
        lf = 0
    }
    /^LH:/ {
        lh = int(substr($0, 4))
    }
    /^LF:/ {
        lf = int(substr($0, 4))
    }
    /^end_of_record/ {
        if (pkg != "" && lf > 0) {
            hit[pkg] += lh
            found[pkg] += lf
        }
        pkg = ""
        lh = 0
        lf = 0
    }
    END {
        for (p in hit) {
            printf "%s\t%d\t%d\n", p, hit[p], found[p]
        }
    }
    ' "$lcov_file"
}

# ─── Report and check function ──────────────────────────────────────────────

# check_coverage LCOV_FILE THRESHOLD EXCLUDES...
#   Parses LCOV, filters exclusions, prints report, and exits 0/1.
#   Returns 0 on PASS, 1 on FAIL.
check_coverage() {
    local lcov_file="$1"
    local threshold="$2"
    shift 2
    local excludes=("$@")

    # Parse LCOV into per-package data.
    local raw_data
    raw_data="$(parse_lcov "$lcov_file")"

    if [[ -z "$raw_data" ]]; then
        echo "error: no coverage data found in $lcov_file" >&2
        return 1
    fi

    # Separate included and excluded packages.
    local included_data=""
    local excluded_lines=()

    while IFS=$'\t' read -r pkg hit found; do
        # Check if package matches any exclude pattern.
        local excluded=false
        local matched_pattern=""
        for pattern in "${excludes[@]}"; do
            if echo "$pkg" | grep -q "$pattern"; then
                excluded=true
                matched_pattern="$pattern"
                break
            fi
        done

        if [[ "$excluded" == "true" ]]; then
            excluded_lines+=("  $pkg (matched: $matched_pattern)")
        else
            if [[ "$found" -eq 0 ]]; then
                echo "warning: skipping $pkg (0 lines found)" >&2
                continue
            fi
            if [[ -n "$included_data" ]]; then
                included_data="${included_data}"$'\n'"${pkg}"$'\t'"${hit}"$'\t'"${found}"
            else
                included_data="${pkg}"$'\t'"${hit}"$'\t'"${found}"
            fi
        fi
    done <<< "$raw_data"

    # Check if any packages remain after exclusion.
    if [[ -z "$included_data" ]]; then
        echo "error: no packages remain after applying exclusions" >&2
        return 1
    fi

    # ─── Output formatting ───────────────────────────────────────────────────

    echo ""
    echo "Coverage Report"
    echo "==============="
    printf "%-40s %s\n" "Package" "Coverage"
    printf "%-40s %s\n" "-------" "--------"

    local total_hit=0
    local total_found=0

    while IFS=$'\t' read -r pkg hit found; do
        local pct
        pct=$(awk "BEGIN { printf \"%.1f\", ($hit / $found) * 100 }")
        printf "%-40s %s%%\n" "$pkg" "$pct"
        total_hit=$((total_hit + hit))
        total_found=$((total_found + found))
    done <<< "$included_data"

    # Print excluded packages.
    if [[ ${#excluded_lines[@]} -gt 0 ]]; then
        echo ""
        echo "Excluded:"
        for line in "${excluded_lines[@]}"; do
            echo "$line"
        done
    fi

    echo ""

    # Calculate total coverage.
    local total_pct
    total_pct=$(awk "BEGIN { printf \"%.1f\", ($total_hit / $total_found) * 100 }")

    echo "Total: ${total_pct}% (threshold: ${threshold}%)"

    # Check threshold.
    local passed
    passed=$(awk "BEGIN { print (($total_hit / $total_found) * 100 >= $threshold) ? 1 : 0 }")

    if [[ "$passed" -eq 1 ]]; then
        echo "PASS"
        return 0
    else
        echo "FAIL: coverage ${total_pct}% is below threshold ${threshold}%"
        return 1
    fi
}

# ─── Self-test mode ─────────────────────────────────────────────────────────

if [[ "$SELF_TEST" == "true" ]]; then
    TMPFILE=$(mktemp /tmp/check-coverage-selftest.XXXXXX)
    trap "rm -f '$TMPFILE'" EXIT

    cat > "$TMPFILE" <<'LCOV'
SF:github.com/anthropics/llm-bridge/internal/config/config.go
FN:1,NewConfig
FNDA:1,NewConfig
FNF:1
FNH:1
DA:1,1
DA:2,1
DA:3,0
LH:2
LF:3
end_of_record
SF:github.com/anthropics/llm-bridge/internal/router/router.go
FN:1,Route
FNDA:1,Route
FNF:1
FNH:1
DA:1,1
DA:2,1
DA:3,1
DA:4,1
LH:4
LF:4
end_of_record
SF:github.com/anthropics/llm-bridge/cmd/llm-bridge/main.go
FN:1,main
FNDA:0,main
FNF:1
FNH:0
DA:1,0
DA:2,0
LH:0
LF:2
end_of_record
LCOV

    PASS_COUNT=0
    FAIL_COUNT=0

    # Test 1: threshold=80, exclude=cmd/ -> PASS (6/7 = 85.7%)
    echo "=== Self-test 1: threshold=80, exclude=cmd/ (expect PASS) ==="
    if check_coverage "$TMPFILE" 80 "cmd/"; then
        echo "--- Self-test 1: OK ---"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        echo "--- Self-test 1: UNEXPECTED FAIL ---" >&2
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi

    echo ""

    # Test 2: threshold=90, exclude=cmd/ -> FAIL (6/7 = 85.7%)
    echo "=== Self-test 2: threshold=90, exclude=cmd/ (expect FAIL) ==="
    if check_coverage "$TMPFILE" 90 "cmd/"; then
        echo "--- Self-test 2: UNEXPECTED PASS ---" >&2
        FAIL_COUNT=$((FAIL_COUNT + 1))
    else
        echo "--- Self-test 2: OK ---"
        PASS_COUNT=$((PASS_COUNT + 1))
    fi

    echo ""
    echo "Self-test results: $PASS_COUNT passed, $FAIL_COUNT failed"

    if [[ "$FAIL_COUNT" -gt 0 ]]; then
        exit 1
    fi
    exit 0
fi

# ─── Main ────────────────────────────────────────────────────────────────────

# Resolve workspace directory (same logic as lint.sh).
if [[ -n "${BUILD_WORKSPACE_DIRECTORY:-}" ]]; then
    WORKSPACE="$BUILD_WORKSPACE_DIRECTORY"
elif [[ -n "${TEST_SRCDIR:-}" ]]; then
    WORKSPACE="$(cd "${TEST_SRCDIR}/../.." 2>/dev/null && pwd)" || true
    if [[ ! -f "${WORKSPACE}/go.mod" ]]; then
        WORKSPACE="$(readlink -f "${TEST_SRCDIR}/_main" 2>/dev/null || echo "")"
    fi
fi

if [[ -z "${WORKSPACE:-}" ]] || [[ ! -f "${WORKSPACE}/go.mod" ]]; then
    WORKSPACE="$(cd "$(dirname "$(readlink -f "$0")")/.." && pwd)"
fi

# Determine LCOV file path.
if [[ -z "$LCOV_FILE" ]]; then
    LCOV_FILE="${WORKSPACE}/${DEFAULT_LCOV_FILE}"
fi

# Validate LCOV file exists and is non-empty.
if [[ ! -f "$LCOV_FILE" ]]; then
    echo "error: LCOV file not found: $LCOV_FILE" >&2
    echo "Run 'bazel coverage //...' first to generate coverage data." >&2
    exit 1
fi

if [[ ! -s "$LCOV_FILE" ]]; then
    echo "error: LCOV file is empty: $LCOV_FILE" >&2
    exit 1
fi

# Run coverage check.
check_coverage "$LCOV_FILE" "$THRESHOLD" "${EXCLUDES[@]}"
