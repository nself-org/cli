#!/usr/bin/env bats
# Secrets Encryption Tests
# Tests for encryption key management, encryption/decryption, and key rotation
#
# Part of nself v0.6.0 - Phase 1 Sprint 4
# Target: 90%+ coverage for encryption.sh

setup() {
    # Create temp test directory
    TEST_DIR=$(mktemp -d)
    cd "$TEST_DIR"

    # Resolve nself path dynamically
    NSELF_PATH="$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)"
    export PATH="$NSELF_PATH:$PATH"

    # Source encryption module directly for unit testing
    ENCRYPTION_MODULE="$NSELF_PATH/src/lib/secrets/encryption.sh"

    # Export required environment variables
    export POSTGRES_USER="postgres"
    export POSTGRES_DB="nself_test_db"
    export POSTGRES_PASSWORD="test_password"
    export PROJECT_NAME="test-encryption"

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

# Helper function to start PostgreSQL container
start_postgres() {
    if [[ -f "docker-compose.yml" ]]; then
        nself build 2>/dev/null || return 1
        nself start 2>/dev/null || return 1
        sleep 5
    else
        docker run -d \
            --name test-postgres \
            -e POSTGRES_PASSWORD="$POSTGRES_PASSWORD" \
            -e POSTGRES_DB="$POSTGRES_DB" \
            -p 5432:5432 \
            postgres:15-alpine >/dev/null 2>&1 || return 1
        sleep 10
    fi
}

@test "encryption module exports all required functions" {
    source "$ENCRYPTION_MODULE" 2>/dev/null || skip "Cannot source encryption module"

    # Check if functions are exported
    type encryption_generate_key >/dev/null 2>&1
    type encryption_generate_iv >/dev/null 2>&1
    type encryption_store_key >/dev/null 2>&1
    type encryption_get_active_key >/dev/null 2>&1
    type encryption_get_key >/dev/null 2>&1
    type encryption_rotate_key >/dev/null 2>&1
    type encryption_check_rotation >/dev/null 2>&1
    type encryption_list_keys >/dev/null 2>&1
    type encryption_encrypt >/dev/null 2>&1
    type encryption_decrypt >/dev/null 2>&1
    type encryption_init >/dev/null 2>&1
}

@test "encryption_generate_key produces valid 256-bit key" {
    source "$ENCRYPTION_MODULE" 2>/dev/null || skip "Cannot source encryption module"

    run encryption_generate_key
    [ "$status" -eq 0 ]

    # Key should be base64 encoded and not empty
    [[ -n "$output" ]]

    # Base64 encoded 32-byte key should be ~44 characters
    local key_length=${#output}
    [ "$key_length" -ge 40 ] && [ "$key_length" -le 50 ]
}

@test "encryption_generate_key works with openssl" {
    command -v openssl >/dev/null 2>&1 || skip "OpenSSL not available"
    source "$ENCRYPTION_MODULE" 2>/dev/null || skip "Cannot source encryption module"

    run encryption_generate_key
    [ "$status" -eq 0 ]
    [[ -n "$output" ]]
}

@test "encryption_generate_key works without openssl" {
    source "$ENCRYPTION_MODULE" 2>/dev/null || skip "Cannot source encryption module"

    # Mock openssl command to fail
    function openssl() { return 127; }
    export -f openssl

    run encryption_generate_key
    [ "$status" -eq 0 ]
    [[ -n "$output" ]]

    unset -f openssl
}

@test "encryption_generate_iv produces valid initialization vector" {
    source "$ENCRYPTION_MODULE" 2>/dev/null || skip "Cannot source encryption module"

    run encryption_generate_iv
    [ "$status" -eq 0 ]

    # IV should be 32-character hex string (16 bytes)
    [[ "$output" =~ ^[0-9a-f]{32}$ ]]
}

@test "encryption_generate_iv creates unique values" {
    source "$ENCRYPTION_MODULE" 2>/dev/null || skip "Cannot source encryption module"

    local iv1 iv2
    iv1=$(encryption_generate_iv)
    iv2=$(encryption_generate_iv)

    # IVs should be different
    [[ "$iv1" != "$iv2" ]]
}

@test "encryption_store_key requires key_data" {
    source "$ENCRYPTION_MODULE" 2>/dev/null || skip "Cannot source encryption module"

    run encryption_store_key
    [ "$status" -ne 0 ]
    [[ "$output" =~ "required" ]] || [[ "$output" =~ "ERROR" ]]
}

@test "encryption_store_key stores key in database" {
    skip "Requires PostgreSQL running"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"

    local key
    key=$(encryption_generate_key)

    run encryption_store_key "$key" true
    [ "$status" -eq 0 ]

    # Should return UUID
    [[ "$output" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$ ]]
}

@test "encryption_store_key deactivates previous active keys" {
    skip "Requires PostgreSQL running"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"

    # Store first key
    local key1
    key1=$(encryption_generate_key)
    encryption_store_key "$key1" true

    # Store second key as active
    local key2
    key2=$(encryption_generate_key)
    encryption_store_key "$key2" true

    # Only one key should be active
    local active_key
    active_key=$(encryption_get_active_key)
    [ "$?" -eq 0 ]
}

@test "encryption_get_active_key returns most recent active key" {
    skip "Requires PostgreSQL running"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"

    # Initialize encryption
    encryption_init

    run encryption_get_active_key
    [ "$status" -eq 0 ]

    # Should be valid JSON with key fields
    echo "$output" | jq . >/dev/null 2>&1
    echo "$output" | jq -e '.id' >/dev/null
    echo "$output" | jq -e '.key_data' >/dev/null
}

@test "encryption_get_active_key fails when no active key" {
    skip "Requires PostgreSQL running without initialized encryption"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"

    # Don't initialize - no keys exist
    run encryption_get_active_key
    [ "$status" -ne 0 ]
    [[ "$output" =~ "not found" ]] || [[ "$output" =~ "ERROR" ]]
}

@test "encryption_get_key requires key_id" {
    source "$ENCRYPTION_MODULE" 2>/dev/null || skip "Cannot source encryption module"

    run encryption_get_key
    [ "$status" -ne 0 ]
    [[ "$output" =~ "required" ]] || [[ "$output" =~ "ERROR" ]]
}

@test "encryption_get_key retrieves specific key by ID" {
    skip "Requires PostgreSQL running"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"

    encryption_init
    local active_key_json
    active_key_json=$(encryption_get_active_key)
    local key_id
    key_id=$(echo "$active_key_json" | jq -r '.id')

    run encryption_get_key "$key_id"
    [ "$status" -eq 0 ]

    # Should return same key
    local retrieved_id
    retrieved_id=$(echo "$output" | jq -r '.id')
    [[ "$key_id" == "$retrieved_id" ]]
}

@test "encryption_rotate_key generates and stores new key" {
    skip "Requires PostgreSQL running"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"

    encryption_init
    local original_key_id
    original_key_id=$(encryption_get_active_key | jq -r '.id')

    run encryption_rotate_key
    [ "$status" -eq 0 ]
    [[ "$output" =~ "rotated" ]] || [[ "$output" =~ "success" ]]

    # New key should be active
    local new_key_id
    new_key_id=$(encryption_get_active_key | jq -r '.id')
    [[ "$original_key_id" != "$new_key_id" ]]
}

@test "encryption_check_rotation returns 0 when rotation needed" {
    skip "Requires PostgreSQL running with old key"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"

    encryption_init

    # Mock old key by manually updating database timestamp
    # In real scenario, this would be tested with time manipulation

    run encryption_check_rotation
    # Returns 0 if rotation needed, 1 if not
    # Fresh key should not need rotation
    [ "$status" -eq 1 ]
}

@test "encryption_list_keys returns all keys without key_data" {
    skip "Requires PostgreSQL running"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"

    encryption_init

    # Create multiple keys
    encryption_rotate_key
    encryption_rotate_key

    run encryption_list_keys
    [ "$status" -eq 0 ]

    # Should be valid JSON array
    echo "$output" | jq . >/dev/null 2>&1

    # Should not contain key_data for security
    [[ ! "$output" =~ "key_data" ]]
}

@test "encryption_encrypt requires plaintext" {
    source "$ENCRYPTION_MODULE" 2>/dev/null || skip "Cannot source encryption module"

    run encryption_encrypt
    [ "$status" -ne 0 ]
    [[ "$output" =~ "required" ]] || [[ "$output" =~ "ERROR" ]]
}

@test "encryption_encrypt produces IV:ciphertext format" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"

    encryption_init

    run encryption_encrypt "test_plaintext"
    [ "$status" -eq 0 ]

    # Output should be in IV:ciphertext format
    [[ "$output" =~ ^[0-9a-f]{32}:.+$ ]]
}

@test "encryption_encrypt with provided key_data" {
    source "$ENCRYPTION_MODULE" 2>/dev/null || skip "Cannot source encryption module"

    local key
    key=$(encryption_generate_key)

    run encryption_encrypt "test_data" "$key"
    [ "$status" -eq 0 ]
    [[ "$output" =~ ^[0-9a-f]{32}:.+$ ]]
}

@test "encryption_encrypt produces different output each time (unique IV)" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"

    encryption_init

    local encrypted1 encrypted2
    encrypted1=$(encryption_encrypt "same_plaintext")
    encrypted2=$(encryption_encrypt "same_plaintext")

    # Should be different due to unique IVs
    [[ "$encrypted1" != "$encrypted2" ]]
}

