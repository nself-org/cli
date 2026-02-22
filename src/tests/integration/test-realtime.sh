#!/usr/bin/env bash
# test-realtime.sh - Real-time collaboration system integration tests
# Part of nself v0.8.0 - Sprint 16: Real-Time Collaboration
#
# Tests WebSocket infrastructure, channels, presence, messaging, and broadcasts
# Verifies database functions, security, and real-time event delivery

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Source utility functions
source "$SCRIPT_DIR/../../lib/utils/platform-compat.sh"

# Test configuration
TEST_USER_ID="123e4567-e89b-12d3-a456-426614174000"
TEST_TENANT_ID="123e4567-e89b-12d3-a456-426614174001"
TEST_CONNECTION_ID="conn_test_$(date +%s)"
TEST_SOCKET_ID="sock_test_$(date +%s)"

test_count=0
passed=0
failed=0

# Colors for output (POSIX-compliant using printf)
print_success() {
  printf "\033[32m✓\033[0m %s\n" "$1"
}

print_failure() {
  printf "\033[31m✗\033[0m %s\n" "$1"
}

print_info() {
  printf "\033[34mℹ\033[0m %s\n" "$1"
}

print_header() {
  printf "\n\033[1m=== %s ===\033[0m\n\n" "$1"
}

assert_equals() {
  test_count=$((test_count + 1))
  if [[ "$1" == "$2" ]]; then
    passed=$((passed + 1))
    print_success "Test $test_count passed"
  else
    failed=$((failed + 1))
    print_failure "Test $test_count failed: expected '$1', got '$2'"
  fi
}

assert_not_empty() {
  test_count=$((test_count + 1))
  if [[ -n "$1" ]]; then
    passed=$((passed + 1))
    print_success "Test $test_count passed"
  else
    failed=$((failed + 1))
    print_failure "Test $test_count failed: expected non-empty value"
  fi
}

assert_true() {
  test_count=$((test_count + 1))
  if [[ "$1" == "true" || "$1" == "t" || "$1" == "1" ]]; then
    passed=$((passed + 1))
    print_success "Test $test_count passed"
  else
    failed=$((failed + 1))
    print_failure "Test $test_count failed: expected true, got '$1'"
  fi
}

assert_false() {
  test_count=$((test_count + 1))
  if [[ "$1" == "false" || "$1" == "f" || "$1" == "0" || -z "$1" ]]; then
    passed=$((passed + 1))
    print_success "Test $test_count passed"
  else
    failed=$((failed + 1))
    print_failure "Test $test_count failed: expected false, got '$1'"
  fi
}

# Database connection helper
get_db_connection() {
  # Try to get database connection string from environment or Docker
  local db_host="${POSTGRES_HOST:-localhost}"
  local db_port="${POSTGRES_PORT:-5432}"
  local db_user="${POSTGRES_USER:-postgres}"
  local db_name="${POSTGRES_DB:-nself}"

  # Check if PostgreSQL container is running (|| true prevents pipefail abort when Docker daemon is not running)
  local pg_container
  pg_container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' 2>/dev/null | head -1) || true

  if [[ -n "$pg_container" ]]; then
    echo "docker exec $pg_container psql -U $db_user -d $db_name -t -A -c"
  else
    echo "psql -h $db_host -p $db_port -U $db_user -d $db_name -t -A -c"
  fi
}

# Execute SQL query
sql_exec() {
  local query="$1"
  local db_cmd
  db_cmd=$(get_db_connection)

  if [[ -z "$db_cmd" ]]; then
    printf "ERROR: Cannot connect to database\n" >&2
    return 1
  fi

  eval "$db_cmd \"$query\"" 2>/dev/null
}

# Check if PostgreSQL is available
check_postgres() {
  if ! docker ps --filter 'name=postgres' --format '{{.Names}}' 2>/dev/null | grep -q postgres; then
    print_info "PostgreSQL not running - skipping tests"
    exit 0
  fi
}

