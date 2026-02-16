#!/usr/bin/env bash
# test-auth.sh - Comprehensive unit tests for auth system
# Part of nself v0.6.0 - Phase 1 Sprint 1 (AUTH-008)
#
# Tests all authentication methods, provider management, and OAuth flows

set -euo pipefail

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NSELF_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

# Source the code under test
source "$NSELF_ROOT/src/lib/utils/display.sh" 2>/dev/null || true
source "$NSELF_ROOT/src/lib/auth/password-utils.sh" 2>/dev/null || true
source "$NSELF_ROOT/src/lib/auth/magic-link.sh" 2>/dev/null || true
source "$NSELF_ROOT/src/lib/auth/providers/oauth/oauth-base.sh" 2>/dev/null || true

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# ============================================================================
# Test Utilities
# ============================================================================

# Assert equals
assert_equals() {
  local expected="$1"
  local actual="$2"
  local test_name="${3:-test}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ "$expected" == "$actual" ]]; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "✓ %s\n" "$test_name"
    return 0
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ %s: expected '%s', got '%s'\n" "$test_name" "$expected" "$actual"
    return 1
  fi
}

# Assert not empty
assert_not_empty() {
  local actual="$1"
  local test_name="${2:-test}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ -n "$actual" ]]; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "✓ %s\n" "$test_name"
    return 0
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ %s: expected non-empty value\n" "$test_name"
    return 1
  fi
}

# Assert function exists
assert_function_exists() {
  local func_name="$1"
  local test_name="${2:-test}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if declare -f "$func_name" >/dev/null 2>&1; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "✓ %s\n" "$test_name"
    return 0
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ %s: function '%s' not found\n" "$test_name" "$func_name"
    return 1
  fi
}

# ============================================================================
# Password Utilities Tests
# ============================================================================

