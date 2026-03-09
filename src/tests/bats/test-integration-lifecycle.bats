#!/usr/bin/env bats
# test-integration-lifecycle.bats
# Full build -> start -> health -> stop integration test.
#
# T-0359 — CLI: full build -> start -> health -> stop integration test
#
# Two tiers of tests:
#   1. No-Docker tier: verifies that nself build generates correct output files
#      with the right structure (always runs in CI).
#   2. Docker tier: verifies actual service lifecycle (start, status, stop).
#      Skipped when SKIP_DOCKER_TESTS=1 (default in CI unless DinD is available).
#
# Usage in CI:
#   SKIP_DOCKER_TESTS=1 bats src/tests/bats/test-integration-lifecycle.bats

load test_helper

# ---------------------------------------------------------------------------
# Setup / teardown
# ---------------------------------------------------------------------------

setup() {
  TEST_PROJECT_DIR=$(mktemp -d)
  export TEST_PROJECT_DIR
}

teardown() {
  # Best-effort cleanup — stop services if running before removing the dir.
  if [ -z "${SKIP_DOCKER_TESTS:-}" ]; then
    if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
      if [ -f "$TEST_PROJECT_DIR/docker-compose.yml" ]; then
        cd "$TEST_PROJECT_DIR"
        nself stop >/dev/null 2>&1 || true
      fi
    fi
  fi
  cd /
  rm -rf "$TEST_PROJECT_DIR"
}

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

docker_available() {
  command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1
}

skip_if_no_docker() {
  if [ -n "${SKIP_DOCKER_TESTS:-}" ]; then
    skip "SKIP_DOCKER_TESTS is set — Docker-dependent tests skipped"
  fi
  if ! docker_available; then
    skip "Docker not available in this environment"
  fi
}

# Write a minimal valid .env into TEST_PROJECT_DIR.
write_minimal_env() {
  printf 'BASE_DOMAIN=localhost\n' > "$TEST_PROJECT_DIR/.env"
  printf 'POSTGRES_PASSWORD=test-lifecycle-postgres-ci\n' >> "$TEST_PROJECT_DIR/.env"
  printf 'HASURA_GRAPHQL_ADMIN_SECRET=test-lifecycle-admin-ci\n' >> "$TEST_PROJECT_DIR/.env"
  printf 'NSELF_JWT_SECRET=test-lifecycle-jwt-secret-ci-32chars\n' >> "$TEST_PROJECT_DIR/.env"
}

# ---------------------------------------------------------------------------
# Phase 1: nself init
# ---------------------------------------------------------------------------

@test "nself init --help exits 0" {
  run nself init --help
  assert_success
  assert_output_length_gt 10
}

@test "nself init creates .env file in project dir" {
  cd "$TEST_PROJECT_DIR"
  run nself init --simple
  # init may exit non-zero in non-interactive mode but should still create .env
  [ -f "$TEST_PROJECT_DIR/.env" ] || {
    printf "Expected .env to be created by nself init\n" >&2
    return 1
  }
}

# ---------------------------------------------------------------------------
# Phase 2: nself build (no Docker required)
# ---------------------------------------------------------------------------

@test "nself build --help exits 0" {
  run nself build --help
  assert_success
  assert_output_length_gt 10
}

@test "nself build generates docker-compose.yml" {
  cd "$TEST_PROJECT_DIR"
  write_minimal_env
  run nself build --allow-insecure
  assert_success
  [ -f "$TEST_PROJECT_DIR/docker-compose.yml" ] || {
    printf "docker-compose.yml not generated after nself build\n" >&2
    return 1
  }
}

@test "nself build docker-compose.yml contains required service names" {
  cd "$TEST_PROJECT_DIR"
  write_minimal_env
  nself build --allow-insecure >/dev/null 2>&1 || true
  [ -f "$TEST_PROJECT_DIR/docker-compose.yml" ] || skip "docker-compose.yml not present — build step failed"
  # Verify required services appear in the generated compose file.
  for svc in postgres hasura auth nginx; do
    if ! grep -q "$svc" "$TEST_PROJECT_DIR/docker-compose.yml"; then
      printf "Expected service '%s' in docker-compose.yml\n" "$svc" >&2
      return 1
    fi
  done
}

