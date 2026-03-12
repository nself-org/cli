#!/usr/bin/env bats
# security_audit_test.bats
# Tests for nself security audit and related security commands.
# Help-flag tests require no running services.
# Audit behavior tests use temp env to simulate missing config.

load test_helper

# ---------------------------------------------------------------------------
# Setup / teardown
# ---------------------------------------------------------------------------

setup() {
  export TEST_PROJECT_DIR
  TEST_PROJECT_DIR=$(mktemp -d)
  # Create a minimal .env
  printf 'POSTGRES_PASSWORD=short\nHASURA_GRAPHQL_ADMIN_SECRET=short\n' \
    > "$TEST_PROJECT_DIR/.env"
}

teardown() {
  rm -rf "$TEST_PROJECT_DIR"
}

# ---------------------------------------------------------------------------
# Help flags
# ---------------------------------------------------------------------------

@test "nself security --help exits 0" {
  run nself security --help
  assert_success
}

@test "nself security audit --help exits 0" {
  run nself security audit --help
  assert_success
}

@test "nself harden --help exits 0" {
  run nself harden --help
  assert_success
}

@test "nself vault --help exits 0" {
  run nself vault --help
  assert_success
}

@test "nself vault set --help exits 0" {
  run nself vault set --help
  assert_success
}

@test "nself vault get --help exits 0" {
  run nself vault get --help
  assert_success
}

# ---------------------------------------------------------------------------
# Audit behavioral tests
# ---------------------------------------------------------------------------

@test "nself security audit detects missing JWT secret" {
  cd "$TEST_PROJECT_DIR"
  # .env has no JWT secret — audit must flag it
  run nself security audit --no-docker
  # Audit exits non-zero when issues found, and mentions JWT
  assert_output --regexp "JWT|jwt"
}

@test "nself security audit detects short admin secret" {
  cd "$TEST_PROJECT_DIR"
  printf 'HASURA_GRAPHQL_ADMIN_SECRET=short\n' >> .env
  run nself security audit --no-docker
  assert_output --regexp "admin secret|ADMIN_SECRET|too short|minimum"
}

@test "nself security audit detects short postgres password" {
  cd "$TEST_PROJECT_DIR"
  printf 'POSTGRES_PASSWORD=weak\n' >> .env
  run nself security audit --no-docker
  assert_output --regexp "POSTGRES_PASSWORD|postgres password|too short|minimum"
}

@test "nself security audit --format json outputs valid json" {
  cd "$TEST_PROJECT_DIR"
  run nself security audit --no-docker --format json
  # Output should be parseable JSON — basic check: starts with { or [
  assert_output --regexp '^\s*[\[{]'
}

# ---------------------------------------------------------------------------
# Build security warnings
# ---------------------------------------------------------------------------

@test "nself build warns on weak secrets" {
  cd "$TEST_PROJECT_DIR"
  run nself build --check
  # Either exits non-zero or outputs a warning about weak secrets
  [ "$status" -ne 0 ] || assert_output --regexp "warning|weak|secret|minimum"
}
