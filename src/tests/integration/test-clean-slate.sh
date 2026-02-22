#!/usr/bin/env bash
# test-clean-slate.sh - Integration test for clean slate deployment
# Tests that nself can build and start all services from scratch with no errors
set -euo pipefail

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test configuration
TEST_PROJECT="${TEST_PROJECT:-test-nself-integration}"
MIN_HEALTHY_SERVICES="${MIN_HEALTHY_SERVICES:-4}"  # At minimum postgres, hasura, auth, nginx

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Clean Slate Integration Test"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Verify nself is available (skip gracefully if not - requires CI environment setup)
if ! command -v nself >/dev/null 2>&1; then
  printf "${YELLOW}⚠ nself command not found - skipping test (requires CI environment)${NC}\n"
  exit 0
fi

# Verify Docker is available
if ! docker ps >/dev/null 2>&1; then
  printf "${YELLOW}⚠ Docker not available - skipping test (requires running Docker daemon)${NC}\n"
  exit 0
fi

# Step 1: Stop services
printf "→ Stopping all services...\n"
if nself stop >/dev/null 2>&1; then
  printf "${GREEN}✓ Services stopped${NC}\n"
else
  printf "${YELLOW}⚠ No services were running${NC}\n"
fi

# Step 2: Clean volumes (optional - controlled by flag)
if [[ "${CLEAN_VOLUMES:-false}" == "true" ]]; then
  printf "→ Cleaning Docker volumes...\n"
  if docker volume ls -q | grep -q "${TEST_PROJECT}"; then
    docker volume ls -q | grep "${TEST_PROJECT}" | xargs docker volume rm -f >/dev/null 2>&1 || true
    printf "${GREEN}✓ Volumes cleaned${NC}\n"
  else
    printf "${YELLOW}⚠ No volumes to clean${NC}\n"
  fi
fi

# Step 3: Build
printf "→ Building docker-compose.yml...\n"
if nself build >/dev/null 2>&1; then
  printf "${GREEN}✓ Build successful${NC}\n"
else
  printf "${RED}✗ FAILED: Build failed${NC}\n"
  exit 1
fi

# Step 4: Verify docker-compose.yml was generated
if [[ ! -f "docker-compose.yml" ]]; then
  printf "${RED}✗ FAILED: docker-compose.yml not generated${NC}\n"
  exit 1
fi

# Step 5: Verify init containers exist (if MinIO or MeiliSearch enabled)
printf "→ Verifying init containers...\n"
if grep -q "MINIO_ENABLED=true" .env* 2>/dev/null; then
  if grep -q "minio-init:" docker-compose.yml; then
    printf "${GREEN}✓ MinIO init container present${NC}\n"
  else
    printf "${RED}✗ FAILED: MinIO init container missing${NC}\n"
    exit 1
  fi
fi

if grep -q "MEILISEARCH_ENABLED=true" .env* 2>/dev/null; then
  if grep -q "meilisearch-init:" docker-compose.yml; then
    printf "${GREEN}✓ MeiliSearch init container present${NC}\n"
  else
    printf "${RED}✗ FAILED: MeiliSearch init container missing${NC}\n"
    exit 1
  fi
fi

# Step 6: Verify network name is correct (not "myproject_network")
printf "→ Verifying network configuration...\n"
if grep -q "myproject_network" docker-compose.yml; then
  printf "${RED}✗ FAILED: Using default 'myproject_network' instead of PROJECT_NAME${NC}\n"
  exit 1
else
  printf "${GREEN}✓ Network name correctly uses PROJECT_NAME${NC}\n"
fi

# Step 7: Start services
printf "→ Starting all services...\n"
if nself start >/dev/null 2>&1; then
  printf "${GREEN}✓ Services started${NC}\n"
else
  printf "${RED}✗ FAILED: Start failed${NC}\n"
  nself logs --tail 20  # Show recent logs for debugging
  exit 1
fi

# Step 8: Wait for services to stabilize
printf "→ Waiting for services to stabilize (15s)...\n"
sleep 15

# Step 9: Check service health
printf "→ Checking service health...\n"

# Try to get health status
if command -v nself >/dev/null 2>&1; then
  # Count healthy services using nself status
  HEALTHY_COUNT=$(nself status 2>/dev/null | grep -c "^✓" || echo "0")

  if [[ "$HEALTHY_COUNT" -ge "$MIN_HEALTHY_SERVICES" ]]; then
    printf "${GREEN}✓ Service health check passed ($HEALTHY_COUNT healthy services)${NC}\n"
  else
    printf "${RED}✗ FAILED: Only $HEALTHY_COUNT/$MIN_HEALTHY_SERVICES services healthy${NC}\n"
    printf "\nService status:\n"
    nself status
    exit 1
  fi
else
  printf "${YELLOW}⚠ Unable to verify service health (nself status not available)${NC}\n"
fi

# Step 10: Verify no permission errors in logs
printf "→ Checking for permission errors...\n"
HAS_ERRORS=false

if docker ps --filter "name=minio" --format "{{.Names}}" | grep -q minio; then
  if docker logs $(docker ps --filter "name=minio" --format "{{.Names}}" | head -1) 2>&1 | grep -qi "permission denied\|access denied"; then
    printf "${RED}✗ MinIO has permission errors${NC}\n"
    HAS_ERRORS=true
  fi
fi

if docker ps --filter "name=meilisearch" --format "{{.Names}}" | grep -q meilisearch; then
  if docker logs $(docker ps --filter "name=meilisearch" --format "{{.Names}}" | head -1) 2>&1 | grep -qi "permission denied"; then
    printf "${RED}✗ MeiliSearch has permission errors${NC}\n"
    HAS_ERRORS=true
  fi
fi

if [[ "$HAS_ERRORS" == "true" ]]; then
  exit 1
else
  printf "${GREEN}✓ No permission errors detected${NC}\n"
fi

# Success!
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
printf "${GREEN}✓ ALL TESTS PASSED${NC}\n"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
printf "Clean slate deployment successful!\n"
printf "Project: %s\n" "$TEST_PROJECT"
printf "Healthy Services: %s\n" "$HEALTHY_COUNT"
echo ""

exit 0
