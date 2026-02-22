#!/usr/bin/env bats
# Test suite for nself plugin system
# Tests plugin installation, activation, deactivation, hooks, and dependencies
# Coverage target: 90%+

# ============================================================================
# Setup & Teardown
# ============================================================================

setup() {
  # Create temporary test environment
  export TEST_DIR=$(mktemp -d)
  export PLUGIN_DIR="$TEST_DIR/.nself/plugins"
  export PLUGIN_CACHE_DIR="$TEST_DIR/.nself/cache/plugins"
  export NSELF_PLUGIN_DIR="$PLUGIN_DIR"
  export NSELF_PLUGIN_CACHE="$PLUGIN_CACHE_DIR"

  mkdir -p "$PLUGIN_DIR"
  mkdir -p "$PLUGIN_CACHE_DIR"

  # Source plugin core functions
  CORE_LIB="$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)/lib/plugin/core.sh"
  REGISTRY_LIB="$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)/lib/plugin/registry.sh"

  if [[ -f "$CORE_LIB" ]]; then
    source "$CORE_LIB"
  fi

  if [[ -f "$REGISTRY_LIB" ]]; then
    source "$REGISTRY_LIB"
  fi

  # Create mock VERSION file
  export VERSION_FILE="$TEST_DIR/VERSION"
  printf "0.9.0" > "$VERSION_FILE"
}

teardown() {
  # Clean up test directory
  rm -rf "$TEST_DIR"
}

# ============================================================================
# Plugin Information Tests
# ============================================================================

@test "plugin_get_info reads manifest fields correctly" {
  # Create test plugin manifest
  local plugin_name="test-plugin"
  mkdir -p "$PLUGIN_DIR/$plugin_name"

  cat > "$PLUGIN_DIR/$plugin_name/plugin.json" <<EOF
{
  "name": "test-plugin",
  "version": "1.0.0",
  "description": "Test plugin",
  "author": "Test Author"
}
EOF

  run plugin_get_info "$plugin_name" "version"
  [ "$status" -eq 0 ]
  [ "$output" = "1.0.0" ]

  run plugin_get_info "$plugin_name" "author"
  [ "$status" -eq 0 ]
  [ "$output" = "Test Author" ]
}

@test "plugin_get_info fails for non-existent plugin" {
  run plugin_get_info "nonexistent-plugin" "version"
  [ "$status" -eq 1 ]
}

@test "plugin_get_info fails for missing manifest" {
  mkdir -p "$PLUGIN_DIR/incomplete-plugin"

  run plugin_get_info "incomplete-plugin" "version"
  [ "$status" -eq 1 ]
}

@test "plugin_list_installed returns empty for no plugins" {
  run plugin_list_installed
  [ "$status" -eq 0 ]
  [ -z "$output" ]
}

@test "plugin_list_installed finds installed plugins" {
  # Create multiple test plugins
  mkdir -p "$PLUGIN_DIR/plugin-one"
  mkdir -p "$PLUGIN_DIR/plugin-two"
  mkdir -p "$PLUGIN_DIR/plugin-three"

  printf '{"name":"plugin-one"}' > "$PLUGIN_DIR/plugin-one/plugin.json"
  printf '{"name":"plugin-two"}' > "$PLUGIN_DIR/plugin-two/plugin.json"
  printf '{"name":"plugin-three"}' > "$PLUGIN_DIR/plugin-three/plugin.json"

  run plugin_list_installed
  [ "$status" -eq 0 ]
  [[ "$output" == *"plugin-one"* ]]
  [[ "$output" == *"plugin-two"* ]]
  [[ "$output" == *"plugin-three"* ]]
}

@test "plugin_list_installed ignores directories without manifest" {
  mkdir -p "$PLUGIN_DIR/incomplete-plugin"
  mkdir -p "$PLUGIN_DIR/valid-plugin"

  printf '{"name":"valid-plugin"}' > "$PLUGIN_DIR/valid-plugin/plugin.json"

  run plugin_list_installed
  [ "$status" -eq 0 ]
  [[ "$output" == *"valid-plugin"* ]]
  [[ "$output" != *"incomplete-plugin"* ]]
}

# ============================================================================
# Version Comparison Tests
# ============================================================================

@test "plugin_version_compare: 1.0.0 >= 1.0.0" {
  run plugin_version_compare "1.0.0" "1.0.0"
  [ "$status" -eq 0 ]
}

