#!/usr/bin/env bash
#
# fuzz-validators.sh — Fuzz test CLI input validation functions.
#
# Generates random strings from /dev/urandom and feeds them to each validator
# function. Checks for unexpected crashes (non-zero exit from the subshell
# other than the expected validation failure code).
#
# Usage:
#   bash scripts/fuzz-validators.sh [iterations]
#
# Default: 10000 iterations per validator.

set -uo pipefail

# Use C locale to avoid "Illegal byte sequence" errors from tr on macOS
export LC_ALL=C

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLI_ROOT="$(dirname "$SCRIPT_DIR")"
ITERATIONS="${1:-10000}"
CRASHES=0
TOTAL_CHECKS=0

# Source the validation library
UTILS_DIR="$CLI_ROOT/src/lib/utils"

printf "=== nSelf CLI Fuzz Testing ===\n"
printf "Iterations per validator: %d\n" "$ITERATIONS"
printf "Source: %s/validation.sh\n\n" "$UTILS_DIR"

# Generate a random string of random length (1-512 bytes).
# Uses /dev/urandom with base64 encoding for printable output,
# plus raw bytes for binary edge cases.
generate_random_input() {
    local mode=$((RANDOM % 5))
    case $mode in
        0)
            # Printable ASCII via base64
            local len=$((RANDOM % 512 + 1))
            head -c "$len" /dev/urandom | base64 | head -c "$len"
            ;;
        1)
            # Raw bytes (may contain nulls, control chars)
            local len=$((RANDOM % 256 + 1))
            head -c "$len" /dev/urandom | tr '\0' 'x'
            ;;
        2)
            # Domain-like strings
            local parts=$((RANDOM % 5 + 1))
            local result=""
            for _p in $(seq 1 "$parts"); do
                local plen=$((RANDOM % 20 + 1))
                local seg
                seg=$(head -c "$plen" /dev/urandom | tr -dc 'a-zA-Z0-9-' | head -c "$plen")
                if [ -n "$result" ]; then
                    result="${result}.${seg}"
                else
                    result="$seg"
                fi
            done
            printf "%s" "$result"
            ;;
        3)
            # Numeric strings (for port fuzzing)
            local nlen=$((RANDOM % 10 + 1))
            head -c "$nlen" /dev/urandom | tr -dc '0-9' | head -c "$nlen"
            ;;
        4)
            # Email-like strings
            local user
            local domain
            user=$(head -c 20 /dev/urandom | tr -dc 'a-zA-Z0-9._%+-' | head -c "$((RANDOM % 20 + 1))")
            domain=$(head -c 20 /dev/urandom | tr -dc 'a-zA-Z0-9.-' | head -c "$((RANDOM % 20 + 1))")
            printf "%s@%s" "$user" "$domain"
            ;;
    esac
}

# Fuzz a single validator function.
# Args: function_name description
fuzz_validator() {
    local func_name="$1"
    local description="$2"
    local crashes=0

    printf "Fuzzing: %s (%s)\n" "$func_name" "$description"
    printf "  "

    for i in $(seq 1 "$ITERATIONS"); do
        local input
        input=$(generate_random_input)

        # Run the validator in a subshell. It should either return 0 (valid)
        # or 1 (invalid). Any other exit code or signal is a crash.
        local exit_code=0
        (
            # Source dependencies silently
            source "$UTILS_DIR/display.sh" 2>/dev/null || true
            source "$UTILS_DIR/platform-compat.sh" 2>/dev/null || true
            source "$UTILS_DIR/validation.sh" 2>/dev/null || true

            # Call the validator; suppress output
            "$func_name" "$input" >/dev/null 2>&1
        ) 2>/dev/null
        exit_code=$?

        # Exit codes 0 (valid) and 1 (invalid) are expected.
        # Codes 126 (permission), 127 (not found), or 128+ (signal) are crashes.
        if [ "$exit_code" -ge 126 ]; then
            crashes=$((crashes + 1))
            printf "\n  CRASH at iteration %d (exit=%d) input=%q\n  " "$i" "$exit_code" "$input"
        fi

        # Progress indicator every 1000 iterations
        if [ $((i % 1000)) -eq 0 ]; then
            printf "."
        fi
    done

    printf "\n  Result: %d/%d iterations, %d crashes\n\n" "$ITERATIONS" "$ITERATIONS" "$crashes"
    TOTAL_CHECKS=$((TOTAL_CHECKS + ITERATIONS))
    CRASHES=$((CRASHES + crashes))
    return "$crashes"
}

# ============================================================================
# Fuzz each validator
# ============================================================================

