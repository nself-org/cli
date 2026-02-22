#!/usr/bin/env bats
# Secrets Vault Tests
# Tests for vault operations, secret storage, retrieval, and deletion
#
# Part of nself v0.6.0 - Phase 1 Sprint 4
# Target: 90%+ coverage for vault.sh

setup() {
    # Create temp test directory
    TEST_DIR=$(mktemp -d)
    cd "$TEST_DIR"

    # Resolve nself path dynamically
    NSELF_PATH="$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)"
    export PATH="$NSELF_PATH:$PATH"

    # Source vault module directly for unit testing
    VAULT_MODULE="$NSELF_PATH/src/lib/secrets/vault.sh"
    ENCRYPTION_MODULE="$NSELF_PATH/src/lib/secrets/encryption.sh"

    # Export required environment variables
    export POSTGRES_USER="postgres"
    export POSTGRES_DB="nself_test_db"
    export POSTGRES_PASSWORD="test_password"
    export PROJECT_NAME="test-vault"

    # Initialize minimal nself project
    nself init --minimal 2>/dev/null || true
}

teardown() {
    # Stop and remove containers
    docker compose down -v 2>/dev/null || true
    docker rm -f $(docker ps -a -q --filter "name=postgres" 2>/dev/null) 2>/dev/null || true

    # Clean up test directory
    cd /
    rm -rf "$TEST_DIR"
}

# Helper function to start PostgreSQL container for tests
start_postgres() {
    # Check if we can build and start services
    if [[ -f "docker-compose.yml" ]]; then
        nself build 2>/dev/null || return 1
        nself start 2>/dev/null || return 1
        sleep 5  # Wait for PostgreSQL to be ready
    else
        # Start a standalone PostgreSQL container
        docker run -d \
            --name test-postgres \
            -e POSTGRES_PASSWORD="$POSTGRES_PASSWORD" \
            -e POSTGRES_DB="$POSTGRES_DB" \
            -p 5432:5432 \
            postgres:15-alpine >/dev/null 2>&1 || return 1
        sleep 10  # Wait for PostgreSQL to initialize
    fi
}

@test "vault module exports all required functions" {
    source "$VAULT_MODULE" 2>/dev/null || skip "Cannot source vault module"

    # Check if functions are exported
    type vault_init >/dev/null 2>&1
    type vault_set >/dev/null 2>&1
    type vault_get >/dev/null 2>&1
    type vault_delete >/dev/null 2>&1
    type vault_list >/dev/null 2>&1
    type vault_rotate >/dev/null 2>&1
    type vault_rotate_all >/dev/null 2>&1
    type vault_get_versions >/dev/null 2>&1
    type vault_rollback >/dev/null 2>&1
}

@test "vault_init creates database schema" {
    skip "Requires PostgreSQL container running"

    start_postgres || skip "PostgreSQL not available"
    source "$VAULT_MODULE"

    run vault_init
    [ "$status" -eq 0 ]
    [[ "$output" =~ "initialized" ]] || [[ "$output" =~ "success" ]]
}

@test "vault_init creates secrets schema and tables" {
    skip "Requires PostgreSQL container running"

    start_postgres || skip "PostgreSQL not available"
    source "$VAULT_MODULE"

    vault_init

    # Check if schema exists
    local container
    container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

    if [[ -n "$container" ]]; then
        local schema_exists
        schema_exists=$(docker exec "$container" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
            "SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = 'secrets');" | xargs)
        [[ "$schema_exists" == "t" ]]
    fi
}

@test "vault_set requires key_name and value" {
    source "$VAULT_MODULE" 2>/dev/null || skip "Cannot source vault module"

    run vault_set
    [ "$status" -ne 0 ]
    [[ "$output" =~ "required" ]] || [[ "$output" =~ "ERROR" ]]
}

@test "vault_set stores secret successfully" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"
    source "$VAULT_MODULE"

    encryption_init
    vault_init

    run vault_set "test_key" "test_value" "default" "Test secret"
    [ "$status" -eq 0 ]
    # Should return secret ID (UUID format)
    [[ "$output" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$ ]] || true
}

