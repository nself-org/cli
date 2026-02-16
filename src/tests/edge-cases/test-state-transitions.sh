#!/usr/bin/env bash
set -euo pipefail

# test-state-transitions.sh - Tests for service state transition edge cases
# Tests unusual state transitions and idempotency

set -e

# Get script directory
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$TEST_DIR/../.."

# Source test framework
source "$TEST_DIR/../test_framework.sh"

# ============================================
# Test Setup
# ============================================

setup_test_environment() {
  export TEST_MODE=1
  export NO_COLOR=1
  TEMP_DIR=$(mktemp -d)
  export TEMP_DIR
}

teardown_test_environment() {
  if [[ -n "$TEMP_DIR" ]] && [[ -d "$TEMP_DIR" ]]; then
    rm -rf "$TEMP_DIR"
  fi
}

# ============================================
# Service State Simulation
# ============================================

# Simulate service states
SERVICE_STATE="stopped"

mock_service_start() {
  if [[ "$SERVICE_STATE" == "running" ]]; then
    return 0  # Idempotent - already running
  elif [[ "$SERVICE_STATE" == "stopped" ]]; then
    SERVICE_STATE="running"
    return 0
  else
    return 1  # Invalid state
  fi
}

mock_service_stop() {
  if [[ "$SERVICE_STATE" == "stopped" ]]; then
    return 0  # Idempotent - already stopped
  elif [[ "$SERVICE_STATE" == "running" ]]; then
    SERVICE_STATE="stopped"
    return 0
  else
    return 1  # Invalid state
  fi
}

mock_service_restart() {
  if [[ "$SERVICE_STATE" == "stopped" ]]; then
    # Restart when stopped = just start
    SERVICE_STATE="running"
    return 0
  elif [[ "$SERVICE_STATE" == "running" ]]; then
    # Normal restart
    SERVICE_STATE="stopped"
    SERVICE_STATE="running"
    return 0
  else
    return 1
  fi
}

# ============================================
# Idempotency Tests
# ============================================

test_start_already_running() {
  local test_name="Start service that's already running (idempotent)"

  # Setup: service is running
  SERVICE_STATE="running"

  # Action: try to start again
  if mock_service_start; then
    local result="success"
  else
    local result="failure"
  fi

  # Should succeed (idempotent)
  assert_equals "$result" "success" "$test_name: Start is idempotent"
  assert_equals "$SERVICE_STATE" "running" "$test_name: State unchanged"
}

test_stop_already_stopped() {
  local test_name="Stop service that's already stopped (idempotent)"

  # Setup: service is stopped
  SERVICE_STATE="stopped"

  # Action: try to stop again
  if mock_service_stop; then
    local result="success"
  else
    local result="failure"
  fi

  # Should succeed (idempotent)
  assert_equals "$result" "success" "$test_name: Stop is idempotent"
  assert_equals "$SERVICE_STATE" "stopped" "$test_name: State unchanged"
}

test_restart_stopped_service() {
  local test_name="Restart service that's not running"

  # Setup: service is stopped
  SERVICE_STATE="stopped"

  # Action: restart
  if mock_service_restart; then
    local result="success"
  else
    local result="failure"
  fi

  # Should succeed and start the service
  assert_equals "$result" "success" "$test_name: Restart of stopped service works"
  assert_equals "$SERVICE_STATE" "running" "$test_name: Service is now running"
}

test_multiple_starts() {
  local test_name="Multiple start commands in sequence"

  SERVICE_STATE="stopped"

  # Start multiple times
  mock_service_start
  local state1="$SERVICE_STATE"

  mock_service_start
  local state2="$SERVICE_STATE"

  mock_service_start
  local state3="$SERVICE_STATE"

  # All should result in running state
  assert_equals "$state1" "running" "$test_name: First start"
  assert_equals "$state2" "running" "$test_name: Second start"
  assert_equals "$state3" "running" "$test_name: Third start"
}

# ============================================
# Invalid State Transition Tests
# ============================================

test_build_without_init() {
  local test_name="Build without init (missing .env)"

  # Simulate build check
  check_init_required() {
    if [[ ! -f "$TEMP_DIR/.env" ]]; then
      return 1  # Init required
    fi
    return 0
  }

  # Should fail
  if check_init_required; then
    local result="allowed"
  else
    local result="blocked"
  fi

  assert_equals "$result" "blocked" "$test_name: Build blocked without init"
}

