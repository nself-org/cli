#!/usr/bin/env bash
set -euo pipefail
# Manual test script for deploy server management features
# Tests all 10 implemented features

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$(dirname "$SCRIPT_DIR")")"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Test results
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Helper functions
test_start() {
  TESTS_RUN=$((TESTS_RUN + 1))
  printf "\n[TEST %d] %s\n" "$TESTS_RUN" "$1"
}

test_pass() {
  printf "${GREEN}✓ PASS${NC} %s\n" "$1"
  TESTS_PASSED=$((TESTS_PASSED + 1))
}

test_fail() {
  printf "${RED}✗ FAIL${NC} %s\n" "$1"
  TESTS_FAILED=$((TESTS_FAILED + 1))
}

test_skip() {
  printf "${YELLOW}⊘ SKIP${NC} %s\n" "$1"
}

# Setup test environment
setup_test_env() {
  printf "\n${YELLOW}Setting up test environment...${NC}\n"

  # Create test environments directory
  mkdir -p .environments/test-staging
  mkdir -p .environments/test-prod

  # Create test server.json files
  cat > .environments/test-staging/server.json <<EOF
{
  "name": "test-staging",
  "type": "remote",
  "host": "test-staging.example.com",
  "port": 22,
  "user": "root",
  "key": "",
  "deploy_path": "/var/www/nself",
  "description": "Test staging server"
}
EOF

  cat > .environments/test-prod/server.json <<EOF
{
  "name": "test-prod",
  "type": "remote",
  "host": "test-prod.example.com",
  "port": 22,
  "user": "deploy",
  "key": "~/.ssh/test_key",
  "deploy_path": "/opt/nself",
  "description": "Test production server"
}
EOF

  # Create test .env files
  cat > .environments/test-staging/.env <<EOF
ENV=staging
BASE_DOMAIN=test-staging.example.com
EOF

  cat > .environments/test-prod/.env <<EOF
ENV=production
BASE_DOMAIN=test-prod.example.com
EOF

  printf "${GREEN}✓ Test environment setup complete${NC}\n"
}

# Cleanup test environment
cleanup_test_env() {
  printf "\n${YELLOW}Cleaning up test environment...${NC}\n"

  rm -rf .environments/test-staging
  rm -rf .environments/test-prod

  printf "${GREEN}✓ Cleanup complete${NC}\n"
}

# Test 1: Server list shows all configured servers
test_server_list() {
  test_start "Server List Command"

  local output
  output=$(bash "$ROOT_DIR/src/cli/deploy.sh" server list 2>&1)

  if echo "$output" | grep -q "Server List"; then
    test_pass "Shows server list header"
  else
    test_fail "Missing server list header"
  fi

  if echo "$output" | grep -q "test-staging"; then
    test_pass "Lists test-staging environment"
  else
    test_fail "Missing test-staging in list"
  fi

  if echo "$output" | grep -q "test-prod"; then
    test_pass "Lists test-prod environment"
  else
    test_fail "Missing test-prod in list"
  fi
}

# Test 2: Server add creates proper configuration
test_server_add() {
  test_start "Server Add Command"

  # Add a new server
  bash "$ROOT_DIR/src/cli/deploy.sh" server add test-new \
    --host new.example.com \
    --user deploy \
    --port 2222 >/dev/null 2>&1 || true

  if [[ -f .environments/test-new/server.json ]]; then
    test_pass "Created server.json file"

    # Check content
    if grep -q "new.example.com" .environments/test-new/server.json; then
      test_pass "Correct host in server.json"
    else
      test_fail "Incorrect host in server.json"
    fi

    if grep -q '"user": "deploy"' .environments/test-new/server.json; then
      test_pass "Correct user in server.json"
    else
      test_fail "Incorrect user in server.json"
    fi

    if grep -q '"port": 2222' .environments/test-new/server.json; then
      test_pass "Correct port in server.json"
    else
      test_fail "Incorrect port in server.json"
    fi
  else
    test_fail "Failed to create server.json"
  fi

  # Cleanup
  rm -rf .environments/test-new
}

# Test 3: Server remove deletes configuration
test_server_remove() {
  test_start "Server Remove Command"

  # Create a temporary server
  mkdir -p .environments/test-temp
  cat > .environments/test-temp/server.json <<EOF
{
  "name": "test-temp",
  "host": "temp.example.com",
  "user": "root",
  "port": 22
}
EOF

  # Remove it (with force)
  echo "y" | bash "$ROOT_DIR/src/cli/deploy.sh" server remove test-temp >/dev/null 2>&1 || true

  if [[ ! -f .environments/test-temp/server.json ]]; then
    test_pass "Removed server.json file"
  else
    test_fail "Failed to remove server.json"
  fi

  if [[ -d .environments/test-temp ]]; then
    test_pass "Preserved environment directory"
  else
    test_fail "Incorrectly removed environment directory"
  fi

  # Cleanup
  rm -rf .environments/test-temp
}

# Test 4: Server info displays configuration
test_server_info() {
  test_start "Server Info Command"

  local output
  output=$(bash "$ROOT_DIR/src/cli/deploy.sh" server info test-staging 2>&1 || true)

  if echo "$output" | grep -q "Server Details"; then
    test_pass "Shows server details header"
  else
    test_fail "Missing server details header"
  fi

  if echo "$output" | grep -q "test-staging.example.com"; then
    test_pass "Displays correct hostname"
  else
    test_fail "Missing or incorrect hostname"
  fi

  if echo "$output" | grep -q "Connection Details"; then
    test_pass "Shows connection details section"
  else
    test_fail "Missing connection details section"
  fi
}

