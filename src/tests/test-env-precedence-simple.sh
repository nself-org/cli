#!/usr/bin/env bash
set -euo pipefail
# Simple test for environment file precedence

echo "Testing Environment File Precedence System"
echo "=========================================="

# Create test directory
TEST_DIR="/tmp/nself-env-test-$$"
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

# Mock log functions
log_debug() { :; }
log_error() { echo "ERROR: $*" >&2; }
log_info() { echo "INFO: $*"; }
log_success() { echo "✓ $*"; }

# Source the env utilities
source "/Users/admin/Sites/nself/src/lib/utils/env.sh"

echo ""
echo "Test 1: Loading .env.dev (team defaults)"
echo "-----------------------------------------"
cat >.env.dev <<'EOF'
PROJECT_NAME=dev-project
BASE_DOMAIN=dev.local
TEST_VAR=from-dev
EOF

load_env_with_priority
echo "PROJECT_NAME = $PROJECT_NAME (expected: dev-project)"
echo "BASE_DOMAIN = $BASE_DOMAIN (expected: dev.local)"
echo "TEST_VAR = $TEST_VAR (expected: from-dev)"

echo ""
echo "Test 2: .env.local overrides .env.dev"
echo "--------------------------------------"
cat >.env.local <<'EOF'
PROJECT_NAME=my-project
TEST_VAR=from-local
LOCAL_ONLY=local-value
EOF

# Reset variables
unset PROJECT_NAME BASE_DOMAIN TEST_VAR LOCAL_ONLY

load_env_with_priority
echo "PROJECT_NAME = $PROJECT_NAME (expected: my-project)"
echo "BASE_DOMAIN = $BASE_DOMAIN (expected: dev.local)"
echo "TEST_VAR = $TEST_VAR (expected: from-local)"
echo "LOCAL_ONLY = $LOCAL_ONLY (expected: local-value)"

echo ""
echo "Test 3: .env overrides everything"
echo "----------------------------------"
cat >.env <<'EOF'
PROJECT_NAME=production
BASE_DOMAIN=prod.com
OVERRIDE=true
EOF

# Reset variables
unset PROJECT_NAME BASE_DOMAIN TEST_VAR LOCAL_ONLY OVERRIDE

load_env_with_priority
echo "PROJECT_NAME = $PROJECT_NAME (expected: production)"
echo "BASE_DOMAIN = $BASE_DOMAIN (expected: prod.com)"
echo "TEST_VAR = $TEST_VAR (expected: empty, .env overrides all)"
echo "OVERRIDE = $OVERRIDE (expected: true)"

echo ""
echo "Test 4: .env.secrets always loaded"
echo "-----------------------------------"
rm -f .env # Remove override

cat >.env.secrets <<'EOF'
DB_PASSWORD=secret123
API_KEY=key456
EOF

# Reset variables
unset PROJECT_NAME BASE_DOMAIN TEST_VAR LOCAL_ONLY OVERRIDE DB_PASSWORD API_KEY

load_env_with_priority
echo "DB_PASSWORD = $DB_PASSWORD (expected: secret123)"
echo "API_KEY = $API_KEY (expected: key456)"
echo "PROJECT_NAME = $PROJECT_NAME (expected: my-project, from .env.local)"

echo ""
echo "Test 5: Environment-specific files (staging)"
echo "---------------------------------------------"
rm -f .env.local
export ENV=staging

cat >.env.staging <<'EOF'
PROJECT_NAME=staging-project
BASE_DOMAIN=staging.company.com
STAGING_VAR=staging-only
EOF

# Reset variables
unset PROJECT_NAME BASE_DOMAIN TEST_VAR STAGING_VAR

load_env_with_priority
echo "ENV = $ENV"
echo "PROJECT_NAME = $PROJECT_NAME (expected: staging-project)"
echo "BASE_DOMAIN = $BASE_DOMAIN (expected: staging.company.com)"
echo "TEST_VAR = $TEST_VAR (expected: from-dev)"
echo "STAGING_VAR = $STAGING_VAR (expected: staging-only)"

# Cleanup
cd /
rm -rf "$TEST_DIR"

echo ""
echo "✅ Environment precedence tests completed!"
echo ""
echo "Precedence order:"
echo "1. .env.secrets (always loaded)"
echo "2. .env (overrides all if exists)"
echo "3. .env.local (personal overrides)"
echo "4. .env.\${ENV} (environment-specific)"
echo "5. .env.dev (team defaults)"