@test "plugin_version_compare: 1.1.0 >= 1.0.0" {
  run plugin_version_compare "1.1.0" "1.0.0"
  [ "$status" -eq 0 ]
}

@test "plugin_version_compare: 2.0.0 >= 1.9.9" {
  run plugin_version_compare "2.0.0" "1.9.9"
  [ "$status" -eq 0 ]
}

@test "plugin_version_compare: 1.0.0 < 1.1.0" {
  run plugin_version_compare "1.0.0" "1.1.0"
  [ "$status" -eq 1 ]
}

@test "plugin_version_compare: 1.0.0 < 2.0.0" {
  run plugin_version_compare "1.0.0" "2.0.0"
  [ "$status" -eq 1 ]
}

@test "plugin_version_compare handles v prefix" {
  run plugin_version_compare "v1.0.0" "v1.0.0"
  [ "$status" -eq 0 ]

  run plugin_version_compare "v2.0.0" "1.0.0"
  [ "$status" -eq 0 ]
}

@test "plugin_version_compare handles missing patch version" {
  run plugin_version_compare "1.0" "1.0.0"
  [ "$status" -eq 0 ]
}

@test "plugin_version_compare: 1.10.0 > 1.9.0" {
  run plugin_version_compare "1.10.0" "1.9.0"
  [ "$status" -eq 0 ]
}

# ============================================================================
# Compatibility Checking Tests
# ============================================================================

@test "plugin_check_compatibility succeeds for compatible version" {
  local plugin_name="test-plugin"
  mkdir -p "$PLUGIN_DIR/$plugin_name"

  cat > "$PLUGIN_DIR/$plugin_name/plugin.json" <<EOF
{
  "name": "test-plugin",
  "version": "1.0.0",
  "minNselfVersion": "0.8.0"
}
EOF

  # Mock current version
  export NSELF_VERSION="0.9.0"

  run plugin_check_compatibility "$plugin_name"
  [ "$status" -eq 0 ]
}

@test "plugin_check_compatibility fails for incompatible version" {
  local plugin_name="test-plugin"
  mkdir -p "$PLUGIN_DIR/$plugin_name"

  cat > "$PLUGIN_DIR/$plugin_name/plugin.json" <<EOF
{
  "name": "test-plugin",
  "version": "1.0.0",
  "minNselfVersion": "1.0.0"
}
EOF

  # Mock current version
  export NSELF_VERSION="0.9.0"

  run plugin_check_compatibility "$plugin_name"
  [ "$status" -eq 1 ]
}

@test "plugin_check_compatibility succeeds when minNselfVersion not specified" {
  local plugin_name="test-plugin"
  mkdir -p "$PLUGIN_DIR/$plugin_name"

  cat > "$PLUGIN_DIR/$plugin_name/plugin.json" <<EOF
{
  "name": "test-plugin",
  "version": "1.0.0"
}
EOF

  run plugin_check_compatibility "$plugin_name"
  [ "$status" -eq 0 ]
}

# ============================================================================
# Plugin Hooks Tests
# ============================================================================

@test "plugin_pre_install checks compatibility" {
  local plugin_name="test-plugin"
  mkdir -p "$PLUGIN_DIR/$plugin_name"

  cat > "$PLUGIN_DIR/$plugin_name/plugin.json" <<EOF
{
  "name": "test-plugin",
  "version": "1.0.0",
  "minNselfVersion": "0.8.0"
}
EOF

  run plugin_pre_install "$plugin_name"
  [ "$status" -eq 0 ]
}

@test "plugin_pre_install fails for incompatible plugin" {
  skip "Requires log_error function to be available"

  local plugin_name="test-plugin"
  mkdir -p "$PLUGIN_DIR/$plugin_name"

  cat > "$PLUGIN_DIR/$plugin_name/plugin.json" <<EOF
{
  "name": "test-plugin",
  "version": "1.0.0",
  "minNselfVersion": "10.0.0"
}
EOF

  run plugin_pre_install "$plugin_name"
  [ "$status" -eq 1 ]
}

@test "plugin_post_install verifies plugin.json exists" {
  skip "Requires log_error function to be available"

  local plugin_name="test-plugin"
  mkdir -p "$PLUGIN_DIR/$plugin_name"

  # No plugin.json created - should fail
  run plugin_post_install "$plugin_name"
  [ "$status" -eq 1 ]

  # Create plugin.json - should succeed
  printf '{"name":"test-plugin"}' > "$PLUGIN_DIR/$plugin_name/plugin.json"
  run plugin_post_install "$plugin_name"
  [ "$status" -eq 0 ]
}

