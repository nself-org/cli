#!/usr/bin/env bats
# commands_test.bats
# Integration tests for frontend, plugin, and license commands.
# Help-flag tests require no running services.
# Live tests require Docker and a valid license key.

load test_helper

# ---------------------------------------------------------------------------
# Frontend commands
# ---------------------------------------------------------------------------

@test "nself frontend --help exits 0" {
  run nself frontend --help
  assert_success
}

@test "nself frontend list --help exits 0" {
  run nself frontend list --help
  assert_success
}

@test "nself frontend add --help exits 0" {
  run nself frontend add --help
  assert_success
}

@test "nself frontend remove --help exits 0" {
  run nself frontend remove --help
  assert_success
}

@test "nself frontend status --help exits 0" {
  run nself frontend status --help
  assert_success
}

# ---------------------------------------------------------------------------
# Plugin commands
# ---------------------------------------------------------------------------

@test "nself plugin --help exits 0" {
  run nself plugin --help
  assert_success
}

@test "nself plugin list --help exits 0" {
  run nself plugin list --help
  assert_success
}

@test "nself plugin install --help exits 0" {
  run nself plugin install --help
  assert_success
}

@test "nself plugin remove --help exits 0" {
  run nself plugin remove --help
  assert_success
}

@test "nself plugin update --help exits 0" {
  run nself plugin update --help
  assert_success
}

@test "nself plugin info --help exits 0" {
  run nself plugin info --help
  assert_success
}

@test "nself plugin install without plugin name fails" {
  run nself plugin install
  assert_failure
  assert_output --regexp "plugin|name|required|usage"
}

@test "nself plugin remove without plugin name fails" {
  run nself plugin remove
  assert_failure
}

@test "nself plugin install unknown-plugin-zzz fails" {
  run nself plugin install unknown-plugin-zzz-that-does-not-exist
  assert_failure
}

# ---------------------------------------------------------------------------
# License commands
# ---------------------------------------------------------------------------

@test "nself license --help exits 0" {
  run nself license --help
  assert_success
}

@test "nself license show --help exits 0" {
  run nself license show --help
  assert_success
}

@test "nself license validate --help exits 0" {
  run nself license validate --help
  assert_success
}

@test "nself license set --help exits 0" {
  run nself license set --help
  assert_success
}

@test "nself license validate without key argument fails" {
  run nself license validate
  assert_failure
  assert_output --regexp "license key|key|required|argument"
}

@test "nself license set without key argument fails" {
  run nself license set
  assert_failure
}

@test "nself license validate with malformed key fails" {
  run nself license validate "not-a-valid-key"
  assert_failure
  assert_output --regexp "invalid|format|key"
}
