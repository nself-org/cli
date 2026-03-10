#!/usr/bin/env bats
# test-plugin-license-commands.bats
# Tests for nself plugin, license, and frontend subcommands.
#
# T-0361 — CLI: frontend + plugin + license command integration tests
#
# No-Docker tier: --help for all subcommands (always runs).
# Integration tier: live operations requiring server or Docker
#   (skipped when SKIP_INTEGRATION_TESTS=1).

load test_helper

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

docker_available() {
  command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1
}

skip_if_no_integration() {
  if [ -n "${SKIP_INTEGRATION_TESTS:-}" ]; then
    skip "SKIP_INTEGRATION_TESTS is set — live operation tests skipped"
  fi
  if [ -n "${SKIP_DOCKER_TESTS:-}" ]; then
    skip "SKIP_DOCKER_TESTS is set — skipping integration tests"
  fi
  if ! docker_available; then
    skip "Docker not available in this environment"
  fi
}

# ---------------------------------------------------------------------------
# nself plugin subcommands
# ---------------------------------------------------------------------------

@test "nself plugin --help exits 0 and has output" {
  run nself plugin --help
  assert_success
  assert_output_length_gt 10
}

@test "nself plugin list --help exits 0 and has output" {
  run nself plugin list --help
  assert_success
  assert_output_length_gt 10
}

@test "nself plugin install --help exits 0 and has output" {
  run nself plugin install --help
  assert_success
  assert_output_length_gt 10
}

@test "nself plugin remove --help exits 0 and has output" {
  run nself plugin remove --help
  assert_success
  assert_output_length_gt 10
}

@test "nself plugin info --help exits 0 and has output" {
  run nself plugin info --help
  assert_success
  assert_output_length_gt 10
}

@test "nself plugin update --help exits 0 and has output" {
  run nself plugin update --help
  assert_success
  assert_output_length_gt 10
}

@test "nself plugin status --help exits 0 and has output" {
  run nself plugin status --help
  assert_success
  assert_output_length_gt 10
}

@test "nself plugin create --help exits 0 and has output" {
  run nself plugin create --help
  assert_success
  assert_output_length_gt 10
}

# ---------------------------------------------------------------------------
# nself license subcommands
# ---------------------------------------------------------------------------

@test "nself license --help exits 0 and has output" {
  run nself license --help
  assert_success
  assert_output_length_gt 10
}

@test "nself license set --help exits 0 and has output" {
  run nself license set --help
  assert_success
  assert_output_length_gt 10
}

@test "nself license show --help exits 0 and has output" {
  run nself license show --help
  assert_success
  assert_output_length_gt 10
}

@test "nself license validate --help exits 0 and has output" {
  run nself license validate --help
  assert_success
  assert_output_length_gt 10
}

@test "nself license clear --help exits 0 and has output" {
  run nself license clear --help
  assert_success
  assert_output_length_gt 10
}

# ---------------------------------------------------------------------------
# nself frontend subcommands (via dev frontend)
# ---------------------------------------------------------------------------

@test "nself frontend --help exits 0 and has output" {
  run nself frontend --help
  assert_success
  assert_output_length_gt 10
}

@test "nself frontend add --help exits 0 and has output" {
  run nself frontend add --help
  assert_success
  assert_output_length_gt 10
}

@test "nself frontend list --help exits 0 and has output" {
  run nself frontend list --help
  assert_success
  assert_output_length_gt 10
}

@test "nself frontend remove --help exits 0 and has output" {
  run nself frontend remove --help
  assert_success
  assert_output_length_gt 10
}

@test "nself frontend config --help exits 0 and has output" {
  run nself frontend config --help
  assert_success
  assert_output_length_gt 10
}

# ---------------------------------------------------------------------------
# nself dev frontend (canonical location per COMMAND-TREE-V1)
# ---------------------------------------------------------------------------

@test "nself dev frontend --help exits 0 and has output" {
  run nself dev frontend --help
  assert_success
  assert_output_length_gt 10
}

@test "nself dev frontend list --help exits 0 and has output" {
  run nself dev frontend list --help
  assert_success
  assert_output_length_gt 10
}

@test "nself dev frontend add --help exits 0 and has output" {
  run nself dev frontend add --help
  assert_success
  assert_output_length_gt 10
}

@test "nself dev frontend remove --help exits 0 and has output" {
  run nself dev frontend remove --help
  assert_success
  assert_output_length_gt 10
}

# ---------------------------------------------------------------------------
# Argument validation (no Docker or network required)
# ---------------------------------------------------------------------------

@test "nself plugin install without plugin name fails gracefully" {
  run nself plugin install
  assert_failure
}

@test "nself plugin remove without plugin name fails gracefully" {
  run nself plugin remove
  assert_failure
}

@test "nself license set without key argument fails gracefully" {
  run nself license set
  assert_failure
}

@test "nself frontend add without name and port fails gracefully" {
  run nself frontend add
  assert_failure
}

@test "nself frontend remove without name fails gracefully" {
  run nself frontend remove
  assert_failure
}

# ---------------------------------------------------------------------------
# Free plugin dry-run installs (no Docker required, no license key required)
# ---------------------------------------------------------------------------

@test "nself plugin install search --dry-run exits 0" {
  run nself plugin install search --dry-run
  assert_success
}

@test "nself plugin install notifications --dry-run exits 0" {
  run nself plugin install notifications --dry-run
  assert_success
}

@test "nself plugin install webhooks --dry-run exits 0" {
  run nself plugin install webhooks --dry-run
  assert_success
}

@test "nself plugin install link-preview --dry-run exits 0" {
  run nself plugin install link-preview --dry-run
  assert_success
}

@test "free plugin install does not require a license key" {
  local saved_key="${NSELF_PLUGIN_LICENSE_KEY:-}"
  unset NSELF_PLUGIN_LICENSE_KEY
  run nself plugin install search --dry-run
  assert_success
  if [ -n "$saved_key" ]; then
    export NSELF_PLUGIN_LICENSE_KEY="$saved_key"
  fi
}

# ---------------------------------------------------------------------------
# Live integration tests (require network + valid license key)
# ---------------------------------------------------------------------------

@test "nself license validate exits 0 with valid key" {
  skip_if_no_integration
  [ -n "${NSELF_PLUGIN_LICENSE_KEY:-}" ] || skip "NSELF_PLUGIN_LICENSE_KEY not set"
  run nself license validate
  assert_success
}

@test "nself plugin install realtime succeeds with running services" {
  skip_if_no_integration
  run nself plugin install realtime
  assert_success
}
