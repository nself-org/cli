#!/usr/bin/env bats
# command_tree_test.bats
# Verifies that all 30 top-level CLI commands respond to --help with exit 0.
# Each test is intentionally lightweight — just confirms the command exists and
# outputs usage. No Docker or network required.

load test_helper

# ---------------------------------------------------------------------------
# Root
# ---------------------------------------------------------------------------

@test "nself --help exits 0" {
  run nself --help
  assert_success
}

@test "nself help exits 0" {
  run nself help
  assert_success
}

@test "nself version exits 0" {
  run nself version
  assert_success
  assert_output --partial "nself"
}

# ---------------------------------------------------------------------------
# Core infrastructure commands
# ---------------------------------------------------------------------------

@test "nself init --help exits 0" {
  run nself init --help
  assert_success
}

@test "nself build --help exits 0" {
  run nself build --help
  assert_success
}

@test "nself start --help exits 0" {
  run nself start --help
  assert_success
}

@test "nself stop --help exits 0" {
  run nself stop --help
  assert_success
}

@test "nself restart --help exits 0" {
  run nself restart --help
  assert_success
}

@test "nself status --help exits 0" {
  run nself status --help
  assert_success
}

@test "nself health --help exits 0" {
  run nself health --help
  assert_success
}

# ---------------------------------------------------------------------------
# Database
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

# ---------------------------------------------------------------------------
# Plugins and licensing
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

# ---------------------------------------------------------------------------
# Frontend
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

# ---------------------------------------------------------------------------
# Tenancy
# ---------------------------------------------------------------------------

@test "nself tenant --help exits 0" {
  run nself tenant --help
  assert_success
}

@test "nself tenant create --help exits 0" {
  run nself tenant create --help
  assert_success
}

@test "nself tenant list --help exits 0" {
  run nself tenant list --help
  assert_success
}

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

@test "nself config --help exits 0" {
  run nself config --help
  assert_success
}

@test "nself config validate --help exits 0" {
  run nself config validate --help
  assert_success
}

@test "nself env --help exits 0" {
  run nself env --help
  assert_success
}

# ---------------------------------------------------------------------------
# Deployment
# ---------------------------------------------------------------------------

@test "nself deploy --help exits 0" {
  run nself deploy --help
  assert_success
}

@test "nself staging --help exits 0" {
  run nself staging --help
  assert_success
}

# ---------------------------------------------------------------------------
# Security
# ---------------------------------------------------------------------------

@test "nself security --help exits 0" {
  run nself security --help
  assert_success
}

@test "nself audit --help exits 0" {
  run nself audit --help
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

# ---------------------------------------------------------------------------
# Auth
# ---------------------------------------------------------------------------

@test "nself auth --help exits 0" {
  run nself auth --help
  assert_success
}

# ---------------------------------------------------------------------------
# Backup and restore
# ---------------------------------------------------------------------------

@test "nself backup --help exits 0" {
  run nself backup --help
  assert_success
}

# ---------------------------------------------------------------------------
# Service and admin
# ---------------------------------------------------------------------------

@test "nself service --help exits 0" {
  run nself service --help
  assert_success
}

@test "nself admin --help exits 0" {
  run nself admin --help
  assert_success
}

@test "nself doctor --help exits 0" {
  run nself doctor --help
  assert_success
}

@test "nself urls --help exits 0" {
  run nself urls --help
  assert_success
}

@test "nself ssl --help exits 0" {
  run nself ssl --help
  assert_success
}

@test "nself completion --help exits 0" {
  run nself completion --help
  assert_success
}

@test "nself update --help exits 0" {
  run nself update --help
  assert_success
}