fuzz_validator "is_valid_domain" "domain name validation"
fuzz_validator "is_valid_email" "email address validation"
fuzz_validator "is_valid_port" "port number validation (1-65535)"
fuzz_validator "is_valid_url" "URL validation"
fuzz_validator "is_valid_ip" "IPv4 address validation"

# ============================================================================
# Additional edge-case inputs (targeted, not random)
# ============================================================================

printf "=== Targeted Edge Cases ===\n\n"

EDGE_CRASHES=0

run_edge_case() {
    local func_name="$1"
    local input="$2"
    local label="$3"

    local exit_code=0
    (
        source "$UTILS_DIR/display.sh" 2>/dev/null || true
        source "$UTILS_DIR/platform-compat.sh" 2>/dev/null || true
        source "$UTILS_DIR/validation.sh" 2>/dev/null || true
        "$func_name" "$input" >/dev/null 2>&1
    ) 2>/dev/null
    exit_code=$?

    if [ "$exit_code" -ge 126 ]; then
        printf "  CRASH: %s — %s (exit=%d)\n" "$func_name" "$label" "$exit_code"
        EDGE_CRASHES=$((EDGE_CRASHES + 1))
    fi
    TOTAL_CHECKS=$((TOTAL_CHECKS + 1))
}

# Domain edge cases
run_edge_case "is_valid_domain" "" "empty string"
run_edge_case "is_valid_domain" "." "single dot"
run_edge_case "is_valid_domain" ".." "double dot"
run_edge_case "is_valid_domain" "-" "single dash"
run_edge_case "is_valid_domain" "a.b.c.d.e.f.g.h.i.j.k.l.m.n.o.p.q.r.s.t.u.v.w.x.y.z" "26-label domain"
run_edge_case "is_valid_domain" "$(printf '%0.sa' $(seq 1 300))" "300-char single label"
run_edge_case "is_valid_domain" "localhost" "localhost"
run_edge_case "is_valid_domain" "xn--nxasmq6b.xn--jxalpdlp" "IDN punycode"

# Port edge cases
run_edge_case "is_valid_port" "" "empty string"
run_edge_case "is_valid_port" "-1" "negative"
run_edge_case "is_valid_port" "0" "zero"
run_edge_case "is_valid_port" "65536" "above max"
run_edge_case "is_valid_port" "999999999999999999999" "huge number"
run_edge_case "is_valid_port" "abc" "non-numeric"
run_edge_case "is_valid_port" "80abc" "mixed numeric"
run_edge_case "is_valid_port" "   80   " "whitespace padded"

# Email edge cases
run_edge_case "is_valid_email" "" "empty string"
run_edge_case "is_valid_email" "@" "bare at"
run_edge_case "is_valid_email" "user@" "no domain"
run_edge_case "is_valid_email" "@domain.com" "no user"
run_edge_case "is_valid_email" "user@domain" "no TLD"
run_edge_case "is_valid_email" "user@.com" "dot-start domain"
run_edge_case "is_valid_email" "a@b.c" "minimal valid-ish"

# URL edge cases
run_edge_case "is_valid_url" "" "empty string"
run_edge_case "is_valid_url" "ftp://example.com" "non-http scheme"
run_edge_case "is_valid_url" "http://" "scheme only"
run_edge_case "is_valid_url" "https://localhost:99999" "huge port"
run_edge_case "is_valid_url" "javascript:alert(1)" "javascript URI"

# IP edge cases
run_edge_case "is_valid_ip" "" "empty string"
run_edge_case "is_valid_ip" "256.1.1.1" "octet > 255"
run_edge_case "is_valid_ip" "1.1.1" "3 octets"
run_edge_case "is_valid_ip" "1.1.1.1.1" "5 octets"
run_edge_case "is_valid_ip" "0.0.0.0" "all zeros"
run_edge_case "is_valid_ip" "255.255.255.255" "all max"
run_edge_case "is_valid_ip" "-1.0.0.0" "negative octet"
run_edge_case "is_valid_ip" "a.b.c.d" "alpha octets"

CRASHES=$((CRASHES + EDGE_CRASHES))

if [ "$EDGE_CRASHES" -eq 0 ]; then
    printf "  All edge cases passed (no crashes)\n"
fi

# ============================================================================
# Summary
# ============================================================================

printf "\n=== Fuzz Test Summary ===\n"
printf "Total checks:  %d\n" "$TOTAL_CHECKS"
printf "Total crashes:  %d\n" "$CRASHES"

if [ "$CRASHES" -eq 0 ]; then
    printf "\nRESULT: PASS — zero crashes across all validators\n"
    exit 0
else
    printf "\nRESULT: FAIL — %d crash(es) detected\n" "$CRASHES"
    exit 1
fi
