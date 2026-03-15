#!/usr/bin/env bats
# db_commands_test.bats
# Tests for nself db subcommands.
# Help-flag tests require no running services.
# Live DB tests require Docker and are skipped without it.

load test_helper

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
# Help flags (no Docker required)
# ---------------------------------------------------------------------------

@test "nself db --help exits 0" {
  run nself db --help
  assert_success
}

@test "nself db migrate --help exits 0" {
  run nself db migrate --help
  assert_success
}

@test "nself db shell --help exits 0" {
  run nself db shell --help
  assert_success
}

@test "nself db seed --help exits 0" {
  run nself db seed --help
  assert_success
}

@test "nself db backup --help exits 0" {
  run nself db backup --help
  assert_success
}

@test "nself db restore --help exits 0" {
  run nself db restore --help
  assert_success
}

@test "nself db hasura --help exits 0" {
  run nself db hasura --help
  assert_success
}

@test "nself db hasura metadata --help exits 0" {
  run nself db hasura metadata --help
  assert_success
}

# ---------------------------------------------------------------------------
# Argument validation (no Docker required)
# ---------------------------------------------------------------------------

@test "nself db migrate with unknown flag fails gracefully" {
  run nself db migrate --nonexistent-flag-xyz
  # Should fail with a meaningful message, not a crash/stack trace
  assert_failure
  assert_output --regexp "error|unknown|usage|help"
}

@test "nself db restore without required args fails gracefully" {
  run nself db restore
  assert_failure
}

@test "nself db seed with invalid file fails gracefully" {
  run nself db seed --file /nonexistent/path/to/seed.sql
  assert_failure
}

# ---------------------------------------------------------------------------
# Live DB tests (Docker required)
# ---------------------------------------------------------------------------

nself_initialized() {
  [[ -f ".env" ]] || return 1
  command -v nself >/dev/null 2>&1 || return 1
  nself status >/dev/null 2>&1 || return 1
}

skip_if_no_integration() {
  skip_if_no_docker
  if \! nself_initialized; then
    skip "nself project not initialized (requires nself init + running services)"
  fi
}

@test "nself db migrate status exits 0 with running services" {
  skip_if_no_integration
  run nself db migrate status
  assert_success
}

@test "nself db migrate applies pending migrations" {
  skip_if_no_docker
  run nself db migrate
  assert_success
}
