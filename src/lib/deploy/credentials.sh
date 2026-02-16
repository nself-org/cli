#!/usr/bin/env bash

# credentials.sh - SSH key detection and credential management
# POSIX-compliant, no Bash 4+ features

# Get the directory where this script is located
DEPLOY_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

LIB_ROOT="$(dirname "$DEPLOY_LIB_DIR")"

# Source dependencies
source "$LIB_ROOT/utils/display.sh" 2>/dev/null || true
source "$LIB_ROOT/utils/platform-compat.sh" 2>/dev/null || true

# Standard SSH key locations
SSH_KEY_PATHS=(
  "$HOME/.ssh/id_ed25519"
  "$HOME/.ssh/id_rsa"
  "$HOME/.ssh/id_ecdsa"
  "$HOME/.ssh/id_dsa"
)

# Detect available SSH keys
creds::detect_ssh_keys() {
  local keys=""

  # Check standard key locations
  for key_path in "${SSH_KEY_PATHS[@]}"; do
    if [[ -f "$key_path" ]]; then
      keys="$keys $key_path"
    fi
  done

  # Check for named keys (environment-specific)
  for key_file in "$HOME"/.ssh/*; do
    if [[ -f "$key_file" ]]; then
      # Skip public keys and known_hosts
      case "$key_file" in
        *.pub | *known_hosts* | *config | *authorized_keys)
          continue
          ;;
      esac

      # Skip if already in standard list
      local already_added=false
      for std_key in "${SSH_KEY_PATHS[@]}"; do
        if [[ "$key_file" == "$std_key" ]]; then
          already_added=true
          break
        fi
      done

      if [[ "$already_added" != "true" ]]; then
        # Verify it's actually a private key
        if head -1 "$key_file" 2>/dev/null | grep -q "PRIVATE KEY"; then
          keys="$keys $key_file"
        fi
      fi
    fi
  done

  # Trim leading space and output
  printf "%s" "${keys# }"
}

# Find SSH key for a specific host/environment
creds::find_key_for_host() {
  local host="$1"
  local env_name="${2:-}"

  # 1. Check for environment-specific key
  if [[ -n "$env_name" ]]; then
    local env_key="$HOME/.ssh/${env_name}_rsa"
    if [[ -f "$env_key" ]]; then
      printf "%s" "$env_key"
      return 0
    fi

    env_key="$HOME/.ssh/${env_name}_ed25519"
    if [[ -f "$env_key" ]]; then
      printf "%s" "$env_key"
      return 0
    fi

    env_key="$HOME/.ssh/id_${env_name}"
    if [[ -f "$env_key" ]]; then
      printf "%s" "$env_key"
      return 0
    fi
  fi

  # 2. Check SSH config for host-specific key
  if [[ -f "$HOME/.ssh/config" ]]; then
    local configured_key
    configured_key=$(awk -v host="$host" '
      /^Host / { current_host = $2 }
      current_host == host && /IdentityFile/ { print $2; exit }
    ' "$HOME/.ssh/config")

    if [[ -n "$configured_key" ]]; then
      # Expand tilde
      local expanded="${configured_key/#\~/$HOME}"
      if [[ -f "$expanded" ]]; then
        printf "%s" "$expanded"
        return 0
      fi
    fi
  fi

  # 3. Check for host-named key
  local host_key="$HOME/.ssh/${host}_rsa"
  if [[ -f "$host_key" ]]; then
    printf "%s" "$host_key"
    return 0
  fi

  # 4. Fall back to default key (prefer ed25519 over rsa)
  if [[ -f "$HOME/.ssh/id_ed25519" ]]; then
    printf "%s" "$HOME/.ssh/id_ed25519"
    return 0
  fi

  if [[ -f "$HOME/.ssh/id_rsa" ]]; then
    printf "%s" "$HOME/.ssh/id_rsa"
    return 0
  fi

  # No key found
  return 1
}

# List available SSH keys with details
creds::list_keys() {
  local keys
  keys=$(creds::detect_ssh_keys)

  if [[ -z "$keys" ]]; then
    printf "No SSH keys found in ~/.ssh/\n"
    printf "\nCreate one with: ssh-keygen -t ed25519 -C \"your_email@example.com\"\n"
    return 1
  fi

  printf "Available SSH keys:\n\n"

  for key in $keys; do
    local key_name
    key_name=$(basename "$key")

    # Get key type and fingerprint
    local key_info
    if command -v ssh-keygen >/dev/null 2>&1; then
      key_info=$(ssh-keygen -lf "$key" 2>/dev/null | awk '{print $2, $4}')
    fi

    # Check if key has passphrase (basic check)
    local has_passphrase="unknown"
    if head -5 "$key" 2>/dev/null | grep -q "ENCRYPTED"; then
      has_passphrase="yes"
    elif head -5 "$key" 2>/dev/null | grep -q "PRIVATE KEY"; then
      has_passphrase="no"
    fi

    printf "  ${COLOR_CYAN}%s${COLOR_RESET}\n" "$key_name"
    printf "    Path: %s\n" "$key"
    if [[ -n "$key_info" ]]; then
      printf "    Fingerprint: %s\n" "$key_info"
    fi
    printf "    Encrypted: %s\n" "$has_passphrase"
    printf "\n"
  done
}

# Verify SSH key has correct permissions
creds::verify_key_permissions() {
  local key_file="$1"

  if [[ ! -f "$key_file" ]]; then
    log_error "Key file not found: $key_file"
    return 1
  fi

  local perms
  perms=$(safe_stat_perms "$key_file")

  if [[ "$perms" != "600" ]] && [[ "$perms" != "400" ]]; then
    log_warning "Insecure permissions on SSH key: $perms (should be 600)"
    printf "Fix with: chmod 600 %s\n" "$key_file"
    return 1
  fi

  return 0
}

# Fix SSH key permissions
creds::fix_key_permissions() {
  local key_file="$1"

  if [[ ! -f "$key_file" ]]; then
    log_error "Key file not found: $key_file"
    return 1
  fi

  chmod 600 "$key_file"
  log_success "Fixed permissions on: $key_file"
}

# Add SSH key to agent
creds::add_to_agent() {
  local key_file="$1"

  # Check if agent is running
  if [[ -z "${SSH_AUTH_SOCK:-}" ]]; then
    log_warning "SSH agent not running. Start with: eval \$(ssh-agent -s)"
    return 1
  fi

  # Expand tilde
  local expanded="${key_file/#\~/$HOME}"

  # Add key
  ssh-add "$expanded" 2>/dev/null
  return $?
}

# Check if key is in SSH agent
creds::key_in_agent() {
  local key_file="$1"

  if [[ -z "${SSH_AUTH_SOCK:-}" ]]; then
    return 1
  fi

  # Get fingerprint of key
  local fingerprint
  fingerprint=$(ssh-keygen -lf "$key_file" 2>/dev/null | awk '{print $2}')

  if [[ -z "$fingerprint" ]]; then
    return 1
  fi

  # Check if fingerprint is in agent
  ssh-add -l 2>/dev/null | grep -q "$fingerprint"
}

# Get SSH key from environment configuration
creds::get_env_key() {
  local env_name="$1"
  local env_dir=".environments/$env_name"

  # Check server.json for key path
  if [[ -f "$env_dir/server.json" ]]; then
    local key_path
    key_path=$(grep '"key"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)

    if [[ -n "$key_path" ]]; then
      # Expand tilde
      local expanded="${key_path/#\~/$HOME}"
      if [[ -f "$expanded" ]]; then
        printf "%s" "$expanded"
        return 0
      fi
    fi
  fi

  # Fall back to auto-detection based on environment name
  creds::find_key_for_host "" "$env_name"
}

# Prompt user to select SSH key
creds::prompt_key_selection() {
  local keys
  keys=$(creds::detect_ssh_keys)

  if [[ -z "$keys" ]]; then
    log_error "No SSH keys found"
    return 1
  fi

  # Convert to array for numbered selection
  local key_array=()
  local i=1

  printf "Select SSH key:\n\n"

  for key in $keys; do
    key_array+=("$key")
    printf "  %d) %s\n" "$i" "$(basename "$key")"
    i=$((i + 1))
  done

  printf "\n"
  printf "Enter number (or full path to key): "
  read -r selection

  # Handle numeric selection
  if [[ "$selection" =~ ^[0-9]+$ ]]; then
    local index=$((selection - 1))
    if [[ $index -ge 0 ]] && [[ $index -lt ${#key_array[@]} ]]; then
      printf "%s" "${key_array[$index]}"
      return 0
    fi
  fi

  # Handle path selection
  local expanded="${selection/#\~/$HOME}"
  if [[ -f "$expanded" ]]; then
    printf "%s" "$expanded"
    return 0
  fi

  log_error "Invalid selection"
  return 1
}

# Store credential in macOS keychain (if available)
creds::store_in_keychain() {
  local service="$1"
  local account="$2"
  local password="$3"

  if ! is_macos; then
    log_warning "Keychain storage only available on macOS"
    return 1
  fi

  # Use security command to store
  security add-generic-password \
    -s "$service" \
    -a "$account" \
    -w "$password" \
    -U 2>/dev/null

  return $?
}

# Get credential from macOS keychain
creds::get_from_keychain() {
  local service="$1"
  local account="$2"

  if ! is_macos; then
    return 1
  fi

  security find-generic-password \
    -s "$service" \
    -a "$account" \
    -w 2>/dev/null
}

# Delete credential from keychain
creds::delete_from_keychain() {
  local service="$1"
  local account="$2"

  if ! is_macos; then
    return 1
  fi

  security delete-generic-password \
    -s "$service" \
    -a "$account" 2>/dev/null

  return $?
}

# Export functions
export -f creds::detect_ssh_keys
export -f creds::find_key_for_host
export -f creds::list_keys
export -f creds::verify_key_permissions
export -f creds::fix_key_permissions
export -f creds::add_to_agent
export -f creds::key_in_agent
export -f creds::get_env_key
export -f creds::prompt_key_selection
export -f creds::store_in_keychain
export -f creds::get_from_keychain
export -f creds::delete_from_keychain