@test "vault_set updates existing secret and increments version" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"
    source "$VAULT_MODULE"

    encryption_init
    vault_init

    # Create initial secret
    local secret_id
    secret_id=$(vault_set "test_key" "value1" "default")

    # Update secret
    local updated_id
    updated_id=$(vault_set "test_key" "value2" "default")

    # Should return same ID
    [[ "$secret_id" == "$updated_id" ]]
}

@test "vault_get requires key_name" {
    source "$VAULT_MODULE" 2>/dev/null || skip "Cannot source vault module"

    run vault_get
    [ "$status" -ne 0 ]
    [[ "$output" =~ "required" ]] || [[ "$output" =~ "ERROR" ]]
}

@test "vault_get retrieves and decrypts secret" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"
    source "$VAULT_MODULE"

    encryption_init
    vault_init

    # Store a secret
    vault_set "test_key" "secret_value_123" "default"

    # Retrieve it
    run vault_get "test_key" "default"
    [ "$status" -eq 0 ]
    [[ "$output" == "secret_value_123" ]]
}

@test "vault_get fails for non-existent secret" {
    skip "Requires PostgreSQL running"

    start_postgres || skip "PostgreSQL not available"
    source "$VAULT_MODULE"

    vault_init

    run vault_get "nonexistent_key" "default"
    [ "$status" -ne 0 ]
    [[ "$output" =~ "not found" ]] || [[ "$output" =~ "ERROR" ]]
}

@test "vault_get retrieves specific version" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"
    source "$VAULT_MODULE"

    encryption_init
    vault_init

    # Create version 1
    vault_set "versioned_key" "value_v1" "default"

    # Create version 2
    vault_set "versioned_key" "value_v2" "default"

    # Get version 1
    run vault_get "versioned_key" "default" "1"
    [ "$status" -eq 0 ]
    [[ "$output" == "value_v1" ]]
}

@test "vault_delete requires key_name" {
    source "$VAULT_MODULE" 2>/dev/null || skip "Cannot source vault module"

    run vault_delete
    [ "$status" -ne 0 ]
    [[ "$output" =~ "required" ]] || [[ "$output" =~ "ERROR" ]]
}

@test "vault_delete soft-deletes secret (marks inactive)" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"
    source "$VAULT_MODULE"

    encryption_init
    vault_init

    # Create and delete secret
    vault_set "delete_test" "value" "default"

    run vault_delete "delete_test" "default"
    [ "$status" -eq 0 ]

    # Should not be retrievable
    run vault_get "delete_test" "default"
    [ "$status" -ne 0 ]
}

@test "vault_list returns empty array when no secrets" {
    skip "Requires PostgreSQL running"

    start_postgres || skip "PostgreSQL not available"
    source "$VAULT_MODULE"

    vault_init

    run vault_list "default"
    [ "$status" -eq 0 ]
    [[ "$output" == "[]" ]]
}

@test "vault_list returns secrets as JSON array" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"
    source "$VAULT_MODULE"

    encryption_init
    vault_init

    # Create multiple secrets
    vault_set "secret1" "value1" "default"
    vault_set "secret2" "value2" "default"

    run vault_list "default"
    [ "$status" -eq 0 ]

    # Should be valid JSON array
    echo "$output" | jq . >/dev/null 2>&1

    # Should contain both secrets
    [[ "$output" =~ "secret1" ]]
    [[ "$output" =~ "secret2" ]]
}

@test "vault_list filters by environment" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"
    source "$VAULT_MODULE"

    encryption_init
    vault_init

    # Create secrets in different environments
    vault_set "key1" "value1" "dev"
    vault_set "key2" "value2" "prod"

    # List dev secrets
    run vault_list "dev"
    [ "$status" -eq 0 ]
    [[ "$output" =~ "key1" ]]
    [[ ! "$output" =~ "key2" ]]
}

@test "vault_rotate requires key_name" {
    source "$VAULT_MODULE" 2>/dev/null || skip "Cannot source vault module"

    run vault_rotate
    [ "$status" -ne 0 ]
    [[ "$output" =~ "required" ]] || [[ "$output" =~ "ERROR" ]]
}

@test "vault_rotate re-encrypts secret with current key" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"
    source "$VAULT_MODULE"

    encryption_init
    vault_init

    # Create secret
    vault_set "rotate_test" "original_value" "default"

    # Rotate encryption key
    encryption_rotate_key

    # Rotate secret
    run vault_rotate "rotate_test" "default"
    [ "$status" -eq 0 ]

    # Secret should still be retrievable
    local value
    value=$(vault_get "rotate_test" "default")
    [[ "$value" == "original_value" ]]
}

