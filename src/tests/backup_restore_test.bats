#!/usr/bin/env bats
# backup_restore_test.bats
# Tests for nself backup and restore commands.
# Help-flag tests require no running services.
# Live backup/restore tests require Docker with services running.

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
# Help flags (no Docker required)
# ---------------------------------------------------------------------------

@test "nself backup --help exits 0" {
  run nself backup --help
  assert_success
}

@test "nself backup create --help exits 0" {
  run nself backup create --help
  assert_success
}

@test "nself backup list --help exits 0" {
  run nself backup list --help
  assert_success
}

@test "nself backup restore --help exits 0" {
  run nself backup restore --help
  assert_success
}

@test "nself backup delete --help exits 0" {
  run nself backup delete --help
  assert_success
}

@test "nself backup schedule --help exits 0" {
  run nself backup schedule --help
  assert_success
}

# ---------------------------------------------------------------------------
# Argument validation (no Docker required)
# ---------------------------------------------------------------------------

@test "nself backup restore without name fails" {
  run nself backup restore
  assert_failure
  assert_output --regexp "name|backup|required|argument|usage"
}

@test "nself backup delete without name fails" {
  run nself backup delete
  assert_failure
}

@test "nself backup restore with nonexistent backup name fails" {
  run nself backup restore --name "backup-that-does-not-exist-zzz"
  assert_failure
  assert_output --regexp "not found|does not exist|no backup"
}

# ---------------------------------------------------------------------------
# Live backup / restore cycle (Docker required)
# ---------------------------------------------------------------------------

@test "nself backup create produces a backup file" {
  skip_if_no_integration
  BACKUP_NAME="test-backup-$(date +%s)"
  run nself backup create --name "$BACKUP_NAME"
  assert_success
  assert_output --regexp "$BACKUP_NAME|created|success"
}

@test "nself backup list shows created backup" {
  skip_if_no_integration
  BACKUP_NAME="test-list-$(date +%s)"
  nself backup create --name "$BACKUP_NAME"
  run nself backup list
  assert_success
  assert_output --regexp "$BACKUP_NAME"
}

@test "nself backup restore succeeds with valid backup" {
  skip_if_no_integration
  BACKUP_NAME="test-restore-$(date +%s)"
  nself backup create --name "$BACKUP_NAME"
  run nself backup restore --name "$BACKUP_NAME" --force
  assert_success
}

@test "nself backup delete removes a backup" {
  skip_if_no_integration
  BACKUP_NAME="test-delete-$(date +%s)"
  nself backup create --name "$BACKUP_NAME"
  run nself backup delete --name "$BACKUP_NAME"
  assert_success
  # Verify it is gone
  run nself backup list
  refute_output --partial "$BACKUP_NAME"
}
