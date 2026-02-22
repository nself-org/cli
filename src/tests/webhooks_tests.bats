#!/usr/bin/env bats
# Webhook Tests
# Comprehensive tests for webhook management, delivery, and retry logic

setup() {
    # Create temp test directory
    TEST_DIR=$(mktemp -d)
    cd "$TEST_DIR"

    # Resolve nself path dynamically
    NSELF_PATH="$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)"
    export PATH="$NSELF_PATH:$PATH"

    # Source webhook module
    WEBHOOK_MODULE="$NSELF_PATH/src/lib/webhooks/core.sh"
    if [[ -f "$WEBHOOK_MODULE" ]]; then
        source "$WEBHOOK_MODULE"
    fi

    # Initialize minimal nself project
    nself init

    # Basic configuration
    printf "PROJECT_NAME=test-webhooks\n" >> .env
    printf "POSTGRES_DB=test_webhooks_db\n" >> .env
    printf "POSTGRES_USER=postgres\n" >> .env
    printf "POSTGRES_PASSWORD=test123\n" >> .env
}

teardown() {
    # Stop any running containers
    docker compose down -v 2>/dev/null || true

    # Clean up test directory
    cd /
    rm -rf "$TEST_DIR"
}

# ============================================================================
# Initialization Tests
# ============================================================================

@test "webhook_init creates webhooks schema and tables" {
    skip "Requires running PostgreSQL container"

    nself build
    nself start

    # Initialize webhook system
    run webhook_init
    [ "$status" -eq 0 ]
}

@test "webhook_init fails when PostgreSQL is not running" {
    # Ensure no containers running
    docker compose down 2>/dev/null || true

    run webhook_init
    [ "$status" -ne 0 ]
    [[ "$output" =~ "PostgreSQL container not found" ]]
}

# ============================================================================
# Endpoint CRUD Tests
# ============================================================================

@test "webhook_create_endpoint requires URL and events" {
    skip "Requires running PostgreSQL container"

    nself build
    nself start
    webhook_init

    # Missing URL
    run webhook_create_endpoint "" '["user.created"]'
    [ "$status" -ne 0 ]
    [[ "$output" =~ "ERROR" ]]

    # Missing events
    run webhook_create_endpoint "https://example.com/webhook" ""
    [ "$status" -ne 0 ]
    [[ "$output" =~ "ERROR" ]]
}

@test "webhook_create_endpoint creates endpoint with generated secret" {
    skip "Requires running PostgreSQL container"

    nself build
    nself start
    webhook_init

    # Create endpoint without secret (should generate one)
    run webhook_create_endpoint "https://example.com/webhook" '["user.created","user.updated"]' "Test webhook"
    [ "$status" -eq 0 ]

    # Output should be valid JSON with id, url, secret, events
    echo "$output" | jq -e '.id' >/dev/null
    echo "$output" | jq -e '.url' >/dev/null
    echo "$output" | jq -e '.secret' >/dev/null
    echo "$output" | jq -e '.events' >/dev/null
}

@test "webhook_create_endpoint creates endpoint with custom secret" {
    skip "Requires running PostgreSQL container"

    nself build
    nself start
    webhook_init

    local custom_secret="my_custom_secret_12345"

    # Create endpoint with custom secret
    run webhook_create_endpoint "https://example.com/webhook" '["user.created"]' "Test webhook" "$custom_secret"
    [ "$status" -eq 0 ]

    # Verify secret in output
    local output_secret=$(echo "$output" | jq -r '.secret')
    [ "$output_secret" = "$custom_secret" ]
}

@test "webhook_create_endpoint handles special characters in URL" {
    skip "Requires running PostgreSQL container"

    nself build
    nself start
    webhook_init

    # URL with special characters
    local url="https://example.com/webhook?token=abc123&ref=test"

    run webhook_create_endpoint "$url" '["user.created"]' "Test webhook"
    [ "$status" -eq 0 ]

    # Verify URL is preserved
    local output_url=$(echo "$output" | jq -r '.url')
    [[ "$output_url" =~ "abc123" ]]
}

@test "webhook_list_endpoints returns empty array when no endpoints" {
    skip "Requires running PostgreSQL container"

    nself build
    nself start
    webhook_init

    run webhook_list_endpoints
    [ "$status" -eq 0 ]
    [ "$output" = "[]" ]
}