@test "nself build generates nginx directory" {
  cd "$TEST_PROJECT_DIR"
  write_minimal_env
  nself build --allow-insecure >/dev/null 2>&1 || true
  [ -d "$TEST_PROJECT_DIR/nginx" ] || {
    printf "nginx/ directory not generated after nself build\n" >&2
    return 1
  }
}

@test "nself build generates nginx.conf" {
  cd "$TEST_PROJECT_DIR"
  write_minimal_env
  nself build --allow-insecure >/dev/null 2>&1 || true
  # nginx.conf may be at nginx/nginx.conf or nginx/conf/nginx.conf
  local found=0
  [ -f "$TEST_PROJECT_DIR/nginx/nginx.conf" ] && found=1
  [ -f "$TEST_PROJECT_DIR/nginx/conf/nginx.conf" ] && found=1
  if [ "$found" -eq 0 ]; then
    printf "nginx.conf not found under nginx/ after nself build\n" >&2
    return 1
  fi
}

@test "nself build nginx config contains server block" {
  cd "$TEST_PROJECT_DIR"
  write_minimal_env
  nself build --allow-insecure >/dev/null 2>&1 || true
  local conf=""
  [ -f "$TEST_PROJECT_DIR/nginx/nginx.conf" ] && conf="$TEST_PROJECT_DIR/nginx/nginx.conf"
  [ -f "$TEST_PROJECT_DIR/nginx/conf/nginx.conf" ] && conf="$TEST_PROJECT_DIR/nginx/conf/nginx.conf"
  [ -n "$conf" ] || skip "nginx.conf not found — build step failed"
  if ! grep -q "server" "$conf" && ! grep -rq "server" "$TEST_PROJECT_DIR/nginx/"; then
    printf "Expected 'server' block in nginx config\n" >&2
    return 1
  fi
}

@test "nself build docker-compose.yml services bind to 127.0.0.1 (not 0.0.0.0)" {
  cd "$TEST_PROJECT_DIR"
  write_minimal_env
  nself build --allow-insecure >/dev/null 2>&1 || true
  [ -f "$TEST_PROJECT_DIR/docker-compose.yml" ] || skip "docker-compose.yml not present"
  # Internal services must NOT be publicly bound.
  # Grep for 0.0.0.0 bindings on internal service ports (5432, 8080, 4000, 9000).
  if grep -E '"0\.0\.0\.0:(5432|8080|4000|9000)' "$TEST_PROJECT_DIR/docker-compose.yml"; then
    printf "SECURITY: Internal service bound to 0.0.0.0 — must use 127.0.0.1\n" >&2
    return 1
  fi
}

# ---------------------------------------------------------------------------
# Phase 3: Docker lifecycle (requires Docker)
# ---------------------------------------------------------------------------

@test "nself start --help exits 0" {
  run nself start --help
  assert_success
  assert_output_length_gt 10
}

@test "nself status --help exits 0" {
  run nself status --help
  assert_success
  assert_output_length_gt 10
}

@test "nself stop --help exits 0" {
  run nself stop --help
  assert_success
  assert_output_length_gt 10
}

@test "nself start launches services and nself status reports healthy" {
  skip_if_no_docker
  cd "$TEST_PROJECT_DIR"
  write_minimal_env
  nself build --allow-insecure
  run nself start
  assert_success
  run nself status
  assert_success
  assert_output --partial "running\|healthy\|up"
}

@test "nself stop shuts down all services cleanly" {
  skip_if_no_docker
  cd "$TEST_PROJECT_DIR"
  write_minimal_env
  nself build --allow-insecure
  nself start >/dev/null 2>&1 || true
  run nself stop
  assert_success
}

@test "nself health reports service state after start" {
  skip_if_no_docker
  cd "$TEST_PROJECT_DIR"
  write_minimal_env
  nself build --allow-insecure
  nself start >/dev/null 2>&1
  run nself health
  assert_success
}
