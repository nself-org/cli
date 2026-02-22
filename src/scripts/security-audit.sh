#!/usr/bin/env bash
#
# Security Audit Script for nself
# Detects potential SQL injection and command injection vulnerabilities
#
# Usage: bash src/scripts/security-audit.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors for output
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Counters
total_files=0
files_with_issues=0
total_issues=0

printf "${BLUE}=== nself Security Audit ===${NC}\n\n"

# ============================================================================
# 1. Check for unquoted psql command variables (COMMAND INJECTION)
# ============================================================================

printf "${YELLOW}[1/5] Checking for command injection vulnerabilities...${NC}\n"

# Pattern: docker exec ... $psql_cmd or $psql_opts (unquoted)
command_injection_files=$(grep -rl 'docker exec.*\$psql_' "$ROOT_DIR/src/lib" 2>/dev/null || true)

if [[ -n "$command_injection_files" ]]; then
  printf "${RED}⚠ CRITICAL: Found unquoted command variables:${NC}\n"
  while IFS= read -r file; do
    printf "  - %s\n" "$file"
    grep -n 'docker exec.*\$psql_' "$file" | head -3 | while IFS= read -r line; do
      printf "    ${RED}%s${NC}\n" "$line"
    done
    files_with_issues=$((files_with_issues + 1))
    total_issues=$((total_issues + 1))
  done <<< "$command_injection_files"
  printf "\n"
else
  printf "${GREEN}✓ No command injection vulnerabilities found${NC}\n\n"
fi

# ============================================================================
# 2. Check for SQL injection via string interpolation (SQL INJECTION)
# ============================================================================

printf "${YELLOW}[2/5] Checking for SQL injection vulnerabilities...${NC}\n"

# Pattern: psql -c "... '$variable' ..." (direct interpolation in queries)
sql_injection_files=$(grep -rl "psql.*-c.*'\$" "$ROOT_DIR/src/lib" 2>/dev/null || true)

if [[ -n "$sql_injection_files" ]]; then
  printf "${RED}⚠ CRITICAL: Found potential SQL injection:${NC}\n"
  while IFS= read -r file; do
    # Skip safe-query.sh itself (it's the solution, not the problem)
    if [[ "$file" == *"safe-query.sh"* ]]; then
      continue
    fi

    count=$(grep -c "psql.*-c.*'\$" "$file" 2>/dev/null || echo "0")
    if [[ $count -gt 0 ]]; then
      printf "  - %s ${RED}(%d instances)${NC}\n" "$file" "$count"
      files_with_issues=$((files_with_issues + 1))
      ((total_issues += count))
    fi
  done <<< "$sql_injection_files"
  printf "\n"
else
  printf "${GREEN}✓ No SQL injection vulnerabilities found${NC}\n\n"
fi

# ============================================================================
# 3. Check for files using direct psql instead of safe-query.sh
# ============================================================================

printf "${YELLOW}[3/5] Checking for unsafe database queries...${NC}\n"

# Find all files with psql commands
psql_files=$(grep -rl 'docker exec.*psql' "$ROOT_DIR/src/lib" 2>/dev/null || true)

unsafe_files=0
if [[ -n "$psql_files" ]]; then
  while IFS= read -r file; do
    # Check if file sources safe-query.sh
    if ! grep -q 'source.*safe-query.sh' "$file" 2>/dev/null; then
      # Skip if it's safe-query.sh itself or billing/core.sh (uses own safe pattern)
      if [[ "$file" != *"safe-query.sh"* ]] && [[ "$file" != *"billing/core.sh"* ]]; then
        if [[ $unsafe_files -eq 0 ]]; then
          printf "${YELLOW}⚠ Files using psql without safe-query.sh:${NC}\n"
        fi
        printf "  - %s\n" "$file"
        unsafe_files=$((unsafe_files + 1))
      fi
    fi
  done <<< "$psql_files"

  if [[ $unsafe_files -eq 0 ]]; then
    printf "${GREEN}✓ All database queries use safe patterns${NC}\n"
  fi
  printf "\n"
else
  printf "${GREEN}✓ No direct psql usage found${NC}\n\n"
