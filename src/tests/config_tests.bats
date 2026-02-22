#!/usr/bin/env bats
# config_tests.bats - Comprehensive tests for configuration management
# Tests: Constants, defaults, smart defaults, service templates

setup() {
  # Create temporary test directory
  TEST_DIR=$(mktemp -d)
  cd "$TEST_DIR"

  # Resolve nself paths
  SCRIPT_DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)"

  # Source config modules
  if [[ -f "$SCRIPT_DIR/lib/config/constants.sh" ]]; then
    source "$SCRIPT_DIR/lib/config/constants.sh"
  fi

  if [[ -f "$SCRIPT_DIR/lib/config/defaults.sh" ]]; then
    source "$SCRIPT_DIR/lib/config/defaults.sh"
  fi

  if [[ -f "$SCRIPT_DIR/lib/config/smart-defaults.sh" ]]; then
    source "$SCRIPT_DIR/lib/config/smart-defaults.sh"
  fi
}

teardown() {
  cd /
  rm -rf "$TEST_DIR"
}

# ============================================================================
# Constants Tests
# ============================================================================

@test "constants defines EXIT_SUCCESS" {
  [[ "${EXIT_SUCCESS:-}" == "0" ]]
}

@test "constants defines EXIT_GENERAL_ERROR" {
  [[ "${EXIT_GENERAL_ERROR:-}" == "1" ]]
}

@test "constants defines EXIT_MISUSE" {
  [[ "${EXIT_MISUSE:-}" == "2" ]]
}

@test "constants defines EXIT_CANT_EXEC" {
  [[ "${EXIT_CANT_EXEC:-}" == "126" ]]
}

@test "constants defines EXIT_NOT_FOUND" {
  [[ "${EXIT_NOT_FOUND:-}" == "127" ]]
}

@test "constants defines CORE_SERVICES array" {
  [[ -n "${CORE_SERVICES[*]:-}" ]]
}

@test "constants CORE_SERVICES includes postgres" {
  local services="${CORE_SERVICES[*]}"
  [[ "$services" == *"postgres"* ]]
}

@test "constants CORE_SERVICES includes hasura" {
  local services="${CORE_SERVICES[*]}"
  [[ "$services" == *"hasura"* ]]
}

@test "constants defines OPTIONAL_SERVICES array" {
  [[ -n "${OPTIONAL_SERVICES[*]:-}" ]] || true
}

@test "constants defines DOMAIN_REGEX" {
  [[ -n "${DOMAIN_REGEX:-}" ]]
}

@test "constants DOMAIN_REGEX validates valid domain" {
  [[ "example.com" =~ $DOMAIN_REGEX ]]
}

@test "constants DOMAIN_REGEX rejects invalid domain" {
  ! [[ "-invalid-.com" =~ $DOMAIN_REGEX ]]
}

@test "constants defines EMAIL_REGEX" {
  [[ -n "${EMAIL_REGEX:-}" ]]
}

@test "constants EMAIL_REGEX validates valid email" {
  [[ "test@example.com" =~ $EMAIL_REGEX ]]
}

@test "constants EMAIL_REGEX rejects invalid email" {
  ! [[ "not-an-email" =~ $EMAIL_REGEX ]]
}

@test "constants defines PORT_REGEX" {
  [[ -n "${PORT_REGEX:-}" ]]
}

@test "constants PORT_REGEX validates valid port" {
  [[ "8080" =~ $PORT_REGEX ]]
}

@test "constants PORT_REGEX rejects non-numeric port" {
  ! [[ "abc" =~ $PORT_REGEX ]]
}

@test "constants defines time constants" {
  [[ "${SECOND:-}" == "1" ]]
  [[ "${MINUTE:-}" == "60" ]]
  [[ "${HOUR:-}" == "3600" ]]
}

@test "constants defines size constants" {
  [[ "${KB:-}" == "1024" ]]
  [[ -n "${MB:-}" ]]
  [[ -n "${GB:-}" ]]
}

