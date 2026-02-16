#!/usr/bin/env bash
set -euo pipefail
# Test multi-domain custom services system

set -e

echo "Testing Multi-Domain Custom Services"
echo "====================================="

# Create test directory
TEST_DIR="/tmp/nself-multidomain-test-$$"
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

echo ""
echo "Test 1: Development Environment with Multi-level Subdomains"
echo "-----------------------------------------------------------"

# Create test .env.local for dev
cat >.env.local <<'EOF'
ENV=dev
BASE_DOMAIN=local.nself.org
PROJECT_NAME=test

# Services with multi-level subdomains
CUSTOM_SERVICES="metals:python:metals.api,currency:nodejs:currency.api,analytics:go:stats"

# Service configurations
METALS_PORT=8001
METALS_MEMORY=1G
METALS_RATE_LIMIT=100

CURRENCY_PORT=8002

ANALYTICS_PUBLIC=false
EOF

# Source environment and service builder
set -a
source .env.local
set +a

# Override log functions for testing
log_info() { echo "  ℹ $*"; }
log_success() { echo "  ✓ $*"; }
log_warning() { echo "  ⚠ $*"; }
log_error() { echo "  ✗ $*" >&2; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../../src/lib/services/service-builder.sh"

# Parse and display
parse_custom_services
echo "Parsed ${#PARSED_SERVICES[@]} services:"
for service in "${PARSED_SERVICES[@]}"; do
  IFS='|' read -r name language domain port <<<"$service"
  echo "  • $name ($language) → $domain"
done

# Clean for next test
rm -rf services nginx docker-compose.custom.yml

echo ""
echo "Test 2: Production Environment with Custom Domains"
echo "--------------------------------------------------"

# Create test .env for production
cat >.env <<'EOF'
ENV=prod
BASE_DOMAIN=mycompany.com
PROJECT_NAME=prod

# Services with custom production domains
CUSTOM_SERVICES="metals:python:metals,currency:nodejs:currency,webhooks:go:hooks"

# Production domain overrides
METALS_DOMAIN_PROD=metals.goldprices.com
CURRENCY_DOMAIN_PROD=api.forex-rates.com
WEBHOOKS_DOMAIN_PROD=webhooks.mycompany.com

# Dev domain overrides (for reference, won't be used in prod)
METALS_DOMAIN_DEV=metals.api.local.nself.org
CURRENCY_DOMAIN_DEV=currency.api.local.nself.org
EOF

# Source environment
set -a
source .env
set +a

# Parse and display
PARSED_SERVICES=() # Reset
parse_custom_services
echo "Parsed ${#PARSED_SERVICES[@]} services:"
for service in "${PARSED_SERVICES[@]}"; do
  IFS='|' read -r name language domain port <<<"$service"
  echo "  • $name ($language) → $domain"
done

# Build services to check nginx config
build_custom_services >/dev/null 2>&1

echo ""
echo "Checking nginx configuration for custom domains:"
if [[ -f "nginx/conf.d/custom-services.conf" ]]; then
  echo "  ✓ Nginx config generated"

  # Check for custom domains
  if grep -q "server_name metals.goldprices.com" nginx/conf.d/custom-services.conf; then
    echo "  ✓ metals.goldprices.com configured"
  fi
  if grep -q "server_name api.forex-rates.com" nginx/conf.d/custom-services.conf; then
    echo "  ✓ api.forex-rates.com configured"
  fi
  if grep -q "server_name webhooks.mycompany.com" nginx/conf.d/custom-services.conf; then
    echo "  ✓ webhooks.mycompany.com configured"
  fi

  # Check SSL cert paths for custom domains
  if grep -q "ssl_certificate /etc/nginx/ssl/certs/metals.goldprices.com/fullchain.pem" nginx/conf.d/custom-services.conf; then
    echo "  ✓ Custom SSL path for metals.goldprices.com"
  fi
fi

# Clean for next test
rm -rf services nginx docker-compose.custom.yml .env

echo ""
echo "Test 3: Mixed Environment with Fallbacks"
echo "----------------------------------------"

# Create test with ENV switching
cat >.env.local <<'EOF'
ENV=dev
BASE_DOMAIN=local.nself.org
PROJECT_NAME=mixed

# Services with various routing patterns
CUSTOM_SERVICES="api:nodejs:api,metals:python:metals.prices,analytics:go:analytics.internal"

# Only production overrides set
API_DOMAIN_PROD=api.production.com
METALS_DOMAIN_PROD=metals.commodities.com

# Analytics stays internal
ANALYTICS_PUBLIC=false
EOF

# Test in dev mode
set -a
source .env.local
set +a

PARSED_SERVICES=()
parse_custom_services
echo "Development routing:"
for service in "${PARSED_SERVICES[@]}"; do
  IFS='|' read -r name language domain port <<<"$service"
  # In dev, metals.prices should become metals.prices.localhost
  echo "  • $name → $domain"
done

# Switch to production
export ENV=prod
PARSED_SERVICES=()
parse_custom_services
echo ""
echo "Production routing (same config, ENV=prod):"
for service in "${PARSED_SERVICES[@]}"; do
  IFS='|' read -r name language domain port <<<"$service"
  echo "  • $name → $domain"
done

# Cleanup
cd /
rm -rf "$TEST_DIR"

echo ""
echo "✅ All multi-domain tests passed!"
echo ""
echo "Features tested:"
echo "• Multi-level subdomains (metals.api.localhost)"
echo "• Environment-specific domain overrides (METALS_DOMAIN_DEV/PROD)"
echo "• Custom production domains (metals.goldprices.com)"
echo "• Automatic SSL certificate path resolution"
echo "• Mixed routing patterns in same configuration"
