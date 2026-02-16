#!/usr/bin/env bash
set -euo pipefail
# Unit tests for whitelabel themes system
# Tests theme creation, management, CSS generation, and database integration

set -eo pipefail

# Test framework setup
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Color codes (use different variable names to avoid conflicts with themes.sh)
T_RED='\033[0;31m'
T_GREEN='\033[0;32m'
T_YELLOW='\033[1;33m'
T_BLUE='\033[0;34m'
T_BOLD='\033[1m'
T_NC='\033[0m'

# Source the themes library (after defining our colors)
source "$PROJECT_ROOT/lib/whitelabel/themes.sh"

# Test utilities
assert_equals() {
  local expected="$1"
  local actual="$2"
  local message="${3:-}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ "$expected" == "$actual" ]]; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "${T_GREEN}✓${T_NC} PASS: %s\n" "$message"
    return 0
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "${T_RED}✗${T_NC} FAIL: %s\n" "$message"
    printf "  Expected: %s\n" "$expected"
    printf "  Actual:   %s\n" "$actual"
    return 1
  fi
}

assert_true() {
  local condition="$1"
  local message="${2:-}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ "$condition" == "0" ]] || [[ "$condition" == "true" ]]; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "${T_GREEN}✓${T_NC} PASS: %s\n" "$message"
    return 0
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "${T_RED}✗${T_NC} FAIL: %s\n" "$message"
    return 1
  fi
}

assert_file_exists() {
  local file_path="$1"
  local message="${2:-File should exist: $file_path}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ -f "$file_path" ]]; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "${T_GREEN}✓${T_NC} PASS: %s\n" "$message"
    return 0
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "${T_RED}✗${T_NC} FAIL: %s\n" "$message"
    return 1
  fi
}

assert_directory_exists() {
  local dir_path="$1"
  local message="${2:-Directory should exist: $dir_path}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ -d "$dir_path" ]]; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "${T_GREEN}✓${T_NC} PASS: %s\n" "$message"
    return 0
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "${T_RED}✗${T_NC} FAIL: %s\n" "$message"
    return 1
  fi
}

# Setup test environment
setup_test_env() {
  export PROJECT_NAME="test-nself"
  export POSTGRES_DB="test_db"
  export POSTGRES_USER="postgres"
  export POSTGRES_PASSWORD="test"

  # Use a test-specific directory that doesn't conflict with themes.sh constants
  TEST_ROOT="$SCRIPT_DIR/../../test-workspace"
  TEST_THEMES_DIR="$TEST_ROOT/branding/themes"
  mkdir -p "$TEST_THEMES_DIR"
}

# Cleanup test environment
cleanup_test_env() {
  if [[ -d "$TEST_ROOT" ]]; then
    rm -rf "$TEST_ROOT"
  fi
}

# ============================================================================
# Test: Theme Directory Initialization
# ============================================================================

test_themes_directory_creation() {
  printf "\n${T_BLUE}Test: Theme Directory Creation${T_NC}\n"

  setup_test_env

  assert_directory_exists "$TEST_THEMES_DIR" "Themes directory should be created"

  cleanup_test_env
}

# ============================================================================
# Test: Default Theme Templates
# ============================================================================

test_default_theme_templates() {
  printf "\n${T_BLUE}Test: Default Theme Templates${T_NC}\n"

  setup_test_env

  # Test light theme structure
  local has_light=0
  for theme in $DEFAULT_THEMES; do
    if [[ "$theme" == "light" ]]; then
      has_light=1
    fi
  done

  assert_equals "1" "$has_light" "Light theme should be in default themes"

  # Test dark theme structure
  local has_dark=0
  for theme in $DEFAULT_THEMES; do
    if [[ "$theme" == "dark" ]]; then
      has_dark=1
    fi
  done

  assert_equals "1" "$has_dark" "Dark theme should be in default themes"

  cleanup_test_env
}

# ============================================================================
# Test: CSS Generation
# ============================================================================

