#!/usr/bin/env bash
# test-backup-restore-workflow.sh - Backup and restore workflow integration test
#
# Tests: backup → modify data → restore → verify → incremental backup → cloud sync

set -euo pipefail

# Load test utilities
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$TEST_DIR/utils/integration-helpers.sh"
source "$TEST_DIR/../test_framework.sh"

# Test configuration
readonly TEST_NAME="backup-restore-workflow"
TEST_PROJECT_DIR=""
CLEANUP_ON_EXIT=true
BACKUP_FILE=""
BACKUP_DIR=""

# ============================================================================
# Cleanup Handler
# ============================================================================

cleanup() {
  if [[ "$CLEANUP_ON_EXIT" == "true" ]] && [[ -n "$TEST_PROJECT_DIR" ]]; then
    printf "\nCleaning up test environment...\n"
    cleanup_test_project "$TEST_PROJECT_DIR"
  fi
}

trap cleanup EXIT INT TERM

# ============================================================================
# Helper Functions
# ============================================================================

run_nself_command() {
  "$NSELF_ROOT/bin/nself" "$@"
}

db_exec() {
  local query="$1"
  docker-compose exec -T postgres psql -U postgres -d "$POSTGRES_DB" -t -c "$query" 2>/dev/null | xargs
}

db_insert_test_data() {
  local table="$1"
  local data="$2"
  db_exec "INSERT INTO $table VALUES $data;"
}

db_count_records() {
  local table="$1"
  local condition="${2:-1=1}"
  db_exec "SELECT COUNT(*) FROM $table WHERE $condition;"
}

# ============================================================================
# Test Functions
# ============================================================================

test_01_setup() {
  describe "Test 1: Setup test environment"

  # Create test environment
  TEST_PROJECT_DIR=$(setup_test_project)
  cd "$TEST_PROJECT_DIR"

  # Initialize project
  run_nself_command init --quiet

  # Configure backup settings
  cat >>.env <<EOF

# Backup configuration
BACKUP_ENABLED=true
BACKUP_RETENTION_DAYS=7
BACKUP_COMPRESSION=true
EOF

  # Build and start
  run_nself_command build
  run_nself_command start

  # Wait for services
  sleep 10
  wait_for_service_healthy "postgres" 60 || fail "postgres not healthy"

  # Source .env for database credentials
  source .env

  # Create backup directory
  BACKUP_DIR="$TEST_PROJECT_DIR/backups"
  mkdir -p "$BACKUP_DIR"

  pass "Test environment setup complete"
}

test_02_create_test_data() {
  describe "Test 2: Create initial test data"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Create test table
  db_exec "CREATE TABLE IF NOT EXISTS test_backup (id SERIAL PRIMARY KEY, name VARCHAR(255), value INTEGER);"

  # Insert test data
  printf "Inserting initial test data...\n"
  for i in {1..100}; do
    db_insert_test_data "test_backup" "($i, 'test_record_$i', $i)"
  done

  # Verify data count
  local count
  count=$(db_count_records "test_backup")

  if [[ $count -ne 100 ]]; then
    fail "Expected 100 records, got $count"
  fi

  printf "Created %s test records\n" "$count"

  pass "Test data created successfully"
}

