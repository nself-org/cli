#!/usr/bin/env bash

# conflict-check.sh - Subdomain conflict detection across registered projects
# Parses server_name directives from nginx site configs and detects duplicates
# Bash 3.2 compatible — printf only, parallel arrays, no Bash 4+ features

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
# _conflicts_match_domain — Check if a domain matches any in a domain list,
# with wildcard-aware matching (*.foo matches bar.foo).
#
# Args:
#   $1 — domain to check
#   $2 — newline-separated list of existing domains
#
# Returns:
#   0 if match found, 1 if no match
# ---------------------------------------------------------------------------
_conflicts_match_domain() {
  local check_domain="$1"
  local domain_list="$2"

  [[ -z "$domain_list" ]] && return 1

  while IFS= read -r existing; do
    [[ -z "$existing" ]] && continue

    # Exact match
    if [[ "$check_domain" == "$existing" ]]; then
      return 0
    fi

    # Wildcard: *.foo.bar matches anything.foo.bar
    # If existing is *.suffix, check if check_domain ends with .suffix
    if [[ "$existing" == \*.* ]]; then
      local suffix="${existing#\*}"
      case "$check_domain" in
        *"$suffix") return 0 ;;
      esac
    fi

    # If check_domain is *.suffix, check if existing ends with .suffix
    if [[ "$check_domain" == \*.* ]]; then
      local suffix="${check_domain#\*}"
      case "$existing" in
        *"$suffix") return 0 ;;
      esac
    fi
  done <<EOF
$domain_list
EOF

  return 1
}

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

  local registry_file="${NSELF_REGISTRY_FILE:-$HOME/.nself/nginx/registry.json}"
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

      # Compare each new domain against registered domains (using here-string
      # to avoid subshell from pipe, so we can set has_conflict directly)
      while IFS= read -r domain; do
        [[ -z "$domain" ]] && continue
        if _conflicts_match_domain "$domain" "$reg_domains"; then
          if type log_error >/dev/null 2>&1; then
            log_error "Domain conflict: '$domain' already claimed by project '$reg_name'"
          else
            printf "ERROR: Domain conflict: '%s' already claimed by project '%s'\n" "$domain" "$reg_name" >&2
          fi
          has_conflict=1
        fi
      done <<EOF
$new_domains
EOF
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

  # Build domain->project map using parallel arrays (Bash 3.2 compatible).
  # Uses here-doc redirect instead of pipe to keep variable mutations in
  # the current shell (avoids the subshell variable scope problem).
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

      # Check each domain against accumulated list (here-doc, not pipe)
      while IFS= read -r domain; do
        [[ -z "$domain" ]] && continue

        # Wildcard-aware match against all previously accumulated domains
        if [[ -n "$all_domains" ]] && _conflicts_match_domain "$domain" "$all_domains"; then
          # Find the first matching owner for error message
          local existing_owner="unknown"
          local _line_idx=1
          while IFS= read -r _acc_domain; do
            if [[ "$_acc_domain" == "$domain" ]] || _conflicts_match_domain "$domain" "$_acc_domain"; then
              existing_owner=$(printf '%s\n' "$all_owners" | sed -n "${_line_idx}p")
              break
            fi
            _line_idx=$((_line_idx + 1))
          done <<OWNERS
$all_domains
OWNERS

          if type log_error >/dev/null 2>&1; then
            log_error "Domain conflict: '$domain' claimed by both '$existing_owner' and '$proj_name'"
          else
            printf "ERROR: Domain conflict: '%s' claimed by both '%s' and '%s'\n" "$domain" "$existing_owner" "$proj_name" >&2
          fi
          has_conflict=1
        fi
      done <<DOMAINS
$domains
DOMAINS

      # Accumulate domains and owners (one owner line per domain line)
      if [[ -n "$domains" ]]; then
        local owner_lines=""
        while IFS= read -r _d; do
          [[ -z "$_d" ]] && continue
          if [[ -n "$owner_lines" ]]; then
            owner_lines=$(printf '%s\n%s' "$owner_lines" "$proj_name")
          else
            owner_lines="$proj_name"
          fi
        done <<DOMS
$domains
DOMS

        if [[ -n "$all_domains" ]]; then
          all_domains=$(printf '%s\n%s' "$all_domains" "$domains")
          all_owners=$(printf '%s\n%s' "$all_owners" "$owner_lines")
        else
          all_domains="$domains"
          all_owners="$owner_lines"
        fi
      fi
    fi

    i=$((i + 1))
  done

  return $has_conflict
}