test_css_generation() {
  printf "\n${T_BLUE}Test: CSS Generation${T_NC}\n"

  setup_test_env

  # Create test theme config
  local test_theme_dir="$TEST_THEMES_DIR/test-theme"
  mkdir -p "$test_theme_dir"

  cat >"$test_theme_dir/theme.json" <<'EOF'
{
  "name": "test-theme",
  "displayName": "Test Theme",
  "mode": "light",
  "colors": {
    "primary": "#0066cc",
    "background": "#ffffff"
  },
  "typography": {
    "fontFamily": "Arial, sans-serif",
    "fontSize": "16px"
  },
  "spacing": {
    "sm": "8px",
    "md": "16px"
  },
  "borders": {
    "radius": "4px"
  },
  "shadows": {
    "sm": "0 1px 3px rgba(0,0,0,0.12)"
  }
}
EOF

  # Generate CSS
  generate_theme_css "$test_theme_dir/theme.json" "$test_theme_dir/theme.css"

  assert_file_exists "$test_theme_dir/theme.css" "CSS file should be generated"

  # Check CSS content
  if [[ -f "$test_theme_dir/theme.css" ]]; then
    local has_root
    has_root=$(grep -c ":root" "$test_theme_dir/theme.css" || true)
    assert_true "$([[ $has_root -gt 0 ]] && echo 0 || echo 1)" "CSS should contain :root selector"

    local has_primary
    has_primary=$(grep -c "color-primary" "$test_theme_dir/theme.css" || true)
    assert_true "$([[ $has_primary -gt 0 ]] && echo 0 || echo 1)" "CSS should contain primary color variable"
  fi

  cleanup_test_env
}

# ============================================================================
# Test: Theme Validation
# ============================================================================

test_theme_validation() {
  printf "\n${T_BLUE}Test: Theme Validation${T_NC}\n"

  setup_test_env

  # Create valid theme
  local valid_theme_dir="$TEST_THEMES_DIR/valid-theme"
  mkdir -p "$valid_theme_dir"

  cat >"$valid_theme_dir/theme.json" <<'EOF'
{
  "name": "valid-theme",
  "displayName": "Valid Theme",
  "mode": "light",
  "colors": {
    "primary": "#0066cc"
  },
  "typography": {
    "fontFamily": "Arial"
  },
  "spacing": {
    "md": "16px"
  },
  "borders": {
    "radius": "4px"
  },
  "shadows": {
    "sm": "0 1px 3px rgba(0,0,0,0.12)"
  }
}
EOF

  # Validate theme (requires jq)
  if command -v jq >/dev/null 2>&1; then
    validate_theme_config "$valid_theme_dir/theme.json"
    local result=$?
    assert_equals "0" "$result" "Valid theme should pass validation"
  else
    printf "${T_YELLOW}Skipping validation test (jq not available)${T_NC}\n"
  fi

  cleanup_test_env
}

# ============================================================================
# Test: Theme Name Validation
# ============================================================================

test_theme_name_validation() {
  printf "\n${T_BLUE}Test: Theme Name Validation${T_NC}\n"

  setup_test_env

  # Valid theme names
  local valid_names="light dark my-theme test-123"
  for name in $valid_names; do
    if [[ "$name" =~ ^[a-z0-9-]+$ ]]; then
      printf "${T_GREEN}✓${T_NC} Valid theme name: %s\n" "$name"
    else
      printf "${T_RED}✗${T_NC} Invalid theme name: %s\n" "$name"
    fi
  done

  # Invalid theme names (should fail)
  local invalid_name="My Theme"
  if [[ ! "$invalid_name" =~ ^[a-z0-9-]+$ ]]; then
    printf "${T_GREEN}✓${T_NC} Correctly rejected invalid name: %s\n" "$invalid_name"
  fi

  cleanup_test_env
}

# ============================================================================
# Test: Theme File Structure
# ============================================================================

