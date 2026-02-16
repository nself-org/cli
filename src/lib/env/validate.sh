#!/usr/bin/env bash

# validate.sh - Environment validation functionality
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

# Validate an environment configuration
env::validate() {
  local env_name="${1:-$(env::get_current 2>/dev/null || echo 'local')}"
  local strict="${2:-false}"
  local env_dir="$ENVIRONMENTS_DIR/$env_name"

  printf "Validating environment: ${COLOR_BLUE}%s${COLOR_RESET}\n\n" "$env_name"

  local errors=0
  local warnings=0

  # Check environment exists
  if [[ ! -d "$env_dir" ]]; then
    log_error "Environment directory not found: $env_dir"
    return 1
  fi

  # Validate .env file
  if [[ -f "$env_dir/.env" ]]; then
    env::validate_env_file "$env_dir/.env" "$env_name"
    errors=$((errors + $?))
  else
    log_warning "No .env file found"
    warnings=$((warnings + 1))
  fi

  # Validate server.json for remote environments
  if [[ -f "$env_dir/server.json" ]]; then
    env::validate_server_config "$env_dir/server.json" "$env_name"
    local server_errors=$?
    if [[ $server_errors -gt 0 ]]; then
      if [[ "$strict" == "true" ]]; then
        errors=$((errors + server_errors))
      else
        warnings=$((warnings + server_errors))
      fi
    fi
  fi

  # Validate secrets file permissions
  if [[ -f "$env_dir/.env.secrets" ]]; then
    env::validate_secrets_file "$env_dir/.env.secrets"
    errors=$((errors + $?))
  fi

  # Check for required variables based on environment type
  if [[ -f "$env_dir/.env" ]]; then
    local env_type
    env_type=$(grep "^ENV=" "$env_dir/.env" 2>/dev/null | cut -d'=' -f2)
    env::validate_required_vars "$env_dir" "$env_type"
    errors=$((errors + $?))
  fi

  # Summary
  printf "\n"
  if [[ $errors -eq 0 ]] && [[ $warnings -eq 0 ]]; then
    printf "${COLOR_GREEN}✓ Environment '%s' is valid${COLOR_RESET}\n" "$env_name"
    return 0
  elif [[ $errors -eq 0 ]]; then
    printf "${COLOR_YELLOW}⚠ Environment '%s' has %d warning(s)${COLOR_RESET}\n" "$env_name" "$warnings"
    return 0
  else
    printf "${COLOR_RED}✗ Environment '%s' has %d error(s) and %d warning(s)${COLOR_RESET}\n" "$env_name" "$errors" "$warnings"
    return 1
  fi
}

# Validate .env file syntax and content
env::validate_env_file() {
  local env_file="$1"
  local env_name="$2"
  local errors=0

  printf "${COLOR_CYAN}Checking .env file...${COLOR_RESET}\n"

  local line_num=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    line_num=$((line_num + 1))

    # Skip empty lines and comments
    [[ -z "$line" ]] && continue
    [[ "$line" =~ ^[[:space:]]*# ]] && continue

    # Check for valid variable assignment
    if [[ ! "$line" =~ ^[A-Za-z_][A-Za-z0-9_]*= ]]; then
      # Skip lines that are only whitespace
      if [[ ! "$line" =~ ^[[:space:]]*$ ]]; then
        printf "  ${COLOR_RED}Line %d: Invalid syntax${COLOR_RESET}: %s\n" "$line_num" "$line"
        errors=$((errors + 1))
      fi
      continue
    fi

    # Extract key and value
    local key value
    key=$(printf "%s" "$line" | cut -d'=' -f1)
    value=$(printf "%s" "$line" | cut -d'=' -f2-)

    # Warn about empty values for important variables
    if [[ -z "$value" ]]; then
      case "$key" in
        POSTGRES_PASSWORD | HASURA_GRAPHQL_ADMIN_SECRET | JWT_SECRET)
          printf "  ${COLOR_YELLOW}Warning: %s is empty${COLOR_RESET}\n" "$key"
          ;;
      esac
    fi

    # Check for common mistakes
    if [[ "$value" =~ \$\{[^}]+\} ]] && [[ ! "$value" =~ \$\{[^}]+:-[^}]+\} ]]; then
      # Variable reference without default
      printf "  ${COLOR_YELLOW}Warning: %s references variable without default${COLOR_RESET}\n" "$key"
    fi

  done <"$env_file"

  if [[ $errors -eq 0 ]]; then
    printf "  ${COLOR_GREEN}✓ .env syntax valid${COLOR_RESET}\n"
  fi

  return $errors
}

