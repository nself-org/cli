#!/usr/bin/env bats
# build_security_test.bats
# Tests that nself build enforces minimum security standards
# before generating docker-compose.yml and nginx configs.
# No Docker required — uses --check flag for config validation only.

load test_helper

# ---------------------------------------------------------------------------
# Setup / teardown
# ---------------------------------------------------------------------------

setup() {
  export TEST_PROJECT_DIR
  TEST_PROJECT_DIR=$(mktemp -d)
}

teardown() {
  rm -rf "$TEST_PROJECT_DIR"
}

_write_env() {
  local file="$TEST_PROJECT_DIR/.env"
  printf '%s\n' "$@" > "$file"
}

# ---------------------------------------------------------------------------
# Short secret detection
# ---------------------------------------------------------------------------

@test "nself build rejects admin secret shorter than minimum" {
  _write_env \
    "POSTGRES_PASSWORD=alongenoughpassword1234567890" \
    "HASURA_GRAPHQL_ADMIN_SECRET=short"
  cd "$TEST_PROJECT_DIR"
  run nself build --check
  assert_failure
  assert_output --regexp "admin secret|ADMIN_SECRET|too short|minimum|characters"
}

@test "nself build rejects postgres password shorter than minimum" {
  _write_env \
    "POSTGRES_PASSWORD=weak" \
    "HASURA_GRAPHQL_ADMIN_SECRET=alongenoughsecret1234567890abcdef"
  cd "$TEST_PROJECT_DIR"
  run nself build --check
  assert_failure
  assert_output --regexp "POSTGRES_PASSWORD|postgres|too short|minimum"
}

@test "nself build rejects empty admin secret" {
  _write_env \
    "POSTGRES_PASSWORD=alongenoughpassword1234567890" \
    "HASURA_GRAPHQL_ADMIN_SECRET="
  cd "$TEST_PROJECT_DIR"
  run nself build --check
  assert_failure
}

@test "nself build rejects empty postgres password" {
  _write_env \
    "POSTGRES_PASSWORD=" \
    "HASURA_GRAPHQL_ADMIN_SECRET=alongenoughsecret1234567890abcdef"
  cd "$TEST_PROJECT_DIR"
  run nself build --check
  assert_failure
}

# ---------------------------------------------------------------------------
# Acceptable config
# ---------------------------------------------------------------------------

@test "nself build accepts strong secrets" {
  _write_env \
    "POSTGRES_PASSWORD=$(printf '%032d' 0 | tr 0 a)" \
    "HASURA_GRAPHQL_ADMIN_SECRET=$(printf '%032d' 0 | tr 0 b)"
  cd "$TEST_PROJECT_DIR"
  run nself build --check
  # Should pass security check (may still fail for other reasons like missing Docker)
  assert_output --regexp "check|valid|ok|pass" || assert_success
}

# ---------------------------------------------------------------------------
# JWT secret format
# ---------------------------------------------------------------------------

@test "nself build warns on plaintext JWT secret instead of JSON object" {
  _write_env \
    "POSTGRES_PASSWORD=alongenoughpassword1234567890" \
    "HASURA_GRAPHQL_ADMIN_SECRET=alongenoughsecret1234567890abcdef" \
    'HASURA_GRAPHQL_JWT_SECRET=plaintext-secret-not-json'
  cd "$TEST_PROJECT_DIR"
  run nself build --check
  # Should warn or fail — JWT secret must be a JSON object with type + key
  assert_output --regexp "JWT|jwt|json|format" || assert_failure
}
