#!/usr/bin/env bats
# free_plugins_remove_test.bats
# Uninstall matrix for all free plugins.
# Uses --dry-run flag to verify remove logic without Docker.

load test_helper

# ---------------------------------------------------------------------------
# Dry-run remove tests (no Docker required)
# ---------------------------------------------------------------------------

@test "free plugin remove content-acquisition --dry-run succeeds" {
  run nself plugin remove content-acquisition --dry-run
  assert_success
}

@test "free plugin remove content-progress --dry-run succeeds" {
  run nself plugin remove content-progress --dry-run
  assert_success
}

@test "free plugin remove feature-flags --dry-run succeeds" {
  run nself plugin remove feature-flags --dry-run
  assert_success
}

@test "free plugin remove github --dry-run succeeds" {
  run nself plugin remove github --dry-run
  assert_success
}

@test "free plugin remove github-runner --dry-run succeeds" {
  run nself plugin remove github-runner --dry-run
  assert_success
}

@test "free plugin remove invitations --dry-run succeeds" {
  run nself plugin remove invitations --dry-run
  assert_success
}

@test "free plugin remove jobs --dry-run succeeds" {
  run nself plugin remove jobs --dry-run
  assert_success
}

@test "free plugin remove link-preview --dry-run succeeds" {
  run nself plugin remove link-preview --dry-run
  assert_success
}

@test "free plugin remove mdns --dry-run succeeds" {
  run nself plugin remove mdns --dry-run
  assert_success
}

@test "free plugin remove notifications --dry-run succeeds" {
  run nself plugin remove notifications --dry-run
  assert_success
}

@test "free plugin remove search --dry-run succeeds" {
  run nself plugin remove search --dry-run
  assert_success
}

@test "free plugin remove subtitle-manager --dry-run succeeds" {
  run nself plugin remove subtitle-manager --dry-run
  assert_success
}

@test "free plugin remove tokens --dry-run succeeds" {
  run nself plugin remove tokens --dry-run
  assert_success
}

@test "free plugin remove torrent-manager --dry-run succeeds" {
  run nself plugin remove torrent-manager --dry-run
  assert_success
}

@test "free plugin remove vpn --dry-run succeeds" {
  run nself plugin remove vpn --dry-run
  assert_success
}

@test "free plugin remove webhooks --dry-run succeeds" {
  run nself plugin remove webhooks --dry-run
  assert_success
}

# ---------------------------------------------------------------------------
# Remove argument validation
# ---------------------------------------------------------------------------

@test "nself plugin remove without name fails" {
  run nself plugin remove
  assert_failure
  assert_output --regexp "name|plugin|required|argument"
}

@test "nself plugin remove unknown plugin fails gracefully" {
  run nself plugin remove no-such-plugin-xyz --dry-run
  assert_failure
  assert_output --regexp "not found|unknown|not installed|error"
}