# Validate server configuration
env::validate_server_config() {
  local server_file="$1"
  local env_name="$2"
  local errors=0

  printf "${COLOR_CYAN}Checking server.json...${COLOR_RESET}\n"

  # Check JSON syntax (basic check)
  if ! grep -q '"host"' "$server_file" 2>/dev/null; then
    printf "  ${COLOR_YELLOW}Warning: No host configured${COLOR_RESET}\n"
    errors=$((errors + 1))
  fi

  # Extract values
  local host port user key_file
  host=$(grep '"host"' "$server_file" 2>/dev/null | cut -d'"' -f4)
  port=$(grep '"port"' "$server_file" 2>/dev/null | cut -d':' -f2 | tr -d ' ,')
  user=$(grep '"user"' "$server_file" 2>/dev/null | cut -d'"' -f4)
  key_file=$(grep '"key"' "$server_file" 2>/dev/null | cut -d'"' -f4)

  # Validate host
  if [[ -z "$host" ]]; then
    printf "  ${COLOR_YELLOW}Warning: Server host not configured${COLOR_RESET}\n"
    errors=$((errors + 1))
  fi

  # Validate port
  if [[ -n "$port" ]] && [[ ! "$port" =~ ^[0-9]+$ ]]; then
    printf "  ${COLOR_RED}Error: Invalid port: %s${COLOR_RESET}\n" "$port"
    errors=$((errors + 1))
  fi

  # Validate SSH key file if specified
  if [[ -n "$key_file" ]]; then
    # Expand tilde
    local expanded_key
    expanded_key="${key_file/#\~/$HOME}"

    if [[ ! -f "$expanded_key" ]]; then
      printf "  ${COLOR_YELLOW}Warning: SSH key file not found: %s${COLOR_RESET}\n" "$key_file"
      errors=$((errors + 1))
    fi
  fi

  if [[ $errors -eq 0 ]]; then
    printf "  ${COLOR_GREEN}✓ Server configuration valid${COLOR_RESET}\n"
    if [[ -n "$host" ]]; then
      printf "    Host: %s\n" "$host"
      printf "    Port: %s\n" "${port:-22}"
      printf "    User: %s\n" "${user:-<not set>}"
    fi
  fi

  return $errors
}

# Validate secrets file permissions
env::validate_secrets_file() {
  local secrets_file="$1"
  local errors=0

  printf "${COLOR_CYAN}Checking secrets file...${COLOR_RESET}\n"

  # Check file permissions
  local perms
  perms=$(safe_stat_perms "$secrets_file")

  if [[ "$perms" != "600" ]]; then
    printf "  ${COLOR_YELLOW}Warning: Insecure permissions on secrets file: %s${COLOR_RESET}\n" "$perms"
    printf "  ${COLOR_DIM}Run: chmod 600 %s${COLOR_RESET}\n" "$secrets_file"
    errors=$((errors + 1))
  else
    printf "  ${COLOR_GREEN}✓ Secrets file permissions secure (600)${COLOR_RESET}\n"
  fi

  # Check for actual secret values
  local has_secrets=false
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" =~ ^[A-Za-z_][A-Za-z0-9_]*=.+ ]] && [[ ! "$line" =~ ^# ]]; then
      local key value
      key=$(printf "%s" "$line" | cut -d'=' -f1)
      value=$(printf "%s" "$line" | cut -d'=' -f2-)

      if [[ -n "$value" ]]; then
        has_secrets=true
        break
      fi
    fi
  done <"$secrets_file"

  if [[ "$has_secrets" != "true" ]]; then
    printf "  ${COLOR_YELLOW}Warning: Secrets file has no configured secrets${COLOR_RESET}\n"
  fi

  return $errors
}

