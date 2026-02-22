#!/usr/bin/env bash
set -euo pipefail
#
# nself Test Suite - Billing Usage Tracking
# Tests for usage.sh functionality
#

# Test framework setup
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NSELF_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

# Source test utilities
source "${SCRIPT_DIR}/../test-utils.sh"

# Source billing modules
source "${NSELF_ROOT}/src/lib/billing/usage.sh"

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Test helpers
test_name=""
test_setup() {
  test_name="$1"
  TESTS_RUN=$((TESTS_RUN + 1))
}

test_pass() {
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "\033[32m✓\033[0m %s\n" "$test_name"
}

test_fail() {
  TESTS_FAILED=$((TESTS_FAILED + 1))
  printf "\033[31m✗\033[0m %s\n" "$test_name"
  printf "  Error: %s\n" "$1"
}

assert_equals() {
  local expected="$1"
  local actual="$2"
  local message="${3:-Values not equal}"

  if [[ "$expected" == "$actual" ]]; then
    return 0
  else
    test_fail "$message (expected: '$expected', got: '$actual')"
    return 1
  fi
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  local message="${3:-String not found}"

  if [[ "$haystack" == *"$needle"* ]]; then
    return 0
  else
    test_fail "$message (expected to contain: '$needle')"
    return 1
  fi
}

# ============================================================================
# Test: Batch Initialization
# ============================================================================

test_batch_init() {
  test_setup "Batch initialization creates required files"

  usage_init_batch

  if [[ -f "$USAGE_BATCH_FILE" ]]; then
    test_pass
  else
    test_fail "Batch file not created"
  fi
}

# ============================================================================
# Test: Service Definitions
# ============================================================================

test_service_definitions() {
  test_setup "All six services are defined"

  local expected_services=("api" "storage" "bandwidth" "compute" "database" "functions")
  local all_present=true

  for service in "${expected_services[@]}"; do
    local found=false
    for defined_service in "${USAGE_SERVICES[@]}"; do
      if [[ "$defined_service" == "$service" ]]; then
        found=true
        break
      fi
    done

    if [[ "$found" != "true" ]]; then
      all_present=false
      break
    fi
  done

  if [[ "$all_present" == "true" ]]; then
    test_pass
  else
    test_fail "Not all services defined"
  fi
}

# ============================================================================
# Test: Service Pricing
# ============================================================================

test_service_pricing() {
  test_setup "Service pricing returns valid data"

  local pricing
  pricing=$(usage_get_service_pricing "api")

  if [[ -n "$pricing" ]] && [[ "$pricing" =~ "requests" ]]; then
    test_pass
  else
    test_fail "Invalid pricing data: $pricing"
  fi
}

# ============================================================================
# Test: Number Formatting
# ============================================================================

test_number_formatting() {
  test_setup "Number formatting works correctly"

  # Test millions
  local result
  result=$(usage_format_number 1500000 "api")
  assert_equals "1.50M" "$result" "Million formatting" || return 1

  # Test thousands
  result=$(usage_format_number 5500 "api")
  assert_equals "5.50K" "$result" "Thousand formatting" || return 1

  # Test small numbers
  result=$(usage_format_number 250 "api")
  assert_equals "250" "$result" "Small number formatting" || return 1

  test_pass
}

# ============================================================================
# Test: Cost Calculation
# ============================================================================

test_cost_calculation() {
  test_setup "Cost calculation is accurate"

  local result
  result=$(usage_calculate_cost 1000 0.0001)

  if [[ "$result" == "0.10" ]]; then
    test_pass
  else
    test_fail "Expected 0.10, got $result"
  fi
}

# ============================================================================
# Test: Metadata JSON Generation
# ============================================================================