# Cleanup function
cleanup_test_data() {
  printf "\nCleaning up test data... "

  # Clean up test connections (|| true: cleanup is best-effort; database may not be running)
  sql_exec "DELETE FROM realtime.connections WHERE connection_id LIKE 'conn_test_%';" >/dev/null 2>&1 || true

  # Clean up test channels
  sql_exec "DELETE FROM realtime.channels WHERE slug LIKE 'test-%';" >/dev/null 2>&1 || true

  # Clean up test messages
  sql_exec "DELETE FROM realtime.messages WHERE user_id = '$TEST_USER_ID';" >/dev/null 2>&1 || true

  # Clean up test presence
  sql_exec "DELETE FROM realtime.presence WHERE user_id = '$TEST_USER_ID';" >/dev/null 2>&1 || true

  # Clean up test broadcasts
  sql_exec "DELETE FROM realtime.broadcasts WHERE user_id = '$TEST_USER_ID';" >/dev/null 2>&1 || true

  print_success "Done"
}

# Trap to ensure cleanup on exit
trap cleanup_test_data EXIT

# ============================================================================
# CONNECTION TESTS
# ============================================================================

test_connection_lifecycle() {
  print_header "Connection Lifecycle Tests"

  # Test 1: Establish WebSocket connection
  printf "Test 1: Establish WebSocket connection... "
  local conn_id
  conn_id=$(sql_exec "SELECT realtime.connect('$TEST_USER_ID', '$TEST_TENANT_ID', '$TEST_CONNECTION_ID', '$TEST_SOCKET_ID', '127.0.0.1', 'test-agent');")
  assert_not_empty "$conn_id"

  # Test 2: Verify connection registered in database
  printf "Test 2: Verify connection in database... "
  local status
  status=$(sql_exec "SELECT status FROM realtime.connections WHERE connection_id = '$TEST_CONNECTION_ID';")
  assert_equals "connected" "$status"

  # Test 3: Verify connection details
  printf "Test 3: Verify connection details... "
  local user_id
  user_id=$(sql_exec "SELECT user_id::text FROM realtime.connections WHERE connection_id = '$TEST_CONNECTION_ID';")
  assert_equals "$TEST_USER_ID" "$user_id"

  # Test 4: Update last seen
  printf "Test 4: Update last_seen_at... "
  sql_exec "UPDATE realtime.connections SET last_seen_at = NOW() WHERE connection_id = '$TEST_CONNECTION_ID';" >/dev/null
  local last_seen
  last_seen=$(sql_exec "SELECT last_seen_at FROM realtime.connections WHERE connection_id = '$TEST_CONNECTION_ID';")
  assert_not_empty "$last_seen"

  # Test 5: Disconnect
  printf "Test 5: Disconnect WebSocket... "
  sql_exec "SELECT realtime.disconnect('$TEST_CONNECTION_ID');" >/dev/null
  status=$(sql_exec "SELECT status FROM realtime.connections WHERE connection_id = '$TEST_CONNECTION_ID';")
  assert_equals "disconnected" "$status"

  # Test 6: Verify cleanup on disconnect
  printf "Test 6: Verify presence cleanup... "
  local presence_count
  presence_count=$(sql_exec "SELECT COUNT(*) FROM realtime.presence WHERE user_id = '$TEST_USER_ID';")
  assert_equals "0" "$presence_count"

  # Test 7: Verify subscriptions cleanup
  printf "Test 7: Verify subscriptions cleanup... "
  local sub_count
  sub_count=$(sql_exec "SELECT COUNT(*) FROM realtime.subscriptions WHERE connection_id = '$conn_id';")
  assert_equals "0" "$sub_count"
}

# ============================================================================
# CHANNEL TESTS
# ============================================================================

