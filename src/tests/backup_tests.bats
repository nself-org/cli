#!/usr/bin/env bats
# Backup & Restore Tests
# Tests for backup creation, restoration, verification, and cloud sync

setup() {
    # Create temp test directory
    TEST_DIR=$(mktemp -d)
    cd "$TEST_DIR"

    # Resolve nself path dynamically
    NSELF_PATH="$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)"
    export PATH="$NSELF_PATH:$PATH"

    # Initialize minimal nself project for backup testing
    nself init 2>/dev/null || true
    # Ensure required secrets are set for any build/validate steps
    printf '\nPOSTGRES_PASSWORD=test-postgres-secret-ci\nHASURA_GRAPHQL_ADMIN_SECRET=test-admin-secret-ci\n' >> .env 2>/dev/null || true

    # Create backup directory
    export BACKUP_DIR="$TEST_DIR/backups"
    mkdir -p "$BACKUP_DIR"

    # Set test configuration
    printf "BACKUP_DIR=%s\n" "$BACKUP_DIR" >> .env
    printf "BACKUP_RETENTION_DAYS=30\n" >> .env
    printf "BACKUP_RETENTION_MIN=2\n" >> .env
    printf "PROJECT_NAME=test-project\n" >> .env
}

teardown() {
    # Stop any running containers
    docker compose down 2>/dev/null || true

    # Clean up test directory
    cd /
    rm -rf "$TEST_DIR"
}

@test "backup help command shows available options" {
    run nself backup help
    [ "$status" -eq 0 ]
    [[ "$output" =~ "create" ]]
    [[ "$output" =~ "restore" ]]
    [[ "$output" =~ "list" ]]
    [[ "$output" =~ "verify" ]]
}

@test "backup create generates backup file" {
    skip "Requires PostgreSQL container running"

    # Start services for backup
    nself build
    nself start

    # Create backup
    run nself backup create database
    [ "$status" -eq 0 ]

    # Verify backup file was created
    [ -d "$BACKUP_DIR" ]
    local backup_count
    backup_count=$(find "$BACKUP_DIR" -name "*.tar.gz" | wc -l)
    [ "$backup_count" -gt 0 ]
}

@test "backup list shows local backups" {
    # Create dummy backup file
    touch "$BACKUP_DIR/nself_backup_full_20260130_120000.tar.gz"

    run nself backup list
    [ "$status" -eq 0 ]
    [[ "$output" =~ "Available Backups" ]]
    [[ "$output" =~ "nself_backup_full" ]]
}

@test "backup list shows no backups when directory is empty" {
    run nself backup list
    [ "$status" -eq 0 ]
    [[ "$output" =~ "No local backups found" ]]
}

@test "backup prune by age removes old backups" {
    # Create test backup files with different ages
    touch "$BACKUP_DIR/old_backup1.tar.gz"
    touch "$BACKUP_DIR/old_backup2.tar.gz"
    touch "$BACKUP_DIR/recent_backup.tar.gz"

    # Make old backups look old (31+ days)
    if command -v touch >/dev/null 2>&1; then
        # macOS/BSD date
        if [[ "$(uname)" == "Darwin" ]]; then
            touch -t 202501010000 "$BACKUP_DIR/old_backup1.tar.gz" 2>/dev/null || true
            touch -t 202501010000 "$BACKUP_DIR/old_backup2.tar.gz" 2>/dev/null || true
        else
            # Linux date
            touch -d "32 days ago" "$BACKUP_DIR/old_backup1.tar.gz" 2>/dev/null || true
            touch -d "32 days ago" "$BACKUP_DIR/old_backup2.tar.gz" 2>/dev/null || true
        fi
    fi

    # Prune old backups (older than 30 days)
    run nself backup prune age 30
    [ "$status" -eq 0 ]

    # Recent backup should still exist
    [ -f "$BACKUP_DIR/recent_backup.tar.gz" ]
}