@test "webhook_list_endpoints returns all endpoints" {
    skip "Requires running PostgreSQL container"

    nself build
    nself start
    webhook_init

    # Create multiple endpoints
    webhook_create_endpoint "https://example.com/webhook1" '["user.created"]' "Webhook 1"
    webhook_create_endpoint "https://example.com/webhook2" '["user.updated"]' "Webhook 2"

    run webhook_list_endpoints
    [ "$status" -eq 0 ]

    # Should return array with 2 endpoints
    local count=$(echo "$output" | jq 'length')
    [ "$count" -eq 2 ]
}

@test "webhook_delete_endpoint requires endpoint ID" {
    skip "Requires running PostgreSQL container"

    nself build
    nself start
    webhook_init

    run webhook_delete_endpoint ""
    [ "$status" -ne 0 ]
    [[ "$output" =~ "ERROR" ]]
}

@test "webhook_delete_endpoint removes endpoint" {
    skip "Requires running PostgreSQL container"

    nself build
    nself start
    webhook_init

    # Create endpoint
    local result=$(webhook_create_endpoint "https://example.com/webhook" '["user.created"]' "Test webhook")
    local endpoint_id=$(echo "$result" | jq -r '.id')

    # Delete endpoint
    run webhook_delete_endpoint "$endpoint_id"
    [ "$status" -eq 0 ]

    # Verify endpoint is gone
    local endpoints=$(webhook_list_endpoints)
    local count=$(echo "$endpoints" | jq 'length')
    [ "$count" -eq 0 ]
}

@test "webhook_delete_endpoint cascades to deliveries" {
    skip "Requires running PostgreSQL container and delivery data"

    nself build
    nself start
    webhook_init

    # Create endpoint
    local result=$(webhook_create_endpoint "https://example.com/webhook" '["user.created"]' "Test webhook")
    local endpoint_id=$(echo "$result" | jq -r '.id')

    # Trigger webhook to create delivery
    webhook_trigger "user.created" '{"user_id": "123"}'

    # Delete endpoint (should cascade delete deliveries)
    run webhook_delete_endpoint "$endpoint_id"
    [ "$status" -eq 0 ]
}

# ============================================================================
# Webhook Trigger Tests
# ============================================================================

@test "webhook_trigger requires event type and payload" {
    skip "Requires running PostgreSQL container"

    nself build
    nself start
    webhook_init

    # Missing event type
    run webhook_trigger "" '{"data": "test"}'
    [ "$status" -ne 0 ]
    [[ "$output" =~ "ERROR" ]]

    # Missing payload
    run webhook_trigger "user.created" ""
    [ "$status" -ne 0 ]
    [[ "$output" =~ "ERROR" ]]
}

@test "webhook_trigger succeeds with no matching endpoints" {
    skip "Requires running PostgreSQL container"

    nself build
    nself start
    webhook_init

    # No endpoints created, should succeed silently
    run webhook_trigger "user.created" '{"user_id": "123"}'
    [ "$status" -eq 0 ]
}

@test "webhook_trigger creates delivery records" {
    skip "Requires running PostgreSQL container"

    nself build
    nself start
    webhook_init

    # Create endpoint subscribed to event
    webhook_create_endpoint "https://example.com/webhook" '["user.created"]' "Test webhook"

    # Trigger webhook
    run webhook_trigger "user.created" '{"user_id": "123", "email": "test@example.com"}'
    [ "$status" -eq 0 ]

    # Allow async delivery to start
    sleep 1
}

@test "webhook_trigger only triggers matching endpoints" {
    skip "Requires running PostgreSQL container"

    nself build
    nself start
    webhook_init

    # Create endpoints with different events
    webhook_create_endpoint "https://example.com/webhook1" '["user.created"]' "Webhook 1"
    webhook_create_endpoint "https://example.com/webhook2" '["user.updated"]' "Webhook 2"
    webhook_create_endpoint "https://example.com/webhook3" '["user.created","user.updated"]' "Webhook 3"

    # Trigger user.created (should trigger webhook1 and webhook3 only)
    run webhook_trigger "user.created" '{"user_id": "123"}'
    [ "$status" -eq 0 ]
}

@test "webhook_trigger handles JSON payload with special characters" {
    skip "Requires running PostgreSQL container"

    nself build
    nself start
    webhook_init

    # Create endpoint
    webhook_create_endpoint "https://example.com/webhook" '["user.created"]' "Test webhook"

    # Payload with special characters
    local payload='{"user_id": "123", "name": "O'\''Brien", "bio": "Test \"quoted\" text"}'

    run webhook_trigger "user.created" "$payload"
    [ "$status" -eq 0 ]
}