test_channel_management() {
  print_header "Channel Management Tests"

  # Reconnect for channel tests
  local conn_id
  conn_id=$(sql_exec "SELECT realtime.connect('$TEST_USER_ID', '$TEST_TENANT_ID', '${TEST_CONNECTION_ID}_ch', '${TEST_SOCKET_ID}_ch', '127.0.0.1', 'test-agent');")

  # Test 8: Create public channel
  printf "Test 8: Create public channel... "
  local channel_id
  channel_id=$(sql_exec "INSERT INTO realtime.channels (tenant_id, name, slug, type, created_by) VALUES ('$TEST_TENANT_ID', 'Test Channel', 'test-channel', 'public', '$TEST_USER_ID') RETURNING id;")
  assert_not_empty "$channel_id"

  # Test 9: Create private channel
  printf "Test 9: Create private channel... "
  local private_channel_id
  private_channel_id=$(sql_exec "INSERT INTO realtime.channels (tenant_id, name, slug, type, created_by) VALUES ('$TEST_TENANT_ID', 'Private Channel', 'test-private', 'private', '$TEST_USER_ID') RETURNING id;")
  assert_not_empty "$private_channel_id"

  # Test 10: Join channel
  printf "Test 10: Join channel... "
  sql_exec "INSERT INTO realtime.channel_members (channel_id, user_id, role) VALUES ('$channel_id', '$TEST_USER_ID', 'owner');" >/dev/null
  local member_exists
  member_exists=$(sql_exec "SELECT EXISTS(SELECT 1 FROM realtime.channel_members WHERE channel_id = '$channel_id' AND user_id = '$TEST_USER_ID');")
  assert_true "$member_exists"

  # Test 11: Join private channel
  printf "Test 11: Join private channel... "
  sql_exec "INSERT INTO realtime.channel_members (channel_id, user_id, role, can_send, can_invite) VALUES ('$private_channel_id', '$TEST_USER_ID', 'owner', true, true);" >/dev/null
  member_exists=$(sql_exec "SELECT EXISTS(SELECT 1 FROM realtime.channel_members WHERE channel_id = '$private_channel_id' AND user_id = '$TEST_USER_ID');")
  assert_true "$member_exists"

  # Test 12: Subscribe to channel
  printf "Test 12: Subscribe to channel... "
  sql_exec "INSERT INTO realtime.subscriptions (user_id, channel_id, connection_id) VALUES ('$TEST_USER_ID', '$channel_id', '$conn_id');" >/dev/null
  local sub_exists
  sub_exists=$(sql_exec "SELECT EXISTS(SELECT 1 FROM realtime.subscriptions WHERE channel_id = '$channel_id' AND user_id = '$TEST_USER_ID');")
  assert_true "$sub_exists"

  # Test 13: Verify member list
  printf "Test 13: Verify member list... "
  local member_count
  member_count=$(sql_exec "SELECT COUNT(*) FROM realtime.channel_members WHERE channel_id = '$channel_id';")
  assert_equals "1" "$member_count"

  # Test 14: Leave channel
  printf "Test 14: Leave channel... "
  sql_exec "DELETE FROM realtime.channel_members WHERE channel_id = '$channel_id' AND user_id = '$TEST_USER_ID';" >/dev/null
  member_exists=$(sql_exec "SELECT EXISTS(SELECT 1 FROM realtime.channel_members WHERE channel_id = '$channel_id' AND user_id = '$TEST_USER_ID');")
  assert_false "$member_exists"

  # Cleanup
  sql_exec "SELECT realtime.disconnect('${TEST_CONNECTION_ID}_ch');" >/dev/null
}

# ============================================================================
# MESSAGING TESTS
# ============================================================================

