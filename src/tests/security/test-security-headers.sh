#!/usr/bin/env bash
set -euo pipefail
# test-security-headers.sh - Security headers testing
# POSIX-compliant, no Bash 4+ features

# Get script directory
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NSELF_ROOT="$(cd "$TEST_DIR/../../.." && pwd)"

# Source test framework
source "$NSELF_ROOT/src/tests/lib/test-framework.sh" 2>/dev/null || {
  printf "Error: Test framework not found\n"
  exit 1
}

# Source security headers library
source "$NSELF_ROOT/src/lib/security/headers.sh" 2>/dev/null || {
  printf "Error: Security headers library not found\n"
  exit 1
}

# Source CSP library
source "$NSELF_ROOT/src/lib/security/csp.sh" 2>/dev/null || {
  printf "Error: CSP library not found\n"
  exit 1
}

# ============================================================================
# CSP Tests
# ============================================================================

test_csp_default_is_strict() {
  # V098-P1-013: Default CSP mode should be strict
  local csp
  csp=$(csp::generate)

  assert_not_contains "$csp" "unsafe-inline" "Default CSP should not contain unsafe-inline"
  assert_not_contains "$csp" "unsafe-eval" "Default CSP should not contain unsafe-eval"
  assert_contains "$csp" "script-src 'self'" "Default CSP should have strict script-src"
}

test_csp_generate_strict() {
  local csp
  csp=$(csp::generate "strict")

  assert_contains "$csp" "default-src 'self'" "CSP strict mode should contain default-src 'self'"
  assert_contains "$csp" "script-src 'self'" "CSP strict mode should have strict script-src"
  assert_not_contains "$csp" "unsafe-inline" "CSP strict mode should not contain unsafe-inline"
  assert_not_contains "$csp" "unsafe-eval" "CSP strict mode should not contain unsafe-eval"
  assert_contains "$csp" "object-src 'none'" "CSP should block objects"
  assert_contains "$csp" "frame-ancestors 'none'" "CSP should prevent framing"
}

test_csp_generate_moderate() {
  local csp
  csp=$(csp::generate "moderate")

  assert_contains "$csp" "default-src 'self'" "CSP moderate mode should contain default-src 'self'"
  assert_contains "$csp" "unsafe-inline" "CSP moderate mode should contain unsafe-inline"
  assert_contains "$csp" "unsafe-eval" "CSP moderate mode should contain unsafe-eval"
  assert_contains "$csp" "upgrade-insecure-requests" "CSP should upgrade insecure requests"
}

test_csp_generate_permissive() {
  local csp
  csp=$(csp::generate "permissive")

  assert_contains "$csp" "unsafe-inline" "CSP permissive mode should contain unsafe-inline"
  assert_contains "$csp" "unsafe-eval" "CSP permissive mode should contain unsafe-eval"
  assert_contains "$csp" "https:" "CSP permissive mode should allow HTTPS"
}

test_csp_add_domain() {
  # Clear any existing domains
  export CSP_CUSTOM_DOMAINS=""

  # Add domain
  csp::add_domain "cdn.example.com" >/dev/null 2>&1

  assert_contains "$CSP_CUSTOM_DOMAINS" "cdn.example.com" "Domain should be added to CSP_CUSTOM_DOMAINS"
}

test_csp_validate_syntax() {
  local valid_csp="default-src 'self'; script-src 'self'; object-src 'none'"

  # Should succeed (return 0)
  if csp::validate "$valid_csp" >/dev/null 2>&1; then
    pass "Valid CSP passes validation"
  else
    fail "Valid CSP failed validation"
  fi
}

test_csp_service_specific() {
  local hasura_csp grafana_csp minio_csp

  hasura_csp=$(csp::generate_for_service "hasura")
  assert_contains "$hasura_csp" "ws:" "Hasura CSP should allow WebSocket"
  assert_contains "$hasura_csp" "wss:" "Hasura CSP should allow secure WebSocket"

  grafana_csp=$(csp::generate_for_service "grafana")
  assert_contains "$grafana_csp" "unsafe-inline" "Grafana CSP should allow inline for dashboards"

  minio_csp=$(csp::generate_for_service "minio")
  assert_contains "$minio_csp" "blob:" "MinIO CSP should allow blob URLs"
}