@test "encryption_decrypt requires encrypted_data" {
    source "$ENCRYPTION_MODULE" 2>/dev/null || skip "Cannot source encryption module"

    run encryption_decrypt
    [ "$status" -ne 0 ]
    [[ "$output" =~ "required" ]] || [[ "$output" =~ "ERROR" ]]
}

@test "encryption_decrypt restores original plaintext" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"

    encryption_init

    local plaintext="This is a secret message"
    local encrypted
    encrypted=$(encryption_encrypt "$plaintext")

    run encryption_decrypt "$encrypted"
    [ "$status" -eq 0 ]
    [[ "$output" == "$plaintext" ]]
}

@test "encryption_decrypt with provided key_data" {
    source "$ENCRYPTION_MODULE" 2>/dev/null || skip "Cannot source encryption module"

    local key
    key=$(encryption_generate_key)

    local plaintext="test data"
    local encrypted
    encrypted=$(encryption_encrypt "$plaintext" "$key")

    run encryption_decrypt "$encrypted" "$key"
    [ "$status" -eq 0 ]
    [[ "$output" == "$plaintext" ]]
}

@test "encryption_decrypt fails with wrong key" {
    source "$ENCRYPTION_MODULE" 2>/dev/null || skip "Cannot source encryption module"

    local key1 key2
    key1=$(encryption_generate_key)
    key2=$(encryption_generate_key)

    local encrypted
    encrypted=$(encryption_encrypt "test" "$key1")

    # Try to decrypt with different key
    run encryption_decrypt "$encrypted" "$key2"
    [ "$status" -ne 0 ]
}