test_messaging() {
  print_header "Messaging Tests"

  # Setup: Create channel and join
  local conn_id
  conn_id=$(sql_exec "SELECT realtime.connect('$TEST_USER_ID', '$TEST_TENANT_ID', '${TEST_CONNECTION_ID}_msg', '${TEST_SOCKET_ID}_msg', '127.0.0.1', 'test-agent');")

  local channel_id
  channel_id=$(sql_exec "INSERT INTO realtime.channels (tenant_id, name, slug, type, created_by) VALUES ('$TEST_TENANT_ID', 'Message Test', 'test-msg-channel', 'public', '$TEST_USER_ID') RETURNING id;")

  sql_exec "INSERT INTO realtime.channel_members (channel_id, user_id, role, can_send) VALUES ('$channel_id', '$TEST_USER_ID', 'member', true);" >/dev/null

  # Test 15: Send message to channel
  printf "Test 15: Send message to channel... "
  local msg_id
  msg_id=$(sql_exec "SELECT realtime.send_message('$channel_id', '$TEST_USER_ID', 'Hello, World!', 'text', '{}'::jsonb);")
  assert_not_empty "$msg_id"

  # Test 16: Verify message persisted
  printf "Test 16: Verify message persistence... "
  local msg_content
  msg_content=$(sql_exec "SELECT content FROM realtime.messages WHERE id = '$msg_id';")
  assert_equals "Hello, World!" "$msg_content"

  # Test 17: Send system message
  printf "Test 17: Send system message... "
  local sys_msg_id
  sys_msg_id=$(sql_exec "SELECT realtime.send_message('$channel_id', '$TEST_USER_ID', 'User joined', 'system', '{}'::jsonb);")
  assert_not_empty "$sys_msg_id"

  # Test 18: Verify message type
  printf "Test 18: Verify message type... "
  local msg_type
  msg_type=$(sql_exec "SELECT message_type FROM realtime.messages WHERE id = '$sys_msg_id';")
  assert_equals "system" "$msg_type"

  # Test 19: Edit message
  printf "Test 19: Edit message... "
  sql_exec "UPDATE realtime.messages SET content = 'Hello, Edited!', edited_at = NOW() WHERE id = '$msg_id';" >/dev/null
  msg_content=$(sql_exec "SELECT content FROM realtime.messages WHERE id = '$msg_id';")
  assert_equals "Hello, Edited!" "$msg_content"

  # Test 20: Verify message count in channel
  printf "Test 20: Verify message count... "
  local msg_count
  msg_count=$(sql_exec "SELECT COUNT(*) FROM realtime.messages WHERE channel_id = '$channel_id';")
  assert_equals "2" "$msg_count"

  # Cleanup
  sql_exec "SELECT realtime.disconnect('${TEST_CONNECTION_ID}_msg');" >/dev/null
}

# ============================================================================
# PRESENCE TESTS
# ============================================================================

test_presence() {
  print_header "Presence Tests"

  # Setup
  local conn_id
  conn_id=$(sql_exec "SELECT realtime.connect('$TEST_USER_ID', '$TEST_TENANT_ID', '${TEST_CONNECTION_ID}_pres', '${TEST_SOCKET_ID}_pres', '127.0.0.1', 'test-agent');")

  local channel_id
  channel_id=$(sql_exec "INSERT INTO realtime.channels (tenant_id, name, slug, type, created_by) VALUES ('$TEST_TENANT_ID', 'Presence Test', 'test-presence', 'presence', '$TEST_USER_ID') RETURNING id;")

  # Test 21: Set user status (online)
  printf "Test 21: Set user status to online... "
  sql_exec "SELECT realtime.update_presence('$TEST_USER_ID', '$channel_id', 'online', '{}'::jsonb);" >/dev/null
  local status
  status=$(sql_exec "SELECT status FROM realtime.presence WHERE user_id = '$TEST_USER_ID' AND channel_id = '$channel_id';")
  assert_equals "online" "$status"

  # Test 22: Set user status (away)
  printf "Test 22: Set user status to away... "
  sql_exec "SELECT realtime.update_presence('$TEST_USER_ID', '$channel_id', 'away', '{}'::jsonb);" >/dev/null
  status=$(sql_exec "SELECT status FROM realtime.presence WHERE user_id = '$TEST_USER_ID' AND channel_id = '$channel_id';")
  assert_equals "away" "$status"

  # Test 23: Set user status (busy)
  printf "Test 23: Set user status to busy... "
  sql_exec "SELECT realtime.update_presence('$TEST_USER_ID', '$channel_id', 'busy', '{}'::jsonb);" >/dev/null
  status=$(sql_exec "SELECT status FROM realtime.presence WHERE user_id = '$TEST_USER_ID' AND channel_id = '$channel_id';")
  assert_equals "busy" "$status"

  # Test 24: Update cursor position
  printf "Test 24: Update cursor position... "
  sql_exec "UPDATE realtime.presence SET cursor_position = '{\"line\": 10, \"column\": 5}'::jsonb WHERE user_id = '$TEST_USER_ID' AND channel_id = '$channel_id';" >/dev/null
  local cursor
  cursor=$(sql_exec "SELECT cursor_position::text FROM realtime.presence WHERE user_id = '$TEST_USER_ID' AND channel_id = '$channel_id';")
  assert_not_empty "$cursor"

  # Test 25: Update selection
  printf "Test 25: Update selection... "
  sql_exec "UPDATE realtime.presence SET selection = '{\"start\": {\"line\": 5, \"column\": 0}, \"end\": {\"line\": 10, \"column\": 20}}'::jsonb WHERE user_id = '$TEST_USER_ID' AND channel_id = '$channel_id';" >/dev/null
  local selection
  selection=$(sql_exec "SELECT selection::text FROM realtime.presence WHERE user_id = '$TEST_USER_ID' AND channel_id = '$channel_id';")
  assert_not_empty "$selection"

  # Test 26: Get online users
  printf "Test 26: Get online users in channel... "
  sql_exec "SELECT realtime.update_presence('$TEST_USER_ID', '$channel_id', 'online', '{}'::jsonb);" >/dev/null
  local online_count
  online_count=$(sql_exec "SELECT COUNT(*) FROM realtime.get_online_users('$channel_id');")
  assert_equals "1" "$online_count"

  # Test 27: Verify presence cleanup on disconnect
  printf "Test 27: Verify presence cleanup on disconnect... "
  sql_exec "SELECT realtime.disconnect('${TEST_CONNECTION_ID}_pres');" >/dev/null
  local presence_count
  presence_count=$(sql_exec "SELECT COUNT(*) FROM realtime.presence WHERE user_id = '$TEST_USER_ID' AND channel_id = '$channel_id';")
  assert_equals "0" "$presence_count"
}