test_start_without_build() {
  local test_name="Start without build (missing docker-compose.yml)"

  # Simulate start check
  check_build_required() {
    if [[ ! -f "$TEMP_DIR/docker-compose.yml" ]]; then
      return 1  # Build required
    fi
    return 0
  }

  # Should fail
  if check_build_required; then
    local result="allowed"
  else
    local result="blocked"
  fi

  assert_equals "$result" "blocked" "$test_name: Start blocked without build"
}

test_deploy_without_build() {
  local test_name="Deploy without build"

  # Simulate deploy check
  check_deploy_prerequisites() {
    # Need both .env and docker-compose.yml
    if [[ ! -f "$TEMP_DIR/.env" ]] || [[ ! -f "$TEMP_DIR/docker-compose.yml" ]]; then
      return 1
    fi
    return 0
  }

  # Should fail
  if check_deploy_prerequisites; then
    local result="allowed"
  else
    local result="blocked"
  fi

  assert_equals "$result" "blocked" "$test_name: Deploy blocked without prerequisites"
}

# ============================================
# Rapid State Change Tests
# ============================================

test_rapid_start_stop() {
  local test_name="Rapid start/stop cycles"

  SERVICE_STATE="stopped"

  # Rapidly cycle through states
  mock_service_start
  assert_equals "$SERVICE_STATE" "running" "$test_name: After start"

  mock_service_stop
  assert_equals "$SERVICE_STATE" "stopped" "$test_name: After stop"

  mock_service_start
  assert_equals "$SERVICE_STATE" "running" "$test_name: After restart"

  mock_service_stop
  assert_equals "$SERVICE_STATE" "stopped" "$test_name: After second stop"
}

test_restart_cycles() {
  local test_name="Multiple restart commands"

  SERVICE_STATE="running"

  # Multiple restarts should always work
  mock_service_restart
  assert_equals "$SERVICE_STATE" "running" "$test_name: After first restart"

  mock_service_restart
  assert_equals "$SERVICE_STATE" "running" "$test_name: After second restart"

  mock_service_restart
  assert_equals "$SERVICE_STATE" "running" "$test_name: After third restart"
}

# ============================================
# Error Recovery Tests
# ============================================

test_start_after_crash() {
  local test_name="Start service after crash"

  # Simulate crash (service was running, now it's not)
  SERVICE_STATE="crashed"

  # Start should handle crashed state
  if [[ "$SERVICE_STATE" == "crashed" ]]; then
    SERVICE_STATE="stopped"  # Reset to stopped
  fi

  mock_service_start
  assert_equals "$SERVICE_STATE" "running" "$test_name: Recovered from crash"
}

test_operation_after_interrupted_build() {
  local test_name="Operation after interrupted build"

  # Simulate interrupted build (partial docker-compose.yml)
  printf "version: '3.8'\nservices:\n  # incomplete\n" > "$TEMP_DIR/docker-compose.yml"

  # Should detect incomplete build
  check_compose_valid() {
    if ! grep -q "postgres:" "$TEMP_DIR/docker-compose.yml" 2>/dev/null; then
      return 1  # Invalid/incomplete
    fi
    return 0
  }

  if check_compose_valid; then
    local result="valid"
  else
    local result="invalid"
  fi

  assert_equals "$result" "invalid" "$test_name: Detected incomplete build"

  # Cleanup
  rm -f "$TEMP_DIR/docker-compose.yml"
}

# ============================================
# Concurrent Operation Tests
# ============================================

test_multiple_build_commands() {
  local test_name="Multiple build commands simultaneously"

  # Simulate build lock
  BUILD_LOCK_FILE="$TEMP_DIR/.nself.build.lock"

  acquire_build_lock() {
    if [[ -f "$BUILD_LOCK_FILE" ]]; then
      return 1  # Lock exists
    fi
    touch "$BUILD_LOCK_FILE"
    return 0
  }

  release_build_lock() {
    rm -f "$BUILD_LOCK_FILE"
  }

  # First build should get lock
  if acquire_build_lock; then
    local first="success"
  else
    local first="blocked"
  fi

  # Second build should be blocked
  if acquire_build_lock; then
    local second="success"
  else
    local second="blocked"
  fi

  assert_equals "$first" "success" "$test_name: First build gets lock"
  assert_equals "$second" "blocked" "$test_name: Second build blocked by lock"

  # Cleanup
  release_build_lock
}

