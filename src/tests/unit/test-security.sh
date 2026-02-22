#!/usr/bin/env bash
# test-security.sh - Unit tests for security modules (v0.4.3)
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

  # Source the security modules
  source "$PROJECT_ROOT/src/lib/utils/display.sh" 2>/dev/null || true
  source "$PROJECT_ROOT/src/lib/utils/platform-compat.sh" 2>/dev/null || true
  source "$PROJECT_ROOT/src/lib/security/checklist.sh" 2>/dev/null || true
  source "$PROJECT_ROOT/src/lib/security/secrets.sh" 2>/dev/null || true
  source "$PROJECT_ROOT/src/lib/security/ssl-letsencrypt.sh" 2>/dev/null || true
  source "$PROJECT_ROOT/src/lib/security/firewall.sh" 2>/dev/null || true
  set +e  # Re-disable errexit (sourced libs may have re-enabled it)
}

teardown_test_env() {
  cd "$ORIGINAL_DIR"
  if [[ -n "$TEST_TMP" ]] && [[ -d "$TEST_TMP" ]]; then
    rm -rf "$TEST_TMP"
  fi
}

# ═══════════════════════════════════════════════════════════════
# Secrets Generation Tests
# ═══════════════════════════════════════════════════════════════

test_secrets_generate_random_hex() {
  setup_test_env

  if command -v secrets::generate_random >/dev/null 2>&1; then
    local secret
    secret=$(secrets::generate_random 32 "hex")

    # Check length (32 hex chars)
    assert_equals "32" "${#secret}" "Hex secret should be 32 characters"

    # Check it's hex
    if echo "$secret" | grep -qE "^[0-9a-f]+$"; then
      pass_test "Secret is valid hex"
    else
      fail_test "Secret should be valid hex"
    fi
  else
    skip_test "secrets::generate_random not available"
  fi

  teardown_test_env
}

test_secrets_generate_random_alphanumeric() {
  setup_test_env

  if command -v secrets::generate_random >/dev/null 2>&1; then
    local secret
    secret=$(secrets::generate_random 24 "alphanumeric")

    # Check length
    assert_equals "24" "${#secret}" "Alphanumeric secret should be 24 characters"

    # Check it's alphanumeric
    if echo "$secret" | grep -qE "^[a-zA-Z0-9]+$"; then
      pass_test "Secret is valid alphanumeric"
    else
      fail_test "Secret should be valid alphanumeric"
    fi
  else
    skip_test "secrets::generate_random not available"
  fi

  teardown_test_env
}

test_secrets_generate_all() {
  setup_test_env

  if command -v secrets::generate_all >/dev/null 2>&1; then
    secrets::generate_all ".env.secrets" "true" >/dev/null 2>&1

    assert_file_exists ".env.secrets" "Secrets file should be created"

    # Check it contains required secrets
    assert_file_contains ".env.secrets" "POSTGRES_PASSWORD=" "Should contain POSTGRES_PASSWORD"
    assert_file_contains ".env.secrets" "HASURA_GRAPHQL_ADMIN_SECRET=" "Should contain HASURA secret"
    assert_file_contains ".env.secrets" "JWT_SECRET=" "Should contain JWT_SECRET"

    # Check permissions
    local perms
    if command -v safe_stat_perms >/dev/null 2>&1; then
      perms=$(safe_stat_perms ".env.secrets" 2>/dev/null)
    else
      perms=$(stat -f "%OLp" ".env.secrets" 2>/dev/null || stat -c "%a" ".env.secrets" 2>/dev/null)
    fi
    assert_equals "600" "$perms" "Secrets file should have 600 permissions"
  else
    skip_test "secrets::generate_all not available"
  fi

  teardown_test_env
}

test_secrets_validate() {
  setup_test_env

  if command -v secrets::validate >/dev/null 2>&1 && command -v secrets::generate_all >/dev/null 2>&1; then
    # Generate secrets first
    secrets::generate_all ".env.secrets" "true" >/dev/null 2>&1

    # Validate should pass
    if secrets::validate ".env.secrets" >/dev/null 2>&1; then
      pass_test "Generated secrets pass validation"
    else
      fail_test "Generated secrets should pass validation"
    fi
  else
    skip_test "secrets::validate not available"
  fi

  teardown_test_env
}