# ============================================================================
# Plugin Environment Tests
# ============================================================================

@test "plugin_load_env sources project .env if exists" {
  cd "$TEST_DIR"

  cat > .env <<EOF
TEST_VAR=test_value
PLUGIN_ENV=loaded
EOF

  plugin_load_env "test-plugin"

  [ "$TEST_VAR" = "test_value" ]
  [ "$PLUGIN_ENV" = "loaded" ]
}

@test "plugin_load_env succeeds when .env missing" {
  cd "$TEST_DIR"

  run plugin_load_env "test-plugin"
  [ "$status" -eq 0 ]
}

@test "plugin_check_env validates required variables" {
  skip "Requires log_warning function to be available"

  local plugin_name="test-plugin"
  mkdir -p "$PLUGIN_DIR/$plugin_name"

  cat > "$PLUGIN_DIR/$plugin_name/plugin.json" <<EOF
{
  "name": "test-plugin",
  "environment": {
    "required": ["DATABASE_URL", "API_KEY"]
  }
}
EOF

  # Missing required vars - should fail
  run plugin_check_env "$plugin_name"
  [ "$status" -ne 0 ]

  # Set required vars - should succeed
  export DATABASE_URL="postgresql://localhost"
  export API_KEY="test-key"

  run plugin_check_env "$plugin_name"
  [ "$status" -eq 0 ]
}

# ============================================================================
# Registry Tests
# ============================================================================

@test "registry_init_cache creates cache directory" {
  rm -rf "$PLUGIN_CACHE_DIR"

  run registry_init_cache
  [ "$status" -eq 0 ]
  [ -d "$PLUGIN_CACHE_DIR" ]
}

@test "registry_cache_is_fresh returns false for missing cache" {
  run registry_cache_is_fresh "$PLUGIN_CACHE_DIR/nonexistent.json"
  [ "$status" -eq 1 ]
}

@test "registry_cache_is_fresh returns true for fresh cache" {
  local cache_file="$PLUGIN_CACHE_DIR/test-cache.json"
  mkdir -p "$PLUGIN_CACHE_DIR"
  printf '{}' > "$cache_file"

  # Cache should be fresh (within TTL)
  run registry_cache_is_fresh "$cache_file" 300
  [ "$status" -eq 0 ]
}

@test "registry_cache_is_fresh returns false for stale cache" {
  local cache_file="$PLUGIN_CACHE_DIR/test-cache.json"
  mkdir -p "$PLUGIN_CACHE_DIR"
  printf '{}' > "$cache_file"

  # Cache should be stale (TTL = 0)
  run registry_cache_is_fresh "$cache_file" 0
  [ "$status" -eq 1 ]
}

@test "registry_list_installed returns plugin names" {
  # Create test plugins
  mkdir -p "$PLUGIN_DIR/plugin-alpha"
  mkdir -p "$PLUGIN_DIR/plugin-beta"
  mkdir -p "$PLUGIN_DIR/_shared"  # Should be excluded

  printf '{"name":"plugin-alpha"}' > "$PLUGIN_DIR/plugin-alpha/plugin.json"
  printf '{"name":"plugin-beta"}' > "$PLUGIN_DIR/plugin-beta/plugin.json"
  printf '{"name":"_shared"}' > "$PLUGIN_DIR/_shared/plugin.json"

  run registry_list_installed
  [ "$status" -eq 0 ]
  [[ "$output" == *"plugin-alpha"* ]]
  [[ "$output" == *"plugin-beta"* ]]
  [[ "$output" != *"_shared"* ]]
}

@test "registry_list_installed handles empty plugin directory" {
  run registry_list_installed
  [ "$status" -eq 0 ]
}

@test "registry_get_installed_version reads version from manifest" {
  if ! command -v jq >/dev/null 2>&1; then
    skip "jq not available"
  fi

  local plugin_name="test-plugin"
  mkdir -p "$PLUGIN_DIR/$plugin_name"

  cat > "$PLUGIN_DIR/$plugin_name/plugin.json" <<EOF
{
  "name": "test-plugin",
  "version": "2.5.3"
}
EOF

  run registry_get_installed_version "$plugin_name"
  [ "$status" -eq 0 ]
  [ "$output" = "2.5.3" ]
}

@test "registry_get_installed_version fails for missing plugin" {
  run registry_get_installed_version "nonexistent-plugin"
  [ "$status" -eq 1 ]
}

