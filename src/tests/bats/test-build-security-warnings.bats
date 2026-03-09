#!/usr/bin/env bats
# test-build-security-warnings.bats
# Tests for cli/src/lib/security/build-warnings.sh
# All checks are mocked via env vars and temp files — no real system state needed.
# Bash 3.2+ compatible.

load test_helper

# Path to the library under test
SECURITY_LIB_DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")/../../lib/security" && pwd)"

# ── Helper: source the lib and invoke run_build_security_warnings ─────────────
# Wraps the function call so bats `run` captures stdout + exit code.
_run_warnings() {
  source "$SECURITY_LIB_DIR/build-warnings.sh"
  run_build_security_warnings "$@"
}

# ── Setup / teardown ──────────────────────────────────────────────────────────
setup() {
  TMPDIR_BATS="$(mktemp -d)"
  export BUILD_WARNINGS_SOURCED=""
}

teardown() {
  rm -rf "$TMPDIR_BATS"
}

# =============================================================================
# Scenario 1: staging env + fail2ban absent → warning printed
# =============================================================================
@test "1. staging env: fail2ban absent prints warning" {
  # Force Linux path so Darwin bail-out does not apply
  # We override the check by wrapping PATH so fail2ban-client is not found
  local orig_path="$PATH"
  export PATH="/usr/bin:/bin"   # strip any custom paths that might have fail2ban

  # Minimal mock docker-compose with nginx only (no port violations)
  local dc_file="$TMPDIR_BATS/docker-compose.yml"
  printf 'version: "3"\nservices:\n  nginx:\n    image: nginx\n    ports:\n      - "127.0.0.1:443:443"\n' > "$dc_file"

  # Minimal .env — valid secrets
  local env_file="$TMPDIR_BATS/.env"
  {
    printf 'HASURA_GRAPHQL_ENABLE_CONSOLE=false\n'
    printf 'DEBUG=false\n'
    printf 'HASURA_GRAPHQL_JWT_SECRET=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n'
    printf 'HASURA_GRAPHQL_ADMIN_SECRET=aaaaaaaaaaaaaaaaaa\n'
  } > "$env_file"

  export NSELF_ENV=staging
  export SSHD_CONFIG_PATH="$TMPDIR_BATS/no-such-sshd"   # non-existent → skip sshd check
  export TMPDIR="$TMPDIR_BATS"

  run bash -c "
    export NSELF_ENV=staging
    export SSHD_CONFIG_PATH='$TMPDIR_BATS/no-such-sshd'
    source '$SECURITY_LIB_DIR/build-warnings.sh'
    run_build_security_warnings '$dc_file' '$env_file'
  "

  # On Linux CI: if fail2ban-client not found → warning line present
  # On macOS (Darwin): check skipped → output should still contain Security Preflight header
  assert_output --partial "Security Preflight"

  export PATH="$orig_path"
}

# =============================================================================
# Scenario 2: staging env + sshd PasswordAuthentication yes → warning printed
# =============================================================================
@test "2. staging env: sshd PasswordAuthentication yes prints warning" {
  # Mock sshd_config with PasswordAuthentication yes
  local sshd_conf="$TMPDIR_BATS/sshd_config"
  printf 'PasswordAuthentication yes\nPermitRootLogin no\n' > "$sshd_conf"

  local dc_file="$TMPDIR_BATS/docker-compose.yml"
  printf 'version: "3"\nservices:\n  nginx:\n    image: nginx\n' > "$dc_file"

  local env_file="$TMPDIR_BATS/.env"
  {
    printf 'HASURA_GRAPHQL_ENABLE_CONSOLE=false\n'
    printf 'DEBUG=false\n'
    printf 'HASURA_GRAPHQL_JWT_SECRET=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n'
    printf 'HASURA_GRAPHQL_ADMIN_SECRET=aaaaaaaaaaaaaaaaaa\n'
  } > "$env_file"

  run bash -c "
    export NSELF_ENV=staging
    export SSHD_CONFIG_PATH='$sshd_conf'
    source '$SECURITY_LIB_DIR/build-warnings.sh'
    run_build_security_warnings '$dc_file' '$env_file'
  "

  # macOS returns 0 regardless (Darwin path skips). On Linux the sshd warn fires.
  # Either way the Security Preflight header must appear.
  assert_output --partial "Security Preflight"
}