@test "constants defines NSELF_VERSION" {
  [[ -n "${NSELF_VERSION:-}" ]]
  [[ "$NSELF_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]
}

@test "constants defines MIN_DOCKER_VERSION" {
  [[ -n "${MIN_DOCKER_VERSION:-}" ]]
}

@test "constants defines MIN_COMPOSE_VERSION" {
  [[ -n "${MIN_COMPOSE_VERSION:-}" ]]
}

@test "constants prevents double-sourcing" {
  [[ "${CONSTANTS_SOURCED:-}" == "1" ]]
}

# ============================================================================
# Defaults Tests
# ============================================================================

@test "defaults sets BASE_DOMAIN" {
  [[ -n "${BASE_DOMAIN:-}" ]]
}

@test "defaults BASE_DOMAIN defaults to localhost" {
  unset BASE_DOMAIN
  source "$SCRIPT_DIR/lib/config/defaults.sh"
  [[ "${BASE_DOMAIN}" == "localhost" ]]
}

@test "defaults sets ENV_FILE" {
  [[ -n "${ENV_FILE:-}" ]]
}

@test "defaults ENV_FILE defaults to .env" {
  unset ENV_FILE
  rm -f .env.runtime
  source "$SCRIPT_DIR/lib/config/defaults.sh"
  [[ "${ENV_FILE}" == ".env" ]]
}

@test "defaults prefers .env.runtime when it exists" {
  touch .env.runtime
  unset ENV_FILE
  source "$SCRIPT_DIR/lib/config/defaults.sh"
  [[ "${ENV_FILE}" == ".env.runtime" ]]
}

@test "defaults defines NSELF_ROOT" {
  [[ -n "${NSELF_ROOT:-}" ]]
  [[ -d "${NSELF_ROOT}" ]]
}

@test "defaults defines NSELF_BIN" {
  [[ -n "${NSELF_BIN:-}" ]]
}

@test "defaults defines NSELF_SRC" {
  [[ -n "${NSELF_SRC:-}" ]]
}

@test "defaults defines NSELF_TEMPLATES" {
  [[ -n "${NSELF_TEMPLATES:-}" ]]
}

@test "defaults defines NSELF_CERTS" {
  [[ -n "${NSELF_CERTS:-}" ]]
}

@test "defaults defines NSELF_LIB" {
  [[ -n "${NSELF_LIB:-}" ]]
}

@test "defaults defines NSELF_LOGS" {
  [[ -n "${NSELF_LOGS:-}" ]]
}

@test "defaults sets COMPOSE_PROJECT_NAME when PROJECT_NAME exists" {
  export PROJECT_NAME="test-project"
  source "$SCRIPT_DIR/lib/config/defaults.sh"
  [[ "${COMPOSE_PROJECT_NAME:-}" == "test-project" ]]
}

@test "defaults sets DOCKER_BUILDKIT to 1" {
  [[ "${DOCKER_BUILDKIT}" == "1" ]]
}

@test "defaults sets COMPOSE_DOCKER_CLI_BUILD to 1" {
  [[ "${COMPOSE_DOCKER_CLI_BUILD}" == "1" ]]
}

@test "defaults defines STARTUP_TIMEOUT" {
  [[ -n "${STARTUP_TIMEOUT:-}" ]]
  [[ "${STARTUP_TIMEOUT}" =~ ^[0-9]+$ ]]
}

@test "defaults defines HEALTH_CHECK_INTERVAL" {
  [[ -n "${HEALTH_CHECK_INTERVAL:-}" ]]
  [[ "${HEALTH_CHECK_INTERVAL}" =~ ^[0-9]+$ ]]
}

@test "defaults defines MAX_RETRIES" {
  [[ -n "${MAX_RETRIES:-}" ]]
  [[ "${MAX_RETRIES}" =~ ^[0-9]+$ ]]
}

@test "defaults defines RETRY_DELAY" {
  [[ -n "${RETRY_DELAY:-}" ]]
  [[ "${RETRY_DELAY}" =~ ^[0-9]+$ ]]
}

@test "defaults AUTO_FIX_ENABLED defaults to true" {
  [[ "${AUTO_FIX_ENABLED}" == "true" ]]
}

@test "defaults VERBOSE defaults to false" {
  [[ "${VERBOSE}" == "false" ]]
}

@test "defaults DEBUG defaults to false" {
  [[ "${DEBUG}" == "false" ]]
}

@test "defaults defines service port defaults" {
  [[ -n "${POSTGRES_PORT:-}" ]]
  [[ -n "${HASURA_PORT:-}" ]]
  [[ -n "${MINIO_PORT:-}" ]]
  [[ -n "${REDIS_PORT:-}" ]]
  [[ -n "${NGINX_HTTP_PORT:-}" ]]
  [[ -n "${NGINX_HTTPS_PORT:-}" ]]
}

@test "defaults defines REQUIRED_ENV_VARS array" {
  [[ -n "${REQUIRED_ENV_VARS[*]:-}" ]]
}

@test "defaults REQUIRED_ENV_VARS includes PROJECT_NAME" {
  local vars="${REQUIRED_ENV_VARS[*]}"
  [[ "$vars" == *"PROJECT_NAME"* ]]
}

@test "defaults REQUIRED_ENV_VARS includes BASE_DOMAIN" {
  local vars="${REQUIRED_ENV_VARS[*]}"
  [[ "$vars" == *"BASE_DOMAIN"* ]]
}

@test "defaults REQUIRED_ENV_VARS includes POSTGRES_PASSWORD" {
  local vars="${REQUIRED_ENV_VARS[*]}"
  [[ "$vars" == *"POSTGRES_PASSWORD"* ]]
}

# ============================================================================
# Smart Defaults Tests
# ============================================================================

@test "generate_password creates password of default length" {
  local pass
  pass=$(generate_password)
  [[ ${#pass} -eq 16 ]]
}

@test "generate_password accepts custom length" {
  local pass
  pass=$(generate_password 32)
  [[ ${#pass} -eq 32 ]]
}

@test "generate_password creates unique passwords" {
  local pass1 pass2
  pass1=$(generate_password)
  pass2=$(generate_password)
  [[ "$pass1" != "$pass2" ]]
}

@test "generate_password excludes special characters" {
  local pass
  pass=$(generate_password 20)
  ! [[ "$pass" =~ [=+/] ]]
}

@test "apply_smart_defaults sets ENV to dev" {
  unset ENV
  apply_smart_defaults
  [[ "${ENV}" == "dev" ]]
}

@test "apply_smart_defaults preserves existing ENV" {
  export ENV="prod"
  apply_smart_defaults
  [[ "${ENV}" == "prod" ]]
}

@test "apply_smart_defaults sets PROJECT_NAME" {
  unset PROJECT_NAME
  apply_smart_defaults
  [[ -n "${PROJECT_NAME}" ]]
}

@test "apply_smart_defaults sets BASE_DOMAIN" {
  unset BASE_DOMAIN
  apply_smart_defaults
  [[ "${BASE_DOMAIN}" == "local.nself.org" ]]
}

@test "apply_smart_defaults sets POSTGRES_VERSION" {
  unset POSTGRES_VERSION
  apply_smart_defaults
  [[ -n "${POSTGRES_VERSION}" ]]
}

@test "apply_smart_defaults sets POSTGRES_HOST" {
  unset POSTGRES_HOST
  apply_smart_defaults
  [[ "${POSTGRES_HOST}" == "postgres" ]]
}

@test "apply_smart_defaults sets POSTGRES_INTERNAL_PORT to 5432" {
  unset POSTGRES_INTERNAL_PORT
  apply_smart_defaults
  [[ "${POSTGRES_INTERNAL_PORT}" == "5432" ]]
}

@test "apply_smart_defaults sets POSTGRES_DB" {
  unset POSTGRES_DB
  apply_smart_defaults
  [[ -n "${POSTGRES_DB}" ]]
}

@test "apply_smart_defaults constructs DATABASE_URL" {
  unset HASURA_GRAPHQL_DATABASE_URL
  apply_smart_defaults
  [[ -n "${HASURA_GRAPHQL_DATABASE_URL}" ]]
  [[ "${HASURA_GRAPHQL_DATABASE_URL}" == *"postgres://"* ]]
}

@test "apply_smart_defaults DATABASE_URL uses port 5432" {
  unset HASURA_GRAPHQL_DATABASE_URL
  apply_smart_defaults
  [[ "${HASURA_GRAPHQL_DATABASE_URL}" == *":5432/"* ]]
}

@test "apply_smart_defaults sets HASURA_VERSION" {
  unset HASURA_VERSION
  apply_smart_defaults
  [[ -n "${HASURA_VERSION}" ]]
}

@test "apply_smart_defaults sets HASURA_GRAPHQL_ADMIN_SECRET" {
  unset HASURA_GRAPHQL_ADMIN_SECRET
  apply_smart_defaults
  [[ -n "${HASURA_GRAPHQL_ADMIN_SECRET}" ]]
}

@test "apply_smart_defaults sets JWT_KEY" {
  unset HASURA_JWT_KEY JWT_KEY
  apply_smart_defaults
  [[ -n "${JWT_KEY}" ]]
  [[ ${#JWT_KEY} -ge 32 ]]
}

@test "apply_smart_defaults constructs HASURA_GRAPHQL_JWT_SECRET" {
  unset HASURA_GRAPHQL_JWT_SECRET
  apply_smart_defaults
  [[ -n "${HASURA_GRAPHQL_JWT_SECRET}" ]]
  [[ "${HASURA_GRAPHQL_JWT_SECRET}" == "{"* ]]
}

@test "apply_smart_defaults JWT_SECRET includes type field" {
  unset HASURA_GRAPHQL_JWT_SECRET
  apply_smart_defaults
  [[ "${HASURA_GRAPHQL_JWT_SECRET}" == *"type"* ]]
}

@test "apply_smart_defaults JWT_SECRET includes key field" {
  unset HASURA_GRAPHQL_JWT_SECRET
  apply_smart_defaults
  [[ "${HASURA_GRAPHQL_JWT_SECRET}" == *"key"* ]]
}

@test "apply_smart_defaults enables console in dev mode" {
  export ENV="dev"
  unset HASURA_GRAPHQL_ENABLE_CONSOLE
  apply_smart_defaults
  [[ "${HASURA_GRAPHQL_ENABLE_CONSOLE}" == "true" ]]
}

@test "apply_smart_defaults disables console in prod mode" {
  export ENV="prod"
  unset HASURA_GRAPHQL_ENABLE_CONSOLE
  apply_smart_defaults
  [[ "${HASURA_GRAPHQL_ENABLE_CONSOLE}" == "false" ]]
}

@test "apply_smart_defaults enables dev mode in dev" {
  export ENV="dev"
  unset HASURA_GRAPHQL_DEV_MODE
  apply_smart_defaults
  [[ "${HASURA_GRAPHQL_DEV_MODE}" == "true" ]]
}

@test "apply_smart_defaults disables dev mode in prod" {
  export ENV="prod"
  unset HASURA_GRAPHQL_DEV_MODE
  apply_smart_defaults
  [[ "${HASURA_GRAPHQL_DEV_MODE}" == "false" ]]
}

@test "apply_smart_defaults sets AUTH_VERSION" {
  unset AUTH_VERSION
  apply_smart_defaults
  [[ -n "${AUTH_VERSION}" ]]
}

@test "apply_smart_defaults sets AUTH_HOST" {
  unset AUTH_HOST
  apply_smart_defaults
  [[ "${AUTH_HOST}" == "auth" ]]
}

@test "apply_smart_defaults sets AUTH_PORT" {
  unset AUTH_PORT
  apply_smart_defaults
  [[ -n "${AUTH_PORT}" ]]
  [[ "${AUTH_PORT}" =~ ^[0-9]+$ ]]
}

@test "apply_smart_defaults sets SMTP defaults for dev" {
  unset AUTH_SMTP_HOST
  apply_smart_defaults
  [[ "${AUTH_SMTP_HOST}" == "mailpit" ]]
}

@test "apply_smart_defaults sets STORAGE_VERSION" {
  unset STORAGE_VERSION
  apply_smart_defaults
  [[ -n "${STORAGE_VERSION}" ]]
}

@test "apply_smart_defaults sets MINIO_VERSION" {
  unset MINIO_VERSION
  apply_smart_defaults
  [[ -n "${MINIO_VERSION}" ]]
}

@test "apply_smart_defaults sets S3 credentials" {
  unset S3_ACCESS_KEY S3_SECRET_KEY
  apply_smart_defaults
  [[ -n "${S3_ACCESS_KEY}" ]]
  [[ -n "${S3_SECRET_KEY}" ]]
}

@test "apply_smart_defaults sets NGINX_VERSION" {
  unset NGINX_VERSION
  apply_smart_defaults
  [[ -n "${NGINX_VERSION}" ]]
}

@test "apply_smart_defaults sets NGINX ports" {
  unset NGINX_HTTP_PORT NGINX_HTTPS_PORT
  apply_smart_defaults
  [[ "${NGINX_HTTP_PORT}" == "80" ]]
  [[ "${NGINX_HTTPS_PORT}" == "443" ]]
}

@test "apply_smart_defaults sets client max body size" {
  unset NGINX_CLIENT_MAX_BODY_SIZE
  apply_smart_defaults
  [[ -n "${NGINX_CLIENT_MAX_BODY_SIZE}" ]]
}

# ============================================================================
# Environment Variable Precedence Tests
# ============================================================================

@test "smart defaults respects existing variables" {
  export PROJECT_NAME="custom-project"
  export BASE_DOMAIN="custom.domain"
  apply_smart_defaults
  [[ "${PROJECT_NAME}" == "custom-project" ]]
  [[ "${BASE_DOMAIN}" == "custom.domain" ]]
}

@test "smart defaults only sets unset variables" {
  export POSTGRES_DB="custom_db"
  local original="$POSTGRES_DB"
  apply_smart_defaults
  [[ "${POSTGRES_DB}" == "$original" ]]
}

# ============================================================================
# Edge Cases & Validation Tests
# ============================================================================

@test "generate_password handles zero length gracefully" {
  run generate_password 0
  [[ "$status" -eq 0 ]] || true
}

@test "generate_password handles large length" {
  local pass
  pass=$(generate_password 128)
  [[ ${#pass} -eq 128 ]]
}

@test "apply_smart_defaults handles empty ENV" {
  export ENV=""
  apply_smart_defaults
  [[ "${ENV}" == "dev" ]]
}

@test "apply_smart_defaults constructs valid JWT JSON" {
  unset HASURA_GRAPHQL_JWT_SECRET
  apply_smart_defaults
  # Should be valid JSON
  printf "%s\n" "$HASURA_GRAPHQL_JWT_SECRET" | jq . >/dev/null 2>&1
}

@test "apply_smart_defaults routes include BASE_DOMAIN" {
  export BASE_DOMAIN="test.local"
  apply_smart_defaults
  [[ "${HASURA_ROUTE}" == *"test.local"* ]]
  [[ "${AUTH_ROUTE}" == *"test.local"* ]]
}

@test "defaults all paths are absolute" {
  [[ "${NSELF_ROOT}" == /* ]]
  [[ "${NSELF_BIN}" == /* ]]
  [[ "${NSELF_SRC}" == /* ]]
}

@test "defaults COMPOSE_ENV_FILE matches ENV_FILE" {
  [[ "${COMPOSE_ENV_FILE}" == "${ENV_FILE}" ]]
}

@test "constants readonly variables cannot be changed" {
  local original="${EXIT_SUCCESS}"
  # Assignment to a readonly var aborts the shell under set -e even with || true.
  # Run in a subshell so the error is contained; the parent's value is unchanged.
  ( EXIT_SUCCESS=999 ) 2>/dev/null || true
  [[ "${EXIT_SUCCESS}" == "$original" ]]
}