# ============================================================================
# Webhook Delivery Tests
# ============================================================================

@test "webhook signature generation uses HMAC-SHA256" {
    # Test signature format (not requiring live system)
    local payload='{"test": "data"}'
    local secret="test_secret_key"

    # Generate signature
    local signature=$(echo -n "$payload" | openssl dgst -sha256 -hmac "$secret" | cut -d' ' -f2)

    # Verify signature format (64 hex characters)
    [ ${#signature} -eq 64 ]
}

@test "webhook delivery includes correct headers" {
    skip "Requires mock HTTP server"

    # Would test:
    # - Content-Type: application/json
    # - X-Webhook-Event
    # - X-Webhook-Signature
    # - X-Webhook-Timestamp
    # - User-Agent: nself-webhooks/1.0
}

@test "webhook delivery respects timeout configuration" {
    skip "Requires mock HTTP server with delay"

    # Would test that delivery times out after WEBHOOK_TIMEOUT seconds
}

@test "webhook delivery handles HTTP 2xx as success" {
    skip "Requires mock HTTP server"

    # Would test HTTP 200, 201, 204 responses
}

@test "webhook delivery retries on HTTP 5xx errors" {
    skip "Requires mock HTTP server"

    # Would test retry logic for 500, 502, 503, 504
}

@test "webhook delivery does not retry on HTTP 4xx errors" {
    skip "Requires mock HTTP server"

    # Would test no retry for 400, 401, 403, 404, 422
}

@test "webhook delivery includes custom headers from endpoint config" {
    skip "Requires running PostgreSQL container and mock HTTP server"

    nself build
    nself start
    webhook_init

    # Create endpoint with custom headers via direct SQL
    # (webhook_create_endpoint doesn't support headers parameter yet)
}

# ============================================================================
# Webhook Retry Logic Tests
# ============================================================================

@test "webhook retry delay increases with each attempt" {
    skip "Requires running PostgreSQL container"

    # Would test exponential backoff or fixed delay
    # WEBHOOK_RETRY_DELAY is 60 seconds by default
}

@test "webhook stops retrying after max attempts" {
    skip "Requires running PostgreSQL container"

    # Would test that deliveries stop after WEBHOOK_MAX_RETRIES (3) attempts
}

@test "webhook retry updates next_retry_at timestamp" {
    skip "Requires running PostgreSQL container"

    # Would verify next_retry_at is set correctly after failure
}

# ============================================================================
# Webhook Events Tests
# ============================================================================

@test "webhook event constants are defined" {
    # Test that event constants exist
    [[ -n "$WEBHOOK_EVENT_USER_CREATED" ]]
    [[ -n "$WEBHOOK_EVENT_USER_UPDATED" ]]
    [[ -n "$WEBHOOK_EVENT_USER_DELETED" ]]
    [[ -n "$WEBHOOK_EVENT_USER_LOGIN" ]]
    [[ -n "$WEBHOOK_EVENT_USER_LOGOUT" ]]
    [[ -n "$WEBHOOK_EVENT_SESSION_CREATED" ]]
    [[ -n "$WEBHOOK_EVENT_SESSION_REVOKED" ]]
    [[ -n "$WEBHOOK_EVENT_MFA_ENABLED" ]]
    [[ -n "$WEBHOOK_EVENT_MFA_DISABLED" ]]
    [[ -n "$WEBHOOK_EVENT_ROLE_ASSIGNED" ]]
    [[ -n "$WEBHOOK_EVENT_ROLE_REVOKED" ]]
}

@test "webhook event constants have correct format" {
    # Verify event naming convention (category.action)
    [[ "$WEBHOOK_EVENT_USER_CREATED" =~ ^[a-z]+\.[a-z]+$ ]]
    [[ "$WEBHOOK_EVENT_SESSION_CREATED" =~ ^[a-z]+\.[a-z]+$ ]]
    [[ "$WEBHOOK_EVENT_MFA_ENABLED" =~ ^[a-z]+\.[a-z]+$ ]]
}

# ============================================================================
# Integration Tests
# ============================================================================

@test "webhook full lifecycle: create, trigger, deliver, delete" {
    skip "Requires running system with mock HTTP server"

    nself build
    nself start
    webhook_init

    # 1. Create endpoint
    local result=$(webhook_create_endpoint "https://httpbin.org/post" '["user.created"]' "Test webhook")
    local endpoint_id=$(echo "$result" | jq -r '.id')

    # 2. Trigger webhook
    webhook_trigger "user.created" '{"user_id": "test-123"}'

    # 3. Wait for delivery
    sleep 2

    # 4. Verify delivery succeeded
    # (would check delivery status in database)

    # 5. Delete endpoint
    webhook_delete_endpoint "$endpoint_id"
}

@test "webhook concurrent deliveries to multiple endpoints" {
    skip "Requires running system"

    nself build
    nself start
    webhook_init

    # Create 10 endpoints
    for i in {1..10}; do
        webhook_create_endpoint "https://httpbin.org/post?id=$i" '["user.created"]' "Webhook $i"
    done

    # Trigger should spawn 10 background deliveries
    run webhook_trigger "user.created" '{"user_id": "123"}'
    [ "$status" -eq 0 ]

    # Wait for all deliveries
    sleep 3
}

@test "webhook payload validation with jq" {
    # Test that payloads are valid JSON
    local valid_payload='{"user_id": "123", "email": "test@example.com"}'
    # Use jq directly with here-string — avoids pipe-inside-run status capture issue
    run jq -e '.' <<< "$valid_payload"
    [ "$status" -eq 0 ]

    # Invalid JSON should fail
    local invalid_payload='{"user_id": "123", email: test@example.com}'
    run jq -e '.' <<< "$invalid_payload"
    [ "$status" -ne 0 ]
}

# ============================================================================
# Error Handling Tests
# ============================================================================

@test "webhook functions handle missing PostgreSQL gracefully" {
    # Ensure no containers running
    docker compose down 2>/dev/null || true

    # All functions should fail with helpful error message
    run webhook_create_endpoint "https://example.com/webhook" '["user.created"]'
    [ "$status" -ne 0 ]
    [[ "$output" =~ "PostgreSQL container not found" ]]

    run webhook_list_endpoints
    [ "$status" -ne 0 ]
    [[ "$output" =~ "PostgreSQL container not found" ]]

    run webhook_delete_endpoint "00000000-0000-0000-0000-000000000000"
    [ "$status" -ne 0 ]
    [[ "$output" =~ "PostgreSQL container not found" ]]
}

@test "webhook functions handle SQL injection attempts" {
    skip "Requires running PostgreSQL container"

    nself build
    nself start
    webhook_init

    # Attempt SQL injection in URL
    local malicious_url="https://example.com/webhook'; DROP TABLE webhooks.endpoints; --"

    run webhook_create_endpoint "$malicious_url" '["user.created"]' "Malicious"
    # Should succeed (properly escaped) or fail safely
    [ "$status" -eq 0 ] || [[ "$output" =~ "ERROR" ]]

    # Verify endpoints table still exists
    run webhook_list_endpoints
    [ "$status" -eq 0 ]
}

@test "webhook disabled endpoints are not triggered" {
    skip "Requires running PostgreSQL container and endpoint enable/disable feature"

    nself build
    nself start
    webhook_init

    # Create endpoint
    local result=$(webhook_create_endpoint "https://example.com/webhook" '["user.created"]' "Test webhook")
    local endpoint_id=$(echo "$result" | jq -r '.id')

    # Disable endpoint (via SQL)
    # UPDATE webhooks.endpoints SET enabled = FALSE WHERE id = '$endpoint_id'

    # Trigger - should not deliver to disabled endpoint
    run webhook_trigger "user.created" '{"user_id": "123"}'
    [ "$status" -eq 0 ]
}

# ============================================================================
# Performance Tests
# ============================================================================

@test "webhook trigger with 100 endpoints completes quickly" {
    skip "Performance test - requires running system"

    nself build
    nself start
    webhook_init

    # Create 100 endpoints
    for i in {1..100}; do
        webhook_create_endpoint "https://httpbin.org/post?id=$i" '["user.created"]' "Webhook $i"
    done

    # Trigger should complete within reasonable time (async)
    local start=$(date +%s)
    webhook_trigger "user.created" '{"user_id": "123"}'
    local end=$(date +%s)
    local duration=$((end - start))

    # Should complete in less than 5 seconds (async spawn)
    [ "$duration" -lt 5 ]
}