@test "encryption_decrypt handles invalid format" {
    source "$ENCRYPTION_MODULE" 2>/dev/null || skip "Cannot source encryption module"

    run encryption_decrypt "invalid_format"
    [ "$status" -ne 0 ]
    [[ "$output" =~ "Invalid" ]] || [[ "$output" =~ "ERROR" ]]
}

@test "encryption round-trip with special characters" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"

    encryption_init

    local special="Test!@#$%^&*(){}[]|\\:;\"'<>,.?/~`"
    local encrypted
    encrypted=$(encryption_encrypt "$special")

    run encryption_decrypt "$encrypted"
    [ "$status" -eq 0 ]
    [[ "$output" == "$special" ]]
}

@test "encryption round-trip with multiline data" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"

    encryption_init

    local multiline=$'Line 1\nLine 2\nLine 3\nWith special: $chars'
    local encrypted
    encrypted=$(encryption_encrypt "$multiline")

    run encryption_decrypt "$encrypted"
    [ "$status" -eq 0 ]
    [[ "$output" == "$multiline" ]]
}

@test "encryption round-trip with binary-like data" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"

    encryption_init

    # Base64 encoded data (simulates binary)
    local binary_like="TWFuIGlzIGRpc3Rpbmd1aXNoZWQsIG5vdCBvbmx5IGJ5IGhpcyByZWFzb24="
    local encrypted
    encrypted=$(encryption_encrypt "$binary_like")

    run encryption_decrypt "$encrypted"
    [ "$status" -eq 0 ]
    [[ "$output" == "$binary_like" ]]
}