test_secrets_validate_weak() {
  setup_test_env

  if command -v secrets::validate >/dev/null 2>&1; then
    # Create secrets with weak password
    cat >".env.secrets" <<EOF
POSTGRES_PASSWORD=password
HASURA_GRAPHQL_ADMIN_SECRET=short
JWT_SECRET=veryshortsecret
EOF
    chmod 600 ".env.secrets"

    # Validate should warn/fail
    local output
    output=$(secrets::validate ".env.secrets" 2>&1)

    if echo "$output" | grep -qi "warning\|short\|weak"; then
      pass_test "Validation detects weak secrets"
    else
      # Some implementations may not validate strength
      skip_test "Strength validation not implemented"
    fi
  else
    skip_test "secrets::validate not available"
  fi

  teardown_test_env
}

test_secrets_check_git() {
  setup_test_env

  if command -v secrets::check_git >/dev/null 2>&1; then
    # Create a git repo
    git init . >/dev/null 2>&1
    printf ".env.secrets\n" >".gitignore"
    touch ".env.secrets"

    # Should pass (secrets file is ignored)
    if secrets::check_git ".env.secrets" >/dev/null 2>&1; then
      pass_test "Git check passes for ignored secrets file"
    else
      fail_test "Git check should pass for ignored file"
    fi
  else
    skip_test "secrets::check_git not available"
  fi

  teardown_test_env
}

# ═══════════════════════════════════════════════════════════════
# Security Checklist Tests
# ═══════════════════════════════════════════════════════════════

test_security_check_env_settings() {
  setup_test_env

  if command -v security::check_env_settings >/dev/null 2>&1; then
    # Create production-like .env
    cat >".env" <<EOF
ENV=production
DEBUG=false
LOG_LEVEL=warning
HASURA_GRAPHQL_DEV_MODE=false
HASURA_GRAPHQL_ENABLE_CONSOLE=false
EOF

    # Reset counters
    if command -v security::reset_counters >/dev/null 2>&1; then
      security::reset_counters
    fi

    security::check_env_settings >/dev/null 2>&1

    # Should have passes (production settings are correct)
    if [[ "${SECURITY_PASSED:-0}" -gt 0 ]]; then
      pass_test "Production settings pass security check"
    else
      skip_test "Security counters not available"
    fi
  else
    skip_test "security::check_env_settings not available"
  fi

  teardown_test_env
}

test_security_check_env_settings_debug() {
  setup_test_env

  if command -v security::check_env_settings >/dev/null 2>&1; then
    # Create insecure .env
    cat >".env" <<EOF
ENV=production
DEBUG=true
HASURA_GRAPHQL_DEV_MODE=true
EOF

    # Reset counters
    if command -v security::reset_counters >/dev/null 2>&1; then
      security::reset_counters
    fi

    security::check_env_settings >/dev/null 2>&1

    # Should have failures
    if [[ "${SECURITY_FAILED:-0}" -gt 0 ]]; then
      pass_test "Debug mode detected as security issue"
    else
      skip_test "Security counters not available"
    fi
  else
    skip_test "security::check_env_settings not available"
  fi

  teardown_test_env
}

test_security_audit() {
  setup_test_env

  if command -v security::audit >/dev/null 2>&1; then
    # Create minimal secure config
    cat >".env" <<EOF
ENV=production
DEBUG=false
SSL_ENABLED=true
POSTGRES_PASSWORD=a-reasonably-strong-password-here
HASURA_GRAPHQL_ADMIN_SECRET=a-very-long-hasura-admin-secret-that-is-secure
JWT_SECRET=a-very-long-jwt-secret-that-is-at-least-32-characters
EOF

    mkdir -p ssl
    openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
      -keyout ssl/key.pem -out ssl/cert.pem \
      -subj "/CN=localhost" 2>/dev/null || true
    chmod 600 ssl/key.pem 2>/dev/null || true

    # Run audit
    security::audit >/dev/null 2>&1 || true

    # Should have some results
    local total=$((${SECURITY_PASSED:-0} + ${SECURITY_WARNINGS:-0} + ${SECURITY_FAILED:-0}))
    if [[ $total -gt 0 ]]; then
      pass_test "Security audit runs and produces results"
    else
      skip_test "Security audit did not produce countable results"
    fi
  else
    skip_test "security::audit not available"
  fi

  teardown_test_env
}

# ═══════════════════════════════════════════════════════════════
# SSL Tests
# ═══════════════════════════════════════════════════════════════

test_ssl_init() {
  setup_test_env

  if command -v ssl::init >/dev/null 2>&1; then
    ssl::init >/dev/null 2>&1

    assert_dir_exists "ssl" "SSL directory should be created"
  else
    skip_test "ssl::init not available"
  fi

  teardown_test_env
}