fi

# ============================================================================
# 4. Check for unquoted variables in WHERE/VALUES/SET clauses
# ============================================================================

printf "${YELLOW}[4/5] Checking for unsafe SQL patterns...${NC}\n"

# Pattern: WHERE/VALUES/SET with unquoted variables
unsafe_patterns=(
  "WHERE.*=.*'\$"
  "VALUES.*'\$"
  "SET.*=.*'\$"
  "INSERT INTO.*'\$"
  "UPDATE.*'\$"
  "DELETE FROM.*'\$"
)

found_unsafe=0
for pattern in "${unsafe_patterns[@]}"; do
  matches=$(grep -rl "$pattern" "$ROOT_DIR/src/lib" 2>/dev/null || true)
  if [[ -n "$matches" ]]; then
    if [[ $found_unsafe -eq 0 ]]; then
      printf "${RED}⚠ Found unsafe SQL patterns:${NC}\n"
      found_unsafe=1
    fi

    while IFS= read -r file; do
      # Skip safe-query.sh and billing/core.sh (safe implementations)
      if [[ "$file" == *"safe-query.sh"* ]] || [[ "$file" == *"billing/core.sh"* ]]; then
        continue
      fi

      count=$(grep -c "$pattern" "$file" 2>/dev/null || echo "0")
      if [[ $count -gt 0 ]]; then
        printf "  - %s: %s ${RED}(%d)${NC}\n" "$file" "$pattern" "$count"
      fi
    done <<< "$matches"
  fi
done

if [[ $found_unsafe -eq 0 ]]; then
  printf "${GREEN}✓ No unsafe SQL patterns found${NC}\n"
fi
printf "\n"

# ============================================================================
# 5. Check for PGPASSWORD in environment (CREDENTIAL EXPOSURE)
# ============================================================================

printf "${YELLOW}[5/5] Checking for credential exposure...${NC}\n"

pgpassword_files=$(grep -rl 'PGPASSWORD=' "$ROOT_DIR/src/lib" 2>/dev/null || true)

if [[ -n "$pgpassword_files" ]]; then
  printf "${YELLOW}⚠ Files using PGPASSWORD environment variable:${NC}\n"
  while IFS= read -r file; do
    # billing/core.sh uses .pgpass file (safe), so check context
    if grep -q 'mktemp.*pgpass' "$file" 2>/dev/null; then
      printf "  - %s ${GREEN}(uses .pgpass file - SAFE)${NC}\n" "$file"
    else
      printf "  - %s ${YELLOW}(check if properly secured)${NC}\n" "$file"
    fi
  done <<< "$pgpassword_files"
  printf "\n"
else
  printf "${GREEN}✓ No PGPASSWORD usage found${NC}\n\n"
fi

# ============================================================================
# Summary
# ============================================================================

printf "${BLUE}=== Audit Summary ===${NC}\n\n"

total_files=$(find "$ROOT_DIR/src/lib" -name "*.sh" -type f | wc -l | xargs)

printf "Total shell scripts scanned: %d\n" "$total_files"
printf "Files with potential issues: %d\n" "$files_with_issues"
printf "Total issues found: %d\n\n" "$total_issues"

if [[ $total_issues -gt 0 ]]; then
  printf "${RED}⚠ SECURITY ISSUES DETECTED${NC}\n\n"
  printf "Next steps:\n"
  printf "1. Review SECURITY-FIX-REPORT.md for detailed findings\n"
  printf "2. Migrate vulnerable code to use safe-query.sh functions\n"
  printf "3. Add input validation for all user-provided data\n"
  printf "4. Run this audit again after fixes\n\n"

  printf "Priority files to fix:\n"
  printf "  🔴 CRITICAL: src/lib/secrets/vault.sh (handles encryption keys)\n"
  printf "  🟠 HIGH:     src/lib/tenant/core.sh (multi-tenant data)\n"
  printf "  🟡 MEDIUM:   src/lib/plugin/core.sh (plugin queries)\n\n"

  exit 1
else
  printf "${GREEN}✓ No security issues found!${NC}\n\n"
  exit 0
fi
