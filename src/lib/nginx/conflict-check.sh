#!/usr/bin/env bash

# conflict-check.sh - Subdomain conflict detection across registered projects
# Parses server_name directives from nginx site configs and detects duplicates
# Bash 3.2 compatible — no echo -e, no ${var,,}, no declare -A, no mapfile

# Prevent double-sourcing
[[ "${CONFLICT_CHECK_SOURCED:-}" == "1" ]] && return 0
export CONFLICT_CHECK_SOURCED=1

set -euo pipefail

_CONFLICT_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source display utilities if available
if [[ -f "$_CONFLICT_LIB_DIR/../utils/display.sh" ]]; then
  . "$_CONFLICT_LIB_DIR/../utils/display.sh"
fi

# ---------------------------------------------------------------------------
# conflicts::parse_server_names — Extract all server_name values from conf files
#
# Args:
#   $1 — directory containing *.conf files
#
# Stdout:
#   One domain per line (excludes _ default server and empty lines)
# ---------------------------------------------------------------------------
conflicts::parse_server_names() {
  local conf_dir="$1"

  if [[ ! -d "$conf_dir" ]]; then
    return 0
  fi

  # Find all server_name directives, strip keyword and semicolons,
  # split multi-domain lines into individual domains
  local conf_files
  conf_files=$(find "$conf_dir" -maxdepth 1 -name '*.conf' 2>/dev/null) || true

  if [[ -z "$conf_files" ]]; then
    return 0
  fi

  printf '%s\n' "$conf_files" | while IFS= read -r f; do
    grep -h 'server_name' "$f" 2>/dev/null || true
  done \
    | sed 's/[[:space:]]*server_name[[:space:]]*//' \
    | sed 's/;[[:space:]]*//' \
    | tr ' ' '\n' \
    | grep -v '^$' \
    | grep -v '^_$' \
    | sort -u
}

# ---------------------------------------------------------------------------
# conflicts::check_new_project — Check a new project against all registered
#
# Args:
#   $1 — path to the new project (not yet registered)
#
# Returns:
#   0 if no conflicts, 1 if conflicts found
#
# Stdout:
#   Conflict details if found
# ---------------------------------------------------------------------------
conflicts::check_new_project() {
  local new_path="$1"
  local new_sites_dir="$new_path/nginx/sites"

  if [[ ! -d "$new_sites_dir" ]]; then
    return 0
  fi

  # Get new project's domains
  local new_domains
  new_domains=$(conflicts::parse_server_names "$new_sites_dir")

  if [[ -z "$new_domains" ]]; then
    return 0
  fi

  # Source registry if not already loaded
  if ! type registry::init >/dev/null 2>&1; then
    if [[ -f "$_CONFLICT_LIB_DIR/registry.sh" ]]; then
      source "$_CONFLICT_LIB_DIR/registry.sh"
    else
      return 0
    fi
  fi

  registry::init 2>/dev/null || true

  local registry_file="$HOME/.nself/nginx/registry.json"
  if [[ ! -f "$registry_file" ]]; then
    return 0
  fi

  local has_conflict=0

  # Check each registered project
  local project_count
  project_count=$(jq -r '.projects | length' "$registry_file" 2>/dev/null) || project_count=0

  local i=0
  while [[ $i -lt $project_count ]]; do
    local reg_name
    local reg_path
    reg_name=$(jq -r ".projects[$i].name" "$registry_file")
    reg_path=$(jq -r ".projects[$i].path" "$registry_file")

    local reg_sites_dir="$reg_path/nginx/sites"
    if [[ -d "$reg_sites_dir" ]]; then
      local reg_domains
      reg_domains=$(conflicts::parse_server_names "$reg_sites_dir")

      # Compare each new domain against registered domains
      printf '%s\n' "$new_domains" | while IFS= read -r domain; do
        if printf '%s\n' "$reg_domains" | grep -qx "$domain" 2>/dev/null; then
          if type log_error >/dev/null 2>&1; then
            log_error "Domain conflict: '$domain' already claimed by project '$reg_name'"
          else
            printf "ERROR: Domain conflict: '%s' already claimed by project '%s'\n" "$domain" "$reg_name" >&2
          fi
          # Signal conflict via temp file (subshell can't set parent vars)
          printf "1" > "/tmp/.nself-conflict-$$"
        fi
      done

      # Check for conflict signal from subshell
      if [[ -f "/tmp/.nself-conflict-$$" ]]; then
        rm -f "/tmp/.nself-conflict-$$"
        has_conflict=1
      fi
    fi

    i=$((i + 1))
  done

  return $has_conflict
}

