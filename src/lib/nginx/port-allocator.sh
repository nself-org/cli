#!/usr/bin/env bash
# port-allocator.sh - Port range allocator for multi-project nginx registry
#
# Each registered project gets a dedicated 20-port block starting from
# the registry's portBase (default 10000). Freed ranges are reused.
#
# Namespace: ports::

# Prevent double-sourcing
[[ "${PORT_ALLOCATOR_SOURCED:-}" == "1" ]] && return 0
export PORT_ALLOCATOR_SOURCED=1

# Ports per project block
PORTS_BLOCK_SIZE=20

# Default starting port when registry has no portBase
PORTS_DEFAULT_BASE=10000

# ---------------------------------------------------------------------------
# ports::parse_range — Split a "START:END" string into two values.
#
# Usage:
#   read -r start end <<< "$(ports::parse_range "10000:10019")"
#
# Args:
#   $1 — range string in START:END format
#
# Stdout:
#   "START END" (space-separated)
#
# Returns:
#   0 on success, 1 if format is invalid
# ---------------------------------------------------------------------------
ports::parse_range() {
  local range_string="${1:-}"

  if [[ -z "$range_string" ]]; then
    printf "Error: range string required\n" >&2
    return 1
  fi

  # Validate format: two integers separated by a colon
  local start=""
  local end=""
  start="${range_string%%:*}"
  end="${range_string##*:}"

  if [[ "$start" == "$range_string" ]] || [[ -z "$start" ]] || [[ -z "$end" ]]; then
    printf "Error: invalid range format '%s' (expected START:END)\n" "$range_string" >&2
    return 1
  fi

  # Validate both are integers
  case "$start" in
    *[!0-9]*) printf "Error: start port '%s' is not a number\n" "$start" >&2; return 1 ;;
  esac
  case "$end" in
    *[!0-9]*) printf "Error: end port '%s' is not a number\n" "$end" >&2; return 1 ;;
  esac

  printf "%s %s" "$start" "$end"
}

# ---------------------------------------------------------------------------
# ports::is_range_free — Check whether a candidate range overlaps any
# existing allocation in the registry.
#
# Args:
#   $1 — candidate start port
#   $2 — candidate end port
#   $3 — path to registry.json
#
# Returns:
#   0 if the range is free (no overlap)
#   1 if the range overlaps an existing allocation
#   2 on error (missing file, bad JSON, etc.)
# ---------------------------------------------------------------------------
ports::is_range_free() {
  local candidate_start="${1:-}"
  local candidate_end="${2:-}"
  local registry_file="${3:-}"

  if [[ -z "$candidate_start" ]] || [[ -z "$candidate_end" ]] || [[ -z "$registry_file" ]]; then
    printf "Error: ports::is_range_free requires start, end, registry_file\n" >&2
    return 2
  fi

  if [[ ! -f "$registry_file" ]]; then
    printf "Error: registry file not found: %s\n" "$registry_file" >&2
    return 2
  fi

  if ! command -v jq >/dev/null 2>&1; then
    printf "Error: jq is required but not installed\n" >&2
    return 2
  fi

  # Extract all existing port ranges as "start end" pairs, one per line.
  # Registry stores portStart and portEnd as separate fields per project.
  local existing_ranges=""
  existing_ranges="$(jq -r '
    .projects[]? |
    select(.portStart != null and .portEnd != null) |
    "\(.portStart) \(.portEnd)"
  ' "$registry_file" 2>/dev/null)" || {
    printf "Error: failed to parse registry JSON\n" >&2
    return 2
  }

  # No existing ranges means it is free
  if [[ -z "$existing_ranges" ]]; then
    return 0
  fi

  # Check each existing range for overlap.
  # Two ranges [A,B] and [C,D] overlap iff A <= D && C <= B.
  local ex_start=""
  local ex_end=""
  while read -r ex_start ex_end; do
    [[ -z "$ex_start" ]] && continue
    if [[ "$candidate_start" -le "$ex_end" ]] && [[ "$ex_start" -le "$candidate_end" ]]; then
      return 1
    fi
  done <<EOF
$existing_ranges
EOF

  return 0
}

# ---------------------------------------------------------------------------
# ports::allocate_range — Find the next free 20-port block in the registry.
#
# Algorithm:
#   1. Read portBase from registry (default 10000).
#   2. Collect all existing portRange start values, sort them.
#   3. Walk from portBase in BLOCK_SIZE increments.
#   4. First block that does not overlap any allocation wins.
#   5. Gaps left by removed projects are reused.
#
# Args:
#   $1 — path to registry.json
#
# Stdout:
#   "START:END" (e.g., "10000:10019")
#
# Returns:
#   0 on success, 1 on error
# ---------------------------------------------------------------------------
ports::allocate_range() {
  local registry_file="${1:-}"

  if [[ -z "$registry_file" ]]; then
    printf "Error: registry file path required\n" >&2
    return 1
  fi

  if [[ ! -f "$registry_file" ]]; then
    printf "Error: registry file not found: %s\n" "$registry_file" >&2
    return 1
  fi

  if ! command -v jq >/dev/null 2>&1; then
    printf "Error: jq is required but not installed\n" >&2
    return 1
  fi

  # Read portBase from registry, default to PORTS_DEFAULT_BASE
  local port_base=""
  port_base="$(jq -r '.portBase // empty' "$registry_file" 2>/dev/null)" || true
  if [[ -z "$port_base" ]]; then
    port_base="$PORTS_DEFAULT_BASE"
  fi

  # Collect all existing range start ports, sorted numerically.
  # This gives us the landscape of allocated blocks.
  local allocated_starts=""
  allocated_starts="$(jq -r '
    .projects[]? |
    select(.portStart != null) |
    .portStart
  ' "$registry_file" 2>/dev/null | sort -n)" || true

  # If no allocations exist, return the first block
  if [[ -z "$allocated_starts" ]]; then
    local first_end=$((port_base + PORTS_BLOCK_SIZE - 1))
    printf "%d:%d" "$port_base" "$first_end"
    return 0
  fi

  # Walk from portBase in BLOCK_SIZE steps, checking for gaps.
  # We try each aligned block and see if it is free.
  local candidate_start="$port_base"

  # Safety limit: don't scan more than 500 blocks (10000 ports)
  local max_iterations=500
  local iteration=0

  while [[ "$iteration" -lt "$max_iterations" ]]; do
    local candidate_end=$((candidate_start + PORTS_BLOCK_SIZE - 1))

    if ports::is_range_free "$candidate_start" "$candidate_end" "$registry_file"; then
      printf "%d:%d" "$candidate_start" "$candidate_end"
      return 0
    fi

    candidate_start=$((candidate_start + PORTS_BLOCK_SIZE))
    iteration=$((iteration + 1))
  done

  printf "Error: no free port range found within %d blocks from port %d\n" "$max_iterations" "$port_base" >&2
  return 1
}