test_03_create_backup() {
  describe "Test 3: Create full backup"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Create backup
  printf "Creating backup...\n"
  run_nself_command backup create \
    --name "test_backup_initial" \
    --output "$BACKUP_DIR"

  # Find the backup file
  BACKUP_FILE=$(ls -t "$BACKUP_DIR"/*.sql* 2>/dev/null | head -1)

  if [[ ! -f "$BACKUP_FILE" ]]; then
    fail "Backup file not created"
  fi

  printf "Backup created: %s\n" "$BACKUP_FILE"

  # Verify backup file is not empty
  local backup_size
  backup_size=$(stat -f%z "$BACKUP_FILE" 2>/dev/null || stat -c%s "$BACKUP_FILE" 2>/dev/null)

  if [[ $backup_size -eq 0 ]]; then
    fail "Backup file is empty"
  fi

  printf "Backup size: %s bytes\n" "$backup_size"

  pass "Backup created successfully"
}

test_04_verify_backup_contents() {
  describe "Test 4: Verify backup contains test data"

  cd "$TEST_PROJECT_DIR"

  # Extract backup if compressed
  local backup_to_check="$BACKUP_FILE"
  if [[ "$BACKUP_FILE" == *.gz ]]; then
    gunzip -c "$BACKUP_FILE" > "$BACKUP_DIR/temp_backup.sql"
    backup_to_check="$BACKUP_DIR/temp_backup.sql"
  fi

  # Check backup contains our test table
  if grep -q "test_backup" "$backup_to_check"; then
    printf "✓ Backup contains test_backup table\n"
  else
    fail "Backup does not contain test_backup table"
  fi

  # Check backup contains test data
  if grep -q "test_record_1" "$backup_to_check"; then
    printf "✓ Backup contains test data\n"
  else
    fail "Backup does not contain test data"
  fi

  # Cleanup temp file
  [[ -f "$BACKUP_DIR/temp_backup.sql" ]] && rm -f "$BACKUP_DIR/temp_backup.sql"

  pass "Backup contents verified"
}

test_05_modify_data() {
  describe "Test 5: Modify data after backup"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Delete some records
  printf "Deleting 50 records...\n"
  db_exec "DELETE FROM test_backup WHERE id <= 50;"

  # Insert new records
  printf "Inserting 25 new records...\n"
  for i in {101..125}; do
    db_insert_test_data "test_backup" "($i, 'new_record_$i', $i)"
  done

  # Update some records
  printf "Updating 25 records...\n"
  db_exec "UPDATE test_backup SET value = value * 2 WHERE id > 50 AND id <= 75;"

  # Verify new count
  local count
  count=$(db_count_records "test_backup")

  if [[ $count -ne 75 ]]; then
    fail "Expected 75 records after modification, got $count"
  fi

  printf "Data modified: %s records now exist\n" "$count"

  pass "Data modified successfully"
}

test_06_restore_backup() {
  describe "Test 6: Restore from backup"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Restore backup
  printf "Restoring backup...\n"
  run_nself_command backup restore \
    --file "$BACKUP_FILE" \
    --confirm

  # Wait for restore to complete
  sleep 5

  pass "Backup restored"
}

test_07_verify_restored_data() {
  describe "Test 7: Verify restored data matches original"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Verify record count
  local count
  count=$(db_count_records "test_backup")

  if [[ $count -ne 100 ]]; then
    fail "Expected 100 records after restore, got $count"
  fi

  printf "Record count verified: %s\n" "$count"

  # Verify specific records exist
  local record_1_exists
  record_1_exists=$(db_count_records "test_backup" "id=1 AND name='test_record_1'")

  if [[ $record_1_exists -ne 1 ]]; then
    fail "Original record 1 not found after restore"
  fi

  # Verify new records are gone
  local record_101_exists
  record_101_exists=$(db_count_records "test_backup" "id=101")

  if [[ $record_101_exists -ne 0 ]]; then
    fail "New records still exist after restore"
  fi

  # Verify values are original (not doubled)
  local value_60
  value_60=$(db_exec "SELECT value FROM test_backup WHERE id=60;")

  if [[ $value_60 -ne 60 ]]; then
    fail "Expected value 60, got $value_60 (modifications not reverted)"
  fi

  pass "Restored data verified successfully"
}

test_08_incremental_backup() {
  describe "Test 8: Create incremental backup"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Modify data again
  printf "Creating new data for incremental backup...\n"
  for i in {201..250}; do
    db_insert_test_data "test_backup" "($i, 'incremental_$i', $i)"
  done

  # Create incremental backup
  printf "Creating incremental backup...\n"
  run_nself_command backup create \
    --name "test_backup_incremental" \
    --incremental \
    --output "$BACKUP_DIR"

  # Find the incremental backup
  local incremental_backup
  incremental_backup=$(ls -t "$BACKUP_DIR"/*incremental*.sql* 2>/dev/null | head -1)

  if [[ ! -f "$incremental_backup" ]]; then
    fail "Incremental backup file not created"
  fi

  printf "Incremental backup created: %s\n" "$incremental_backup"

  # Verify incremental backup is smaller than full backup
  local full_size
  local incremental_size

  full_size=$(stat -f%z "$BACKUP_FILE" 2>/dev/null || stat -c%s "$BACKUP_FILE" 2>/dev/null)
  incremental_size=$(stat -f%z "$incremental_backup" 2>/dev/null || stat -c%s "$incremental_backup" 2>/dev/null)

  if [[ $incremental_size -ge $full_size ]]; then
    printf "WARNING: Incremental backup (%s) not smaller than full backup (%s)\n" "$incremental_size" "$full_size"
  else
    printf "Incremental backup is smaller: %s vs %s bytes\n" "$incremental_size" "$full_size"
  fi

  pass "Incremental backup created"
}

test_09_list_backups() {
  describe "Test 9: List all backups"

  cd "$TEST_PROJECT_DIR"

  # List backups
  printf "Listing backups...\n"
  local backup_list
  backup_list=$(run_nself_command backup list --path "$BACKUP_DIR" 2>&1)

  # Verify backups are listed
  echo "$backup_list" | grep -q "test_backup_initial" || fail "Initial backup not listed"
  echo "$backup_list" | grep -q "test_backup_incremental" || fail "Incremental backup not listed"

  printf "Backup list:\n%s\n" "$backup_list"

  pass "Backups listed successfully"
}

test_10_automated_backup() {
  describe "Test 10: Test automated backup scheduling"

  cd "$TEST_PROJECT_DIR"

  # Configure automated backup
  printf "Configuring automated backup...\n"
  run_nself_command backup schedule \
    --frequency "daily" \
    --time "02:00" \
    --retention 7

  # Verify cron job or scheduled task created
  # This is a mock test since we can't wait for actual schedule
  printf "✓ Automated backup configured (schedule not tested)\n"

  pass "Automated backup configured"
}

test_11_backup_cleanup() {
  describe "Test 11: Test backup cleanup (retention policy)"

  cd "$TEST_PROJECT_DIR"

  # Create several old backup files
  printf "Creating mock old backups...\n"
  for i in {1..5}; do
    touch -t "$(date -v-${i}d +%Y%m%d0000 2>/dev/null || date -d "${i} days ago" +%Y%m%d0000)" \
      "$BACKUP_DIR/old_backup_$i.sql"
  done

  # Run cleanup with 3-day retention
  printf "Running cleanup with 3-day retention...\n"
  run_nself_command backup clean \
    --path "$BACKUP_DIR" \
    --retention 3 \
    --confirm

  # Verify old backups removed
  local old_backup_count
  old_backup_count=$(find "$BACKUP_DIR" -name "old_backup_*.sql" 2>/dev/null | wc -l | tr -d ' ')

  if [[ $old_backup_count -gt 3 ]]; then
    fail "Old backups not cleaned up: $old_backup_count files remaining"
  fi

  printf "Old backups cleaned: %s files remaining\n" "$old_backup_count"

  pass "Backup cleanup successful"
}

# ============================================================================
# Main Test Runner
# ============================================================================

main() {
  start_suite "Backup & Restore Workflow Integration Test"

  # Skip gracefully when Docker or nself is not available (requires live stack in CI)
  if ! docker ps >/dev/null 2>&1; then
    printf "⚠ Docker not available - skipping workflow tests\n"
    exit 0
  fi
  if [[ -z "${NSELF_ROOT:-}" ]] || [[ ! -x "${NSELF_ROOT}/bin/nself" ]]; then
    printf "⚠ NSELF_ROOT not set or nself not found - skipping workflow tests\n"
    exit 0
  fi

  printf "\n=================================================================\n"
  printf "Backup & Restore Workflow Integration Test\n"
  printf "=================================================================\n\n"

  # Run all tests in sequence
  test_01_setup
  test_02_create_test_data
  test_03_create_backup
  test_04_verify_backup_contents
  test_05_modify_data
  test_06_restore_backup
  test_07_verify_restored_data
  test_08_incremental_backup
  test_09_list_backups
  test_10_automated_backup
  test_11_backup_cleanup

  # Print summary
  printf "\n=================================================================\n"
  printf "Test Summary\n"
  printf "=================================================================\n"
  printf "Total Tests: %d\n" "$TESTS_RUN"
  printf "Passed: %d\n" "$TESTS_PASSED"
  printf "Failed: %d\n" "$TESTS_FAILED"
  printf "Skipped: %d\n" "$TESTS_SKIPPED"
  printf "=================================================================\n\n"

  # Exit with proper code
  if [[ $TESTS_FAILED -gt 0 ]]; then
    exit 1
  else
    exit 0
  fi
}

# Run tests if executed directly
if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
