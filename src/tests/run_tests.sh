#!/usr/bin/env bash
set -euo pipefail

# Test runner for nself test suite

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo ""
printf "${BLUE}╔══════════════════════════════════════════════╗${NC}\n"
printf "${BLUE}║          nself Test Suite Runner             ║${NC}\n"
printf "${BLUE}╚══════════════════════════════════════════════╝${NC}\n"
echo ""

# Check if bats is installed
if ! command -v bats >/dev/null 2>&1; then
  printf "${YELLOW}⚠️  Warning: bats not installed${NC}\n"
  printf "${BLUE}Installing bats would enable full test suite${NC}\n"
  printf "${BLUE}Visit: https://github.com/bats-core/bats-core${NC}\n"
  echo ""
  printf "${BLUE}Running basic tests without bats...${NC}\n"
  echo ""

  # Run basic tests without bats
  TESTS_PASSED=0
  TESTS_FAILED=0

  # Test 1: Check if install.sh exists
  echo -n "Testing install.sh existence... "
  if [ -f "../../install.sh" ]; then
    printf "${GREEN}✓${NC}\n"
    ((TESTS_PASSED++))
  else
    printf "${RED}✗${NC}\n"
    ((TESTS_FAILED++))
  fi

  # Test 2: Check if nself binary exists
  echo -n "Testing nself binary existence... "
  if [ -f "../../bin/nself" ]; then
    printf "${GREEN}✓${NC}\n"
    ((TESTS_PASSED++))
  else
    printf "${RED}✗${NC}\n"
    ((TESTS_FAILED++))
  fi

  # Test 3: Check VERSION file
  echo -n "Testing VERSION file... "
  if [ -f "../VERSION" ]; then
    VERSION=$(cat ../VERSION)
    printf "${GREEN}✓${NC} (v%s)\n" "$VERSION"
    ((TESTS_PASSED++))
  else
    printf "${RED}✗${NC}\n"
    ((TESTS_FAILED++))
  fi

  # Test 4: Check shell script syntax
  echo -n "Testing install.sh syntax... "
  if bash -n ../../install.sh 2>/dev/null; then
    printf "${GREEN}✓${NC}\n"
    ((TESTS_PASSED++))
  else
    printf "${RED}✗${NC}\n"
    ((TESTS_FAILED++))
  fi

  echo -n "Testing nself.sh syntax... "
  if bash -n ../cli/nself.sh 2>/dev/null; then
    printf "${GREEN}✓${NC}\n"
    ((TESTS_PASSED++))
  else
    printf "${RED}✗${NC}\n"
    ((TESTS_FAILED++))
  fi

  # Test 5: Check for required functions in install.sh
  echo -n "Testing install.sh functions... "
  if grep -q "check_existing_installation" ../../install.sh &&
    grep -q "check_requirements" ../../install.sh &&
    grep -q "show_spinner" ../../install.sh; then
    printf "${GREEN}✓${NC}\n"
    ((TESTS_PASSED++))
  else
    printf "${RED}✗${NC}\n"
    ((TESTS_FAILED++))
  fi

  # Test 6: Check for required functions in nself.sh
  echo -n "Testing nself.sh functions... "
  if grep -q "cmd_update" ../cli/nself.sh &&
    grep -q "cmd_init" ../cli/nself.sh &&
    grep -q "show_spinner" ../cli/nself.sh; then
    printf "${GREEN}✓${NC}\n"
    ((TESTS_PASSED++))
  else
    printf "${RED}✗${NC}\n"
    ((TESTS_FAILED++))
  fi

  echo ""
  printf "${BLUE}═══════════════════════════════════════════════${NC}\n"
  printf "${GREEN}Passed: %s${NC} | ${RED}Failed: %s${NC}\n" "$TESTS_PASSED" "$TESTS_FAILED"

  if [ $TESTS_FAILED -eq 0 ]; then
    printf "${GREEN}✅ All basic tests passed!${NC}\n"
  else
    printf "${RED}❌ Some tests failed${NC}\n"
    exit 1
  fi
else
  printf "${GREEN}✓ bats is installed${NC}\n"
  echo ""

  # Run bats tests
  printf "${BLUE}Running test suites...${NC}\n"
  echo ""

  # Run each test file
  for test_file in *.bats; do
    if [ -f "$test_file" ]; then
      printf "${BLUE}Running %s...${NC}\n" "$test_file"
      bats "$test_file"
      echo ""
    fi
  done

  printf "${GREEN}✅ All tests completed!${NC}\n"
fi

echo ""
