#!/usr/bin/env bash
# plugin-schema-validator.sh — Validate plugin.json against schema v2
# Bash 3.2 compatible. No echo -e, no ${var,,}, no declare -A.
#
# Schema v2 adds optional fields:
#   language:         "rust" | "typescript" (default: typescript)
#   binary_name:      string (name of compiled binary)
#   health_endpoint:  string (default: /health)
#   arch_support:     array  (e.g. ["linux-x86_64", "linux-arm64", "darwin-arm64"])
#   min_memory_mb:    number (default: 128)
#   systemd_after:    array  (systemd After= units)
#
# Usage (sourced by plugin.sh / plugin install):
#   plugin_schema_validate <plugin_json_path>   → 0=valid, 1=invalid

set -o pipefail

# ---------------------------------------------------------------------------
# plugin_schema_validate <json_file>
# Returns 0 if valid, 1 on error. Prints error messages to stderr.
# ---------------------------------------------------------------------------
plugin_schema_validate() {
  local json_file="$1"

  if [ -z "$json_file" ]; then
    printf 'plugin_schema_validate: no file specified\n' >&2
    return 1
  fi

  if [ ! -f "$json_file" ]; then
    printf 'plugin_schema_validate: file not found: %s\n' "$json_file" >&2
    return 1
  fi

  local content
  content=$(cat "$json_file" 2>/dev/null)
  if [ -z "$content" ]; then
    printf 'plugin_schema_validate: empty plugin.json: %s\n' "$json_file" >&2
    return 1
  fi

  local errors=0

  # ---------------------------------------------------------------------------
  # Required fields
  # ---------------------------------------------------------------------------
  local required_fields="name slug description version category"
  local field
  for field in $required_fields; do
    if ! printf '%s' "$content" | grep -q "\"${field}\""; then
      printf 'plugin.json: missing required field: %s\n' "$field" >&2
      errors=$((errors + 1))
    fi
  done

  # ---------------------------------------------------------------------------
  # Validate "language" field (optional, must be rust|typescript if present)
  # ---------------------------------------------------------------------------
  if printf '%s' "$content" | grep -q '"language"'; then
    local lang
    lang=$(printf '%s' "$content" | grep -o '"language"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | grep -o '"[^"]*"$' | tr -d '"')
    case "$lang" in
      rust|typescript)
        ;;  # valid
      *)
        printf 'plugin.json: invalid "language" value: "%s" (must be rust or typescript)\n' "$lang" >&2
        errors=$((errors + 1))
        ;;
    esac
  fi

  # ---------------------------------------------------------------------------
  # Validate "binary_name" field (optional, string, required when language=rust)
  # ---------------------------------------------------------------------------
  local lang_value
  lang_value=$(printf '%s' "$content" | grep -o '"language"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | grep -o '"[^"]*"$' | tr -d '"')
  if [ "$lang_value" = "rust" ]; then
    if ! printf '%s' "$content" | grep -q '"binary_name"'; then
      printf 'plugin.json: "binary_name" is required when language is rust\n' >&2
      errors=$((errors + 1))
    fi
  fi

  # ---------------------------------------------------------------------------
  # Validate "health_endpoint" (optional, must start with /)
  # ---------------------------------------------------------------------------
  if printf '%s' "$content" | grep -q '"health_endpoint"'; then
    local health_ep
    health_ep=$(printf '%s' "$content" | grep -o '"health_endpoint"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | grep -o '"[^"]*"$' | tr -d '"')
    case "$health_ep" in
      /*)
        ;;  # valid — starts with /
      *)
        printf 'plugin.json: "health_endpoint" must start with /: %s\n' "$health_ep" >&2
        errors=$((errors + 1))
        ;;
    esac
  fi

  # ---------------------------------------------------------------------------
  # Validate "min_memory_mb" (optional, must be a positive integer)
  # ---------------------------------------------------------------------------
  if printf '%s' "$content" | grep -q '"min_memory_mb"'; then
    local mem_val
    mem_val=$(printf '%s' "$content" | grep -o '"min_memory_mb"[[:space:]]*:[[:space:]]*[0-9]*' | grep -o '[0-9]*$')
    if [ -z "$mem_val" ]; then
      printf 'plugin.json: "min_memory_mb" must be a positive integer\n' >&2
      errors=$((errors + 1))
    fi
  fi

  # ---------------------------------------------------------------------------
  # Validate "arch_support" (optional, must be array — basic check)
  # ---------------------------------------------------------------------------
  if printf '%s' "$content" | grep -q '"arch_support"'; then
    # Valid arch tokens we recognize
    local valid_arches="linux-x86_64 linux-arm64 darwin-arm64 darwin-x86_64 windows-x86_64"
    # Extract arch values from the array
    local arch_section
    arch_section=$(printf '%s' "$content" | grep -o '"arch_support"[[:space:]]*:[[:space:]]*\[[^]]*\]' | head -1)
    if [ -z "$arch_section" ]; then
      printf 'plugin.json: "arch_support" must be a JSON array\n' >&2
      errors=$((errors + 1))
    fi
    # At least one valid arch must be present
    local found_valid=false
    local arch
    for arch in $valid_arches; do
      if printf '%s' "$arch_section" | grep -q "\"${arch}\""; then
        found_valid=true
        break
      fi
    done
    if [ "$found_valid" = "false" ]; then
      printf 'plugin.json: "arch_support" contains no recognized arch (expected one of: %s)\n' "$valid_arches" >&2
      # Warning only — not a hard failure
    fi
  fi

  # ---------------------------------------------------------------------------
  # Return result
  # ---------------------------------------------------------------------------
  if [ "$errors" -gt 0 ]; then
    return 1
  fi
  return 0
}

# ---------------------------------------------------------------------------
# plugin_schema_get_field <json_file> <field_name>
# Extract a string field value from plugin.json.
# Prints value to stdout, empty string if not found.
# ---------------------------------------------------------------------------
plugin_schema_get_field() {
  local json_file="$1"
  local field="$2"
  if [ ! -f "$json_file" ]; then
    printf ''
    return 1
  fi
  grep -o "\"${field}\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" "$json_file" 2>/dev/null \
    | head -1 \
    | grep -o '"[^"]*"$' \
    | tr -d '"'
}

# ---------------------------------------------------------------------------
# plugin_schema_get_language <json_file>
# Returns "rust" or "typescript" (default: typescript)
# ---------------------------------------------------------------------------
plugin_schema_get_language() {
  local json_file="$1"
  local lang
  lang=$(plugin_schema_get_field "$json_file" "language")
  if [ -z "$lang" ]; then
    printf 'typescript'
  else
    printf '%s' "$lang"
  fi
}

# ---------------------------------------------------------------------------
# plugin_schema_get_health_endpoint <json_file>
# Returns health endpoint path (default: /health)
# ---------------------------------------------------------------------------
plugin_schema_get_health_endpoint() {
  local json_file="$1"
  local ep
  ep=$(plugin_schema_get_field "$json_file" "health_endpoint")
  if [ -z "$ep" ]; then
    printf '/health'
  else
    printf '%s' "$ep"
  fi
}