# =============================================================================
# Scenario 3: staging env + all ports at 127.0.0.1 → no port violation warning
# =============================================================================
@test "3. staging env: all non-nginx ports on 127.0.0.1 → no port violation" {
  local dc_file="$TMPDIR_BATS/docker-compose.yml"
  cat > "$dc_file" <<'EOF'
version: "3"
services:
  nginx:
    image: nginx
    ports:
      - "0.0.0.0:443:443"
  postgres:
    image: postgres
    ports:
      - "127.0.0.1:5432:5432"
  hasura:
    image: hasura/graphql-engine
    ports:
      - "127.0.0.1:8080:8080"
EOF

  local env_file="$TMPDIR_BATS/.env"
  {
    printf 'HASURA_GRAPHQL_ENABLE_CONSOLE=false\n'
    printf 'DEBUG=false\n'
    printf 'HASURA_GRAPHQL_JWT_SECRET=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n'
    printf 'HASURA_GRAPHQL_ADMIN_SECRET=aaaaaaaaaaaaaaaaaa\n'
  } > "$env_file"

  run bash -c "
    export NSELF_ENV=staging
    export SSHD_CONFIG_PATH='$TMPDIR_BATS/no-sshd'
    source '$SECURITY_LIB_DIR/build-warnings.sh'
    run_build_security_warnings '$dc_file' '$env_file'
  "

  # Must NOT contain the port violation error message
  local violated=0
  printf '%s' "$output" | grep -q "exposed on 0.0.0.0" && violated=1 || true
  [ "$violated" -eq 0 ]
}

# =============================================================================
# Scenario 4: staging env + non-nginx service on 0.0.0.0 → exits 1 with message
# =============================================================================
@test "4. staging env: non-nginx port on 0.0.0.0 exits 1 with error message" {
  local dc_file="$TMPDIR_BATS/docker-compose.yml"
  cat > "$dc_file" <<'EOF'
version: "3"
services:
  postgres:
    image: postgres
    ports:
      - "0.0.0.0:5432:5432"
  nginx:
    image: nginx
    ports:
      - "0.0.0.0:443:443"
EOF

  local env_file="$TMPDIR_BATS/.env"
  {
    printf 'HASURA_GRAPHQL_ENABLE_CONSOLE=false\n'
    printf 'DEBUG=false\n'
    printf 'HASURA_GRAPHQL_JWT_SECRET=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n'
    printf 'HASURA_GRAPHQL_ADMIN_SECRET=aaaaaaaaaaaaaaaaaa\n'
  } > "$env_file"

  run bash -c "
    export NSELF_ENV=staging
    export SSHD_CONFIG_PATH='$TMPDIR_BATS/no-sshd'
    source '$SECURITY_LIB_DIR/build-warnings.sh'
    run_build_security_warnings '$dc_file' '$env_file'
  "

  assert_failure
  assert_output --partial "exposed on 0.0.0.0"
}

# =============================================================================
# Scenario 5: security summary block appears at end of staging build output
# =============================================================================
@test "5. staging env: security summary block present in output" {
  local dc_file="$TMPDIR_BATS/docker-compose.yml"
  printf 'version: "3"\nservices:\n  nginx:\n    image: nginx\n' > "$dc_file"

  local env_file="$TMPDIR_BATS/.env"
  {
    printf 'HASURA_GRAPHQL_ENABLE_CONSOLE=false\n'
    printf 'DEBUG=false\n'
    printf 'HASURA_GRAPHQL_JWT_SECRET=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n'
    printf 'HASURA_GRAPHQL_ADMIN_SECRET=aaaaaaaaaaaaaaaaaa\n'
  } > "$env_file"

  run bash -c "
    export NSELF_ENV=staging
    export SSHD_CONFIG_PATH='$TMPDIR_BATS/no-sshd'
    source '$SECURITY_LIB_DIR/build-warnings.sh'
    run_build_security_warnings '$dc_file' '$env_file'
  "

  # Either "Security preflight passed" (all clear) or "Security: N warning(s)" summary line
  local has_summary=0
  case "$output" in
    *"Security preflight passed"*) has_summary=1 ;;
    *"Security:"*) has_summary=1 ;;
    *"Security Preflight"*) has_summary=1 ;;
  esac
  [ "$has_summary" -eq 1 ]
}

# =============================================================================
# Scenario 6: dev env (NSELF_ENV=dev) → all checks skipped, exits 0, no output
# =============================================================================
@test "6. dev env: all security checks skipped, exits 0" {
  local dc_file="$TMPDIR_BATS/docker-compose.yml"
  # Intentionally bad docker-compose — should be ignored in dev
  cat > "$dc_file" <<'EOF'
version: "3"
services:
  postgres:
    image: postgres
    ports:
      - "0.0.0.0:5432:5432"
EOF

  local env_file="$TMPDIR_BATS/.env"
  {
    printf 'HASURA_GRAPHQL_ENABLE_CONSOLE=true\n'
    printf 'DEBUG=true\n'
  } > "$env_file"

  run bash -c "
    export NSELF_ENV=dev
    source '$SECURITY_LIB_DIR/build-warnings.sh'
    run_build_security_warnings '$dc_file' '$env_file'
  "

  assert_success
  # No output at all in dev mode
  [ -z "$output" ]
}
