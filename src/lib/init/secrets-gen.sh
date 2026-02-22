#!/usr/bin/env bash
# secrets-gen.sh - Strong Secret Generation for nself init
# Part of nself v0.9.6+ - Security First Implementation
#
# This module generates strong random secrets during init
# and replaces default weak values with secure ones

set -euo pipefail

# Get module directory
INIT_MODULE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SECURITY_LIB_DIR="$INIT_MODULE_DIR/../security"

# Source secret generation functions
if [[ -f "$SECURITY_LIB_DIR/secrets.sh" ]]; then
  source "$SECURITY_LIB_DIR/secrets.sh"
fi

# ============================================================================
# Secret Generation for Init
# ============================================================================

# Generate strong random secret (fallback if secrets.sh not available)
generate_random_secret() {
  local length="${1:-32}"
  local type="${2:-hex}"

  # Try to use secrets::generate_random if available
  if command -v secrets::generate_random >/dev/null 2>&1; then
    secrets::generate_random "$length" "$type"
    return $?
  fi

  # Fallback implementation
  case "$type" in
    hex)
      if command -v openssl >/dev/null 2>&1; then
        openssl rand -hex "$((length / 2))" | head -c "$length"
      elif [[ -f /dev/urandom ]]; then
        head -c "$((length / 2))" /dev/urandom | od -An -tx1 | tr -d ' \n' | head -c "$length"
      else
        # Last resort: date + process ID
        printf "%s" "$(date +%s%N)$$" | sha256sum | cut -c1-"$length"
      fi
      ;;
    alphanumeric)
      if command -v openssl >/dev/null 2>&1; then
        openssl rand -base64 "$((length * 2))" | tr -dc 'a-zA-Z0-9' | head -c "$length"
      elif [[ -f /dev/urandom ]]; then
        head -c "$((length * 2))" /dev/urandom | base64 | tr -dc 'a-zA-Z0-9' | head -c "$length"
      else
        # Fallback
        printf "%s%s" "$(date +%s%N)" "$$" | sha256sum | tr -dc 'a-zA-Z0-9' | head -c "$length"
      fi
      ;;
    *)
      # Default to hex
      generate_random_secret "$length" "hex"
      ;;
  esac
}