# ============================================================================
# BROADCAST TESTS
# ============================================================================

test_broadcasts() {
  print_header "Broadcast Tests"

  # Setup
  local conn_id
  conn_id=$(sql_exec "SELECT realtime.connect('$TEST_USER_ID', '$TEST_TENANT_ID', '${TEST_CONNECTION_ID}_bc', '${TEST_SOCKET_ID}_bc', '127.0.0.1', 'test-agent');")

  local channel_id
  channel_id=$(sql_exec "INSERT INTO realtime.channels (tenant_id, name, slug, type, created_by) VALUES ('$TEST_TENANT_ID', 'Broadcast Test', 'test-broadcast', 'public', '$TEST_USER_ID') RETURNING id;")

  # Test 28: Send ephemeral event (typing indicator)
  printf "Test 28: Send typing indicator... "
  local broadcast_id
  broadcast_id=$(sql_exec "SELECT realtime.broadcast('$channel_id', '$TEST_USER_ID', 'typing', '{\"typing\": true}'::jsonb);")
  assert_not_empty "$broadcast_id"

  # Test 29: Verify broadcast stored
  printf "Test 29: Verify broadcast stored... "
  local event_type
  event_type=$(sql_exec "SELECT event_type FROM realtime.broadcasts WHERE id = '$broadcast_id';")
  assert_equals "typing" "$event_type"

  # Test 30: Send cursor move event
  printf "Test 30: Send cursor move event... "
  local cursor_broadcast_id
  cursor_broadcast_id=$(sql_exec "SELECT realtime.broadcast('$channel_id', '$TEST_USER_ID', 'cursor_move', '{\"x\": 100, \"y\": 200}'::jsonb);")
  assert_not_empty "$cursor_broadcast_id"

  # Test 31: Verify broadcast payload
  printf "Test 31: Verify broadcast payload... "
  local payload
  payload=$(sql_exec "SELECT payload::text FROM realtime.broadcasts WHERE id = '$cursor_broadcast_id';")
  assert_not_empty "$payload"

  # Test 32: Test broadcast expiry
  printf "Test 32: Test broadcast expiry... "
  local expires_at
  expires_at=$(sql_exec "SELECT expires_at FROM realtime.broadcasts WHERE id = '$broadcast_id';")
  assert_not_empty "$expires_at"

  # Test 33: Cleanup expired broadcasts
  printf "Test 33: Cleanup expired broadcasts... "
  # Set broadcast to expired
  sql_exec "UPDATE realtime.broadcasts SET expires_at = NOW() - INTERVAL '1 minute' WHERE id = '$broadcast_id';" >/dev/null
  local deleted_count
  deleted_count=$(sql_exec "SELECT realtime.cleanup_expired_broadcasts();")
  # Should have deleted at least the one we expired
  local remaining
  remaining=$(sql_exec "SELECT COUNT(*) FROM realtime.broadcasts WHERE id = '$broadcast_id';")
  assert_equals "0" "$remaining"

  # Cleanup
  sql_exec "SELECT realtime.disconnect('${TEST_CONNECTION_ID}_bc');" >/dev/null
}

