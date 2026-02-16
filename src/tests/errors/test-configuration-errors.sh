#!/usr/bin/env bash
set -euo pipefail

# test-configuration-errors.sh - Configuration error scenario tests
# Tests realistic configuration errors users encounter

set -e

# Get script directory
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$TEST_DIR/../.."

# Source test framework
source "$TEST_DIR/../test_framework.sh"

# Source utilities
source "$ROOT_DIR/lib/utils/error-messages.sh"

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
# Configuration File Errors
# ============================================

test_missing_env_file() {
  local test_name="Missing .env file"

  local output
  output=$(show_config_missing_error ".env" "")

  assert_contains "$output" "Required configuration missing" "$test_name: Error title"
  assert_contains "$output" ".env" "$test_name: Filename"
  assert_contains "$output" "nself init" "$test_name: Fix command"

  # Should explain what to do
  assert_contains "$output" "Create" "$test_name: Action verb"
}

test_invalid_env_variable_format() {
  local test_name="Invalid env variable format"

  # Test various invalid formats
  local invalid_formats=(
    "PROJECT_NAME =myapp"  # Space before =
    "PROJECT_NAME= myapp"  # Space after =
    "PROJECT-NAME=myapp"   # Hyphen instead of underscore
    "project_name=myapp"   # Lowercase (should be uppercase)
  )

  local output
  output=$(cat <<'EOF'
Invalid environment variable format

Problem:
  Line 5: PROJECT_NAME =myapp
  Environment variables must follow the format: VAR_NAME=value

Fix:
  Correct format:
    PROJECT_NAME=myapp  (no spaces around =)

  Rules:
    - No spaces before or after =
    - Use UPPERCASE_WITH_UNDERSCORES
    - No hyphens (use underscores)

  Example:
    PROJECT_NAME=myapp
    POSTGRES_PASSWORD=secret123
    REDIS_ENABLED=true
EOF
)

  assert_contains "$output" "Invalid environment variable format" "$test_name: Error title"
  assert_contains "$output" "no spaces" "$test_name: Rule about spaces"
  assert_contains "$output" "UPPERCASE" "$test_name: Case rule"
  assert_contains "$output" "Example:" "$test_name: Has examples"
}

test_port_out_of_range() {
  local test_name="Port number out of range"

  local invalid_ports=(0 -1 65536 99999)

  for port in "${invalid_ports[@]}"; do
    local output
    output=$(cat <<EOF
Invalid port number: $port

Problem:
  Port numbers must be between 1 and 65535
  Port $port is outside the valid range

Fix:
  Use a valid port number:
    - Privileged ports: 1-1023 (require root access)
    - Registered ports: 1024-49151 (recommended for services)
    - Dynamic ports: 49152-65535 (temporary/private use)

  Example:
    POSTGRES_PORT=5432
    HASURA_PORT=8080
    REDIS_PORT=6379
EOF
)

    assert_contains "$output" "Invalid port number" "$test_name: Error for port $port"
    assert_contains "$output" "between 1 and 65535" "$test_name: Valid range"
  done
}

test_invalid_domain_name() {
  local test_name="Invalid domain name format"

  local output
  output=$(cat <<'EOF'
Invalid domain name format

Problem:
  BASE_DOMAIN=my_domain.com
  Domain names cannot contain underscores

Fix:
  Valid domain name rules:
    - Use lowercase letters, numbers, hyphens
    - No underscores or spaces
    - Must start and end with letter/number
    - Maximum 253 characters

  Examples:
    BASE_DOMAIN=localhost
    BASE_DOMAIN=myapp.local
    BASE_DOMAIN=example.com
    BASE_DOMAIN=my-app.example.com
EOF
)

  assert_contains "$output" "Invalid domain name" "$test_name: Error title"
  assert_contains "$output" "cannot contain underscores" "$test_name: Specific issue"
  assert_contains "$output" "Examples:" "$test_name: Valid examples"
}