# ============================================================================
# Checksum Verification Tests
# ============================================================================

@test "registry_verify_checksum succeeds with no checksum" {
  local test_file="$TEST_DIR/test.txt"
  printf "test content" > "$test_file"

  run registry_verify_checksum "$test_file" ""
  [ "$status" -eq 0 ]
}

@test "registry_verify_checksum validates sha256" {
  if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
    skip "sha256sum or shasum not available"
  fi

  local test_file="$TEST_DIR/test.txt"
  printf "test content" > "$test_file"

  # Calculate correct checksum
  local checksum
  if command -v sha256sum >/dev/null 2>&1; then
    checksum=$(sha256sum "$test_file" | cut -d' ' -f1)
  else
    checksum=$(shasum -a 256 "$test_file" | cut -d' ' -f1)
  fi

  run registry_verify_checksum "$test_file" "sha256:$checksum"
  [ "$status" -eq 0 ]
}

@test "registry_verify_checksum fails for wrong checksum" {
  if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
    skip "sha256sum or shasum not available"
  fi

  local test_file="$TEST_DIR/test.txt"
  printf "test content" > "$test_file"

  run registry_verify_checksum "$test_file" "sha256:0000000000000000000000000000000000000000000000000000000000000000"
  [ "$status" -eq 1 ]
  [[ "$output" == *"Checksum mismatch"* ]]
}

@test "registry_verify_checksum handles md5 algorithm" {
  if ! command -v md5sum >/dev/null 2>&1 && ! command -v md5 >/dev/null 2>&1; then
    skip "md5sum or md5 not available"
  fi

  local test_file="$TEST_DIR/test.txt"
  printf "test content" > "$test_file"

  # Calculate correct checksum
  local checksum
  if command -v md5sum >/dev/null 2>&1; then
    checksum=$(md5sum "$test_file" | cut -d' ' -f1)
  else
    checksum=$(md5 -q "$test_file")
  fi

  run registry_verify_checksum "$test_file" "md5:$checksum"
  [ "$status" -eq 0 ]
}

# ============================================================================
# Database Operations Tests (Mock-based)
# ============================================================================

@test "plugin_db_connection returns correct connection string" {
  export POSTGRES_HOST="testhost"
  export POSTGRES_PORT="5433"
  export POSTGRES_DB="testdb"
  export POSTGRES_USER="testuser"
  export POSTGRES_PASSWORD="testpass"

  run plugin_db_connection
  [ "$status" -eq 0 ]
  [ "$output" = "postgresql://testuser:testpass@testhost:5433/testdb" ]
}

@test "plugin_db_connection uses defaults when vars not set" {
  unset POSTGRES_HOST POSTGRES_PORT POSTGRES_DB POSTGRES_USER POSTGRES_PASSWORD

  run plugin_db_connection
  [ "$status" -eq 0 ]
  [[ "$output" == *"localhost"* ]]
  [[ "$output" == *"5432"* ]]
}

# ============================================================================
# Registry Version Comparison Tests
# ============================================================================

@test "registry_version_compare: same versions" {
  run registry_version_compare "1.0.0" "1.0.0"
  [ "$status" -eq 0 ]
}

@test "registry_version_compare: major version difference" {
  run registry_version_compare "2.0.0" "1.0.0"
  [ "$status" -eq 0 ]

  run registry_version_compare "1.0.0" "2.0.0"
  [ "$status" -eq 1 ]
}

@test "registry_version_compare: minor version difference" {
  run registry_version_compare "1.5.0" "1.4.0"
  [ "$status" -eq 0 ]

  run registry_version_compare "1.4.0" "1.5.0"
  [ "$status" -eq 1 ]
}

@test "registry_version_compare: patch version difference" {
  run registry_version_compare "1.0.10" "1.0.9"
  [ "$status" -eq 0 ]

  run registry_version_compare "1.0.9" "1.0.10"
  [ "$status" -eq 1 ]
}

@test "registry_version_compare: handles v prefix correctly" {
  run registry_version_compare "v1.0.0" "v1.0.0"
  [ "$status" -eq 0 ]

  run registry_version_compare "v2.0.0" "v1.9.9"
  [ "$status" -eq 0 ]
}

# ============================================================================
# Edge Cases & Error Handling Tests
# ============================================================================

