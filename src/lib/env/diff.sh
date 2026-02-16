#!/usr/bin/env bash

# diff.sh - Environment comparison functionality
# POSIX-compliant, no Bash 4+ features

# Get the directory where this script is located
ENV_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

LIB_ROOT="$(dirname "$ENV_LIB_DIR")"

# Source dependencies
source "$LIB_ROOT/utils/display.sh" 2>/dev/null || true
source "$LIB_ROOT/utils/platform-compat.sh" 2>/dev/null || true

# Environment directory
ENVIRONMENTS_DIR="${ENVIRONMENTS_DIR:-./.environments}"

# Compare two environments
env::diff() {
  local env_a="$1"
  local env_b="$2"
  local show_values="${3:-false}"

  if [[ -z "$env_a" ]] || [[ -z "$env_b" ]]; then
    log_error "Two environment names are required"
    printf "Usage: nself env diff <env1> <env2>\n"
    return 1
  fi

  # Try to find env files - check multiple locations
  local file_a=""
  local file_b=""

  # Check .environments/ directory first
  if [[ -f "$ENVIRONMENTS_DIR/$env_a/.env" ]]; then
    file_a="$ENVIRONMENTS_DIR/$env_a/.env"
  # Then check root-level .env.<name> files
  elif [[ -f ".env.$env_a" ]]; then
    file_a=".env.$env_a"
  # Special case: "local" might mean .env.local or just .env
  elif [[ "$env_a" == "local" ]] && [[ -f ".env.local" ]]; then
    file_a=".env.local"
  elif [[ "$env_a" == "dev" ]] && [[ -f ".env.dev" ]]; then
    file_a=".env.dev"
  fi

  if [[ -f "$ENVIRONMENTS_DIR/$env_b/.env" ]]; then
    file_b="$ENVIRONMENTS_DIR/$env_b/.env"
  elif [[ -f ".env.$env_b" ]]; then
    file_b=".env.$env_b"
  elif [[ "$env_b" == "local" ]] && [[ -f ".env.local" ]]; then
    file_b=".env.local"
  elif [[ "$env_b" == "dev" ]] && [[ -f ".env.dev" ]]; then
    file_b=".env.dev"
  fi

  # Validate files exist
  if [[ -z "$file_a" ]]; then
    log_error "Environment '$env_a' not found"
    log_info "Checked: $ENVIRONMENTS_DIR/$env_a/.env and .env.$env_a"
    return 1
  fi

  if [[ -z "$file_b" ]]; then
    log_error "Environment '$env_b' not found"
    log_info "Checked: $ENVIRONMENTS_DIR/$env_b/.env and .env.$env_b"
    return 1
  fi

  printf "Comparing: ${COLOR_BLUE}%s${COLOR_RESET} vs ${COLOR_BLUE}%s${COLOR_RESET}\n" "$env_a" "$env_b"
  printf "Files: %s vs %s\n\n" "$file_a" "$file_b"

  # Compare .env files
  env::compare_env_files "$file_a" "$file_b" "$env_a" "$env_b" "$show_values"

  # Compare server configurations if available
  local server_a="$ENVIRONMENTS_DIR/$env_a/server.json"
  local server_b="$ENVIRONMENTS_DIR/$env_b/server.json"
  if [[ -f "$server_a" ]] || [[ -f "$server_b" ]]; then
    printf "\n${COLOR_CYAN}Server Configuration:${COLOR_RESET}\n"
    env::compare_server_configs "$server_a" "$server_b" "$env_a" "$env_b"
  fi

  return 0
}