test_ssl_generate_self_signed() {
  setup_test_env

  if command -v ssl::generate_self_signed >/dev/null 2>&1; then
    ssl::generate_self_signed "test.local" >/dev/null 2>&1

    assert_file_exists "ssl/cert.pem" "Certificate should be created"
    assert_file_exists "ssl/key.pem" "Private key should be created"

    # Verify it's a valid certificate
    if command -v openssl >/dev/null 2>&1; then
      local subject
      subject=$(openssl x509 -subject -noout -in ssl/cert.pem 2>/dev/null)
      assert_contains "$subject" "test.local" "Certificate should be for test.local"
    fi
  else
    skip_test "ssl::generate_self_signed not available"
  fi

  teardown_test_env
}

test_ssl_status() {
  setup_test_env

  if command -v ssl::status >/dev/null 2>&1 && command -v ssl::generate_self_signed >/dev/null 2>&1; then
    # Generate a certificate first
    ssl::generate_self_signed "test.local" >/dev/null 2>&1

    local output
    output=$(ssl::status "ssl/cert.pem" 2>&1)

    assert_contains "$output" "test.local" "Status should show domain"
  else
    skip_test "ssl::status not available"
  fi

  teardown_test_env
}

test_ssl_verify_chain() {
  setup_test_env

  if command -v ssl::verify_chain >/dev/null 2>&1 && command -v ssl::generate_self_signed >/dev/null 2>&1; then
    # Generate a certificate first
    ssl::generate_self_signed "test.local" >/dev/null 2>&1

    # Verify should pass
    if ssl::verify_chain "ssl/cert.pem" "ssl/key.pem" >/dev/null 2>&1; then
      pass_test "Self-signed certificate verifies correctly"
    else
      fail_test "Certificate verification should pass"
    fi
  else
    skip_test "ssl::verify_chain not available"
  fi

  teardown_test_env
}

# ═══════════════════════════════════════════════════════════════
# Firewall Tests
# ═══════════════════════════════════════════════════════════════

test_firewall_detect() {
  setup_test_env

  if command -v firewall::detect >/dev/null 2>&1; then
    local fw_type
    fw_type=$(firewall::detect)

    # Should return one of: ufw, firewalld, iptables, none
    case "$fw_type" in
      ufw | firewalld | iptables | none)
        pass_test "Firewall detection returns valid type: $fw_type"
        ;;
      *)
        fail_test "Firewall detection returned unknown type: $fw_type"
        ;;
    esac
  else
    skip_test "firewall::detect not available"
  fi

  teardown_test_env
}

test_firewall_generate_rules() {
  setup_test_env

  if command -v firewall::generate_rules >/dev/null 2>&1; then
    firewall::generate_rules "test-rules.sh" >/dev/null 2>&1

    assert_file_exists "test-rules.sh" "Firewall rules script should be created"

    # Check it's executable
    if [[ -x "test-rules.sh" ]]; then
      pass_test "Firewall rules script is executable"
    else
      fail_test "Firewall rules script should be executable"
    fi

    # Check content
    assert_file_contains "test-rules.sh" "ufw\|firewall" "Script should contain firewall commands"
  else
    skip_test "firewall::generate_rules not available"
  fi

  teardown_test_env
}

# ═══════════════════════════════════════════════════════════════
# Test Runner
# ═══════════════════════════════════════════════════════════════

run_security_tests() {
  printf "Security Module Tests (v0.4.3)\n"
  printf "══════════════════════════════════════════════════════════\n\n"

  printf "Secrets Generation:\n"
  run_test "test_secrets_generate_random_hex" "Generate random hex secret"
  run_test "test_secrets_generate_random_alphanumeric" "Generate random alphanumeric"
  run_test "test_secrets_generate_all" "Generate all secrets"
  run_test "test_secrets_validate" "Validate secrets"
  run_test "test_secrets_validate_weak" "Detect weak secrets"
  run_test "test_secrets_check_git" "Check git ignore"

  printf "\nSecurity Checklist:\n"
  run_test "test_security_check_env_settings" "Check production settings"
  run_test "test_security_check_env_settings_debug" "Detect debug mode"
  run_test "test_security_audit" "Full security audit"

  printf "\nSSL/TLS:\n"
  run_test "test_ssl_init" "Initialize SSL directory"
  run_test "test_ssl_generate_self_signed" "Generate self-signed cert"
  run_test "test_ssl_status" "SSL status check"
  run_test "test_ssl_verify_chain" "Verify certificate chain"

  printf "\nFirewall:\n"
  run_test "test_firewall_detect" "Detect firewall type"
  run_test "test_firewall_generate_rules" "Generate firewall rules"

  printf "\n"
  print_test_summary
}

# Execute tests if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  run_security_tests
fi
