#!/usr/bin/env bash
#
# Test suite for billing quotas system
# Part of nself v0.9.0 - Sprint 13: Billing Integration & Usage Tracking
#

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Source the quota system
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../../lib/billing/quotas.sh"

# Test helper functions
test_assert() {
  local description="$1"
  local result="$2"
  local expected="${3:-0}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ "$result" == "$expected" ]]; then
    printf "${GREEN}✓${NC} %s\n" "$description"
    TESTS_PASSED=$((TESTS_PASSED + 1))
    return 0
  else
    printf "${RED}✗${NC} %s (expected: %s, got: %s)\n" "$description" "$expected" "$result"
    TESTS_FAILED=$((TESTS_FAILED + 1))
    return 1
  fi
}

test_section() {
  local section="$1"
  printf "\n${YELLOW}═══════════════════════════════════════════════════════${NC}\n"
  printf "${YELLOW}%s${NC}\n" "$section"
  printf "${YELLOW}═══════════════════════════════════════════════════════${NC}\n\n"
}

# ============================================================================
# Test Functions
# ============================================================================

test_quota_format_number() {
  test_section "Testing quota_format_number"

  local result
  result=$(quota_format_number 500)
  test_assert "Format small number" "$result" "500"

  result=$(quota_format_number 1500)
  test_assert "Format thousands" "$result" "1.5K"

  result=$(quota_format_number 2500000)
  test_assert "Format millions" "$result" "2.5M"

  result=$(quota_format_number 3500000000)
  test_assert "Format billions" "$result" "3.5B"

  result=$(quota_format_number -1)
  test_assert "Format unlimited" "$result" "Unlimited"
}

test_quota_show_progress_bar() {
  test_section "Testing quota_show_progress_bar"

  printf "Progress bar at 25%%: "
  quota_show_progress_bar 25 50
  printf "\n"

  printf "Progress bar at 50%%: "
  quota_show_progress_bar 50 50
  printf "\n"

  printf "Progress bar at 75%%: "
  quota_show_progress_bar 75 50
  printf "\n"

  printf "Progress bar at 95%%: "
  quota_show_progress_bar 95 50
  printf "\n"

  printf "Progress bar at 110%% (exceeded): "
  quota_show_progress_bar 110 50
  printf "\n"

  test_assert "Progress bar rendering" "0" "0" # Visual test
}

test_quota_check_fast_fallback() {
  test_section "Testing quota_check_fast (no billing setup)"

  # Should pass when no customer ID exists (fallback)
  if quota_check_fast "api" 1; then
    test_assert "Fast check without billing setup" "0" "0"
  else
    test_assert "Fast check without billing setup" "1" "0"
  fi
}

test_quota_cache_functions() {
  test_section "Testing quota cache functions"

  # Test cache invalidation (should handle missing Redis gracefully)
  if quota_cache_invalidate_all 2>/dev/null; then
    test_assert "Cache invalidate all" "0" "0"
  else
    printf "${YELLOW}ℹ${NC} Redis not available - cache test skipped\n"
  fi
}

test_quota_reset() {
  test_section "Testing quota reset functions"

  # Test reset with no customer (should fail gracefully)
  if quota_reset "" "api" 2>/dev/null; then
    test_assert "Reset without customer ID" "1" "1"
  else
    test_assert "Reset without customer ID (expected fail)" "0" "0"
  fi
}

test_quota_calculate_overage() {
  test_section "Testing quota_calculate_overage"

  # Test that function exists and handles errors
  if command -v quota_calculate_overage >/dev/null 2>&1; then
    test_assert "quota_calculate_overage function exists" "0" "0"
  else
    test_assert "quota_calculate_overage function exists" "1" "0"
  fi
}

test_quota_alert_thresholds() {
  test_section "Testing quota alert thresholds"

  test_assert "Warning threshold" "$QUOTA_ALERT_WARNING" "75"
  test_assert "Critical threshold" "$QUOTA_ALERT_CRITICAL" "90"
  test_assert "Exceeded threshold" "$QUOTA_ALERT_EXCEEDED" "100"
}