test_password_hashing() {
  printf "\n=== Password Hashing Tests ===\n"

  # Test password hashing
  local password="Test@123"
  local hash
  hash=$(hash_password "$password" 2>/dev/null || echo "")

  assert_not_empty "$hash" "password hashing produces hash"

  # Test password verification (positive case)
  if [[ -n "$hash" ]]; then
    if verify_password "$password" "$hash" 2>/dev/null; then
      TESTS_RUN=$((TESTS_RUN + 1))
      TESTS_PASSED=$((TESTS_PASSED + 1))
      printf "✓ password verification (correct password)\n"
    else
      TESTS_RUN=$((TESTS_RUN + 1))
      TESTS_FAILED=$((TESTS_FAILED + 1))
      printf "✗ password verification (correct password): failed\n"
    fi

    # Test password verification (negative case)
    if ! verify_password "WrongPassword" "$hash" 2>/dev/null; then
      TESTS_RUN=$((TESTS_RUN + 1))
      TESTS_PASSED=$((TESTS_PASSED + 1))
      printf "✓ password verification (wrong password)\n"
    else
      TESTS_RUN=$((TESTS_RUN + 1))
      TESTS_FAILED=$((TESTS_FAILED + 1))
      printf "✗ password verification (wrong password): should have failed\n"
    fi
  fi

  # Test password strength validation (valid password)
  if validate_password_strength "ValidPass123" 2>/dev/null; then
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "✓ password strength validation (valid password)\n"
  else
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ password strength validation (valid password): should have passed\n"
  fi

  # Test password strength validation (too short)
  if ! validate_password_strength "Short1" 2>/dev/null; then
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "✓ password strength validation (too short)\n"
  else
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ password strength validation (too short): should have failed\n"
  fi

  # Test password strength validation (no uppercase)
  if ! validate_password_strength "nouppercase1" 2>/dev/null; then
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "✓ password strength validation (no uppercase)\n"
  else
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ password strength validation (no uppercase): should have failed\n"
  fi

  # Test password strength validation (no lowercase)
  if ! validate_password_strength "NOLOWERCASE1" 2>/dev/null; then
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "✓ password strength validation (no lowercase)\n"
  else
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ password strength validation (no lowercase): should have failed\n"
  fi

  # Test password strength validation (no digit)
  if ! validate_password_strength "NoDigitPass" 2>/dev/null; then
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "✓ password strength validation (no digit)\n"
  else
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ password strength validation (no digit): should have failed\n"
  fi

  # Test random password generation
  local random_pass
  random_pass=$(generate_password 16 2>/dev/null || echo "")

  assert_not_empty "$random_pass" "random password generation"

  if [[ -n "$random_pass" ]]; then
    local length=${#random_pass}
    assert_equals "16" "$length" "random password length"
  fi
}

# ============================================================================
# Magic Link Tests
# ============================================================================

test_magic_link() {
  printf "\n=== Magic Link Tests ===\n"

  # Test magic link token generation
  local token
  token=$(generate_magic_link_token 2>/dev/null || echo "")

  assert_not_empty "$token" "magic link token generation"

  if [[ -n "$token" ]]; then
    local length=${#token}
    assert_equals "64" "$length" "magic link token length (32 bytes = 64 hex chars)"
  fi
}

# ============================================================================
# OAuth Base Tests
# ============================================================================

test_oauth_base() {
  printf "\n=== OAuth Base Tests ===\n"

  # Test OAuth state generation
  local state
  state=$(oauth_generate_state 2>/dev/null || echo "")

  assert_not_empty "$state" "OAuth state generation"

  if [[ -n "$state" ]]; then
    local length=${#state}
    assert_equals "32" "$length" "OAuth state length (16 bytes = 32 hex chars)"
  fi

  # Test OAuth authorization URL building
  local auth_url
  auth_url=$(oauth_build_auth_url \
    "https://example.com/oauth/authorize" \
    "test_client_id" \
    "https://example.com/callback" \
    "openid profile email" \
    "test_state" 2>/dev/null || echo "")

  assert_not_empty "$auth_url" "OAuth auth URL building"

  # Verify URL contains required parameters
  if [[ -n "$auth_url" ]]; then
    if echo "$auth_url" | grep -q "client_id=test_client_id"; then
      TESTS_RUN=$((TESTS_RUN + 1))
      TESTS_PASSED=$((TESTS_PASSED + 1))
      printf "✓ OAuth auth URL contains client_id\n"
    else
      TESTS_RUN=$((TESTS_RUN + 1))
      TESTS_FAILED=$((TESTS_FAILED + 1))
      printf "✗ OAuth auth URL missing client_id\n"
    fi

    if echo "$auth_url" | grep -q "state=test_state"; then
      TESTS_RUN=$((TESTS_RUN + 1))
      TESTS_PASSED=$((TESTS_PASSED + 1))
      printf "✓ OAuth auth URL contains state\n"
    else
      TESTS_RUN=$((TESTS_RUN + 1))
      TESTS_FAILED=$((TESTS_FAILED + 1))
      printf "✗ OAuth auth URL missing state\n"
    fi
  fi
}

# ============================================================================
# Function Existence Tests
# ============================================================================

test_function_existence() {
  printf "\n=== Function Existence Tests ===\n"

  # Password utilities
  assert_function_exists "hash_password" "hash_password function exists"
  assert_function_exists "verify_password" "verify_password function exists"
  assert_function_exists "generate_password" "generate_password function exists"
  assert_function_exists "validate_password_strength" "validate_password_strength function exists"

  # Magic link utilities
  assert_function_exists "generate_magic_link_token" "generate_magic_link_token function exists"
  assert_function_exists "create_magic_link" "create_magic_link function exists"
  assert_function_exists "verify_magic_link" "verify_magic_link function exists"

  # OAuth base utilities
  assert_function_exists "oauth_generate_state" "oauth_generate_state function exists"
  assert_function_exists "oauth_build_auth_url" "oauth_build_auth_url function exists"
  assert_function_exists "oauth_exchange_code" "oauth_exchange_code function exists"
  assert_function_exists "oauth_refresh_token" "oauth_refresh_token function exists"
  assert_function_exists "oauth_get_user_info" "oauth_get_user_info function exists"
  assert_function_exists "oauth_revoke_token" "oauth_revoke_token function exists"
  assert_function_exists "oauth_store_state" "oauth_store_state function exists"
  assert_function_exists "oauth_verify_state" "oauth_verify_state function exists"
}

# ============================================================================
# OAuth Provider Tests
# ============================================================================

test_oauth_providers() {
  printf "\n=== OAuth Provider Tests ===\n"

  # Test Google provider
  if [[ -f "$NSELF_ROOT/src/lib/auth/providers/oauth/google.sh" ]]; then
    source "$NSELF_ROOT/src/lib/auth/providers/oauth/google.sh" 2>/dev/null || true

    assert_function_exists "google_get_auth_url" "google_get_auth_url function exists"
    assert_function_exists "google_exchange_code" "google_exchange_code function exists"
    assert_function_exists "google_get_user_info" "google_get_user_info function exists"
    assert_function_exists "google_refresh_token" "google_refresh_token function exists"
    assert_function_exists "google_revoke_token" "google_revoke_token function exists"
  else
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ Google provider file not found\n"
  fi

  # Test GitHub provider
  if [[ -f "$NSELF_ROOT/src/lib/auth/providers/oauth/github.sh" ]]; then
    source "$NSELF_ROOT/src/lib/auth/providers/oauth/github.sh" 2>/dev/null || true

    assert_function_exists "github_get_auth_url" "github_get_auth_url function exists"
    assert_function_exists "github_exchange_code" "github_exchange_code function exists"
    assert_function_exists "github_get_user_info" "github_get_user_info function exists"
    assert_function_exists "github_refresh_token" "github_refresh_token function exists"
    assert_function_exists "github_revoke_token" "github_revoke_token function exists"
  else
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ GitHub provider file not found\n"
  fi

  # Test Apple provider
  if [[ -f "$NSELF_ROOT/src/lib/auth/providers/oauth/apple.sh" ]]; then
    source "$NSELF_ROOT/src/lib/auth/providers/oauth/apple.sh" 2>/dev/null || true

    assert_function_exists "apple_get_auth_url" "apple_get_auth_url function exists"
    assert_function_exists "apple_exchange_code" "apple_exchange_code function exists"
    assert_function_exists "apple_get_user_info" "apple_get_user_info function exists"
    assert_function_exists "apple_refresh_token" "apple_refresh_token function exists"
    assert_function_exists "apple_revoke_token" "apple_revoke_token function exists"
  else
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ Apple provider file not found\n"
  fi

  # Test Facebook provider
  if [[ -f "$NSELF_ROOT/src/lib/auth/providers/oauth/facebook.sh" ]]; then
    source "$NSELF_ROOT/src/lib/auth/providers/oauth/facebook.sh" 2>/dev/null || true

    assert_function_exists "facebook_get_auth_url" "facebook_get_auth_url function exists"
    assert_function_exists "facebook_exchange_code" "facebook_exchange_code function exists"
    assert_function_exists "facebook_get_user_info" "facebook_get_user_info function exists"
    assert_function_exists "facebook_refresh_token" "facebook_refresh_token function exists"
    assert_function_exists "facebook_revoke_token" "facebook_revoke_token function exists"
  else
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ Facebook provider file not found\n"
  fi

  # Test Twitter provider
  if [[ -f "$NSELF_ROOT/src/lib/auth/providers/oauth/twitter.sh" ]]; then
    source "$NSELF_ROOT/src/lib/auth/providers/oauth/twitter.sh" 2>/dev/null || true

    assert_function_exists "twitter_get_auth_url" "twitter_get_auth_url function exists"
    assert_function_exists "twitter_exchange_code" "twitter_exchange_code function exists"
    assert_function_exists "twitter_get_user_info" "twitter_get_user_info function exists"
    assert_function_exists "twitter_refresh_token" "twitter_refresh_token function exists"
    assert_function_exists "twitter_revoke_token" "twitter_revoke_token function exists"
  else
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ Twitter provider file not found\n"
  fi
}

# ============================================================================
# Main Test Runner
# ============================================================================

main() {
  printf "\n╔═══════════════════════════════════════════════╗\n"
  printf "║  nself Authentication Unit Tests (AUTH-008)   ║\n"
  printf "╚═══════════════════════════════════════════════╝\n"

  # Run all test suites
  test_function_existence
  test_password_hashing
  test_magic_link
  test_oauth_base
  test_oauth_providers

  # Print results
  printf "\n╔═══════════════════════════════════════════════╗\n"
  printf "║                Test Results                   ║\n"
  printf "╚═══════════════════════════════════════════════╝\n"
  printf "Total tests:  %d\n" "$TESTS_RUN"
  printf "Passed:       %d ✓\n" "$TESTS_PASSED"
  printf "Failed:       %d ✗\n" "$TESTS_FAILED"

  if [[ $TESTS_FAILED -eq 0 ]]; then
    printf "\n✓ All tests passed! (100%%)\n"
    exit 0
  else
    local pass_rate=$(((TESTS_PASSED * 100) / TESTS_RUN))
    printf "\n⚠ Pass rate: %d%%\n" "$pass_rate"
    exit 1
  fi
}

# Run tests
main "$@"