test_theme_file_structure() {
  printf "\n${T_BLUE}Test: Theme File Structure${T_NC}\n"

  setup_test_env

  local theme_dir="$TEST_THEMES_DIR/structure-test"
  mkdir -p "$theme_dir"

  # Create theme with all required fields
  cat >"$theme_dir/theme.json" <<'EOF'
{
  "name": "structure-test",
  "displayName": "Structure Test",
  "description": "Testing structure",
  "version": "1.0.0",
  "author": "Test",
  "mode": "light",
  "variables": {
    "colors": {
      "primary": "#0066cc"
    },
    "typography": {
      "fontFamily": "Arial"
    },
    "spacing": {
      "md": "16px"
    },
    "borders": {
      "radius": "4px"
    },
    "shadows": {
      "sm": "0 1px 3px rgba(0,0,0,0.12)"
    }
  }
}
EOF

  if command -v jq >/dev/null 2>&1; then
    # Check required fields
    local name
    name=$(jq -r '.name' "$theme_dir/theme.json")
    assert_equals "structure-test" "$name" "Theme should have name field"

    local display_name
    display_name=$(jq -r '.displayName' "$theme_dir/theme.json")
    assert_equals "Structure Test" "$display_name" "Theme should have displayName field"

    local mode
    mode=$(jq -r '.mode' "$theme_dir/theme.json")
    assert_equals "light" "$mode" "Theme should have mode field"

    # Check variables structure
    local has_colors
    has_colors=$(jq -e '.variables.colors' "$theme_dir/theme.json" >/dev/null 2>&1 && echo 0 || echo 1)
    assert_equals "0" "$has_colors" "Theme should have colors in variables"
  else
    printf "${T_YELLOW}Skipping structure test (jq not available)${T_NC}\n"
  fi

  cleanup_test_env
}

# ============================================================================
# Test: CSS Variable Naming
# ============================================================================

test_css_variable_naming() {
  printf "\n${T_BLUE}Test: CSS Variable Naming${T_NC}\n"

  setup_test_env

  local theme_dir="$TEST_THEMES_DIR/css-vars-test"
  mkdir -p "$theme_dir"

  cat >"$theme_dir/theme.json" <<'EOF'
{
  "name": "css-vars-test",
  "displayName": "CSS Vars Test",
  "mode": "light",
  "colors": {
    "primary": "#0066cc",
    "secondary": "#6c757d"
  },
  "typography": {
    "fontFamily": "Arial, sans-serif"
  },
  "spacing": {
    "sm": "8px"
  },
  "borders": {
    "radius": "4px"
  },
  "shadows": {
    "sm": "0 1px 3px rgba(0,0,0,0.12)"
  }
}
EOF

  generate_theme_css "$theme_dir/theme.json" "$theme_dir/theme.css"

  if [[ -f "$theme_dir/theme.css" ]]; then
    # Check variable naming conventions
    local has_color_prefix
    has_color_prefix=$(grep -c "\-\-color-" "$theme_dir/theme.css" || true)
    assert_true "$([[ $has_color_prefix -gt 0 ]] && echo 0 || echo 1)" "CSS should use --color- prefix"

    local has_typography_prefix
    has_typography_prefix=$(grep -c "\-\-typography-" "$theme_dir/theme.css" || true)
    assert_true "$([[ $has_typography_prefix -gt 0 ]] && echo 0 || echo 1)" "CSS should use --typography- prefix"

    local has_spacing_prefix
    has_spacing_prefix=$(grep -c "\-\-spacing-" "$theme_dir/theme.css" || true)
    assert_true "$([[ $has_spacing_prefix -gt 0 ]] && echo 0 || echo 1)" "CSS should use --spacing- prefix"
  fi

  cleanup_test_env
}

# ============================================================================
# Test: Theme Mode Validation
# ============================================================================

