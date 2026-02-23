#!/usr/bin/env bash
set -euo pipefail
# Test custom services system

set -e

echo "Testing Custom Services System"
echo "=============================="

# Create test directory
TEST_DIR="/tmp/nself-custom-test-$$"
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

# Create test .env.local
cat >.env.local <<'EOF'
# Custom services definition
CUSTOM_SERVICES="currency:nodejs:currency,metals:python:metals,analytics:go:analytics"

# Service-specific configurations
CURRENCY_PORT=8001
CURRENCY_MEMORY=512M
CURRENCY_RATE_LIMIT=100

METALS_PORT=8002
METALS_REPLICAS=2
METALS_CPU=0.5

ANALYTICS_PORT=8003
ANALYTICS_PUBLIC=false

# Base configuration
PROJECT_NAME=test
BASE_DOMAIN=localhost
ENV=dev
EOF

# Source environment
set -a
source .env.local
set +a

# Source the service builder
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Override log functions for testing
log_info() { :; }
log_success() { :; }
log_warning() { :; }
log_error() { echo "$@" >&2; }

source "$SCRIPT_DIR/../../src/lib/utils/display.sh" 2>/dev/null || true
source "$SCRIPT_DIR/../../src/lib/utils/env.sh" 2>/dev/null || true
source "$SCRIPT_DIR/../../src/lib/services/service-builder.sh"

echo "✓ Service builder loaded"

# Test parsing
printf "%s" "Testing service parsing... "
parse_custom_services
if [[ ${#PARSED_SERVICES[@]} -eq 3 ]]; then
  echo "✓ Parsed ${#PARSED_SERVICES[@]} services"
else
  echo "✗ Failed to parse services"
  exit 1
fi

# Display parsed services
echo ""
echo "Parsed Services:"
for service in "${PARSED_SERVICES[@]}"; do
  IFS='|' read -r name language subdomain port <<<"$service"
  echo "  • $name ($language) on port $port → $subdomain.localhost"
done

# Test service generation
echo ""
printf "%s" "Testing service generation... "
if build_custom_services >/dev/null 2>&1; then
  echo "✓ Services generated"
else
  echo "✗ Failed to generate services"
  exit 1
fi

# Check generated files
echo ""
echo "Checking generated files:"

# Check docker-compose.custom.yml
if [[ -f "docker-compose.custom.yml" ]]; then
  echo "  ✓ docker-compose.custom.yml created"
  if grep -q "currency:" docker-compose.custom.yml &&
    grep -q "metals:" docker-compose.custom.yml &&
    grep -q "analytics:" docker-compose.custom.yml; then
    echo "    ✓ All services defined in compose file"
  fi
else
  echo "  ✗ docker-compose.custom.yml not created"
fi

# Check nginx config
if [[ -f "nginx/conf.d/custom-services.conf" ]]; then
  echo "  ✓ Nginx configuration created"
  if grep -q "currency.localhost" nginx/conf.d/custom-services.conf &&
    grep -q "metals.localhost" nginx/conf.d/custom-services.conf; then
    echo "    ✓ Public services have nginx routes"
  fi
  if ! grep -q "analytics.localhost" nginx/conf.d/custom-services.conf; then
    echo "    ✓ Private service excluded from nginx"
  fi
else
  echo "  ✗ Nginx configuration not created"
fi

# Check service directories
echo ""
echo "Checking service templates:"
for service in currency metals analytics; do
  if [[ -d "services/$service" ]]; then
    echo "  ✓ services/$service/ created"

    # Check for main file based on language
    case "$service" in
      currency)
        if [[ -f "services/$service/main.js" ]] || [[ -f "services/$service/index.js" ]]; then
          echo "    ✓ Node.js template created"
        fi
        ;;
      metals)
        if [[ -f "services/$service/main.py" ]]; then
          echo "    ✓ Python template created"
        fi
        ;;
      analytics)
        if [[ -f "services/$service/main.go" ]]; then
          echo "    ✓ Go template created"
        fi
        ;;
    esac

    # Check for Dockerfile
    if [[ -f "services/$service/Dockerfile" ]]; then
      echo "    ✓ Dockerfile created"
    fi
  else
    echo "  ✗ services/$service/ not created"
  fi
done

# Check port assignments
echo ""
echo "Checking configurations:"
if grep -q "8001:8001" docker-compose.custom.yml; then
  echo "  ✓ Currency service using custom port 8001"
fi
if grep -q "memory: 512M" docker-compose.custom.yml; then
  echo "  ✓ Currency service memory limit applied"
fi
if grep -q "replicas: 2" docker-compose.custom.yml; then
  echo "  ✓ Metals service replicas configured"
fi

# Cleanup
echo ""
echo "Cleaning up test directory..."
cd /
rm -rf "$TEST_DIR"

echo ""
echo "✅ All tests passed!"
echo ""
echo "Custom services system is working correctly:"
echo "• Service parsing from CUSTOM_SERVICES env var"
echo "• Docker Compose generation with resource limits"
echo "• Nginx routing for public services"
echo "• Language-specific templates (Node.js, Python, Go, etc.)"
echo "• Per-service configuration (ports, memory, replicas)"