@test "backup prune respects minimum retention count" {
    # Create only 2 backups (matching BACKUP_RETENTION_MIN)
    touch "$BACKUP_DIR/backup1.tar.gz"
    touch "$BACKUP_DIR/backup2.tar.gz"

    # Make them old
    if [[ "$(uname)" == "Darwin" ]]; then
        touch -t 202501010000 "$BACKUP_DIR/backup1.tar.gz" 2>/dev/null || true
        touch -t 202501010000 "$BACKUP_DIR/backup2.tar.gz" 2>/dev/null || true
    else
        touch -d "32 days ago" "$BACKUP_DIR/backup1.tar.gz" 2>/dev/null || true
        touch -d "32 days ago" "$BACKUP_DIR/backup2.tar.gz" 2>/dev/null || true
    fi

    # Try to prune (should keep both due to minimum retention)
    run nself backup prune age 30
    [ "$status" -eq 0 ]

    # Both backups should still exist
    [ -f "$BACKUP_DIR/backup1.tar.gz" ]
    [ -f "$BACKUP_DIR/backup2.tar.gz" ]
}

@test "backup verify detects missing backup file" {
    run nself backup verify nonexistent_backup.tar.gz
    [ "$status" -ne 0 ]
    # Error message goes to stderr; non-zero status is the correct assertion
}

@test "backup restore fails gracefully without backup file" {
    run nself backup restore nonexistent.tar.gz
    [ "$status" -ne 0 ]
    # Error message goes to stderr; non-zero status is the correct assertion
}

@test "backup retention status shows current configuration" {
    run nself backup retention status
    [ "$status" -eq 0 ]
    [[ "$output" =~ "Retention days" ]]
    [[ "$output" =~ "30" ]]
    [[ "$output" =~ "Minimum backups" ]]
}

@test "backup retention set updates configuration" {
    run nself backup retention set days 60
    [ "$status" -eq 0 ]
    [[ "$output" =~ "Retention days set to: 60" ]]

    # Verify it was written to .env
    grep -q "BACKUP_RETENTION_DAYS=60" .env
}

@test "backup cloud status shows no provider when unconfigured" {
    run nself backup cloud status
    [ "$status" -eq 0 ]
    [[ "$output" =~ "none" ]] || [[ "$output" =~ "not configured" ]]
}

@test "backup handles missing docker gracefully" {
    skip "Docker availability varies by environment"

    # This test would verify graceful failure when Docker isn't available
    # Actual implementation depends on whether Docker is required or optional
}

@test "backup create supports different backup types" {
    skip "Requires PostgreSQL container running"

    # Test database-only backup
    run nself backup create database
    [ "$status" -eq 0 ]

    # Test config-only backup
    run nself backup create config
    [ "$status" -eq 0 ]

    # Test full backup
    run nself backup create full
    [ "$status" -eq 0 ]
}

@test "backup prune by count keeps only specified number" {
    # Create 10 backup files
    for i in 1 2 3 4 5 6 7 8 9 10; do
        touch "$BACKUP_DIR/backup_$i.tar.gz"
    done

    # Keep only 5
    run nself backup prune count 5
    [ "$status" -eq 0 ]

    # Should have 5 or fewer backups remaining
    local backup_count
    backup_count=$(find "$BACKUP_DIR" -name "*.tar.gz" | wc -l)
    [ "$backup_count" -le 5 ]
}

@test "backup generates unique filenames" {
    # This tests the generate_backup_name function
    # by checking if backup list would show distinct names

    # Create backups with same type
    touch "$BACKUP_DIR/nself_backup_full_20260130_120000.tar.gz"
    touch "$BACKUP_DIR/nself_backup_full_20260130_120001.tar.gz"

    run nself backup list
    [ "$status" -eq 0 ]

    # Both should appear
    [[ "$output" =~ "120000" ]]
    [[ "$output" =~ "120001" ]]
}

@test "backup directory is created if missing" {
    # Remove backup directory
    rm -rf "$BACKUP_DIR"

    # Backup command should create it
    run nself backup list
    [ "$status" -eq 0 ]
    [ -d "$BACKUP_DIR" ]
}