# ============================================================================
# SECURITY TESTS
# ============================================================================

test_security() {
  print_header "Security Tests"

  local unauthorized_user_id="999e4567-e89b-12d3-a456-426614174000"

  # Setup: Create private channel
  local channel_id
  channel_id=$(sql_exec "INSERT INTO realtime.channels (tenant_id, name, slug, type, created_by) VALUES ('$TEST_TENANT_ID', 'Private Channel', 'test-security', 'private', '$TEST_USER_ID') RETURNING id;")

  # Add authorized user
  sql_exec "INSERT INTO realtime.channel_members (channel_id, user_id, role, can_send) VALUES ('$channel_id', '$TEST_USER_ID', 'owner', true);" >/dev/null

  # Test 34: Attempt to send message without permission
  printf "Test 34: Unauthorized message send (should fail)... "
  local error_occurred=false
  sql_exec "SELECT realtime.send_message('$channel_id', '$unauthorized_user_id', 'Unauthorized', 'text', '{}'::jsonb);" 2>/dev/null || error_occurred=true
  assert_true "$error_occurred"

  # Test 35: Add member without send permission
  printf "Test 35: Add member without send permission... "
  sql_exec "INSERT INTO realtime.channel_members (channel_id, user_id, role, can_send) VALUES ('$channel_id', '$unauthorized_user_id', 'member', false);" >/dev/null
  local can_send
  can_send=$(sql_exec "SELECT can_send FROM realtime.channel_members WHERE channel_id = '$channel_id' AND user_id = '$unauthorized_user_id';")
  assert_false "$can_send"

  # Test 36: Verify unauthorized user cannot send
  printf "Test 36: Verify member without permission cannot send... "
  error_occurred=false
  sql_exec "SELECT realtime.send_message('$channel_id', '$unauthorized_user_id', 'Still unauthorized', 'text', '{}'::jsonb);" 2>/dev/null || error_occurred=true
  assert_true "$error_occurred"

  # Test 37: Grant send permission
  printf "Test 37: Grant send permission... "
  sql_exec "UPDATE realtime.channel_members SET can_send = true WHERE channel_id = '$channel_id' AND user_id = '$unauthorized_user_id';" >/dev/null
  can_send=$(sql_exec "SELECT can_send FROM realtime.channel_members WHERE channel_id = '$channel_id' AND user_id = '$unauthorized_user_id';")
  assert_true "$can_send"

  # Test 38: Verify authorized send works
  printf "Test 38: Verify authorized send works... "
  local msg_id
  msg_id=$(sql_exec "SELECT realtime.send_message('$channel_id', '$unauthorized_user_id', 'Now authorized', 'text', '{}'::jsonb);" 2>/dev/null)
  assert_not_empty "$msg_id"

  # Test 39: Verify invite permission
  printf "Test 39: Verify invite permission... "
  local can_invite
  can_invite=$(sql_exec "SELECT can_invite FROM realtime.channel_members WHERE channel_id = '$channel_id' AND user_id = '$unauthorized_user_id';")
  assert_false "$can_invite"

  # Test 40: Attempt to send to non-existent channel
  printf "Test 40: Send to non-existent channel (should fail)... "
  local fake_channel_id="00000000-0000-0000-0000-000000000000"
  error_occurred=false
  sql_exec "SELECT realtime.send_message('$fake_channel_id', '$TEST_USER_ID', 'Test', 'text', '{}'::jsonb);" 2>/dev/null || error_occurred=true
  assert_true "$error_occurred"
}

