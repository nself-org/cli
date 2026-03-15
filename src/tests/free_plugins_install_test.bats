#!/usr/bin/env bats
# free_plugins_install_test.bats
# Install matrix for all free plugins.
# Uses --dry-run flag to verify install logic without Docker.
# Live install tests require Docker and are marked accordingly.

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

nself_initialized() {
  [[ -f ".env" ]] || return 1
  command -v nself >/dev/null 2>&1 || return 1
  nself status >/dev/null 2>&1 || return 1
}

skip_if_no_integration() {
  skip_if_no_docker
  if ! nself_initialized; then
    skip "nself project not initialized (requires nself init + running services)"
  fi
}

# ---------------------------------------------------------------------------
# Dry-run install tests (no Docker required)
# ---------------------------------------------------------------------------

@test "free plugin install content-acquisition --dry-run succeeds" {
  run nself plugin install content-acquisition --dry-run
  assert_success
}

@test "free plugin install content-progress --dry-run succeeds" {
  run nself plugin install content-progress --dry-run
  assert_success
}

@test "free plugin install feature-flags --dry-run succeeds" {
  run nself plugin install feature-flags --dry-run
  assert_success
}

@test "free plugin install github --dry-run succeeds" {
  run nself plugin install github --dry-run
  assert_success
}

@test "free plugin install github-runner --dry-run succeeds" {
  run nself plugin install github-runner --dry-run
  assert_success
}

@test "free plugin install invitations --dry-run succeeds" {
  run nself plugin install invitations --dry-run
  assert_success
}

@test "free plugin install jobs --dry-run succeeds" {
  run nself plugin install jobs --dry-run
  assert_success
}

@test "free plugin install link-preview --dry-run succeeds" {
  run nself plugin install link-preview --dry-run
  assert_success
}

@test "free plugin install mdns --dry-run succeeds" {
  run nself plugin install mdns --dry-run
  assert_success
}

@test "free plugin install notifications --dry-run succeeds" {
  run nself plugin install notifications --dry-run
  assert_success
}

@test "free plugin install search --dry-run succeeds" {
  run nself plugin install search --dry-run
  assert_success
}

@test "free plugin install subtitle-manager --dry-run succeeds" {
  run nself plugin install subtitle-manager --dry-run
  assert_success
}

@test "free plugin install tokens --dry-run succeeds" {
  run nself plugin install tokens --dry-run
  assert_success
}

@test "free plugin install torrent-manager --dry-run succeeds" {
  run nself plugin install torrent-manager --dry-run
  assert_success
}

@test "free plugin install vpn --dry-run succeeds" {
  run nself plugin install vpn --dry-run
  assert_success
}

@test "free plugin install webhooks --dry-run succeeds" {
  run nself plugin install webhooks --dry-run
  assert_success
}

# ---------------------------------------------------------------------------
# No license key required for free plugins
# ---------------------------------------------------------------------------

@test "free plugin install does not require a license key" {
  # Unset any license key, verify free plugin still installs (dry-run)
  OLD_KEY="${NSELF_PLUGIN_LICENSE_KEY:-}"
  unset NSELF_PLUGIN_LICENSE_KEY
  run nself plugin install search --dry-run
  assert_success
  if [ -n "$OLD_KEY" ]; then
    export NSELF_PLUGIN_LICENSE_KEY="$OLD_KEY"
  fi
}

# ---------------------------------------------------------------------------
# Live install tests (Docker required)
# ---------------------------------------------------------------------------

@test "free plugin install search runs health check" {
  skip_if_no_integration
  nself plugin install search
  run curl -fsS "http://localhost:3302/health"
  assert_success
  assert_output --regexp "ok|healthy|status"
}

@test "free plugin install webhooks runs health check" {
  skip_if_no_integration
  nself plugin install webhooks
  # Webhooks plugin uses the main Hasura port — check Hasura is up
  run nself health --service webhooks
  assert_success
}