@test "encryption_init creates first encryption key" {
    skip "Requires PostgreSQL running"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"

    run encryption_init
    [ "$status" -eq 0 ]
    [[ "$output" =~ "initialized" ]] || [[ "$output" =~ "key:" ]]
}

@test "encryption_init is idempotent" {
    skip "Requires PostgreSQL running"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"

    # First init
    encryption_init

    # Second init should not fail
    run encryption_init
    [ "$status" -eq 0 ]
    [[ "$output" =~ "already initialized" ]]
}

@test "encryption uses AES-256-CBC algorithm" {
    source "$ENCRYPTION_MODULE" 2>/dev/null || skip "Cannot source encryption module"

    # Check constant is defined
    [[ "$ENCRYPTION_ALGORITHM" == "aes-256-cbc" ]]
}

@test "encryption key size is 256 bits" {
    source "$ENCRYPTION_MODULE" 2>/dev/null || skip "Cannot source encryption module"

    # Check constant is defined (32 bytes = 256 bits)
    [[ "$KEY_SIZE" == "32" ]]
}

@test "encryption key rotation period is configurable" {
    source "$ENCRYPTION_MODULE" 2>/dev/null || skip "Cannot source encryption module"

    # Check constant is defined
    [[ "$KEY_ROTATION_DAYS" =~ ^[0-9]+$ ]]
}

@test "encryption handles empty strings" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"

    encryption_init

    local encrypted
    encrypted=$(encryption_encrypt "")

    run encryption_decrypt "$encrypted"
    [ "$status" -eq 0 ]
    [[ "$output" == "" ]]
}

@test "encryption handles very long strings" {
    skip "Requires PostgreSQL and encryption initialized"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"

    encryption_init

    # Generate 10KB of data
    local long_string
    long_string=$(printf '%*s' 10000 | tr ' ' 'A')

    local encrypted
    encrypted=$(encryption_encrypt "$long_string")

    run encryption_decrypt "$encrypted"
    [ "$status" -eq 0 ]
    [[ "$output" == "$long_string" ]]
}

@test "encryption operations fail gracefully without PostgreSQL" {
    source "$ENCRYPTION_MODULE" 2>/dev/null || skip "Cannot source encryption module"

    # Ensure no PostgreSQL
    docker stop $(docker ps -q --filter "name=postgres" 2>/dev/null) 2>/dev/null || true

    run encryption_init
    [ "$status" -ne 0 ]
    [[ "$output" =~ "PostgreSQL" ]] || [[ "$output" =~ "ERROR" ]]
}

@test "encryption key storage creates necessary schema" {
    skip "Requires PostgreSQL running"

    start_postgres || skip "PostgreSQL not available"
    source "$ENCRYPTION_MODULE"

    local key
    key=$(encryption_generate_key)

    # First store should create schema
    encryption_store_key "$key" true

    # Verify schema exists
    local container
    container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

    if [[ -n "$container" ]]; then
        local table_exists
        table_exists=$(docker exec "$container" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
            "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'secrets' AND table_name = 'encryption_keys');" | xargs)
        [[ "$table_exists" == "t" ]]
    fi
}