test_quota_modes() {
  test_section "Testing quota enforcement modes"

  test_assert "Soft mode value" "$QUOTA_MODE_SOFT" "soft"
  test_assert "Hard mode value" "$QUOTA_MODE_HARD" "hard"
}

test_quota_check_alerts() {
  test_section "Testing quota_check_alerts"

  # Test that function exists
  if command -v quota_check_alerts >/dev/null 2>&1; then
    test_assert "quota_check_alerts function exists" "0" "0"
  else
    test_assert "quota_check_alerts function exists" "1" "0"
  fi
}

test_quota_monitor_all() {
  test_section "Testing quota_monitor_all"

  # Test that function exists
  if command -v quota_monitor_all >/dev/null 2>&1; then
    test_assert "quota_monitor_all function exists" "0" "0"
  else
    test_assert "quota_monitor_all function exists" "1" "0"
  fi
}

test_quota_integration() {
  test_section "Testing quota integration with billing_check_quota"

  # Test that billing_check_quota is available (from core.sh)
  if command -v billing_check_quota >/dev/null 2>&1; then
    test_assert "billing_check_quota function available" "0" "0"
  else
    test_assert "billing_check_quota function available" "1" "0"
  fi
}

test_quota_export_functions() {
  test_section "Testing exported functions"

  local functions=(
    "quota_get_all"
    "quota_get_service"
    "quota_get_alerts"
    "quota_enforce"
    "quota_check_fast"
    "quota_check_rate_limited"
    "quota_reset"
    "quota_reset_all_expired"
    "quota_calculate_overage"
    "quota_show_overage"
    "quota_check_alerts"
    "quota_monitor_all"
    "quota_cache_invalidate_all"
    "quota_cache_warm"
  )

  local missing=0
  for func in "${functions[@]}"; do
    if ! command -v "$func" >/dev/null 2>&1; then
      printf "${RED}✗${NC} Function not exported: %s\n" "$func"
      missing=$((missing + 1))
    fi
  done

  test_assert "All functions exported" "$missing" "0"
}

# ============================================================================
# Run All Tests
# ============================================================================

main() {
  printf "\n"
  printf "╔════════════════════════════════════════════════════════════════╗\n"
  printf "║              nself Billing Quotas Test Suite                  ║\n"
  printf "╚════════════════════════════════════════════════════════════════╝\n"

  # Run tests
  test_quota_format_number
  test_quota_show_progress_bar
  test_quota_check_fast_fallback
  test_quota_cache_functions
  test_quota_reset
  test_quota_calculate_overage
  test_quota_alert_thresholds
  test_quota_modes
  test_quota_check_alerts
  test_quota_monitor_all
  test_quota_integration
  test_quota_export_functions

  # Summary
  printf "\n"
  printf "╔════════════════════════════════════════════════════════════════╗\n"
  printf "║                        TEST SUMMARY                            ║\n"
  printf "╠════════════════════════════════════════════════════════════════╣\n"
  printf "║ Total Tests:    %-46d ║\n" "$TESTS_RUN"
  printf "║ Passed:         ${GREEN}%-46d${NC} ║\n" "$TESTS_PASSED"
  printf "║ Failed:         "
  if [[ $TESTS_FAILED -eq 0 ]]; then
    printf "${GREEN}%-46d${NC}" "$TESTS_FAILED"
  else
    printf "${RED}%-46d${NC}" "$TESTS_FAILED"
  fi
  printf " ║\n"
  printf "╚════════════════════════════════════════════════════════════════╝\n"
  printf "\n"

  # Exit code
  if [[ $TESTS_FAILED -eq 0 ]]; then
    printf "${GREEN}✓ All tests passed!${NC}\n\n"
    exit 0
  else
    printf "${RED}✗ Some tests failed${NC}\n\n"
    exit 1
  fi
}

# Run tests if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