# ============================================================================
# Security Headers Tests
# ============================================================================

test_headers_generate_all() {
  local headers
  headers=$(headers::generate_all "strict" "true")

  assert_not_empty "$headers" "Generated headers should not be empty"
  assert_contains "$headers" "Content-Security-Policy" "Should include CSP header"
  assert_contains "$headers" "X-Frame-Options" "Should include X-Frame-Options"
  assert_contains "$headers" "X-Content-Type-Options" "Should include X-Content-Type-Options"
  assert_contains "$headers" "Strict-Transport-Security" "Should include HSTS when SSL enabled"
  assert_contains "$headers" "Referrer-Policy" "Should include Referrer-Policy"
}

test_headers_generate_hsts() {
  export HSTS_MAX_AGE=31536000
  export HSTS_INCLUDE_SUBDOMAINS=true
  export HSTS_PRELOAD=false

  local hsts
  hsts=$(headers::generate_hsts)

  assert_contains "$hsts" "max-age=31536000" "HSTS should have correct max-age"
  assert_contains "$hsts" "includeSubDomains" "HSTS should include subdomains"
  assert_not_contains "$hsts" "preload" "HSTS should not have preload when disabled"

  # Test with preload enabled
  export HSTS_PRELOAD=true
  hsts=$(headers::generate_hsts)
  assert_contains "$hsts" "preload" "HSTS should have preload when enabled"
}

test_headers_generate_permissions_policy() {
  export PERMISSIONS_POLICY_CAMERA="()"
  export PERMISSIONS_POLICY_MICROPHONE="()"
  export PERMISSIONS_POLICY_GEOLOCATION="()"

  local policy
  policy=$(headers::generate_permissions_policy)

  assert_contains "$policy" "camera=()" "Permissions policy should deny camera"
  assert_contains "$policy" "microphone=()" "Permissions policy should deny microphone"
  assert_contains "$policy" "geolocation=()" "Permissions policy should deny geolocation"
  assert_contains "$policy" "interest-cohort=()" "Permissions policy should deny FLoC"
}

test_headers_no_hsts_without_ssl() {
  local headers
  headers=$(headers::generate_all "moderate" "false")

  assert_not_contains "$headers" "Strict-Transport-Security" "Should not include HSTS when SSL disabled"
}

# ============================================================================
# Integration Tests
# ============================================================================

test_export_nginx_format() {
  local temp_file
  temp_file=$(mktemp)

  # Export headers
  if headers::export_nginx "$temp_file" "true" >/dev/null 2>&1; then
    # Check file exists
    assert_file_exists "$temp_file" "Exported nginx config should exist"

    # Check content
    local content
    content=$(cat "$temp_file")
    assert_contains "$content" "add_header" "Nginx config should contain add_header directives"
    assert_contains "$content" "Content-Security-Policy" "Nginx config should include CSP"

    rm -f "$temp_file"
  else
    fail "Failed to export nginx format"
  fi
}

test_csp_export_nginx() {
  local temp_file
  temp_file=$(mktemp)

  # Export CSP
  if csp::export_nginx "moderate" "$temp_file" >/dev/null 2>&1; then
    assert_file_exists "$temp_file" "Exported CSP config should exist"

    local content
    content=$(cat "$temp_file")
    assert_contains "$content" "Content-Security-Policy" "CSP export should contain CSP header"
    assert_contains "$content" "add_header" "CSP export should use nginx format"

    rm -f "$temp_file"
  else
    fail "Failed to export CSP to nginx format"
  fi
}

# ============================================================================
# Test Suite Configuration
# ============================================================================

test_suite_description="Security Headers Testing"

# Run all tests
run_test_suite \
  test_csp_default_is_strict \
  test_csp_generate_strict \
  test_csp_generate_moderate \
  test_csp_generate_permissive \
  test_csp_add_domain \
  test_csp_validate_syntax \
  test_csp_service_specific \
  test_headers_generate_all \
  test_headers_generate_hsts \
  test_headers_generate_permissions_policy \
  test_headers_no_hsts_without_ssl \
  test_export_nginx_format \
  test_csp_export_nginx