# ============================================================================
# VIEWS AND MONITORING TESTS
# ============================================================================

test_views_and_monitoring() {
  print_header "Views and Monitoring Tests"

  # Setup: Create connection and channel
  local conn_id
  conn_id=$(sql_exec "SELECT realtime.connect('$TEST_USER_ID', '$TEST_TENANT_ID', '${TEST_CONNECTION_ID}_view', '${TEST_SOCKET_ID}_view', '127.0.0.1', 'test-agent');")

  local channel_id
  channel_id=$(sql_exec "INSERT INTO realtime.channels (tenant_id, name, slug, type, created_by) VALUES ('$TEST_TENANT_ID', 'Monitor Test', 'test-monitor', 'public', '$TEST_USER_ID') RETURNING id;")

  sql_exec "INSERT INTO realtime.channel_members (channel_id, user_id, role) VALUES ('$channel_id', '$TEST_USER_ID', 'member');" >/dev/null
  sql_exec "INSERT INTO realtime.subscriptions (user_id, channel_id, connection_id) VALUES ('$TEST_USER_ID', '$channel_id', '$conn_id');" >/dev/null

  # Test 41: Query active connections view
  printf "Test 41: Query active connections view... "
  local active_conn_count
  active_conn_count=$(sql_exec "SELECT COUNT(*) FROM realtime.active_connections WHERE connection_id = '${TEST_CONNECTION_ID}_view';")
  assert_equals "1" "$active_conn_count"

  # Test 42: Verify subscribed channels count
  printf "Test 42: Verify subscribed channels count... "
  local subscribed_channels
  subscribed_channels=$(sql_exec "SELECT subscribed_channels FROM realtime.active_connections WHERE connection_id = '${TEST_CONNECTION_ID}_view';")
  assert_equals "1" "$subscribed_channels"

  # Test 43: Query channel activity view
  printf "Test 43: Query channel activity view... "
  local activity_exists
  activity_exists=$(sql_exec "SELECT EXISTS(SELECT 1 FROM realtime.channel_activity WHERE slug = 'test-monitor');")
  assert_true "$activity_exists"

  # Test 44: Verify member count in activity view
  printf "Test 44: Verify member count... "
  local total_members
  total_members=$(sql_exec "SELECT total_members FROM realtime.channel_activity WHERE slug = 'test-monitor';")
  assert_equals "1" "$total_members"

  # Test 45: Cleanup stale connections
  printf "Test 45: Cleanup stale connections... "
  # Mark connection as old
  sql_exec "UPDATE realtime.connections SET last_seen_at = NOW() - INTERVAL '10 minutes' WHERE connection_id = '${TEST_CONNECTION_ID}_view';" >/dev/null
  local cleaned
  cleaned=$(sql_exec "SELECT realtime.cleanup_stale_connections();")
  # Should mark as disconnected
  local status
  status=$(sql_exec "SELECT status FROM realtime.connections WHERE connection_id = '${TEST_CONNECTION_ID}_view';")
  assert_equals "disconnected" "$status"
}

# ============================================================================
# MAIN TEST EXECUTION
# ============================================================================

main() {
  print_header "Real-Time Collaboration Integration Tests"

  # Check PostgreSQL availability
  check_postgres

  # Run test suites
  test_connection_lifecycle
  test_channel_management
  test_messaging
  test_presence
  test_broadcasts
  test_security
  test_views_and_monitoring

  # Print summary
  print_header "Test Summary"
  printf "Total tests: %d\n" "$test_count"
  printf "Passed: %d\n" "$passed"
  printf "Failed: %d\n" "$failed"

  if [[ $failed -eq 0 ]]; then
    printf "\n"
    print_success "All tests passed!"
    printf "\nSprint 16: Real-time collaboration tests complete!\n\n"
    exit 0
  else
    printf "\n"
    print_failure "Some tests failed"
    exit 1
  fi
}

# Run tests
main