# Validate required variables based on environment type
env::validate_required_vars() {
  local env_dir="$1"
  local env_type="$2"
  local errors=0

  printf "${COLOR_CYAN}Checking required variables...${COLOR_RESET}\n"

  local required_vars=""

  # Define required variables per environment type
  case "$env_type" in
    production | prod)
      required_vars="BASE_DOMAIN POSTGRES_DB"
      # Secrets required
      if [[ ! -f "$env_dir/.env.secrets" ]]; then
        printf "  ${COLOR_RED}Error: Production requires .env.secrets file${COLOR_RESET}\n"
        errors=$((errors + 1))
      fi
      ;;
    staging | stage)
      required_vars="BASE_DOMAIN POSTGRES_DB"
      ;;
    development | dev | local)
      required_vars="POSTGRES_DB"
      ;;
  esac

  # Check each required variable
  for var in $required_vars; do
    local value
    value=$(grep "^${var}=" "$env_dir/.env" 2>/dev/null | cut -d'=' -f2-)

    # Also check secrets file
    if [[ -z "$value" ]] && [[ -f "$env_dir/.env.secrets" ]]; then
      value=$(grep "^${var}=" "$env_dir/.env.secrets" 2>/dev/null | cut -d'=' -f2-)
    fi

    if [[ -z "$value" ]]; then
      printf "  ${COLOR_YELLOW}Warning: %s not set${COLOR_RESET}\n" "$var"
    fi
  done

  # Check for weak passwords in production
  if [[ "$env_type" == "production" ]] || [[ "$env_type" == "prod" ]]; then
    local weak_passwords="password password123 changeme admin postgres secret"
    local files_to_check="$env_dir/.env"
    [[ -f "$env_dir/.env.secrets" ]] && files_to_check="$files_to_check $env_dir/.env.secrets"

    for file in $files_to_check; do
      for weak in $weak_passwords; do
        if grep -qi "PASSWORD=${weak}" "$file" 2>/dev/null ||
          grep -qi "SECRET=${weak}" "$file" 2>/dev/null; then
          printf "  ${COLOR_RED}Error: Weak password/secret detected in production config${COLOR_RESET}\n"
          errors=$((errors + 1))
          break 2
        fi
      done
    done
  fi

  if [[ $errors -eq 0 ]]; then
    printf "  ${COLOR_GREEN}✓ Required variables present${COLOR_RESET}\n"
  fi

  return $errors
}

