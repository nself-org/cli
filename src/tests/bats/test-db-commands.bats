#!/usr/bin/env bats
# test-db-commands.bats
# Tests for nself db subcommands.
#
# T-0360 — CLI: db command integration tests
#
# No-Docker tier: --help and argument validation (always runs).
# Docker tier: actual db operations (skipped when SKIP_DOCKER_TESTS=1).

load test_helper

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

# ---------------------------------------------------------------------------
# Top-level db command
# ---------------------------------------------------------------------------

@test "nself db --help exits 0 and has output" {
  run nself db --help
  assert_success
  assert_output_length_gt 10
}

@test "nself db without args shows usage" {
  run nself db
  # Either prints usage (exit 0) or exits non-zero with usage text.
  assert_output_length_gt 10
}

# ---------------------------------------------------------------------------
# db migrate subcommand
# ---------------------------------------------------------------------------

@test "nself db migrate --help exits 0" {
  run nself db migrate --help
  assert_success
  assert_output_length_gt 10
}

@test "nself db migrate up --help exits 0" {
  run nself db migrate up --help
  assert_success
  assert_output_length_gt 10
}

@test "nself db migrate status --help exits 0" {
  run nself db migrate status --help
  assert_success
  assert_output_length_gt 10
}

# ---------------------------------------------------------------------------
# db shell subcommand
# ---------------------------------------------------------------------------

@test "nself db shell --help exits 0" {
  run nself db shell --help
  assert_success
  assert_output_length_gt 10
}

# ---------------------------------------------------------------------------
# db seed subcommand
# ---------------------------------------------------------------------------

@test "nself db seed --help exits 0" {
  run nself db seed --help
  assert_success
  assert_output_length_gt 10
}

# ---------------------------------------------------------------------------
# db backup / restore subcommands
# ---------------------------------------------------------------------------

@test "nself db backup --help exits 0" {
  run nself db backup --help
  assert_success
  assert_output_length_gt 10
}

@test "nself db restore --help exits 0" {
  run nself db restore --help
  assert_success
  assert_output_length_gt 10
}

# ---------------------------------------------------------------------------
# db hasura subcommands
# ---------------------------------------------------------------------------

@test "nself db hasura --help exits 0" {
  run nself db hasura --help
  assert_success
  assert_output_length_gt 10
}

@test "nself db hasura metadata --help exits 0" {
  run nself db hasura metadata --help
  assert_success
  assert_output_length_gt 10
}

@test "nself db hasura console --help exits 0" {
  run nself db hasura console --help
  assert_success
  assert_output_length_gt 10
}

# ---------------------------------------------------------------------------
# Additional db subcommands from COMMAND-TREE-V1
# ---------------------------------------------------------------------------

@test "nself db schema --help exits 0" {
  run nself db schema --help
  assert_success
  assert_output_length_gt 10
}

@test "nself db query --help exits 0" {
  run nself db query --help
  assert_success
  assert_output_length_gt 10
}

@test "nself db inspect --help exits 0" {
  run nself db inspect --help
  assert_success
  assert_output_length_gt 10
}

# ---------------------------------------------------------------------------
# Argument validation (no Docker required)
# ---------------------------------------------------------------------------

@test "nself db restore without required file argument fails gracefully" {
  run nself db restore
  assert_failure
}

@test "nself db seed with nonexistent file fails gracefully" {
  run nself db seed --file /nonexistent/path/to/seed.sql
  assert_failure
}

@test "nself db migrate with unknown flag fails gracefully" {
  run nself db migrate --nonexistent-flag-xyz-abc
  assert_failure
}

# ---------------------------------------------------------------------------
# Live DB operations (Docker required)
# ---------------------------------------------------------------------------

@test "nself db migrate --status exits 0 with running services" {
  skip_if_no_docker
  run nself db migrate --status
  assert_success
}

@test "nself db migrate applies pending migrations" {
  skip_if_no_docker
  run nself db migrate up
  assert_success
}

@test "nself db backup creates a backup file" {
  skip_if_no_docker
  local backup_file
  backup_file=$(mktemp /tmp/nself-backup-XXXX.sql)
  run nself db backup --output "$backup_file"
  assert_success
  [ -s "$backup_file" ] || {
    printf "Expected backup file to be non-empty: %s\n" "$backup_file" >&2
    return 1
  }
  rm -f "$backup_file"
}

@test "nself db inspect lists tables" {
  skip_if_no_docker
  run nself db inspect
  assert_success
}
