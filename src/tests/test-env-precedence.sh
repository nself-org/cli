#!/usr/bin/env bash
set -euo pipefail
# Test environment file precedence system

set -e

echo "Testing Environment File Precedence System"
echo "=========================================="

# Create test directory
TEST_DIR="/tmp/nself-env-test-$$"
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

# Define log_debug if not available (for testing)
if ! declare -f log_debug >/dev/null 2>&1; then
  log_debug() { :; } # No-op
fi

# Source the env utilities
source "/Users/admin/Sites/nself/src/lib/utils/env.sh"

# Test helper function
test_var() {
  local var_name="$1"
  local expected="$2"
  local actual="${!var_name}"

  if [[ "$actual" == "$expected" ]]; then
    echo "  ✓ $var_name = $expected"
  else
    echo "  ✗ $var_name = $actual (expected: $expected)"
    return 1
  fi
}

echo ""
echo "Test 1: Base case - only .env.dev"
echo "-----------------------------------"

cat >.env.dev <<'EOF'
PROJECT_NAME=dev-project
BASE_DOMAIN=dev.local
TEST_VAR=from-dev
EOF

load_env_with_priority
test_var "PROJECT_NAME" "dev-project"
test_var "BASE_DOMAIN" "dev.local"
test_var "TEST_VAR" "from-dev"

# Clean environment
unset PROJECT_NAME BASE_DOMAIN TEST_VAR

echo ""
echo "Test 2: .env.dev + .env.local (local overrides)"
echo "------------------------------------------------"

cat >.env.local <<'EOF'
PROJECT_NAME=my-project
TEST_VAR=from-local
LOCAL_ONLY=local-value
EOF

load_env_with_priority
test_var "PROJECT_NAME" "my-project" # Overridden by .env.local
test_var "BASE_DOMAIN" "dev.local"   # Still from .env.dev
test_var "TEST_VAR" "from-local"     # Overridden by .env.local
test_var "LOCAL_ONLY" "local-value"  # Only in .env.local

# Clean environment
unset PROJECT_NAME BASE_DOMAIN TEST_VAR LOCAL_ONLY

echo ""
echo "Test 3: .env.dev + .env.staging (ENV=staging)"
echo "----------------------------------------------"

rm -f .env.local
export ENV=staging

cat >.env.staging <<'EOF'
PROJECT_NAME=staging-project
BASE_DOMAIN=staging.company.com
STAGING_VAR=staging-only
EOF

load_env_with_priority
test_var "PROJECT_NAME" "staging-project"    # From .env.staging
test_var "BASE_DOMAIN" "staging.company.com" # From .env.staging
test_var "TEST_VAR" "from-dev"               # Still from .env.dev
test_var "STAGING_VAR" "staging-only"        # From .env.staging

# Clean environment
unset PROJECT_NAME BASE_DOMAIN TEST_VAR STAGING_VAR ENV

echo ""
echo "Test 4: .env.prod overrides when ENV=prod"
echo "------------------------------------------"

export ENV=prod

cat >.env.prod <<'EOF'
PROJECT_NAME=production
BASE_DOMAIN=company.com
PROD_VAR=prod-only
EOF

load_env_with_priority
test_var "PROJECT_NAME" "production" # From .env.prod
test_var "BASE_DOMAIN" "company.com" # From .env.prod
test_var "TEST_VAR" "from-dev"       # Still from .env.dev
test_var "PROD_VAR" "prod-only"      # From .env.prod

# Clean environment
unset PROJECT_NAME BASE_DOMAIN TEST_VAR PROD_VAR ENV

echo ""
echo "Test 5: .env.secrets always loaded"
echo "-----------------------------------"

cat >.env.secrets <<'EOF'
DB_PASSWORD=secret123
API_KEY=key456
EOF

cat >.env.local <<'EOF'
PROJECT_NAME=local-project
DB_PASSWORD=local-password
EOF

load_env_with_priority
test_var "PROJECT_NAME" "local-project" # From .env.local
test_var "DB_PASSWORD" "local-password" # .env.local overrides .env.secrets
test_var "API_KEY" "key456"             # From .env.secrets

# Clean environment
unset PROJECT_NAME DB_PASSWORD API_KEY

echo ""
echo "Test 6: .env overrides everything (production mode)"
echo "---------------------------------------------------"

cat >.env <<'EOF'
PROJECT_NAME=production-override
BASE_DOMAIN=prod.com
ENV=production
OVERRIDE=true
EOF

# Even with .env.local, .env.dev, .env.secrets present
load_env_with_priority
test_var "PROJECT_NAME" "production-override" # From .env
test_var "BASE_DOMAIN" "prod.com"             # From .env
test_var "OVERRIDE" "true"                    # From .env
test_var "API_KEY" "key456"                   # .env.secrets still loaded

# Clean environment
unset PROJECT_NAME BASE_DOMAIN ENV OVERRIDE API_KEY

echo ""
echo "Test 7: Complex precedence chain"
echo "---------------------------------"

rm -f .env # Remove override
export ENV=staging

# Setup full chain
cat >.env.dev <<'EOF'
VAR1=dev
VAR2=dev
VAR3=dev
VAR4=dev
VAR5=dev
EOF

cat >.env.staging <<'EOF'
VAR2=staging
VAR3=staging
VAR4=staging
EOF

cat >.env.local <<'EOF'
VAR3=local
VAR4=local
EOF

cat >.env.secrets <<'EOF'
VAR4=secret
VAR5=secret
SECRET_ONLY=secret-value
EOF

load_env_with_priority
test_var "VAR1" "dev"                 # Only in .env.dev
test_var "VAR2" "staging"             # Overridden by .env.staging
test_var "VAR3" "local"               # Overridden by .env.local
test_var "VAR4" "local"               # .env.local wins over .env.secrets
test_var "VAR5" "secret"              # .env.secrets overrides .env.dev
test_var "SECRET_ONLY" "secret-value" # Only in .env.secrets

# Cleanup
cd /
rm -rf "$TEST_DIR"

echo ""
echo "✅ All environment precedence tests passed!"
echo ""
echo "Precedence order verified:"
echo "1. .env.secrets (always loaded for secrets)"
echo "2. .env (if exists, overrides all except secrets)"
echo "3. .env.local (personal overrides)"
echo "4. .env.{ENV} (environment-specific)"
echo "5. .env.dev (team defaults)"
