#!/usr/bin/env bash
#
# Show Untested Branches
# Identifies specific branches in code that lack test coverage
#

set -euo pipefail

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Get project root
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_DIR="$PROJECT_ROOT/src"
TEST_DIR="$PROJECT_ROOT/src/tests"

printf "${BLUE}=== Untested Branches Analysis ===${NC}\n\n"

# Find source files without corresponding tests
find_untested_files() {
  printf "${YELLOW}Source Files Without Test Coverage:${NC}\n"

  local count=0
  local files

  files=$(find "$SRC_DIR/lib" "$SRC_DIR/cli" -name "*.sh" -type f 2>/dev/null || true)

  while IFS= read -r file; do
    if [[ -z "$file" ]]; then
      continue
    fi

    local basename
    basename=$(basename "$file" .sh)

    # Check if there's a test file
    if ! find "$TEST_DIR" -name "*${basename}*test*.sh" -o -name "test-${basename}*.sh" 2>/dev/null | grep -q .; then
      printf "  ${RED}✗${NC} %s\n" "${file#$PROJECT_ROOT/}"
      count=$((count + 1))
    fi

  done <<< "$files"

  if [[ $count -eq 0 ]]; then
    printf "  ${GREEN}✓${NC} All source files have test coverage\n"
  else
    printf "\n  Total: ${RED}%d${NC} files without tests\n" "$count"
  fi

  printf "\n"
}

# Find if statements that may lack branch coverage
find_untested_conditionals() {
  printf "${YELLOW}Potential Untested Conditionals:${NC}\n"

  local files
  files=$(find "$SRC_DIR/lib" "$SRC_DIR/cli" -name "*.sh" -type f 2>/dev/null || true)

  while IFS= read -r file; do
    if [[ -z "$file" ]]; then
      continue
    fi

    local basename
    basename=$(basename "$file" .sh)
    local has_test=false

    # Check if there's a test file
    if find "$TEST_DIR" -name "*${basename}*test*.sh" -o -name "test-${basename}*.sh" 2>/dev/null | grep -q .; then
      has_test=true
    fi

    # If no test file, all conditionals are untested
    if [[ "$has_test" == "false" ]]; then
      local if_count
      if_count=$(grep -c "^[[:space:]]*if \[" "$file" 2>/dev/null || echo "0")

      if [[ $if_count -gt 0 ]]; then
        printf "  ${RED}✗${NC} %s - ${RED}%d${NC} if statements (no tests)\n" "${file#$PROJECT_ROOT/}" "$if_count"

        # Show first few if statements
        grep -n "^[[:space:]]*if \[" "$file" 2>/dev/null | head -3 | while IFS=: read -r line_num line_content; do
          printf "      Line %d: %s\n" "$line_num" "$(echo "$line_content" | sed 's/^[[:space:]]*//')"
        done

        if [[ $if_count -gt 3 ]]; then
          printf "      ... and %d more\n" "$((if_count - 3))"
        fi
        printf "\n"
      fi
    fi

  done <<< "$files"
}

# Find complex case statements
find_untested_case_statements() {
  printf "${YELLOW}Case Statements Needing Branch Coverage:${NC}\n"

  local files
  files=$(find "$SRC_DIR/lib" "$SRC_DIR/cli" -name "*.sh" -type f 2>/dev/null || true)

  while IFS= read -r file; do
    if [[ -z "$file" ]]; then
      continue
    fi

    # Find case statements
    local in_case=0
    local case_line=0
    local branch_count=0
    local case_var=""

    while IFS= read -r line; do
      if [[ "$line" =~ ^[[:space:]]*case.*in ]]; then
        in_case=1
        case_line=$((case_line + 1))
        branch_count=0
        case_var=$(echo "$line" | sed 's/^[[:space:]]*case[[:space:]]*//' | sed 's/[[:space:]]*in.*//')
      elif [[ $in_case -eq 1 ]]; then
        if [[ "$line" =~ ^[[:space:]]*esac ]]; then
          # End of case - report if significant
          if [[ $branch_count -ge 3 ]]; then
            printf "  ${YELLOW}!${NC} %s\n" "${file#$PROJECT_ROOT/}"
            printf "      Case on: %s (%d branches)\n" "$case_var" "$branch_count"
            printf "      Recommend: Test all %d branches\n\n" "$branch_count"
          fi
          in_case=0
        elif [[ "$line" =~ \)[[:space:]]*$ ]] || [[ "$line" =~ \)\;* ]]; then
          branch_count=$((branch_count + 1))
        fi
      fi
    done < "$file"

  done <<< "$files"
}

# Find error handling that needs testing
find_error_handling_branches() {
  printf "${YELLOW}Error Handling Branches:${NC}\n"

  local files
  files=$(find "$SRC_DIR/lib" "$SRC_DIR/cli" -name "*.sh" -type f 2>/dev/null || true)

  while IFS= read -r file; do
    if [[ -z "$file" ]]; then
      continue
    fi

    # Look for error handling patterns
    local error_count=0

    # Count || patterns (error alternatives)
    error_count=$(grep -c " || " "$file" 2>/dev/null || echo "0")

    if [[ $error_count -gt 5 ]]; then
      printf "  ${YELLOW}!${NC} %s - ${YELLOW}%d${NC} error alternative branches\n" "${file#$PROJECT_ROOT/}" "$error_count"
    fi

  done <<< "$files"

  printf "\n"
}

# Generate recommendations
generate_recommendations() {
  printf "${BLUE}=== Recommendations ===${NC}\n\n"

  printf "1. ${YELLOW}Create tests for files without coverage${NC}\n"
  printf "   - Use test-branch-coverage-template.sh as a starting point\n"
  printf "   - Focus on high-impact files first (core functionality)\n\n"

  printf "2. ${YELLOW}Test all branches in conditionals${NC}\n"
  printf "   - Every if statement needs TRUE and FALSE tests\n"
  printf "   - Every case statement needs all branch tests\n"
  printf "   - Test both && and || operator paths\n\n"

  printf "3. ${YELLOW}Test platform-specific code paths${NC}\n"
  printf "   - macOS vs Linux branches\n"
  printf "   - Command availability (timeout, gtimeout, etc.)\n"
  printf "   - Use environment-control.sh mocks\n\n"

  printf "4. ${YELLOW}Test error handling gracefully${NC}\n"
  printf "   - Error paths should succeed (handled gracefully)\n"
  printf "   - Don't fail tests on expected errors\n"
  printf "   - Verify degradation behavior\n\n"

  printf "5. ${YELLOW}Use resilient test patterns${NC}\n"
  printf "   - Mock environment for control\n"
  printf "   - Handle missing commands gracefully\n"
  printf "   - Test should pass on all platforms\n\n"
}

# Main execution
main() {
  find_untested_files
  find_untested_conditionals
  find_untested_case_statements
  find_error_handling_branches
  generate_recommendations
}

main "$@"