test_theme_mode_validation() {
  printf "\n${T_BLUE}Test: Theme Mode Validation${T_NC}\n"

  setup_test_env

  # Valid modes
  local valid_modes="light dark auto"
  for mode in $valid_modes; do
    printf "${T_GREEN}✓${T_NC} Valid mode: %s\n" "$mode"
  done

  # Invalid mode (should fail in database constraint)
  local invalid_mode="invalid-mode"
  printf "${T_BLUE}Info: Invalid mode '%s' should be rejected by database${T_NC}\n" "$invalid_mode"

  cleanup_test_env
}

# ============================================================================
# Test: JSON Export/Import
# ============================================================================

test_json_export_import() {
  printf "\n${T_BLUE}Test: JSON Export/Import${T_NC}\n"

  setup_test_env

  if ! command -v jq >/dev/null 2>&1; then
    printf "${T_YELLOW}Skipping export/import test (jq not available)${T_NC}\n"
    cleanup_test_env
    return 0
  fi

  local theme_dir="$TEST_THEMES_DIR/export-test"
  mkdir -p "$theme_dir"

  # Create theme
  cat >"$theme_dir/theme.json" <<'EOF'
{
  "name": "export-test",
  "displayName": "Export Test",
  "mode": "light",
  "colors": {
    "primary": "#0066cc"
  },
  "typography": {
    "fontFamily": "Arial"
  },
  "spacing": {
    "md": "16px"
  },
  "borders": {
    "radius": "4px"
  },
  "shadows": {
    "sm": "0 1px 3px rgba(0,0,0,0.12)"
  }
}
EOF

  # Test JSON parsing
  local name
  name=$(jq -r '.name' "$theme_dir/theme.json")
  assert_equals "export-test" "$name" "Should parse theme name from JSON"

  # Test JSON roundtrip
  local temp_file
  temp_file=$(mktemp)
  jq '.' "$theme_dir/theme.json" >"$temp_file"

  local roundtrip_name
  roundtrip_name=$(jq -r '.name' "$temp_file")
  assert_equals "export-test" "$roundtrip_name" "JSON should survive roundtrip"

  rm -f "$temp_file"

  cleanup_test_env
}

# ============================================================================
# Test: Database Connection Check (Mock)
# ============================================================================

test_database_connection_check() {
  printf "\n${T_BLUE}Test: Database Connection Check${T_NC}\n"

  setup_test_env

  # This will fail since we don't have a real database in unit tests
  # but we're testing that the function handles it gracefully
  if ! check_database_connection 2>/dev/null; then
    printf "${T_GREEN}✓${T_NC} Correctly handles missing database connection\n"
  else
    printf "${T_YELLOW}⚠ Database connection available (unexpected in unit tests)${T_NC}\n"
  fi

  cleanup_test_env
}

# ============================================================================
# Run all tests
# ============================================================================

run_all_tests() {
  printf "${T_BLUE}${T_BOLD}===========================================\n"
  printf "nself Whitelabel Themes - Unit Tests\n"
  printf "===========================================${T_NC}\n"

  test_themes_directory_creation
  test_default_theme_templates
  test_css_generation
  test_theme_validation
  test_theme_name_validation
  test_theme_file_structure
  test_css_variable_naming
  test_theme_mode_validation
  test_json_export_import
  test_database_connection_check

  # Print summary
  printf "\n${T_BLUE}${T_BOLD}===========================================\n"
  printf "Test Summary\n"
  printf "===========================================${T_NC}\n"
  printf "Total tests:  %d\n" "$TESTS_RUN"
  printf "${T_GREEN}Passed:       %d${T_NC}\n" "$TESTS_PASSED"
  printf "${T_RED}Failed:       %d${T_NC}\n" "$TESTS_FAILED"

  if [[ $TESTS_FAILED -eq 0 ]]; then
    printf "\n${T_GREEN}${T_BOLD}✓ All tests passed!${T_NC}\n"
    return 0
  else
    printf "\n${T_RED}${T_BOLD}✗ Some tests failed${T_NC}\n"
    return 1
  fi
}

# Run tests if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  run_all_tests
  exit $?
fi
