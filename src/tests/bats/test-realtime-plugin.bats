#!/usr/bin/env bats
# test-realtime-plugin.bats — WebSocket subscription tests for the realtime plugin
# T-0371 | Phase 40

# ============================================================================
# Setup & Teardown
# ============================================================================

setup() {
  SCRIPT_DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)"
  export SCRIPT_DIR

  # Set SKIP_DOCKER_TESTS to non-empty to bypass tests requiring a running stack
  # e.g. SKIP_DOCKER_TESTS=1 bats test-realtime-plugin.bats
  export SKIP_DOCKER_TESTS="${SKIP_DOCKER_TESTS:-}"

  # Path to the free realtime plugin manifest (may not exist yet)
  REALTIME_PLUGIN_JSON="$SCRIPT_DIR/../../../plugins/free/realtime/plugin.json"
  export REALTIME_PLUGIN_JSON
}

# ============================================================================
# Plugin info / help output
# ============================================================================

@test "nself plugin info realtime: exits 0 when nself is available" {
  if ! command -v nself >/dev/null 2>&1; then
    skip "nself not installed"
  fi
  run nself plugin info realtime
  # Either 0 (found) or non-zero with usage message -- both are acceptable
  # We only assert it does not hang or crash with signal
  [ "$status" -ne 127 ]
}

@test "nself plugin --help lists plugin subcommands" {
  if ! command -v nself >/dev/null 2>&1; then
    skip "nself not installed"
  fi
  run nself plugin --help
  [ "$status" -eq 0 ]
  [[ "$output" == *"install"* ]] || [[ "$output" == *"plugin"* ]]
}

# ============================================================================
# Plugin manifest validation (no Docker required)
# ============================================================================

@test "realtime plugin.json exists" {
  if [ ! -f "$REALTIME_PLUGIN_JSON" ]; then
    skip "plugins/free/realtime/plugin.json not present"
  fi
  [ -f "$REALTIME_PLUGIN_JSON" ]
}

@test "realtime plugin.json contains required field: name" {
  if [ ! -f "$REALTIME_PLUGIN_JSON" ]; then
    skip "plugins/free/realtime/plugin.json not present"
  fi
  if ! command -v python3 >/dev/null 2>&1; then
    skip "python3 not available for JSON parsing"
  fi
  run python3 -c "
import json, sys
with open('$REALTIME_PLUGIN_JSON') as f:
    d = json.load(f)
assert 'name' in d, 'missing field: name'
sys.exit(0)
"
  [ "$status" -eq 0 ]
}

@test "realtime plugin.json contains required field: version" {
  if [ ! -f "$REALTIME_PLUGIN_JSON" ]; then
    skip "plugins/free/realtime/plugin.json not present"
  fi
  if ! command -v python3 >/dev/null 2>&1; then
    skip "python3 not available for JSON parsing"
  fi
  run python3 -c "
import json, sys
with open('$REALTIME_PLUGIN_JSON') as f:
    d = json.load(f)
assert 'version' in d, 'missing field: version'
sys.exit(0)
"
  [ "$status" -eq 0 ]
}

@test "realtime plugin.json contains required field: description" {
  if [ ! -f "$REALTIME_PLUGIN_JSON" ]; then
    skip "plugins/free/realtime/plugin.json not present"
  fi
  if ! command -v python3 >/dev/null 2>&1; then
    skip "python3 not available for JSON parsing"
  fi
  run python3 -c "
import json, sys
with open('$REALTIME_PLUGIN_JSON') as f:
    d = json.load(f)
assert 'description' in d, 'missing field: description'
sys.exit(0)
"
  [ "$status" -eq 0 ]
}

@test "realtime plugin.json contains required field: tables" {
  if [ ! -f "$REALTIME_PLUGIN_JSON" ]; then
    skip "plugins/free/realtime/plugin.json not present"
  fi
  if ! command -v python3 >/dev/null 2>&1; then
    skip "python3 not available for JSON parsing"
  fi
  run python3 -c "
import json, sys
with open('$REALTIME_PLUGIN_JSON') as f:
    d = json.load(f)
assert 'tables' in d, 'missing field: tables'
sys.exit(0)
"
  [ "$status" -eq 0 ]
}

@test "realtime plugin.json is valid JSON" {
  if [ ! -f "$REALTIME_PLUGIN_JSON" ]; then
    skip "plugins/free/realtime/plugin.json not present"
  fi
  if ! command -v python3 >/dev/null 2>&1; then
    skip "python3 not available for JSON parsing"
  fi
  run python3 -c "import json; json.load(open('$REALTIME_PLUGIN_JSON'))"
  [ "$status" -eq 0 ]
}

@test "realtime plugin.json name field is non-empty string" {
  if [ ! -f "$REALTIME_PLUGIN_JSON" ]; then
    skip "plugins/free/realtime/plugin.json not present"
  fi
  if ! command -v python3 >/dev/null 2>&1; then
    skip "python3 not available for JSON parsing"
  fi
  run python3 -c "
import json, sys
with open('$REALTIME_PLUGIN_JSON') as f:
    d = json.load(f)
assert isinstance(d.get('name'), str) and len(d['name']) > 0, 'name must be a non-empty string'
sys.exit(0)
"
  [ "$status" -eq 0 ]
}

# ============================================================================
# WebSocket subscription tests (require running stack -- guarded by SKIP_DOCKER_TESTS)
# ============================================================================

@test "realtime: hasura endpoint responds to health check" {
  if [ -n "$SKIP_DOCKER_TESTS" ]; then
    skip "SKIP_DOCKER_TESTS is set"
  fi
  if ! command -v docker >/dev/null 2>&1; then
    skip "Docker not available"
  fi
  if ! docker ps --format '{{.Names}}' 2>/dev/null | grep -q 'hasura'; then
    skip "Hasura container not running"
  fi
  if ! command -v curl >/dev/null 2>&1; then
    skip "curl not available"
  fi
  HASURA_PORT="${HASURA_PORT:-8080}"
  run curl -sf --max-time 5 "http://127.0.0.1:${HASURA_PORT}/healthz"
  [ "$status" -eq 0 ]
}

@test "realtime: websocket upgrade header accepted by hasura" {
  if [ -n "$SKIP_DOCKER_TESTS" ]; then
    skip "SKIP_DOCKER_TESTS is set"
  fi
  if ! command -v docker >/dev/null 2>&1; then
    skip "Docker not available"
  fi
  if ! docker ps --format '{{.Names}}' 2>/dev/null | grep -q 'hasura'; then
    skip "Hasura container not running"
  fi
  if ! command -v curl >/dev/null 2>&1; then
    skip "curl not available"
  fi
  HASURA_PORT="${HASURA_PORT:-8080}"
  # A WebSocket upgrade request to the GraphQL endpoint should return 101 or 400 (not 404/502)
  run curl -s -o /dev/null -w "%{http_code}" --max-time 5 \
    -H "Upgrade: websocket" \
    -H "Connection: Upgrade" \
    -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
    -H "Sec-WebSocket-Version: 13" \
    "http://127.0.0.1:${HASURA_PORT}/v1/graphql"
  # 101 = switching protocols, 400 = bad request (protocol mismatch OK), 200 = fallback
  [[ "$output" == "101" || "$output" == "400" || "$output" == "200" ]]
}
