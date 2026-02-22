#!/usr/bin/env bash
# test-redis.sh - Redis integration tests
# Part of nself v0.7.0 - Sprint 6: RDS-005

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../../lib/redis/core.sh"
source "$SCRIPT_DIR/../../lib/redis/rate-limit-distributed.sh"
source "$SCRIPT_DIR/../../lib/redis/sessions.sh"
source "$SCRIPT_DIR/../../lib/redis/cache.sh"

printf "\n=== Redis Integration Tests ===\n\n"

# Check if Redis is available (|| true prevents pipefail abort when Docker daemon is not running)
redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' 2>/dev/null | head -1) || true
if [[ -z "$redis_container" ]]; then
  printf "⚠ Redis not running - skipping tests\n"
  exit 0
fi

# Test 1: Initialize Redis configuration
printf "Test 1: Initialize Redis... "
redis_init && printf "✓\n" || printf "✗\n"

# Test 2: Add Redis connection
printf "Test 2: Add connection... "
conn_id=$(redis_connection_add "test" "localhost" 6379 0 "" 2>/dev/null)
[[ -n "$conn_id" ]] && printf "✓\n" || printf "✗\n"

# Test 3: Test connection
printf "Test 3: Test connection... "
redis_connection_test "test" >/dev/null 2>&1 && printf "✓\n" || printf "✗\n"

# Test 4: Distributed rate limiting
printf "Test 4: Distributed rate limit... "
redis_rate_limit_check "test_ip" 10 60 "test" 2>/dev/null && printf "✓\n" || printf "✗\n"

# Test 5: Cache set and get
printf "Test 5: Cache operations... "
redis_cache_set "test_key" "test_value" 60 "test" 2>/dev/null
value=$(redis_cache_get "test_key" "test" 2>/dev/null)
[[ "$value" == "test_value" ]] && printf "✓\n" || printf "✗\n"

# Test 6: Session management
printf "Test 6: Session create/get... "
session_id=$(redis_session_create "sess_test" "user_123" '{"ip":"127.0.0.1"}' 300 "test" 2>/dev/null)
session=$(redis_session_get "$session_id" "test" 2>/dev/null)
[[ "$session" != "null" ]] && printf "✓\n" || printf "✗\n"

# Test 7: Health monitoring
printf "Test 7: Health check... "
health=$(redis_health_status "test" 2>/dev/null)
[[ -n "$health" ]] && printf "✓\n" || printf "✗\n"

# Test 8: Pool configuration
printf "Test 8: Pool config... "
redis_pool_configure "test" 20 5 10 300 2>/dev/null && printf "✓\n" || printf "✗\n"

# Cleanup
printf "\nCleaning up test data... "
redis_connection_delete "test" 2>/dev/null && printf "✓\n" || printf "✗\n"

printf "\n=== Test Summary ===\n"
printf "Total: 8 tests\n"
printf "Sprint 6: Redis integration tests complete!\n\n"