# Compare two .env files
env::compare_env_files() {
  local file_a="$1"
  local file_b="$2"
  local name_a="$3"
  local name_b="$4"
  local show_values="${5:-false}"

  # Extract keys from both files (allow any valid env var pattern)
  local keys_a keys_b
  keys_a=$(grep -E "^[A-Za-z_][A-Za-z0-9_]*=" "$file_a" 2>/dev/null | cut -d'=' -f1 | sort -u || echo "")
  keys_b=$(grep -E "^[A-Za-z_][A-Za-z0-9_]*=" "$file_b" 2>/dev/null | cut -d'=' -f1 | sort -u || echo "")

  # Check if either file has no variables
  if [[ -z "$keys_a" ]] && [[ -z "$keys_b" ]]; then
    printf "${COLOR_YELLOW}Both environment files appear to be empty or have no parseable variables${COLOR_RESET}\n"
    return 0
  fi

  # Find all unique keys from both files
  local all_keys
  all_keys=$(printf "%s\n%s" "$keys_a" "$keys_b" | grep -v "^$" | sort -u)

  # Count differences using simple counters (Bash 3.2 compatible)
  local diff_count=0
  local only_a_count=0
  local only_b_count=0

  # Store results in temp files for Bash 3.2 compatibility
  local diff_file only_a_file only_b_file
  diff_file=$(mktemp)
  only_a_file=$(mktemp)
  only_b_file=$(mktemp)

  while IFS= read -r key; do
    [[ -z "$key" ]] && continue

    local val_a val_b
    # Use || true to handle pipefail when grep finds no matches
    val_a=$(grep "^${key}=" "$file_a" 2>/dev/null | head -1 | cut -d'=' -f2- || true)
    val_b=$(grep "^${key}=" "$file_b" 2>/dev/null | head -1 | cut -d'=' -f2- || true)

    # Check if key exists in file_a (use || true for set -e compatibility)
    local in_a=false in_b=false
    grep -q "^${key}=" "$file_a" 2>/dev/null && in_a=true || true
    grep -q "^${key}=" "$file_b" 2>/dev/null && in_b=true || true

    if [[ "$in_a" == "false" ]]; then
      printf "%s\n" "$key" >>"$only_b_file"
    elif [[ "$in_b" == "false" ]]; then
      printf "%s\n" "$key" >>"$only_a_file"
    elif [[ "$val_a" != "$val_b" ]]; then
      printf "%s|%s|%s\n" "$key" "$val_a" "$val_b" >>"$diff_file"
    fi
  done <<<"$all_keys"

  # Display differences
  diff_count=$(wc -l <"$diff_file" | tr -d ' ')
  if [[ "$diff_count" -gt 0 ]]; then
    printf "${COLOR_YELLOW}Different values:${COLOR_RESET}\n"
    while IFS='|' read -r key val_a val_b; do
      [[ -z "$key" ]] && continue
      printf "  ${COLOR_CYAN}%s${COLOR_RESET}\n" "$key"

      if [[ "$show_values" == "true" ]]; then
        # Mask sensitive values
        case "$key" in
          *PASSWORD* | *SECRET* | *KEY* | *TOKEN*)
            printf "    %s: %s\n" "$name_a" "********"
            printf "    %s: %s\n" "$name_b" "********"
            ;;
          *)
            printf "    %s: %s\n" "$name_a" "$val_a"
            printf "    %s: %s\n" "$name_b" "$val_b"
            ;;
        esac
      fi
    done <"$diff_file"
  fi

  # Display only in A
  only_a_count=$(wc -l <"$only_a_file" | tr -d ' ')
  if [[ "$only_a_count" -gt 0 ]]; then
    printf "\n${COLOR_GREEN}Only in %s:${COLOR_RESET}\n" "$name_a"
    while IFS= read -r key; do
      [[ -n "$key" ]] && printf "  + %s\n" "$key"
    done <"$only_a_file"
  fi

  # Display only in B
  only_b_count=$(wc -l <"$only_b_file" | tr -d ' ')
  if [[ "$only_b_count" -gt 0 ]]; then
    printf "\n${COLOR_RED}Only in %s:${COLOR_RESET}\n" "$name_b"
    while IFS= read -r key; do
      [[ -n "$key" ]] && printf "  + %s\n" "$key"
    done <"$only_b_file"
  fi

  # Summary
  local total_diff=$((diff_count + only_a_count + only_b_count))
  if [[ $total_diff -eq 0 ]]; then
    printf "${COLOR_GREEN}✓ Environments have identical configuration${COLOR_RESET}\n"
  else
    printf "\n${COLOR_YELLOW}Summary: %d difference(s)${COLOR_RESET}\n" "$total_diff"
    printf "  %d different values\n" "$diff_count"
    printf "  %d only in %s\n" "$only_a_count" "$name_a"
    printf "  %d only in %s\n" "$only_b_count" "$name_b"
  fi

  # Cleanup temp files
  rm -f "$diff_file" "$only_a_file" "$only_b_file"
}

# Compare server configurations
env::compare_server_configs() {
  local file_a="$1"
  local file_b="$2"
  local name_a="$3"
  local name_b="$4"

  if [[ -f "$file_a" ]] && [[ -f "$file_b" ]]; then
    # Extract key fields
    local fields="host port user type"

    for field in $fields; do
      local val_a val_b
      val_a=$(grep "\"$field\"" "$file_a" 2>/dev/null | cut -d'"' -f4)
      val_b=$(grep "\"$field\"" "$file_b" 2>/dev/null | cut -d'"' -f4)

      if [[ "$val_a" != "$val_b" ]]; then
        printf "  ${COLOR_CYAN}%s${COLOR_RESET}: %s → %s\n" "$field" "${val_a:-<not set>}" "${val_b:-<not set>}"
      fi
    done
  elif [[ -f "$file_a" ]]; then
    printf "  ${COLOR_YELLOW}Server config only in %s${COLOR_RESET}\n" "$name_a"
  elif [[ -f "$file_b" ]]; then
    printf "  ${COLOR_YELLOW}Server config only in %s${COLOR_RESET}\n" "$name_b"
  fi
}

# Show what would change when switching environments
env::preview_switch() {
  local target_env="$1"
  local current_env="${2:-$(env::get_current 2>/dev/null || echo 'local')}"

  if [[ -z "$target_env" ]]; then
    log_error "Target environment name is required"
    return 1
  fi

  if [[ "$target_env" == "$current_env" ]]; then
    log_info "Already on environment: $target_env"
    return 0
  fi

  printf "Preview: Switch from ${COLOR_BLUE}%s${COLOR_RESET} to ${COLOR_BLUE}%s${COLOR_RESET}\n\n" "$current_env" "$target_env"

  env::diff "$current_env" "$target_env" "true"

  printf "\n${COLOR_YELLOW}Note: This is a preview. No changes have been made.${COLOR_RESET}\n"
  printf "Run ${COLOR_CYAN}nself env switch %s${COLOR_RESET} to apply these changes.\n" "$target_env"
}

# Get current environment (imported from create.sh but defined here for standalone use)
if ! command -v env::get_current >/dev/null 2>&1; then
  env::get_current() {
    local current_file=".current-env"
    if [[ -f "$current_file" ]]; then
      cat "$current_file"
    else
      echo "local"
    fi
  }
fi

# Export functions
export -f env::diff
export -f env::compare_env_files
export -f env::compare_server_configs
export -f env::preview_switch
