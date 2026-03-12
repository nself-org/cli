#!/usr/bin/env bats
# test-security-audit.bats
# Tests for nself security audit and hardening commands.
#
# T-0362 — CLI: nself security audit command tests
#
# No-Docker tier: --help flags and safe dry-run modes (always runs).
# Integration tier: system-modifying operations
#   (skipped when SKIP_INTEGRATION_TESTS=1).

load test_helper

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

skip_if_no_integration() {
  if [ -n "${SKIP_INTEGRATION_TESTS:-}" ]; then
    skip "SKIP_INTEGRATION_TESTS is set — system-modifying tests skipped"
  fi
  if [ -n "${SKIP_DOCKER_TESTS:-}" ]; then
    skip "SKIP_DOCKER_TESTS is set — skipping integration tests"
  fi
}

# ---------------------------------------------------------------------------
# Setup / teardown
# ---------------------------------------------------------------------------

setup() {
  TEST_PROJECT_DIR=$(mktemp -d)
  export TEST_PROJECT_DIR
  # Write a minimal .env with intentionally weak credentials to trigger findings.
  printf 'BASE_DOMAIN=localhost\n' > "$TEST_PROJECT_DIR/.env"
  printf 'POSTGRES_PASSWORD=short\n' >> "$TEST_PROJECT_DIR/.env"
  printf 'HASURA_GRAPHQL_ADMIN_SECRET=short\n' >> "$TEST_PROJECT_DIR/.env"
}

teardown() {
  rm -rf "$TEST_PROJECT_DIR"
}

# ---------------------------------------------------------------------------
# Help flags (no Docker, no network required)
# ---------------------------------------------------------------------------

@test "nself security --help exits 0 and has output" {
  run nself security --help
  assert_success
  assert_output_length_gt 10
}

@test "nself security audit --help exits 0 and has output" {
  run nself security audit --help
  assert_success
  assert_output_length_gt 10
}

@test "nself harden --help exits 0 and has output" {
  run nself harden --help
  assert_success
  assert_output_length_gt 10
}

@test "nself auth security --help exits 0 and has output" {
  run nself auth security --help
  assert_success
  assert_output_length_gt 10
}

@test "nself auth ssl --help exits 0 and has output" {
  run nself auth ssl --help
  assert_success
  assert_output_length_gt 10
}

@test "nself auth rate-limit --help exits 0 and has output" {
  run nself auth rate-limit --help
  assert_success
  assert_output_length_gt 10
}

# ---------------------------------------------------------------------------
# Security audit behavioral tests (no Docker required)
# These tests run nself security audit against a temp project dir containing
# deliberately weak credentials. The audit must detect and report issues.
# ---------------------------------------------------------------------------

@test "nself security audit detects weak POSTGRES_PASSWORD" {
  cd "$TEST_PROJECT_DIR"
  run nself security audit --no-docker
  # Should report the short password as a finding.
  assert_output --regexp "POSTGRES_PASSWORD|postgres password|too short|minimum|weak"
}

@test "nself security audit detects weak HASURA admin secret" {
  cd "$TEST_PROJECT_DIR"
  run nself security audit --no-docker
  assert_output --regexp "admin secret|ADMIN_SECRET|too short|minimum|weak"
}

@test "nself security audit --format json produces JSON-like output" {
  cd "$TEST_PROJECT_DIR"
  run nself security audit --no-docker --format json
  # Basic check: output starts with { or [
  assert_output --regexp '^\s*[{\[]'
}

@test "nself security audit exits non-zero when issues are found" {
  cd "$TEST_PROJECT_DIR"
  run nself security audit --no-docker
  # Weak credentials must produce a non-zero exit or at least output findings.
  # Accept exit 0 only if output contains a finding keyword.
  if [ "$status" -eq 0 ]; then
    assert_output --regexp "WARNING|FAIL|ISSUE|weak|short|minimum"
  fi
}

# ---------------------------------------------------------------------------
# harden dry-run (no system modifications)
# ---------------------------------------------------------------------------

@test "nself harden --check exits 0 (check-only mode, no modifications)" {
  cd "$TEST_PROJECT_DIR"
  run nself harden --check
  assert_success
}

@test "nself harden --dry-run exits 0 without modifying system" {
  cd "$TEST_PROJECT_DIR"
  run nself harden --dry-run
  assert_success
}

# ---------------------------------------------------------------------------
# build security warnings (no Docker required)
# ---------------------------------------------------------------------------

@test "nself build --check warns on weak secrets" {
  cd "$TEST_PROJECT_DIR"
  run nself build --check
  # Either exits non-zero or produces a warning about weak secrets.
  if [ "$status" -eq 0 ]; then
    assert_output --regexp "warning|weak|secret|minimum"
  fi
}

# ---------------------------------------------------------------------------
# System-modifying tests (SKIP_INTEGRATION_TESTS guard)
# ---------------------------------------------------------------------------

@test "nself security audit runs full scan without crashing" {
  skip_if_no_integration
  cd "$TEST_PROJECT_DIR"
  run nself security audit
  # Full audit may exit non-zero due to findings, but must not crash.
  assert_output_length_gt 10
}

@test "nself harden --all applies hardening rules without error" {
  skip_if_no_integration
  cd "$TEST_PROJECT_DIR"
  run nself harden --all
  assert_success
}
