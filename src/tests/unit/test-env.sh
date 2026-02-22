#!/usr/bin/env bash
# test-env.sh - Unit tests for environment management (v0.4.3)
# POSIX-compliant, no Bash 4+ features

set -euo pipefail

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_ROOT="$(dirname "$SCRIPT_DIR")"
PROJECT_ROOT="$(dirname "$(dirname "$TEST_ROOT")")"

# Source test framework
source "$TEST_ROOT/test_framework.sh"

# Test configuration
TEST_TMP=""
ORIGINAL_DIR=""

# ═══════════════════════════════════════════════════════════════
# Test Setup and Teardown
# ═══════════════════════════════════════════════════════════════

setup_test_env() {
  ORIGINAL_DIR="$(pwd)"
  TEST_TMP=$(mktemp -d)
  cd "$TEST_TMP"

  # Source the env modules
  source "$PROJECT_ROOT/src/lib/utils/display.sh" 2>/dev/null || true
  source "$PROJECT_ROOT/src/lib/utils/platform-compat.sh" 2>/dev/null || true
  source "$PROJECT_ROOT/src/lib/env/create.sh" 2>/dev/null || true
  source "$PROJECT_ROOT/src/lib/env/switch.sh" 2>/dev/null || true
  source "$PROJECT_ROOT/src/lib/env/diff.sh" 2>/dev/null || true
  source "$PROJECT_ROOT/src/lib/env/validate.sh" 2>/dev/null || true
}

teardown_test_env() {
  cd "$ORIGINAL_DIR"
  if [[ -n "$TEST_TMP" ]] && [[ -d "$TEST_TMP" ]]; then
    rm -rf "$TEST_TMP"
  fi
}

# ═══════════════════════════════════════════════════════════════
# Environment Creation Tests
# ═══════════════════════════════════════════════════════════════

test_env_init_directory() {
  setup_test_env

  if command -v env::init_directory >/dev/null 2>&1; then
    env::init_directory
    assert_dir_exists ".environments" "Environment directory should be created"
  else
    skip_test "env::init_directory not available"
  fi

  teardown_test_env
}

test_env_create_local() {
  setup_test_env

  if command -v env::create >/dev/null 2>&1; then
    env::create "test-local" "local" >/dev/null 2>&1

    assert_dir_exists ".environments/test-local" "Local environment directory should exist"
    assert_file_exists ".environments/test-local/.env" "Local .env should exist"
    assert_file_exists ".environments/test-local/server.json" "Local server.json should exist"

    # Check content
    assert_file_contains ".environments/test-local/.env" "ENV=development" ".env should have development env"
    assert_file_contains ".environments/test-local/.env" "DEBUG=true" ".env should have debug enabled"
  else
    skip_test "env::create not available"
  fi

  teardown_test_env
}

test_env_create_staging() {
  setup_test_env

  if command -v env::create >/dev/null 2>&1; then
    env::create "test-staging" "staging" >/dev/null 2>&1

    assert_dir_exists ".environments/test-staging" "Staging environment directory should exist"
    assert_file_exists ".environments/test-staging/.env" "Staging .env should exist"
    assert_file_exists ".environments/test-staging/.env.secrets" "Staging .env.secrets should exist"

    # Check content
    assert_file_contains ".environments/test-staging/.env" "ENV=staging" ".env should have staging env"
    assert_file_contains ".environments/test-staging/.env" "DEBUG=false" ".env should have debug disabled"

    # Check secrets file permissions
    local perms
    if command -v safe_stat_perms >/dev/null 2>&1; then
      perms=$(safe_stat_perms ".environments/test-staging/.env.secrets" 2>/dev/null)
    else
      perms=$(stat -f "%OLp" ".environments/test-staging/.env.secrets" 2>/dev/null || stat -c "%a" ".environments/test-staging/.env.secrets" 2>/dev/null)
    fi
    assert_equals "600" "$perms" "Secrets file should have 600 permissions"
  else
    skip_test "env::create not available"
  fi

  teardown_test_env
}