# Generate environment-appropriate secrets with varying strength
auto_generate_secrets_for_env() {
  local env="${ENV:-dev}"
  local env_file="${1:-.env}"

  if [[ ! -f "$env_file" ]]; then
    return 0
  fi

  # Secret strength by environment
  local postgres_length=32
  local hasura_length=64
  local jwt_length=64
  local minio_length=32
  local search_length=32

  if [[ "$env" == "production" ]] || [[ "$env" == "prod" ]]; then
    postgres_length=48
    hasura_length=96
    jwt_length=96
    minio_length=48
    search_length=48
  elif [[ "$env" == "staging" ]]; then
    postgres_length=40
    hasura_length=80
    jwt_length=80
    minio_length=40
    search_length=40
  fi

  # Generate POSTGRES_PASSWORD if empty
  if ! grep -q "^POSTGRES_PASSWORD=.\\+" "$env_file" 2>/dev/null; then
    local pg_pass
    pg_pass=$(generate_random_secret "$postgres_length" "alphanumeric")
    if grep -q "^POSTGRES_PASSWORD=" "$env_file" 2>/dev/null; then
      sed -i.bak "s|^POSTGRES_PASSWORD=.*|POSTGRES_PASSWORD=$pg_pass|" "$env_file"
    else
      printf "POSTGRES_PASSWORD=%s\n" "$pg_pass" >> "$env_file"
    fi
  fi

  # Generate HASURA_GRAPHQL_ADMIN_SECRET if empty
  if ! grep -q "^HASURA_GRAPHQL_ADMIN_SECRET=.\\+" "$env_file" 2>/dev/null; then
    local hasura_secret
    hasura_secret=$(generate_random_secret "$hasura_length" "hex")
    if grep -q "^HASURA_GRAPHQL_ADMIN_SECRET=" "$env_file" 2>/dev/null; then
      sed -i.bak "s|^HASURA_GRAPHQL_ADMIN_SECRET=.*|HASURA_GRAPHQL_ADMIN_SECRET=$hasura_secret|" "$env_file"
    else
      printf "HASURA_GRAPHQL_ADMIN_SECRET=%s\n" "$hasura_secret" >> "$env_file"
    fi
  fi

  # Generate HASURA_JWT_KEY if empty
  if ! grep -q "^HASURA_JWT_KEY=.\\+" "$env_file" 2>/dev/null; then
    local jwt_key
    jwt_key=$(generate_random_secret "$jwt_length" "hex")
    if grep -q "^HASURA_JWT_KEY=" "$env_file" 2>/dev/null; then
      sed -i.bak "s|^HASURA_JWT_KEY=.*|HASURA_JWT_KEY=$jwt_key|" "$env_file"
    else
      printf "HASURA_JWT_KEY=%s\n" "$jwt_key" >> "$env_file"
    fi
  fi

  # Generate MINIO_ROOT_PASSWORD if empty and MinIO enabled
  if grep -q "^MINIO_ENABLED=true" "$env_file" 2>/dev/null && ! grep -q "^MINIO_ROOT_PASSWORD=.\\+" "$env_file" 2>/dev/null; then
    local minio_pass
    minio_pass=$(generate_random_secret "$minio_length" "alphanumeric")
    if grep -q "^MINIO_ROOT_PASSWORD=" "$env_file" 2>/dev/null; then
      sed -i.bak "s|^MINIO_ROOT_PASSWORD=.*|MINIO_ROOT_PASSWORD=$minio_pass|" "$env_file"
    else
      printf "MINIO_ROOT_PASSWORD=%s\n" "$minio_pass" >> "$env_file"
    fi
    # Also set MINIO_ROOT_USER if not set
    if ! grep -q "^MINIO_ROOT_USER=.\\+" "$env_file" 2>/dev/null; then
      if grep -q "^MINIO_ROOT_USER=" "$env_file" 2>/dev/null; then
        sed -i.bak "s|^MINIO_ROOT_USER=.*|MINIO_ROOT_USER=admin|" "$env_file"
      else
        printf "MINIO_ROOT_USER=admin\n" >> "$env_file"
      fi
    fi
  fi

  # Generate GRAFANA_ADMIN_PASSWORD if empty and monitoring enabled
  if grep -q "^MONITORING_ENABLED=true" "$env_file" 2>/dev/null && ! grep -q "^GRAFANA_ADMIN_PASSWORD=.\\+" "$env_file" 2>/dev/null; then
    local grafana_pass
    grafana_pass=$(generate_random_secret "$postgres_length" "alphanumeric")
    if grep -q "^GRAFANA_ADMIN_PASSWORD=" "$env_file" 2>/dev/null; then
      sed -i.bak "s|^GRAFANA_ADMIN_PASSWORD=.*|GRAFANA_ADMIN_PASSWORD=$grafana_pass|" "$env_file"
    else
      printf "GRAFANA_ADMIN_PASSWORD=%s\n" "$grafana_pass" >> "$env_file"
    fi
    # Also set GRAFANA_ADMIN_USER if not set
    if ! grep -q "^GRAFANA_ADMIN_USER=.\\+" "$env_file" 2>/dev/null; then
      if grep -q "^GRAFANA_ADMIN_USER=" "$env_file" 2>/dev/null; then
        sed -i.bak "s|^GRAFANA_ADMIN_USER=.*|GRAFANA_ADMIN_USER=admin|" "$env_file"
      else
        printf "GRAFANA_ADMIN_USER=admin\n" >> "$env_file"
      fi
    fi
  fi

  # Generate MEILISEARCH_MASTER_KEY if empty and search enabled
  if grep -q "^MEILISEARCH_ENABLED=true" "$env_file" 2>/dev/null && ! grep -q "^MEILISEARCH_MASTER_KEY=.\\+" "$env_file" 2>/dev/null; then
    local search_key
    search_key=$(generate_random_secret "$search_length" "hex")
    if grep -q "^MEILISEARCH_MASTER_KEY=" "$env_file" 2>/dev/null; then
      sed -i.bak "s|^MEILISEARCH_MASTER_KEY=.*|MEILISEARCH_MASTER_KEY=$search_key|" "$env_file"
    else
      printf "MEILISEARCH_MASTER_KEY=%s\n" "$search_key" >> "$env_file"
    fi
  fi

  # Generate ADMIN_SECRET_KEY if empty and admin enabled
  if grep -q "^NSELF_ADMIN_ENABLED=true" "$env_file" 2>/dev/null && ! grep -q "^ADMIN_SECRET_KEY=.\\+" "$env_file" 2>/dev/null; then
    local admin_key
    admin_key=$(generate_random_secret "$jwt_length" "hex")
    if grep -q "^ADMIN_SECRET_KEY=" "$env_file" 2>/dev/null; then
      sed -i.bak "s|^ADMIN_SECRET_KEY=.*|ADMIN_SECRET_KEY=$admin_key|" "$env_file"
    else
      printf "ADMIN_SECRET_KEY=%s\n" "$admin_key" >> "$env_file"
    fi
  fi

  # Clean up backup files
  rm -f "$env_file.bak"
}