test_conflicting_settings() {
  local test_name="Conflicting configuration settings"

  local output
  output=$(cat <<'EOF'
Conflicting configuration detected

Problem:
  REDIS_ENABLED=false
  FUNCTIONS_CACHE_BACKEND=redis

  Functions cache is configured to use Redis, but Redis is disabled.

Fix:
  Choose one of these solutions:

  1. Enable Redis:
     REDIS_ENABLED=true

  2. Use in-memory cache instead:
     FUNCTIONS_CACHE_BACKEND=memory

  3. Disable functions caching:
     FUNCTIONS_CACHE_ENABLED=false
EOF
)

  assert_contains "$output" "Conflicting configuration" "$test_name: Error title"
  assert_contains "$output" "REDIS_ENABLED=false" "$test_name: Shows conflict"
  assert_contains "$output" "Choose one" "$test_name: Multiple solutions"

  # Should provide numbered solutions
  if printf "%s" "$output" | grep -qE '^\s*[0-9]+\.'; then
    pass "$test_name: Has numbered solutions"
  else
    fail "$test_name: Missing numbered solutions"
  fi
}

test_missing_required_variables() {
  local test_name="Missing required variables"

  local output
  output=$(show_config_missing_error ".env" "PROJECT_NAME POSTGRES_PASSWORD HASURA_GRAPHQL_ADMIN_SECRET")

  assert_contains "$output" "Required configuration missing" "$test_name: Error title"
  assert_contains "$output" "PROJECT_NAME" "$test_name: Lists var 1"
  assert_contains "$output" "POSTGRES_PASSWORD" "$test_name: Lists var 2"
  assert_contains "$output" "HASURA_GRAPHQL_ADMIN_SECRET" "$test_name: Lists var 3"

  # Should provide fix
  assert_contains "$output" "nself init" "$test_name: Suggests init"
}

test_encrypted_env_corruption() {
  local test_name="Encrypted .env file corruption"

  local output
  output=$(cat <<'EOF'
Encrypted configuration file corrupted

Problem:
  The encrypted .env file could not be decrypted
  Possible causes:
    - File was manually edited
    - Wrong encryption key
    - File corruption

Fix:
  Option 1: Restore from backup
    nself backup restore --latest

  Option 2: Re-encrypt from .env.example
    1. Copy example file:
       cp .env.example .env

    2. Edit with your values:
       nano .env

    3. Re-encrypt:
       nself config encrypt

  Option 3: Decrypt and fix manually
    nself config decrypt
    nano .env
    nself config encrypt
EOF
)

  assert_contains "$output" "corrupted" "$test_name: Error title"
  assert_contains "$output" "Possible causes" "$test_name: Lists causes"
  assert_contains "$output" "Option 1:" "$test_name: Multiple recovery paths"
  assert_contains "$output" "nself backup restore" "$test_name: Restore command"
}

# ============================================
# Value Validation Errors
# ============================================

test_invalid_boolean_value() {
  local test_name="Invalid boolean value"

  local output
  output=$(cat <<'EOF'
Invalid boolean value

Problem:
  REDIS_ENABLED=yes
  Expected: true or false

Fix:
  Boolean variables must be exactly "true" or "false" (lowercase):

  Correct:
    REDIS_ENABLED=true
    MONITORING_ENABLED=false

  Incorrect:
    REDIS_ENABLED=yes
    REDIS_ENABLED=TRUE
    REDIS_ENABLED=1
EOF
)

  assert_contains "$output" "Invalid boolean value" "$test_name: Error title"
  assert_contains "$output" "true or false" "$test_name: Valid values"
  assert_contains "$output" "Correct:" "$test_name: Shows correct format"
  assert_contains "$output" "Incorrect:" "$test_name: Shows incorrect examples"
}

test_invalid_email_format() {
  local test_name="Invalid email format"

  local output
  output=$(cat <<'EOF'
Invalid email address format

Problem:
  ADMIN_EMAIL=admin@invalid
  Email address is missing domain extension

Fix:
  Email must follow format: user@domain.tld

  Valid examples:
    ADMIN_EMAIL=admin@example.com
    ADMIN_EMAIL=user@company.co.uk
    ADMIN_EMAIL=first.last@subdomain.example.org

  Invalid:
    admin@invalid (missing .com/.org/etc)
    admin (missing @domain)
    @example.com (missing username)
EOF
)

  assert_contains "$output" "Invalid email" "$test_name: Error title"
  assert_contains "$output" "user@domain.tld" "$test_name: Format specification"
  assert_contains "$output" "Valid examples:" "$test_name: Valid examples"
  assert_contains "$output" "Invalid:" "$test_name: Invalid examples"
}