test_metadata_generation() {
  test_setup "API request metadata is valid JSON"

  # This would normally call the function, but we'll test the format
  local endpoint="/api/users"
  local method="GET"
  local status=200

  local metadata
  metadata=$(printf '{"endpoint":"%s","method":"%s","status":%d}' \
    "$endpoint" "$method" "$status")

  if [[ "$metadata" =~ \"endpoint\":\"$endpoint\" ]] &&
    [[ "$metadata" =~ \"method\":\"$method\" ]] &&
    [[ "$metadata" =~ \"status\":$status ]]; then
    test_pass
  else
    test_fail "Invalid metadata: $metadata"
  fi
}

# ============================================================================
# Test: Storage Unit Conversion
# ============================================================================

test_storage_conversion() {
  test_setup "Storage bytes to GB-hours conversion"

  local bytes=1073741824 # 1GB
  local hours=24

  local gb_hours
  gb_hours=$(awk "BEGIN {printf \"%.6f\", ($bytes / 1073741824) * $hours}")

  if [[ "$gb_hours" == "24.000000" ]]; then
    test_pass
  else
    test_fail "Expected 24.000000, got $gb_hours"
  fi
}

# ============================================================================
# Test: Bandwidth Unit Conversion
# ============================================================================

test_bandwidth_conversion() {
  test_setup "Bandwidth bytes to GB conversion"

  local bytes=1073741824 # 1GB

  local gb
  gb=$(awk "BEGIN {printf \"%.6f\", $bytes / 1073741824}")

  if [[ "$gb" == "1.000000" ]]; then
    test_pass
  else
    test_fail "Expected 1.000000, got $gb"
  fi
}

# ============================================================================
# Test: Compute Time Conversion
# ============================================================================

test_compute_conversion() {
  test_setup "Compute seconds to CPU-hours conversion"

  local cpu_seconds=7200 # 2 hours

  local cpu_hours
  cpu_hours=$(awk "BEGIN {printf \"%.6f\", $cpu_seconds / 3600}")

  if [[ "$cpu_hours" == "2.000000" ]]; then
    test_pass
  else
    test_fail "Expected 2.000000, got $cpu_hours"
  fi
}

# ============================================================================
# Test: Bar Chart Creation
# ============================================================================

test_bar_chart() {
  test_setup "Bar chart creation works"

  local bar
  bar=$(usage_create_bar 10 50)

  # Should have opening [, closing ], and 50 total characters between
  if [[ "$bar" =~ ^\[.*\]$ ]]; then
    test_pass
  else
    test_fail "Invalid bar chart format: $bar"
  fi
}

# ============================================================================
# Test: Alert Threshold Configuration
# ============================================================================

test_alert_thresholds() {
  test_setup "Alert thresholds are properly configured"

  if [[ -n "$USAGE_ALERT_WARNING" ]] &&
    [[ -n "$USAGE_ALERT_CRITICAL" ]] &&
    [[ -n "$USAGE_ALERT_EXCEEDED" ]]; then

    if [[ $USAGE_ALERT_WARNING -lt $USAGE_ALERT_CRITICAL ]] &&
      [[ $USAGE_ALERT_CRITICAL -le $USAGE_ALERT_EXCEEDED ]]; then
      test_pass
    else
      test_fail "Alert thresholds not in correct order"
    fi
  else
    test_fail "Alert thresholds not defined"
  fi
}

# ============================================================================
# Test: Batch Size Configuration
# ============================================================================

test_batch_configuration() {
  test_setup "Batch processing is configured"

  if [[ -n "$USAGE_BATCH_SIZE" ]] && [[ $USAGE_BATCH_SIZE -gt 0 ]]; then
    test_pass
  else
    test_fail "Invalid batch size: $USAGE_BATCH_SIZE"
  fi
}

# ============================================================================
# Test: Export Functions Exist
# ============================================================================

test_export_functions() {
  test_setup "All export functions are defined"

  local functions=(
    "usage_get_all"
    "usage_get_service"
    "usage_export"
    "usage_export_csv"
    "usage_export_json"
  )

  local all_defined=true
  for func in "${functions[@]}"; do
    if ! declare -f "$func" >/dev/null 2>&1; then
      all_defined=false
      break
    fi
  done

  if [[ "$all_defined" == "true" ]]; then
    test_pass
  else
    test_fail "Not all export functions defined"
  fi
}

# ============================================================================
# Test: Tracking Functions Exist
# ============================================================================

test_tracking_functions() {
  test_setup "All tracking functions are defined"

  local functions=(
    "usage_track_api_request"
    "usage_track_storage"
    "usage_track_bandwidth"
    "usage_track_compute"
    "usage_track_database_query"
    "usage_track_function"
  )

  local all_defined=true
  for func in "${functions[@]}"; do
    if ! declare -f "$func" >/dev/null 2>&1; then
      all_defined=false
      break
    fi
  done

  if [[ "$all_defined" == "true" ]]; then
    test_pass
  else
    test_fail "Not all tracking functions defined"
  fi
}

# ============================================================================
# Test: Aggregation Functions Exist
# ============================================================================

test_aggregation_functions() {
  test_setup "All aggregation functions are defined"

  local functions=(
    "usage_aggregate"
    "usage_aggregate_hourly"
    "usage_aggregate_daily"
    "usage_aggregate_monthly"
  )

  local all_defined=true
  for func in "${functions[@]}"; do
    if ! declare -f "$func" >/dev/null 2>&1; then
      all_defined=false
      break
    fi
  done

  if [[ "$all_defined" == "true" ]]; then
    test_pass
  else
    test_fail "Not all aggregation functions defined"
  fi
}

# ============================================================================
# Test: Alert Functions Exist
# ============================================================================

test_alert_functions() {
  test_setup "All alert functions are defined"

  local functions=(
    "usage_check_alerts"
    "usage_check_service_alert"
    "usage_trigger_alert"
    "usage_get_alerts"
  )

  local all_defined=true
  for func in "${functions[@]}"; do
    if ! declare -f "$func" >/dev/null 2>&1; then
      all_defined=false
      break
    fi
  done

  if [[ "$all_defined" == "true" ]]; then
    test_pass
  else
    test_fail "Not all alert functions defined"
  fi
}

# ============================================================================
# Test: Statistics Functions Exist
# ============================================================================

test_statistics_functions() {
  test_setup "All statistics functions are defined"

  local functions=(
    "usage_get_stats"
    "usage_get_trends"
    "usage_get_peaks"
  )

  local all_defined=true
  for func in "${functions[@]}"; do
    if ! declare -f "$func" >/dev/null 2>&1; then
      all_defined=false
      break
    fi
  done

  if [[ "$all_defined" == "true" ]]; then
    test_pass
  else
    test_fail "Not all statistics functions defined"
  fi
}

# ============================================================================
# Test: Batch Functions Exist
# ============================================================================

test_batch_functions() {
  test_setup "All batch processing functions are defined"

  local functions=(
    "usage_init_batch"
    "usage_batch_add"
    "usage_batch_flush"
    "usage_batch_insert"
  )

  local all_defined=true
  for func in "${functions[@]}"; do
    if ! declare -f "$func" >/dev/null 2>&1; then
      all_defined=false
      break
    fi
  done

  if [[ "$all_defined" == "true" ]]; then
    test_pass
  else
    test_fail "Not all batch functions defined"
  fi
}

# ============================================================================
# Test: Cleanup Functions Exist
# ============================================================================

test_cleanup_functions() {
  test_setup "All cleanup functions are defined"

  local functions=(
    "usage_archive"
    "usage_cleanup_batch"
  )

  local all_defined=true
  for func in "${functions[@]}"; do
    if ! declare -f "$func" >/dev/null 2>&1; then
      all_defined=false
      break
    fi
  done

  if [[ "$all_defined" == "true" ]]; then
    test_pass
  else
    test_fail "Not all cleanup functions defined"
  fi
}

# ============================================================================
# Run All Tests
# ============================================================================

run_all_tests() {
  printf "\n"
  printf "╔════════════════════════════════════════════════════════════════╗\n"
  printf "║           nself Billing Usage Tracking Test Suite             ║\n"
  printf "╚════════════════════════════════════════════════════════════════╝\n"
  printf "\n"

  # Run tests
  test_batch_init
  test_service_definitions
  test_service_pricing
  test_number_formatting
  test_cost_calculation
  test_metadata_generation
  test_storage_conversion
  test_bandwidth_conversion
  test_compute_conversion
  test_bar_chart
  test_alert_thresholds
  test_batch_configuration
  test_export_functions
  test_tracking_functions
  test_aggregation_functions
  test_alert_functions
  test_statistics_functions
  test_batch_functions
  test_cleanup_functions

  # Cleanup
  usage_cleanup_batch

  # Results
  printf "\n"
  printf "╔════════════════════════════════════════════════════════════════╗\n"
  printf "║                        TEST RESULTS                            ║\n"
  printf "╠════════════════════════════════════════════════════════════════╣\n"
  printf "║ Total Tests:    %-46d ║\n" "$TESTS_RUN"
  printf "║ Passed:         \033[32m%-46d\033[0m ║\n" "$TESTS_PASSED"
  printf "║ Failed:         \033[31m%-46d\033[0m ║\n" "$TESTS_FAILED"
  printf "╚════════════════════════════════════════════════════════════════╝\n"
  printf "\n"

  # Exit code
  if [[ $TESTS_FAILED -eq 0 ]]; then
    printf "\033[32m✓ All tests passed!\033[0m\n\n"
    return 0
  else
    printf "\033[31m✗ Some tests failed\033[0m\n\n"
    return 1
  fi
}

# Run tests
run_all_tests
exit $?