@test "plugin_get_info handles special characters in field values" {
  local plugin_name="test-plugin"
  mkdir -p "$PLUGIN_DIR/$plugin_name"

  cat > "$PLUGIN_DIR/$plugin_name/plugin.json" <<EOF
{
  "name": "test-plugin",
  "description": "Plugin with special chars: !@#$%"
}
EOF

  run plugin_get_info "$plugin_name" "description"
  [ "$status" -eq 0 ]
  [[ "$output" == *"special chars"* ]]
}

@test "plugin_version_compare handles empty versions" {
  run plugin_version_compare "" "1.0.0"
  [ "$status" -eq 1 ]

  run plugin_version_compare "1.0.0" ""
  [ "$status" -eq 0 ]
}

@test "plugin_list_installed handles permission denied" {
  # This test checks resilience when directories aren't readable
  mkdir -p "$PLUGIN_DIR/restricted-plugin"
  printf '{"name":"restricted"}' > "$PLUGIN_DIR/restricted-plugin/plugin.json"
  chmod 000 "$PLUGIN_DIR/restricted-plugin"

  run plugin_list_installed
  # Should not crash, even if one directory is unreadable
  [ "$status" -eq 0 ]

  # Cleanup
  chmod 755 "$PLUGIN_DIR/restricted-plugin"
}

# ============================================================================
# Integration Tests
# ============================================================================

@test "end-to-end: create plugin manifest and read info" {
  local plugin_name="integration-test"
  mkdir -p "$PLUGIN_DIR/$plugin_name"

  cat > "$PLUGIN_DIR/$plugin_name/plugin.json" <<EOF
{
  "name": "integration-test",
  "version": "1.0.0",
  "description": "Integration test plugin",
  "author": "Test Suite",
  "minNselfVersion": "0.8.0"
}
EOF

  # Test getting various fields
  run plugin_get_info "$plugin_name" "version"
  [ "$status" -eq 0 ]
  [ "$output" = "1.0.0" ]

  run plugin_get_info "$plugin_name" "author"
  [ "$status" -eq 0 ]
  [ "$output" = "Test Suite" ]

  # Test compatibility check
  export NSELF_VERSION="0.9.0"
  run plugin_check_compatibility "$plugin_name"
  [ "$status" -eq 0 ]

  # Test listing
  run plugin_list_installed
  [ "$status" -eq 0 ]
  [[ "$output" == *"integration-test"* ]]
}

@test "end-to-end: plugin lifecycle with version checks" {
  if ! command -v jq >/dev/null 2>&1; then
    skip "jq not available"
  fi

  local plugin_name="lifecycle-test"
  mkdir -p "$PLUGIN_DIR/$plugin_name"

  # Create plugin v1.0.0
  cat > "$PLUGIN_DIR/$plugin_name/plugin.json" <<EOF
{
  "name": "lifecycle-test",
  "version": "1.0.0"
}
EOF

  # Get version
  run registry_get_installed_version "$plugin_name"
  [ "$status" -eq 0 ]
  [ "$output" = "1.0.0" ]

  # Upgrade to v2.0.0
  cat > "$PLUGIN_DIR/$plugin_name/plugin.json" <<EOF
{
  "name": "lifecycle-test",
  "version": "2.0.0"
}
EOF

  # Verify version updated
  run registry_get_installed_version "$plugin_name"
  [ "$status" -eq 0 ]
  [ "$output" = "2.0.0" ]

  # Check if v2.0.0 > v1.0.0
  run registry_version_compare "2.0.0" "1.0.0"
  [ "$status" -eq 0 ]
}

# ============================================================================
# Performance & Stress Tests
# ============================================================================

@test "performance: list_installed handles many plugins efficiently" {
  # Create 50 test plugins
  for i in {1..50}; do
    mkdir -p "$PLUGIN_DIR/plugin-$i"
    printf '{"name":"plugin-%s"}' "$i" > "$PLUGIN_DIR/plugin-$i/plugin.json"
  done

  run plugin_list_installed
  [ "$status" -eq 0 ]

  # Verify we got all plugins
  # Use echo (adds trailing newline) so wc -l counts correctly vs printf which strips it
  local count
  count=$(echo "$output" | wc -l | tr -d ' ')
  [ "$count" -eq 50 ]
}

@test "stress: version_compare with many comparisons" {
  # Run 100 version comparisons
  for i in {1..100}; do
    registry_version_compare "1.$i.0" "1.0.0" >/dev/null
  done

  # If we got here without crashing, test passes
  [ "$?" -eq 0 ]
}