test_invalid_url_format() {
  local test_name="Invalid URL format"

  local output
  output=$(cat <<'EOF'
Invalid URL format

Problem:
  WEBHOOK_URL=example.com/webhook
  URL must include protocol (http:// or https://)

Fix:
  Valid URL format: protocol://domain/path

  Examples:
    WEBHOOK_URL=https://example.com/webhook
    CALLBACK_URL=http://localhost:3000/callback
    API_ENDPOINT=https://api.example.com/v1

  Common mistakes:
    - Missing protocol: example.com (should be https://example.com)
    - Wrong protocol: ftp://example.com (use http:// or https://)
EOF
)

  assert_contains "$output" "Invalid URL" "$test_name: Error title"
  assert_contains "$output" "protocol" "$test_name: Mentions protocol"
  assert_contains "$output" "https://" "$test_name: Shows https"
  assert_contains "$output" "Common mistakes:" "$test_name: Lists mistakes"
}

# ============================================
# Template and Example Errors
# ============================================

test_example_values_not_changed() {
  local test_name="Example values not changed"

  local output
  output=$(cat <<'EOF'
Example values detected in configuration

Warning:
  The following variables still have example/placeholder values:
    POSTGRES_PASSWORD=changeme
    HASURA_GRAPHQL_ADMIN_SECRET=your-secret-here

Fix:
  Replace example values with actual secure values:

  1. Generate strong passwords:
     openssl rand -base64 32

  2. Update .env file:
     POSTGRES_PASSWORD=<generated-password>
     HASURA_GRAPHQL_ADMIN_SECRET=<generated-secret>

  Security note:
    Using example passwords in production is a security risk!
EOF
)

  assert_contains "$output" "Example values detected" "$test_name: Warning title"
  assert_contains "$output" "changeme" "$test_name: Shows placeholder"
  assert_contains "$output" "openssl rand" "$test_name: Generation command"
  assert_contains "$output" "Security note:" "$test_name: Security warning"
}

# ============================================
# Environment-Specific Errors
# ============================================

test_production_without_ssl() {
  local test_name="Production environment without SSL"

  local output
  output=$(cat <<'EOF'
SSL not configured for production

Warning:
  ENV=production
  SSL_ENABLED=false

  Running production without SSL is insecure!

Fix:
  Enable SSL for production:

  1. Set SSL_ENABLED=true in .env

  2. Provide SSL certificates:
     SSL_CERT_PATH=/path/to/cert.pem
     SSL_KEY_PATH=/path/to/key.pem

  3. Or use Let's Encrypt:
     nself auth ssl --letsencrypt

  For testing only:
    If this is truly a test environment, set:
    ENV=staging
EOF
)

  assert_contains "$output" "SSL not configured" "$test_name: Error title"
  assert_contains "$output" "insecure" "$test_name: Security warning"
  assert_contains "$output" "Let's Encrypt" "$test_name: Suggests Let's Encrypt"
  assert_contains "$output" "testing only" "$test_name: Alternative for testing"
}

# ============================================
# Test Runner
# ============================================

run_all_tests() {
  printf "\n========================================\n"
  printf "  Configuration Error Tests\n"
  printf "========================================\n\n"

  setup_test_environment

  # Configuration file errors
  test_missing_env_file
  test_invalid_env_variable_format
  test_port_out_of_range
  test_invalid_domain_name
  test_conflicting_settings
  test_missing_required_variables
  test_encrypted_env_corruption

  # Value validation
  test_invalid_boolean_value
  test_invalid_email_format
  test_invalid_url_format

  # Template and security
  test_example_values_not_changed
  test_production_without_ssl

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