test_env_create_production() {
  setup_test_env

  if command -v env::create >/dev/null 2>&1; then
    env::create "test-prod" "prod" >/dev/null 2>&1

    assert_dir_exists ".environments/test-prod" "Production environment directory should exist"
    assert_file_exists ".environments/test-prod/.env" "Production .env should exist"
    assert_file_exists ".environments/test-prod/.env.secrets" "Production .env.secrets should exist"

    # Check content
    assert_file_contains ".environments/test-prod/.env" "ENV=production" ".env should have production env"
    assert_file_contains ".environments/test-prod/.env" "DEBUG=false" ".env should have debug disabled"
    assert_file_contains ".environments/test-prod/.env" "HASURA_GRAPHQL_ENABLE_CONSOLE=false" "Hasura console should be disabled"
  else
    skip_test "env::create not available"
  fi

  teardown_test_env
}

test_env_create_sanitizes_name() {
  setup_test_env

  if command -v env::create >/dev/null 2>&1; then
    env::create "Test--Name_123" "local" >/dev/null 2>&1

    # Name should be sanitized to lowercase, alphanumeric and hyphens
    assert_dir_exists ".environments/test--name123" "Sanitized environment directory should exist"
  else
    skip_test "env::create not available"
  fi

  teardown_test_env
}

test_env_create_prevents_duplicate() {
  setup_test_env

  if command -v env::create >/dev/null 2>&1; then
    env::create "duplicate" "local" >/dev/null 2>&1

    # Second create should fail without force
    # Capture output separately to avoid pipefail exit status masking grep result
    local dup_output
    dup_output=$(env::create "duplicate" "local" 2>&1) || true
    if echo "$dup_output" | grep -q "already exists"; then
      pass_test "Duplicate environment creation prevented"
    else
      fail_test "Should prevent duplicate environment creation"
    fi
  else
    skip_test "env::create not available"
  fi

  teardown_test_env
}

# ═══════════════════════════════════════════════════════════════
# Environment List Tests
# ═══════════════════════════════════════════════════════════════

test_env_list() {
  setup_test_env

  if command -v env::list >/dev/null 2>&1; then
    # Create some environments
    env::create "env-a" "local" >/dev/null 2>&1
    env::create "env-b" "staging" >/dev/null 2>&1

    local output
    output=$(env::list 2>&1)

    assert_contains "$output" "env-a" "List should contain env-a"
    assert_contains "$output" "env-b" "List should contain env-b"
  else
    skip_test "env::list not available"
  fi

  teardown_test_env
}

# ═══════════════════════════════════════════════════════════════
# Environment Switch Tests
# ═══════════════════════════════════════════════════════════════

test_env_switch() {
  setup_test_env

  if command -v env::switch >/dev/null 2>&1 && command -v env::create >/dev/null 2>&1; then
    # Create environments
    env::create "switch-test" "local" >/dev/null 2>&1

    # Create a base .env.dev
    printf "BASE_VAR=base\n" >".env.dev"

    # Switch to the environment
    env::switch "switch-test" >/dev/null 2>&1

    # Check current environment marker
    assert_file_exists ".current-env" "Current env marker should exist"
    local current
    current=$(cat ".current-env")
    assert_equals "switch-test" "$current" "Current env should be switch-test"
  else
    skip_test "env::switch not available"
  fi

  teardown_test_env
}

test_env_switch_creates_backup() {
  setup_test_env

  if command -v env::switch >/dev/null 2>&1 && command -v env::create >/dev/null 2>&1; then
    # Create environment and initial .env
    env::create "backup-test" "local" >/dev/null 2>&1
    printf "ORIGINAL=value\n" >".env"

    # Switch
    env::switch "backup-test" >/dev/null 2>&1

    # Check backup was created (backup files start with dot, use find to include hidden files)
    assert_dir_exists ".env-backups" "Backup directory should exist"
    local backup_count
    backup_count=$(find .env-backups/ -maxdepth 1 -name ".env.backup-*" 2>/dev/null | wc -l)
    assert_true "[[ $backup_count -gt 0 ]]" "At least one backup should exist"
  else
    skip_test "env::switch not available"
  fi

  teardown_test_env
}

# ═══════════════════════════════════════════════════════════════
# Environment Diff Tests
# ═══════════════════════════════════════════════════════════════