# ---------------------------------------------------------------------------
# conflicts::check_all — Check all registered projects for conflicts
#
# Returns:
#   0 if no conflicts, 1 if conflicts found
# ---------------------------------------------------------------------------
conflicts::check_all() {
  # Source registry if not already loaded
  if ! type registry::init >/dev/null 2>&1; then
    if [[ -f "$_CONFLICT_LIB_DIR/registry.sh" ]]; then
      source "$_CONFLICT_LIB_DIR/registry.sh"
    else
      return 0
    fi
  fi

  registry::init 2>/dev/null || true

  local registry_file="$HOME/.nself/nginx/registry.json"
  if [[ ! -f "$registry_file" ]]; then
    return 0
  fi

  local project_count
  project_count=$(jq -r '.projects | length' "$registry_file" 2>/dev/null) || project_count=0

  if [[ $project_count -lt 2 ]]; then
    return 0
  fi

  # Build domain→project map using parallel arrays (Bash 3.2 compatible)
  local all_domains=""
  local all_owners=""
  local has_conflict=0

  local i=0
  while [[ $i -lt $project_count ]]; do
    local proj_name
    local proj_path
    proj_name=$(jq -r ".projects[$i].name" "$registry_file")
    proj_path=$(jq -r ".projects[$i].path" "$registry_file")

    local sites_dir="$proj_path/nginx/sites"
    if [[ -d "$sites_dir" ]]; then
      local domains
      domains=$(conflicts::parse_server_names "$sites_dir")

      printf '%s\n' "$domains" | while IFS= read -r domain; do
        if [[ -z "$domain" ]]; then
          continue
        fi
        # Check if domain already seen
        if printf '%s\n' "$all_domains" | grep -qx "$domain" 2>/dev/null; then
          # Find existing owner
          local line_num
          line_num=$(printf '%s\n' "$all_domains" | grep -nx "$domain" | head -1 | cut -d: -f1)
          local existing_owner
          existing_owner=$(printf '%s\n' "$all_owners" | sed -n "${line_num}p")

          if type log_error >/dev/null 2>&1; then
            log_error "Domain conflict: '$domain' claimed by both '$existing_owner' and '$proj_name'"
          else
            printf "ERROR: Domain conflict: '%s' claimed by both '%s' and '%s'\n" "$domain" "$existing_owner" "$proj_name" >&2
          fi
          printf "1" > "/tmp/.nself-conflict-all-$$"
        fi
      done

      # Accumulate (outside subshell for the parallel-array approach)
      local dom_list
      dom_list=$(conflicts::parse_server_names "$sites_dir")
      if [[ -n "$dom_list" ]]; then
        if [[ -n "$all_domains" ]]; then
          all_domains=$(printf '%s\n%s' "$all_domains" "$dom_list")
          local owner_lines=""
          local count
          count=$(printf '%s\n' "$dom_list" | wc -l | tr -d ' ')
          local j=0
          while [[ $j -lt $count ]]; do
            if [[ -n "$owner_lines" ]]; then
              owner_lines=$(printf '%s\n%s' "$owner_lines" "$proj_name")
            else
              owner_lines="$proj_name"
            fi
            j=$((j + 1))
          done
          all_owners=$(printf '%s\n%s' "$all_owners" "$owner_lines")
        else
          all_domains="$dom_list"
          local owner_lines=""
          local count
          count=$(printf '%s\n' "$dom_list" | wc -l | tr -d ' ')
          local j=0
          while [[ $j -lt $count ]]; do
            if [[ -n "$owner_lines" ]]; then
              owner_lines=$(printf '%s\n%s' "$owner_lines" "$proj_name")
            else
              owner_lines="$proj_name"
            fi
            j=$((j + 1))
          done
          all_owners="$owner_lines"
        fi
      fi
    fi

    i=$((i + 1))
  done

  if [[ -f "/tmp/.nself-conflict-all-$$" ]]; then
    rm -f "/tmp/.nself-conflict-all-$$"
    has_conflict=1
  fi

  return $has_conflict
}