# Test 5: Server status shows all servers
test_server_status() {
  test_start "Server Status Command"

  local output
  output=$(bash "$ROOT_DIR/src/cli/deploy.sh" server status 2>&1)

  if echo "$output" | grep -q "Check server connectivity"; then
    test_pass "Shows status header"
  else
    test_fail "Missing status header"
  fi

  # Should show servers or "no servers configured"
  if echo "$output" | grep -qE "test-staging|test-prod|No remote servers"; then
    test_pass "Shows server status or appropriate message"
  else
    test_fail "Missing server status information"
  fi
}

# Test 6: Sync status shows sync information
test_sync_status() {
  test_start "Sync Status Command"

  local output
  output=$(bash "$ROOT_DIR/src/cli/deploy.sh" sync status 2>&1)

  if echo "$output" | grep -q "Synchronization Status"; then
    test_pass "Shows sync status header"
  else
    test_fail "Missing sync status header"
  fi

  if echo "$output" | grep -qE "ENVIRONMENT|STATUS"; then
    test_pass "Shows status table headers"
  else
    test_fail "Missing status table headers"
  fi
}

# Test 7: Help commands work
test_help_commands() {
  test_start "Help Commands"

  # Main deploy help
  if bash "$ROOT_DIR/src/cli/deploy.sh" --help 2>&1 | grep -q "Remote Server Management"; then
    test_pass "Main deploy help works"
  else
    test_fail "Main deploy help missing server management"
  fi

  # Server help
  if bash "$ROOT_DIR/src/cli/deploy.sh" server --help 2>&1 | grep -q "VPS server management"; then
    test_pass "Server help works"
  else
    test_fail "Server help missing or incorrect"
  fi

  # Sync help
  if bash "$ROOT_DIR/src/cli/deploy.sh" sync --help 2>&1 | grep -q "Environment synchronization"; then
    test_pass "Sync help works"
  else
    test_fail "Sync help missing or incorrect"
  fi
}

# Test 8: Error handling for missing arguments
test_error_handling() {
  test_start "Error Handling"

  # Server add without host
  if bash "$ROOT_DIR/src/cli/deploy.sh" server add test 2>&1 | grep -q "Host is required"; then
    test_pass "Server add requires host"
  else
    test_fail "Missing error for server add without host"
  fi

  # Server info without name
  if bash "$ROOT_DIR/src/cli/deploy.sh" server info 2>&1 | grep -q "Server name required"; then
    test_pass "Server info requires name"
  else
    test_fail "Missing error for server info without name"
  fi

  # Server remove without name
  if bash "$ROOT_DIR/src/cli/deploy.sh" server remove 2>&1 | grep -q "Server name required"; then
    test_pass "Server remove requires name"
  else
    test_fail "Missing error for server remove without name"
  fi
}

# Test 9: Subcommand routing works
test_subcommand_routing() {
  test_start "Subcommand Routing"

  # Test that subcommands are recognized
  local commands=("list" "add" "remove" "ssh" "info" "status" "diagnose" "check" "init")

  for cmd in "${commands[@]}"; do
    if bash "$ROOT_DIR/src/cli/deploy.sh" server "$cmd" --help 2>&1 | grep -qE "Usage:|required"; then
      test_pass "Command 'server $cmd' is recognized"
    else
      # Some commands may not have --help, check for error instead
      if bash "$ROOT_DIR/src/cli/deploy.sh" server "$cmd" 2>&1 | grep -qE "required|not found|Usage"; then
        test_pass "Command 'server $cmd' is recognized (error output)"
      else
        test_fail "Command 'server $cmd' not recognized"
      fi
    fi
  done
}

# Test 10: Sync command routing works
test_sync_routing() {
  test_start "Sync Command Routing"

  local commands=("pull" "push" "status" "full")

  for cmd in "${commands[@]}"; do
    # These commands require environment argument, so expect error
    if bash "$ROOT_DIR/src/cli/deploy.sh" sync "$cmd" 2>&1 | grep -qE "staging|production|Environment|not found|Synchronization"; then
      test_pass "Command 'sync $cmd' is recognized"
    else
      test_fail "Command 'sync $cmd' not recognized"
    fi
  done
}

# Main test execution
main() {
  printf "\n╔════════════════════════════════════════════════════════════════╗\n"
  printf "║  Deploy Server Management - Feature Tests                     ║\n"
  printf "╚════════════════════════════════════════════════════════════════╝\n"

  setup_test_env

  # Run all tests
  test_server_list
  test_server_add
  test_server_remove
  test_server_info
  test_server_status
  test_sync_status
  test_help_commands
  test_error_handling
  test_subcommand_routing
  test_sync_routing

  cleanup_test_env

  # Summary
  printf "\n╔════════════════════════════════════════════════════════════════╗\n"
  printf "║  Test Summary                                                  ║\n"
  printf "╚════════════════════════════════════════════════════════════════╝\n\n"

  printf "  Tests Run:    %d\n" "$TESTS_RUN"
  printf "  ${GREEN}Tests Passed: %d${NC}\n" "$TESTS_PASSED"
  printf "  ${RED}Tests Failed: %d${NC}\n" "$TESTS_FAILED"

  if [[ $TESTS_FAILED -eq 0 ]]; then
    printf "\n${GREEN}✓ All tests passed!${NC}\n\n"
    exit 0
  else
    printf "\n${RED}✗ Some tests failed${NC}\n\n"
    exit 1
  fi
}

# Run tests
main
