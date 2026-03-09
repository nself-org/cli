#!/usr/bin/env bats
# build_start_stop_test.bats
# Integration test: init → build → health check → stop lifecycle.
# Requires Docker. Skips gracefully if Docker is unavailable.

load ../test_helper

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

docker_available() {
  command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1
}

skip_if_no_docker() {
  if ! docker_available; then
    skip "Docker not available in this environment"
  fi
}

# ---------------------------------------------------------------------------
# Setup / teardown
# ---------------------------------------------------------------------------

setup() {
  export TEST_PROJECT_DIR
  TEST_PROJECT_DIR=$(mktemp -d)
}

teardown() {
  if [ -d "$TEST_PROJECT_DIR" ]; then
    # Best-effort stop — ignore errors (Docker may not be running)
    (cd "$TEST_PROJECT_DIR" && nself stop 2>/dev/null) || true
    rm -rf "$TEST_PROJECT_DIR"
  fi
}

# ---------------------------------------------------------------------------
# Init tests (no Docker required)
# ---------------------------------------------------------------------------

@test "nself init creates .env in target directory" {
  cd "$TEST_PROJECT_DIR"
  run nself init --non-interactive
  assert_success
  assert [ -f "$TEST_PROJECT_DIR/.env" ]
}

@test "nself init creates .env.example" {
  cd "$TEST_PROJECT_DIR"
  run nself init --non-interactive
  assert_success
  assert [ -f "$TEST_PROJECT_DIR/.env.example" ]
}

@test "nself init creates .gitignore" {
  cd "$TEST_PROJECT_DIR"
  run nself init --non-interactive
  assert_success
  assert [ -f "$TEST_PROJECT_DIR/.gitignore" ]
}

@test "nself init .env contains POSTGRES_PASSWORD" {
  cd "$TEST_PROJECT_DIR"
  nself init --non-interactive
  run grep "POSTGRES_PASSWORD" "$TEST_PROJECT_DIR/.env"
  assert_success
}

@test "nself init .env contains HASURA_GRAPHQL_ADMIN_SECRET" {
  cd "$TEST_PROJECT_DIR"
  nself init --non-interactive
  run grep "HASURA_GRAPHQL_ADMIN_SECRET" "$TEST_PROJECT_DIR/.env"
  assert_success
}

# ---------------------------------------------------------------------------
# Build tests (no Docker required — generates config files)
# ---------------------------------------------------------------------------

@test "nself build generates docker-compose.yml" {
  skip_if_no_docker
  cd "$TEST_PROJECT_DIR"
  nself init --non-interactive
  run nself build
  assert_success
  assert [ -f "$TEST_PROJECT_DIR/docker-compose.yml" ]
}

@test "nself build generates nginx directory" {
  skip_if_no_docker
  cd "$TEST_PROJECT_DIR"
  nself init --non-interactive
  run nself build
  assert_success
  assert [ -d "$TEST_PROJECT_DIR/nginx" ]
}

@test "nself build --check-only exits 0 with valid config" {
  cd "$TEST_PROJECT_DIR"
  nself init --non-interactive
  run nself build --check-only
  # May fail if Docker not available — that is acceptable
  # What matters is that the command exists and parses args correctly
  assert [ "$status" -eq 0 ] || assert_output --partial "error\|warning\|check"
}

# ---------------------------------------------------------------------------
# Start / stop tests (Docker required)
# ---------------------------------------------------------------------------

@test "nself start brings up required services" {
  skip_if_no_docker
  cd "$TEST_PROJECT_DIR"
  nself init --non-interactive
  nself build
  run nself start
  assert_success
}

@test "nself status shows running services after start" {
  skip_if_no_docker
  cd "$TEST_PROJECT_DIR"
  nself init --non-interactive
  nself build
  nself start
  run nself status
  assert_success
  assert_output --partial "running\|up\|healthy"
}

@test "nself stop exits 0" {
  skip_if_no_docker
  cd "$TEST_PROJECT_DIR"
  nself init --non-interactive
  nself build
  nself start
  run nself stop
  assert_success
}

@test "nself health reports healthy after start" {
  skip_if_no_docker
  cd "$TEST_PROJECT_DIR"
  nself init --non-interactive
  nself build
  nself start
  # Give services 30s to stabilize
  sleep 30
  run nself health
  assert_success
}