# Validate all environments
env::validate_all() {
  local errors=0

  for env_dir in "$ENVIRONMENTS_DIR"/*/; do
    if [[ -d "$env_dir" ]]; then
      local env_name
      env_name=$(basename "$env_dir")

      env::validate "$env_name"
      if [[ $? -ne 0 ]]; then
        errors=$((errors + 1))
      fi
      printf "\n"
    fi
  done

  if [[ $errors -eq 0 ]]; then
    printf "${COLOR_GREEN}✓ All environments valid${COLOR_RESET}\n"
    return 0
  else
    printf "${COLOR_RED}✗ %d environment(s) have issues${COLOR_RESET}\n" "$errors"
    return 1
  fi
}

# Validate variable types and formats
env::validate_types() {
  local env_file="$1"
  local errors=0
  local warnings=0

  printf "${COLOR_CYAN}Checking variable formats...${COLOR_RESET}\n"

  while IFS= read -r line || [[ -n "$line" ]]; do
    # Skip empty lines and comments
    [[ -z "$line" ]] && continue
    [[ "$line" =~ ^[[:space:]]*# ]] && continue
    [[ ! "$line" =~ ^[A-Za-z_][A-Za-z0-9_]*= ]] && continue

    local key value
    key=$(printf "%s" "$line" | cut -d'=' -f1)
    value=$(printf "%s" "$line" | cut -d'=' -f2-)

    # Remove quotes from value if present
    value="${value#\"}"
    value="${value%\"}"
    value="${value#\'}"
    value="${value%\'}"

    # Skip empty values
    [[ -z "$value" ]] && continue

    # Validate port numbers (1-65535)
    if [[ "$key" =~ _PORT$ ]] || [[ "$key" == "PORT" ]]; then
      if [[ ! "$value" =~ ^[0-9]+$ ]]; then
        printf "  ${COLOR_RED}Error: %s must be a number (got: %s)${COLOR_RESET}\n" "$key" "$value"
        errors=$((errors + 1))
      elif [[ "$value" -lt 1 ]] || [[ "$value" -gt 65535 ]]; then
        printf "  ${COLOR_RED}Error: %s must be 1-65535 (got: %s)${COLOR_RESET}\n" "$key" "$value"
        errors=$((errors + 1))
      fi
    fi

    # Validate email addresses
    if [[ "$key" =~ _EMAIL$ ]] || [[ "$key" == "EMAIL" ]] || [[ "$key" =~ SMTP_FROM ]]; then
      if [[ -n "$value" ]] && [[ ! "$value" =~ ^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$ ]]; then
        printf "  ${COLOR_YELLOW}Warning: %s may not be a valid email: %s${COLOR_RESET}\n" "$key" "$value"
        warnings=$((warnings + 1))
      fi
    fi

    # Validate boolean values
    if [[ "$key" =~ _ENABLED$ ]] || [[ "$key" == "DEBUG" ]] || [[ "$key" =~ ^ENABLE_ ]]; then
      local lower_value
      lower_value=$(printf "%s" "$value" | tr '[:upper:]' '[:lower:]')
      if [[ "$lower_value" != "true" ]] && [[ "$lower_value" != "false" ]] &&
        [[ "$lower_value" != "1" ]] && [[ "$lower_value" != "0" ]] &&
        [[ "$lower_value" != "yes" ]] && [[ "$lower_value" != "no" ]]; then
        printf "  ${COLOR_YELLOW}Warning: %s should be true/false (got: %s)${COLOR_RESET}\n" "$key" "$value"
        warnings=$((warnings + 1))
      fi
    fi

    # Validate URLs
    if [[ "$key" =~ _URL$ ]] || [[ "$key" =~ _URI$ ]]; then
      if [[ -n "$value" ]] && [[ ! "$value" =~ ^(https?|postgres|redis|mongodb|amqp):// ]]; then
        printf "  ${COLOR_YELLOW}Warning: %s may not be a valid URL: %s${COLOR_RESET}\n" "$key" "$value"
        warnings=$((warnings + 1))
      fi
    fi

    # Validate domain names (BASE_DOMAIN, etc.)
    if [[ "$key" =~ DOMAIN$ ]] || [[ "$key" == "BASE_DOMAIN" ]]; then
      if [[ "$value" != "localhost" ]] && [[ ! "$value" =~ ^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*$ ]]; then
        printf "  ${COLOR_YELLOW}Warning: %s may not be a valid domain: %s${COLOR_RESET}\n" "$key" "$value"
        warnings=$((warnings + 1))
      fi
    fi

    # Check password/secret minimum length
    if [[ "$key" =~ PASSWORD$ ]] || [[ "$key" =~ SECRET$ ]] || [[ "$key" =~ _KEY$ ]]; then
      local len=${#value}
      if [[ $len -lt 8 ]] && [[ $len -gt 0 ]]; then
        printf "  ${COLOR_YELLOW}Warning: %s is short (%d chars, recommend 8+)${COLOR_RESET}\n" "$key" "$len"
        warnings=$((warnings + 1))
      fi
    fi

  done <"$env_file"

  if [[ $errors -eq 0 ]] && [[ $warnings -eq 0 ]]; then
    printf "  ${COLOR_GREEN}✓ Variable formats valid${COLOR_RESET}\n"
  elif [[ $errors -eq 0 ]]; then
    printf "  ${COLOR_YELLOW}⚠ %d format warning(s)${COLOR_RESET}\n" "$warnings"
  fi

  return $errors
}

# Validate current project .env (not environment directory)
env::validate_project() {
  local env_file="${1:-.env}"
  local errors=0
  local warnings=0

  if [[ ! -f "$env_file" ]]; then
    # Try common alternatives
    for alt in .env.dev .env.local; do
      if [[ -f "$alt" ]]; then
        env_file="$alt"
        break
      fi
    done
  fi

  if [[ ! -f "$env_file" ]]; then
    printf "${COLOR_RED}No environment file found${COLOR_RESET}\n"
    printf "Create one with: nself init\n"
    return 1
  fi

  printf "Validating: ${COLOR_BLUE}%s${COLOR_RESET}\n\n" "$env_file"

  # Syntax validation
  env::validate_env_file "$env_file" "project"
  errors=$((errors + $?))

  # Type validation
  env::validate_types "$env_file"
  errors=$((errors + $?))

  # Check for required core variables
  printf "${COLOR_CYAN}Checking core variables...${COLOR_RESET}\n"
  local core_vars="PROJECT_NAME POSTGRES_DB"
  for var in $core_vars; do
    if ! grep -q "^${var}=" "$env_file" 2>/dev/null; then
      printf "  ${COLOR_YELLOW}Warning: %s not set${COLOR_RESET}\n" "$var"
      warnings=$((warnings + 1))
    fi
  done

  if [[ $warnings -eq 0 ]]; then
    printf "  ${COLOR_GREEN}✓ Core variables present${COLOR_RESET}\n"
  fi

  # Summary
  printf "\n"
  if [[ $errors -eq 0 ]]; then
    printf "${COLOR_GREEN}✓ Configuration is valid${COLOR_RESET}\n"
    return 0
  else
    printf "${COLOR_RED}✗ Found %d error(s)${COLOR_RESET}\n" "$errors"
    return 1
  fi
}

# Export functions
export -f env::validate
export -f env::validate_env_file
export -f env::validate_server_config
export -f env::validate_secrets_file
export -f env::validate_required_vars
export -f env::validate_all
export -f env::validate_types
export -f env::validate_project
