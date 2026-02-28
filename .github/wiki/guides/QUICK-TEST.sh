#!/usr/bin/env bash
#
# Quick Test Script for Billing Core Library
# Tests all major functions without requiring database connection
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NSELF_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Source the billing core library
source "${NSELF_ROOT}/src/lib/billing/core.sh"

printf "\n=== Billing Core Library - Quick Test ===\n\n"

# Test 1: Configuration Validation
printf "Test 1: Configuration Validation\n"
printf "  BILLING_DB_HOST: %s\n" "$BILLING_DB_HOST"
printf "  BILLING_DB_PORT: %s\n" "$BILLING_DB_PORT"
printf "  BILLING_DB_NAME: %s\n" "$BILLING_DB_NAME"
printf "  BILLING_DB_USER: %s\n" "$BILLING_DB_USER"
printf "\n"

# Test 2: Check Environment Defaults
printf "Test 2: Environment Defaults\n"
printf "  BILLING_DATA_DIR: %s\n" "$BILLING_DATA_DIR"
printf "  BILLING_CACHE_DIR: %s\n" "$BILLING_CACHE_DIR"
printf "  BILLING_EXPORT_DIR: %s\n" "$BILLING_EXPORT_DIR"
printf "  BILLING_LOG_FILE: %s\n" "$BILLING_LOG_FILE"
printf "\n"

# Test 3: Check Function Exports
printf "Test 3: Exported Functions\n"
exported_functions=(
    billing_init
    billing_validate_config
    billing_test_db_connection
    billing_test_stripe_connection
    billing_db_query
    billing_get_customer_id
    billing_get_subscription
    billing_record_usage
    billing_check_quota
    billing_get_quota_status
    billing_generate_invoice
    billing_export_all
    billing_log
    billing_get_summary
    billing_init_db
    billing_check_db_health
    billing_create_customer
    billing_create_default_subscription
    billing_get_customer
    billing_update_customer
    billing_delete_customer
    billing_list_customers
    billing_get_customer_plan
)

missing_functions=0
for func in "${exported_functions[@]}"; do
    if ! declare -F "$func" >/dev/null 2>&1; then
        printf "  ✗ Missing: %s\n" "$func"
        ((missing_functions++))
    else
        printf "  ✓ Found: %s\n" "$func"
    fi
done

printf "\n"

# Test 4: Test Logging Function
printf "Test 4: Logging Function\n"
mkdir -p "$(dirname "$BILLING_LOG_FILE")"
billing_log "TEST" "core" "1" "Quick test log entry"
if [[ -f "$BILLING_LOG_FILE" ]]; then
    printf "  ✓ Log file created: %s\n" "$BILLING_LOG_FILE"
    printf "  Last log entry: %s\n" "$(tail -1 "$BILLING_LOG_FILE")"
else
    printf "  ✗ Log file not created\n"
fi
printf "\n"

# Test 5: Test Platform Detection (Date Handling)
printf "Test 5: Platform Detection\n"
if date -v+1m >/dev/null 2>&1; then
    printf "  ✓ Detected: BSD date (macOS)\n"
else
    printf "  ✓ Detected: GNU date (Linux)\n"
fi
printf "\n"

# Test 6: Test Random Generation
printf "Test 6: Random Generation\n"
random_id=$(openssl rand -hex 4 2>/dev/null || printf "%08x" $RANDOM)
printf "  ✓ Generated random ID: %s\n" "$random_id"
printf "\n"

# Summary
printf "=== Test Summary ===\n"
printf "Total Functions: %d\n" "${#exported_functions[@]}"
printf "Missing Functions: %d\n" "$missing_functions"

if [[ $missing_functions -eq 0 ]]; then
    printf "\n✓ All tests passed! Billing core library is properly loaded.\n\n"
    exit 0
else
    printf "\n✗ Some functions are missing. Check implementation.\n\n"
    exit 1
fi
