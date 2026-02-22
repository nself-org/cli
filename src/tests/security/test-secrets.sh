#!/usr/bin/env bash
# test-secrets.sh - Secret Scanning Security Tests
# Scans for hardcoded secrets and validates .gitignore

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

pass() {
  printf "${GREEN}✓${NC} %s\n" "$1"
  TESTS_PASSED=$((TESTS_PASSED + 1))
  TESTS_RUN=$((TESTS_RUN + 1))
}

fail() {
  printf "${RED}✗${NC} %s\n" "$1"
  TESTS_FAILED=$((TESTS_FAILED + 1))
  TESTS_RUN=$((TESTS_RUN + 1))
}

warn() {
  printf "${YELLOW}⚠${NC} %s\n" "$1"
}

section() {
  printf "\n${BLUE}=== %s ===${NC}\n\n" "$1"
}

# Test 1: .gitignore coverage
test_gitignore_coverage() {
  section "Test 1: .gitignore Coverage"

  cd "$PROJECT_ROOT"

  if [[ ! -f ".gitignore" ]]; then
    fail ".gitignore does not exist"
    return 1
  fi

  # Required patterns
  local required_patterns=(
    ".env.local"
    ".env.staging"
    ".env.prod"
    ".secrets"
    "*.pem"
    "*.key"
  )

  for pattern in "${required_patterns[@]}"; do
    if grep -q "^${pattern}" .gitignore; then
      pass ".gitignore includes $pattern"
    else
      fail ".gitignore missing $pattern"
    fi
  done
}

# Test 2: No hardcoded passwords
test_no_hardcoded_passwords() {
  section "Test 2: No Hardcoded Passwords"

  cd "$PROJECT_ROOT"

  # Scan for password= patterns (excluding .example and test files)
  local password_matches
  password_matches=$(grep -riE '(password|passwd|pwd)=[^$]' src/lib/ \
    --exclude="*.md" \
    --exclude=".example" \
    --exclude="test-secrets.sh" \
    2>/dev/null | grep -v "change-this" | wc -l | xargs)

  if [[ "$password_matches" -eq 0 ]]; then
    pass "No hardcoded passwords found"
  else
    warn "Found $password_matches potential hardcoded passwords (review manually)"
  fi
}

# Test 3: No hardcoded API keys
test_no_hardcoded_keys() {
  section "Test 3: No Hardcoded API Keys"

  cd "$PROJECT_ROOT"

  # Look for common API key patterns
  local key_patterns=(
    "api_key="
    "apikey="
    "api-key="
    "secret_key="
    "secret="
  )

  local found_keys=0
  for pattern in "${key_patterns[@]}"; do
    local matches
    matches=$(grep -riE "${pattern}['\"][a-zA-Z0-9]{20,}" src/lib/ \
      --exclude="*.md" \
      --exclude=".example" \
      --exclude="test-secrets.sh" \
      2>/dev/null | wc -l | xargs)
    found_keys=$((found_keys + matches))
  done

  if [[ "$found_keys" -eq 0 ]]; then
    pass "No hardcoded API keys found"
  else
    fail "Found $found_keys potential hardcoded API keys"
  fi
}

# Test 4: .env.example has only placeholders
test_env_example_safe() {
  section "Test 4: .env.example Has Only Placeholders"

  cd "$PROJECT_ROOT"

  if [[ ! -f ".env.example" ]]; then
    warn ".env.example not found (OK if not using)"
    return 0
  fi

  # Check for suspiciously long values (real secrets)
  local suspicious
  suspicious=$(grep -E '=.{40,}' .env.example 2>/dev/null | grep -v "change-this" | wc -l | xargs)

  if [[ "$suspicious" -eq 0 ]]; then
    pass ".env.example contains only placeholders"
  else
    fail ".env.example may contain real secrets (values too long)"
  fi
}

# Test 5: No secrets in git history (recent commits)
test_no_secrets_in_git() {
  section "Test 5: No Secrets in Recent Git Commits"

  cd "$PROJECT_ROOT"

  if [[ ! -d ".git" ]]; then
    warn "Not a git repository, skipping"
    return 0
  fi

  # Check last 10 commits for .env files
  local env_in_git
  env_in_git=$(git log --all --full-history -10 --name-only -- '.env' '.secrets' 2>/dev/null | wc -l | xargs)

  if [[ "$env_in_git" -eq 0 ]]; then
    pass "No .env or .secrets files in recent git history"
  else
    fail "Found .env or .secrets in git history (CRITICAL)"
    warn "Run: git filter-repo --path .env --path .secrets --invert-paths"
  fi
}

# Main
main() {
  printf "\n${BLUE}╔════════════════════════════════════════════════════════╗${NC}\n"
  printf "${BLUE}║         Secret Scanning Security Test Suite           ║${NC}\n"
  printf "${BLUE}╚════════════════════════════════════════════════════════╝${NC}\n"

  test_gitignore_coverage
  test_no_hardcoded_passwords
  test_no_hardcoded_keys
  test_env_example_safe
  test_no_secrets_in_git

  # Summary
  printf "\n${BLUE}═══════════════════════════════════════════════════════${NC}\n"
  printf "Total: %d | ${GREEN}Passed: %d${NC} | ${RED}Failed: %d${NC}\n" "$TESTS_RUN" "$TESTS_PASSED" "$TESTS_FAILED"

  if [[ $TESTS_FAILED -eq 0 ]]; then
    printf "${GREEN}✓ All secret scanning tests passed!${NC}\n"
    return 0
  else
    printf "${RED}✗ Secret leaks detected! Review immediately.${NC}\n"
    return 1
  fi
}

main "$@"