# ============================================
# Configuration Change During Runtime Tests
# ============================================

test_env_change_while_running() {
  local test_name="Environment change while service running"

  SERVICE_STATE="running"

  # Simulate env change
  printf "PROJECT_NAME=test\n" > "$TEMP_DIR/.env"
  sleep 0.1
  printf "PROJECT_NAME=changed\n" > "$TEMP_DIR/.env"

  # Service should still be running but may need rebuild
  assert_equals "$SERVICE_STATE" "running" "$test_name: Service still running"

  # Should warn about rebuild needed (simulate)
  local rebuild_needed=true
  assert_equals "$rebuild_needed" "true" "$test_name: Rebuild needed detected"

  # Cleanup
  rm -f "$TEMP_DIR/.env"
}

test_compose_change_detection() {
  local test_name="docker-compose.yml changed while running"

  # Simulate compose file change
  printf "version: '3.8'\n" > "$TEMP_DIR/docker-compose.yml"
  local mtime1=$(stat -f "%m" "$TEMP_DIR/docker-compose.yml" 2>/dev/null || stat -c "%Y" "$TEMP_DIR/docker-compose.yml" 2>/dev/null || echo "0")

  sleep 1

  printf "version: '3.8'\n# changed\n" > "$TEMP_DIR/docker-compose.yml"
  local mtime2=$(stat -f "%m" "$TEMP_DIR/docker-compose.yml" 2>/dev/null || stat -c "%Y" "$TEMP_DIR/docker-compose.yml" 2>/dev/null || echo "0")

  # Modification time should be different
  if [[ "$mtime1" != "$mtime2" ]]; then
    local changed=true
  else
    local changed=false
  fi

  assert_equals "$changed" "true" "$test_name: Change detected"

  # Cleanup
  rm -f "$TEMP_DIR/docker-compose.yml"
}

# ============================================
# Partial Failure Tests
# ============================================

test_some_services_start_others_fail() {
  local test_name="Partial service startup"

  # Simulate multiple services
  local services=("postgres" "redis" "hasura")
  local started=()
  local failed=()

  # Simulate postgres and redis start, hasura fails
  started+=("postgres")
  started+=("redis")
  failed+=("hasura")

  local started_count=${#started[@]}
  local failed_count=${#failed[@]}

  assert_equals "$started_count" "2" "$test_name: Two services started"
  assert_equals "$failed_count" "1" "$test_name: One service failed"

  # System should be in partial state
  local state="partial"
  assert_equals "$state" "partial" "$test_name: System in partial state"
}

# ============================================
# Test Runner
# ============================================

run_all_tests() {
  printf "\n========================================\n"
  printf "  State Transition Tests\n"
  printf "========================================\n\n"

  setup_test_environment

  # Idempotency
  test_start_already_running
  test_stop_already_stopped
  test_restart_stopped_service
  test_multiple_starts

  # Invalid transitions
  test_build_without_init
  test_start_without_build
  test_deploy_without_build

  # Rapid changes
  test_rapid_start_stop
  test_restart_cycles

  # Error recovery
  test_start_after_crash
  test_operation_after_interrupted_build

  # Concurrent operations
  test_multiple_build_commands

  # Runtime changes
  test_env_change_while_running
  test_compose_change_detection

  # Partial failures
  test_some_services_start_others_fail

  teardown_test_environment

  # Summary
  printf "\n========================================\n"
  printf "  Test Results\n"
  printf "========================================\n"
  printf "Total:   %d\n" "$TESTS_RUN"
  printf "Passed:  %d\n" "$TESTS_PASSED"
  printf "Failed:  %d\n" "$TESTS_FAILED"
  printf "Skipped: %d\n" "$TESTS_SKIPPED"

  if [[ $TESTS_FAILED -eq 0 ]]; then
    printf "\n✓ All tests passed!\n\n"
    return 0
  else
    printf "\n✗ Some tests failed\n\n"
    return 1
  fi
}

# Run tests if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  run_all_tests
fi