# Replace default secrets in environment file
replace_default_secrets_in_file() {
  local env_file="$1"
  local skip_replacement="${2:-false}"

  if [[ ! -f "$env_file" ]]; then
    return 0
  fi

  # Skip replacement if flag is set (for --keep-defaults)
  if [[ "$skip_replacement" == "true" ]]; then
    return 0
  fi

  local temp_file
  temp_file=$(mktemp)

  # Define default secrets to replace
  # Bash 3.2 compatible: parallel arrays instead of associative array (no local -A)
  local _sr_keys="POSTGRES_PASSWORD HASURA_GRAPHQL_ADMIN_SECRET HASURA_JWT_KEY MINIO_ROOT_PASSWORD S3_SECRET_KEY S3_ACCESS_KEY"
  local _sr_vals="postgres-dev-password hasura-admin-secret-dev development-secret-key-minimum-32-characters-long minioadmin storage-secret-key-dev storage-access-key-dev"

  # Track if we made any replacements
  local replaced=false

  # Process each line
  while IFS= read -r line || [[ -n "$line" ]]; do
    local modified_line="$line"
    local line_modified=false

    # Check each secret pattern using parallel arrays (Bash 3.2 compatible)
    local _key_idx=0
    for var_name in $_sr_keys; do
      _key_idx=$((_key_idx + 1))
      local default_value
      default_value=$(printf "%s" "$_sr_vals" | tr ' ' '\n' | awk "NR==$_key_idx")

      # Check if this line sets this variable with the default value
      # Match lines starting with VAR=value (Bash 3.2 compatible, no regex)
      case "$line" in
        "${var_name}=${default_value}"|"${var_name}=${default_value} "*)
          # Generate strong replacement
          local new_value

          case "$var_name" in
            *PASSWORD*)
              # Passwords: 32 char alphanumeric
              new_value=$(generate_random_secret 32 alphanumeric)
              ;;
            *SECRET*|*KEY*)
              # Secrets/keys: 64 char hex
              new_value=$(generate_random_secret 64 hex)
              ;;
            *)
              # Default: 32 char hex
              new_value=$(generate_random_secret 32 hex)
              ;;
          esac

          modified_line="${var_name}=${new_value}"
          line_modified=true
          replaced=true
          break
          ;;
      esac
    done

    printf "%s\n" "$modified_line" >>"$temp_file"
  done <"$env_file"

  # Only replace file if we made changes
  if [[ "$replaced" == "true" ]]; then
    mv "$temp_file" "$env_file"
    chmod 600 "$env_file"
    return 0
  else
    rm -f "$temp_file"
    return 1
  fi
}

# Generate secrets section for new .env file
generate_secrets_section() {
  local env_type="${1:-dev}"

  cat <<EOF

#####################################
# 🔐 Security Secrets
#####################################
# IMPORTANT: These are randomly generated secure values
# DO NOT commit these to version control
# For production, use: nself auth security rotate <SECRET_NAME>

EOF

  # Generate based on environment type
  if [[ "$env_type" == "production" ]] || [[ "$env_type" == "prod" ]]; then
    # Production: Ultra-strong secrets
    printf "POSTGRES_PASSWORD=%s\n" "$(generate_random_secret 48 alphanumeric)"
    printf "HASURA_GRAPHQL_ADMIN_SECRET=%s\n" "$(generate_random_secret 96 hex)"
    printf "HASURA_JWT_KEY=%s\n" "$(generate_random_secret 96 hex)"
    printf "MINIO_ROOT_PASSWORD=%s\n" "$(generate_random_secret 48 alphanumeric)"
    printf "S3_SECRET_KEY=%s\n" "$(generate_random_secret 64 hex)"
    printf "S3_ACCESS_KEY=%s\n" "$(generate_random_secret 32 alphanumeric)"
  else
    # Development: Strong but shorter
    printf "POSTGRES_PASSWORD=%s\n" "$(generate_random_secret 32 alphanumeric)"
    printf "HASURA_GRAPHQL_ADMIN_SECRET=%s\n" "$(generate_random_secret 64 hex)"
    printf "HASURA_JWT_KEY=%s\n" "$(generate_random_secret 64 hex)"
    printf "MINIO_ROOT_PASSWORD=%s\n" "$(generate_random_secret 32 alphanumeric)"
    printf "S3_SECRET_KEY=%s\n" "$(generate_random_secret 48 hex)"
    printf "S3_ACCESS_KEY=%s\n" "$(generate_random_secret 24 alphanumeric)"
  fi

  printf "\n"
}

# Add strong secrets to environment file if they're missing or weak
enhance_env_file_security() {
  local env_file="$1"
  local force="${2:-false}"

  if [[ ! -f "$env_file" ]]; then
    return 0
  fi

  local needs_enhancement=false

  # Check for weak default secrets
  local weak_patterns=(
    "postgres-dev-password"
    "hasura-admin-secret-dev"
    "development-secret-key"
    "minioadmin"
    "storage-secret-key-dev"
    "storage-access-key-dev"
    "admin"
    "password"
    "secret"
  )

  for pattern in "${weak_patterns[@]}"; do
    if grep -qi "$pattern" "$env_file" 2>/dev/null; then
      needs_enhancement=true
      break
    fi
  done

  if [[ "$needs_enhancement" == "true" ]] || [[ "$force" == "true" ]]; then
    replace_default_secrets_in_file "$env_file" "false"
    return $?
  fi

  return 1
}

# Export functions
export -f generate_random_secret
export -f auto_generate_secrets_for_env
export -f replace_default_secrets_in_file
export -f generate_secrets_section
export -f enhance_env_file_security