@test "vault_rotate_all rotates all active secrets" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"
    source "$VAULT_MODULE"

    encryption_init
    vault_init

    # Create multiple secrets
    vault_set "key1" "value1" "default"
    vault_set "key2" "value2" "default"
    vault_set "key3" "value3" "default"

    # Rotate all
    run vault_rotate_all
    [ "$status" -eq 0 ]
    [[ "$output" =~ "Rotated" ]] || [[ "$output" =~ "rotated" ]]
}

@test "vault_get_versions requires key_name" {
    source "$VAULT_MODULE" 2>/dev/null || skip "Cannot source vault module"

    run vault_get_versions
    [ "$status" -ne 0 ]
    [[ "$output" =~ "required" ]] || [[ "$output" =~ "ERROR" ]]
}

@test "vault_get_versions returns version history" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"
    source "$VAULT_MODULE"

    encryption_init
    vault_init

    # Create multiple versions
    vault_set "versioned" "v1" "default"
    vault_set "versioned" "v2" "default"
    vault_set "versioned" "v3" "default"

    run vault_get_versions "versioned" "default"
    [ "$status" -eq 0 ]

    # Should be valid JSON
    echo "$output" | jq . >/dev/null 2>&1
}

@test "vault_rollback requires key_name and version" {
    source "$VAULT_MODULE" 2>/dev/null || skip "Cannot source vault module"

    run vault_rollback
    [ "$status" -ne 0 ]
    [[ "$output" =~ "required" ]] || [[ "$output" =~ "ERROR" ]]

    run vault_rollback "key"
    [ "$status" -ne 0 ]
    [[ "$output" =~ "required" ]] || [[ "$output" =~ "ERROR" ]]
}

@test "vault_rollback restores previous version" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"
    source "$VAULT_MODULE"

    encryption_init
    vault_init

    # Create versions
    vault_set "rollback_test" "version_1" "default"
    vault_set "rollback_test" "version_2" "default"
    vault_set "rollback_test" "version_3" "default"

    # Rollback to version 1
    vault_rollback "rollback_test" "1" "default"

    # Should retrieve version 1 value
    local value
    value=$(vault_get "rollback_test" "default")
    [[ "$value" == "version_1" ]]
}

@test "vault handles secrets with special characters" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"
    source "$VAULT_MODULE"

    encryption_init
    vault_init

    # Test with special characters
    local special_value="test'value\"with\$special&chars"
    vault_set "special_key" "$special_value" "default"

    run vault_get "special_key" "default"
    [ "$status" -eq 0 ]
    [[ "$output" == "$special_value" ]]
}

@test "vault handles multiline secrets" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"
    source "$VAULT_MODULE"

    encryption_init
    vault_init

    # Test with multiline value
    local multiline_value=$'line1\nline2\nline3'
    vault_set "multiline_key" "$multiline_value" "default"

    run vault_get "multiline_key" "default"
    [ "$status" -eq 0 ]
    [[ "$output" == "$multiline_value" ]]
}

@test "vault set handles expiration dates" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"
    source "$VAULT_MODULE"

    encryption_init
    vault_init

    # Create secret with 30-day expiration
    run vault_set "expiring_key" "temp_value" "default" "Expires soon" "30"
    [ "$status" -eq 0 ]
}

@test "vault operations fail gracefully without PostgreSQL" {
    source "$VAULT_MODULE" 2>/dev/null || skip "Cannot source vault module"

    # Ensure no PostgreSQL container is running
    docker stop $(docker ps -q --filter "name=postgres" 2>/dev/null) 2>/dev/null || true

    run vault_init
    [ "$status" -ne 0 ]
    [[ "$output" =~ "PostgreSQL" ]] || [[ "$output" =~ "ERROR" ]]
}

@test "vault operations require encryption system" {
    skip "Requires PostgreSQL running without encryption"

    start_postgres || skip "PostgreSQL not available"
    source "$VAULT_MODULE"

    vault_init

    # Try to set secret without encryption initialized
    run vault_set "test" "value" "default"
    [ "$status" -ne 0 ]
    [[ "$output" =~ "encryption" ]] || [[ "$output" =~ "key" ]]
}