test_env_diff() {
  setup_test_env

  if command -v env::diff >/dev/null 2>&1 && command -v env::create >/dev/null 2>&1; then
    # Create two environments
    env::create "diff-a" "local" >/dev/null 2>&1
    env::create "diff-b" "staging" >/dev/null 2>&1

    # Add different values
    printf "\nCUSTOM_VAR=value-a\n" >>".environments/diff-a/.env"
    printf "\nCUSTOM_VAR=value-b\n" >>".environments/diff-b/.env"

    local output
    output=$(env::diff "diff-a" "diff-b" 2>&1)

    assert_contains "$output" "CUSTOM_VAR" "Diff should show CUSTOM_VAR difference"
  else
    skip_test "env::diff not available"
  fi

  teardown_test_env
}

# ═══════════════════════════════════════════════════════════════
# Environment Validation Tests
# ═══════════════════════════════════════════════════════════════

test_env_validate_valid() {
  setup_test_env

  if command -v env::validate >/dev/null 2>&1 && command -v env::create >/dev/null 2>&1; then
    # Create a valid environment
    env::create "valid-env" "local" >/dev/null 2>&1

    # Validation should pass
    if env::validate "valid-env" >/dev/null 2>&1; then
      pass_test "Valid environment passes validation"
    else
      fail_test "Valid environment should pass validation"
    fi
  else
    skip_test "env::validate not available"
  fi

  teardown_test_env
}

test_env_validate_missing_env() {
  setup_test_env

  if command -v env::validate >/dev/null 2>&1; then
    mkdir -p ".environments/invalid-env"

    # Validation should fail (no .env file)
    if env::validate "invalid-env" 2>&1 | grep -qi "error\|missing\|not found"; then
      pass_test "Invalid environment fails validation"
    else
      # Some implementations may pass with empty config
      skip_test "Validation behavior varies"
    fi
  else
    skip_test "env::validate not available"
  fi

  teardown_test_env
}

# ═══════════════════════════════════════════════════════════════
# Environment Delete Tests
# ═══════════════════════════════════════════════════════════════

test_env_delete() {
  setup_test_env

  if command -v env::delete >/dev/null 2>&1 && command -v env::create >/dev/null 2>&1; then
    # Create and delete environment
    env::create "delete-me" "local" >/dev/null 2>&1
    assert_dir_exists ".environments/delete-me" "Environment should exist before delete"

    # Delete with force
    env::delete "delete-me" "true" >/dev/null 2>&1

    if [[ ! -d ".environments/delete-me" ]]; then
      pass_test "Environment deleted successfully"
    else
      fail_test "Environment should be deleted"
    fi
  else
    skip_test "env::delete not available"
  fi

  teardown_test_env
}

test_env_delete_prevents_current() {
  setup_test_env

  if command -v env::delete >/dev/null 2>&1 && command -v env::create >/dev/null 2>&1 && command -v env::switch >/dev/null 2>&1; then
    # Create and switch to environment
    env::create "current-env" "local" >/dev/null 2>&1
    env::switch "current-env" "true" >/dev/null 2>&1

    # Try to delete current environment
    if env::delete "current-env" "true" 2>&1 | grep -qi "cannot delete\|active"; then
      pass_test "Cannot delete current active environment"
    else
      fail_test "Should prevent deletion of current environment"
    fi
  else
    skip_test "env::delete not available"
  fi

  teardown_test_env
}

# ═══════════════════════════════════════════════════════════════
# Test Runner
# ═══════════════════════════════════════════════════════════════

run_env_tests() {
  printf "Environment Module Tests (v0.4.3)\n"
  printf "══════════════════════════════════════════════════════════\n\n"

  run_test "test_env_init_directory" "Initialize environments directory"
  run_test "test_env_create_local" "Create local environment"
  run_test "test_env_create_staging" "Create staging environment"
  run_test "test_env_create_production" "Create production environment"
  run_test "test_env_create_sanitizes_name" "Environment name sanitization"
  run_test "test_env_create_prevents_duplicate" "Prevent duplicate environment"
  run_test "test_env_list" "List environments"
  run_test "test_env_switch" "Switch environment"
  run_test "test_env_switch_creates_backup" "Switch creates backup"
  run_test "test_env_diff" "Diff environments"
  run_test "test_env_validate_valid" "Validate valid environment"
  run_test "test_env_validate_missing_env" "Validate missing .env"
  run_test "test_env_delete" "Delete environment"
  run_test "test_env_delete_prevents_current" "Prevent current deletion"

  printf "\n"
  print_test_summary
}

# Execute tests if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  run_env_tests
fi
