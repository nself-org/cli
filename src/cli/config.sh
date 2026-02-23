#!/usr/bin/env bash
# config.sh - Unified configuration management
# Consolidates: config, env, secrets, vault, validate
# v1.0.0 - Command tree consolidation

# Early help check - before sourcing anything that might fail
if [[ "${1:-}" == "--help" ]] || [[ "${1:-}" == "-h" ]] || [[ "${1:-}" == "help" ]]; then
  cat <<'EOF'
nself config - Unified configuration management

Usage: nself config <subcommand> [options]

CONFIGURATION SUBCOMMANDS:
  show [key]                     Show current configuration
  edit [key]                     Edit configuration in $EDITOR
  get <key>                      Get specific configuration value
  set <key> <value>              Set configuration value
  list                           List all configuration keys
  export <file>                  Export configuration (redacted secrets)
  import <file>                  Import configuration from file
  sync <action>                  Sync configuration across environments

ENVIRONMENT MANAGEMENT:
  env list                       List all environments
  env switch <name>              Switch to an environment
  env create <name> [template]   Create new environment
  env delete <name>              Delete an environment
  env sync <env>                 Sync with environment

SECRETS MANAGEMENT:
  secrets list [--env ENV]       List secrets
  secrets get <key>              Get secret value
  secrets set <key> <value>      Set secret value
  secrets delete <key>           Delete secret
  secrets rotate [key]           Rotate secrets (single or all)

VAULT INTEGRATION:
  vault init                     Initialize encrypted vault
  vault status                   Show vault status
  vault config                   Configure vault settings

VALIDATION:
  validate [env]                 Validate configuration
  validate --security            Security-only validation
  validate --deploy              Deployment readiness check

Options:
  --env NAME                Target environment (default: current)
  --reveal                  Show secret values (use with caution)
  --json                    Output in JSON format
  --strict                  Treat warnings as errors
  --fix                     Attempt automatic fixes
  -h, --help                Show this help message

Examples:
  # Configuration
  nself config show                        # Show current config
  nself config get POSTGRES_HOST           # Get specific value
  nself config set REDIS_ENABLED true      # Enable Redis
  nself config export --json               # Export as JSON

  # Environment management
  nself config env list                    # List environments
  nself config env create staging          # Create staging env
  nself config env switch prod             # Switch to production

  # Secrets
  nself config secrets list --env prod     # List production secrets
  nself config secrets rotate --all        # Rotate all secrets

  # Vault
  nself config vault init                  # Initialize vault
  nself config vault status                # Check vault status

  # Validation
  nself config validate prod               # Validate production config
  nself config validate --security         # Security-only check
  nself config validate --fix              # Auto-fix issues

For detailed help on a subcommand:
  nself config <subcommand> --help
EOF
  exit 0
fi

set -euo pipefail

# Get script directory
CLI_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT_DIR="$CLI_SCRIPT_DIR"
LIB_DIR="$CLI_SCRIPT_DIR/../lib"

# Source utilities (turn off errexit temporarily for optional modules)
set +e
source "$LIB_DIR/utils/cli-output.sh" 2>/dev/null
source "$LIB_DIR/utils/env.sh" 2>/dev/null
source "$LIB_DIR/utils/display.sh" 2>/dev/null
source "$LIB_DIR/utils/platform-compat.sh" 2>/dev/null
set -e

# Don't source header.sh or hooks - not needed for config command

# Source configuration modules
source "$LIB_DIR/env/create.sh" 2>/dev/null || true
source "$LIB_DIR/env/switch.sh" 2>/dev/null || true
source "$LIB_DIR/env/diff.sh" 2>/dev/null || true
source "$LIB_DIR/env/validate.sh" 2>/dev/null || true
source "$LIB_DIR/deploy/security-preflight.sh" 2>/dev/null || true

# Source vault modules if available (with guards to prevent re-sourcing)
# Temporarily disable errexit for optional module loading
set +e
if [[ -f "$LIB_DIR/secrets/vault.sh" ]] && [[ -z "${VAULT_LIB_LOADED:-}" ]]; then
  source "$LIB_DIR/secrets/vault.sh" 2>/dev/null
fi
if [[ -f "$LIB_DIR/secrets/encryption.sh" ]] && [[ -z "${ENCRYPTION_LIB_LOADED:-}" ]]; then
  source "$LIB_DIR/secrets/encryption.sh" 2>/dev/null
fi
if [[ -f "$LIB_DIR/secrets/audit.sh" ]] && [[ -z "${AUDIT_LIB_LOADED:-}" ]]; then
  source "$LIB_DIR/secrets/audit.sh" 2>/dev/null
fi
if [[ -f "$LIB_DIR/secrets/environment.sh" ]] && [[ -z "${ENVIRONMENT_LIB_LOADED:-}" ]]; then
  source "$LIB_DIR/secrets/environment.sh" 2>/dev/null
fi
set -e

# Fallback logging if cli-output.sh not available
if ! command -v cli_success >/dev/null 2>&1; then
  cli_success() { printf "\033[0;32m✓\033[0m %s\n" "$1"; }
  cli_error() { printf "\033[0;31m✗\033[0m %s\n" "$1" >&2; }
  cli_warning() { printf "\033[0;33m⚠\033[0m %s\n" "$1"; }
  cli_info() { printf "\033[0;34mℹ\033[0m %s\n" "$1"; }
  cli_section() { printf "\n\033[0;36m→\033[0m \033[1m%s\033[0m\n" "$1"; }
fi

# ============================================================
# Help
# ============================================================

show_config_help() {
  cat <<'EOF'
nself config - Unified configuration management

Usage: nself config <subcommand> [options]

CONFIGURATION SUBCOMMANDS:
  show [key]                     Show current configuration
  edit [key]                     Edit configuration in $EDITOR
  get <key>                      Get specific configuration value
  set <key> <value>              Set configuration value
  list                           List all configuration keys
  export <file>                  Export configuration (redacted secrets)
  import <file>                  Import configuration from file
  sync <action>                  Sync configuration across environments

ENVIRONMENT MANAGEMENT:
  env list                       List all environments
  env switch <name>              Switch to an environment
  env create <name> [template]   Create new environment
  env delete <name>              Delete an environment
  env sync <env>                 Sync with environment

SECRETS MANAGEMENT:
  secrets list [--env ENV]       List secrets
  secrets get <key>              Get secret value
  secrets set <key> <value>      Set secret value
  secrets delete <key>           Delete secret
  secrets rotate [key]           Rotate secrets (single or all)

VAULT INTEGRATION:
  vault init                     Initialize encrypted vault
  vault status                   Show vault status
  vault config                   Configure vault settings

VALIDATION:
  validate [env]                 Validate configuration
  validate --security            Security-only validation
  validate --deploy              Deployment readiness check

Options:
  --env NAME                Target environment (default: current)
  --reveal                  Show secret values (use with caution)
  --json                    Output in JSON format
  --strict                  Treat warnings as errors
  --fix                     Attempt automatic fixes
  -h, --help                Show this help message

Examples:
  # Configuration
  nself config show                        # Show current config
  nself config get POSTGRES_HOST           # Get specific value
  nself config set REDIS_ENABLED true      # Enable Redis
  nself config export --json               # Export as JSON

  # Environment management
  nself config env list                    # List environments
  nself config env create staging          # Create staging env
  nself config env switch prod             # Switch to production

  # Secrets
  nself config secrets list --env prod     # List production secrets
  nself config secrets rotate --all        # Rotate all secrets

  # Vault
  nself config vault init                  # Initialize vault
  nself config vault status                # Check vault status

  # Validation
  nself config validate prod               # Validate production config
  nself config validate --security         # Security-only check
  nself config validate --fix              # Auto-fix issues

For detailed help on a subcommand:
  nself config <subcommand> --help
EOF
}

# ============================================================
# Configuration Subcommands
# ============================================================

# Show configuration
cmd_config_show() {
  local reveal="${REVEAL:-false}"
  local json_mode="${JSON_OUTPUT:-false}"

  load_env_with_priority

  local env="${ENV:-local}"
  local env_file=".env"

  # Determine environment file
  case "$env" in
    staging) env_file=".env.staging" ;;
    prod | production) env_file=".env.prod" ;;
  esac

  [[ ! -f "$env_file" ]] && env_file=".env"

  if [[ ! -f "$env_file" ]]; then
    cli_error "Configuration file not found: $env_file"
    return 1
  fi

  if [[ "$json_mode" != "true" ]]; then
    cli_section "Configuration"
    printf "Environment: %s\n" "$env"
    printf "File: %s\n\n" "$env_file"
  fi

  # Display configuration grouped by category
  local core_vars=""
  local service_vars=""
  local custom_vars=""

  while IFS= read -r line; do
    [[ "$line" =~ ^[[:space:]]*# ]] && continue
    [[ -z "${line// /}" ]] && continue

    local key="${line%%=*}"
    local value="${line#*=}"
    value="${value#\"}"
    value="${value%\"}"
    value="${value#\'}"
    value="${value%\'}"

    # Redact secrets unless --reveal
    if echo "$key" | grep -qiE "PASSWORD|SECRET|TOKEN|KEY|CREDENTIAL"; then
      if [[ "$reveal" != "true" ]]; then
        value="********"
      fi
    fi

    # Categorize
    if [[ "$key" =~ ^(PROJECT_|ENV|BASE_DOMAIN|POSTGRES_|HASURA_|AUTH_) ]]; then
      core_vars+="  $key=$value\n"
    elif [[ "$key" =~ ^(REDIS_|MINIO_|MAILPIT_|MEILISEARCH_|MLFLOW_|FUNCTIONS_|MONITORING_) ]]; then
      service_vars+="  $key=$value\n"
    elif [[ "$key" =~ ^(CS_|FRONTEND_APP_) ]]; then
      custom_vars+="  $key=$value\n"
    fi
  done <"$env_file"

  if [[ "$json_mode" == "true" ]]; then
    printf '{"env": "%s", "file": "%s", "config": {' "$env" "$env_file"
    local first=true
    while IFS= read -r line; do
      [[ "$line" =~ ^[[:space:]]*# ]] && continue
      [[ -z "${line// /}" ]] && continue
      local key="${line%%=*}"
      local value="${line#*=}"
      [[ "$first" != "true" ]] && printf ", "
      first=false
      printf '"%s": "%s"' "$key" "$value"
    done <"$env_file"
    printf '}}\n'
  else
    [[ -n "$core_vars" ]] && printf "Core:\n$core_vars\n"
    [[ -n "$service_vars" ]] && printf "Services:\n$service_vars\n"
    [[ -n "$custom_vars" ]] && printf "Custom:\n$custom_vars\n"
    [[ "$reveal" != "true" ]] && cli_info "Use --reveal to show secret values"
  fi
}

# Get specific value
cmd_config_get() {
  local key="$1"
  local reveal="${REVEAL:-false}"

  if [[ -z "$key" ]]; then
    cli_error "Configuration key required"
    printf "Usage: nself config get <key>\n"
    return 1
  fi

  load_env_with_priority

  local value="${!key:-}"
  if [[ -z "$value" ]]; then
    cli_error "Key not found: $key"
    return 1
  fi

  # Redact secrets unless --reveal
  if echo "$key" | grep -qiE "PASSWORD|SECRET|TOKEN|KEY|CREDENTIAL"; then
    if [[ "$reveal" != "true" ]]; then
      value="********"
    fi
  fi

  printf "%s\n" "$value"
}

# Set configuration value
cmd_config_set() {
  local key="$1"
  local value="$2"
  local no_backup="${NO_BACKUP:-false}"

  if [[ -z "$key" ]] || [[ -z "$value" ]]; then
    cli_error "Both key and value required"
    printf "Usage: nself config set <key> <value>\n"
    return 1
  fi

  load_env_with_priority

  local env_file=".env"
  if [[ ! -f "$env_file" ]]; then
    cli_error "Configuration file not found: $env_file"
    return 1
  fi

  # Backup
  if [[ "$no_backup" != "true" ]]; then
    cp "$env_file" "${env_file}.bak"
  fi

  # Update or add
  if grep -q "^${key}=" "$env_file"; then
    if [[ "$OSTYPE" == "darwin"* ]]; then
      sed -i '' "s|^${key}=.*|${key}=${value}|" "$env_file"
    else
      sed -i "s|^${key}=.*|${key}=${value}|" "$env_file"
    fi
    cli_success "Updated: $key"
  else
    printf "%s=%s\n" "$key" "$value" >>"$env_file"
    cli_success "Added: $key"
  fi

  cli_info "Run 'nself build && nself restart' to apply changes"
}

# List all keys
cmd_config_list() {
  load_env_with_priority

  local env_file=".env"
  if [[ ! -f "$env_file" ]]; then
    cli_error "Configuration file not found: $env_file"
    return 1
  fi

  while IFS= read -r line; do
    [[ "$line" =~ ^[[:space:]]*# ]] && continue
    [[ -z "${line// /}" ]] && continue
    printf "%s\n" "${line%%=*}"
  done <"$env_file"
}

# Edit configuration
cmd_config_edit() {
  load_env_with_priority

  local env_file=".env"
  if [[ ! -f "$env_file" ]]; then
    cli_error "Configuration file not found: $env_file"
    return 1
  fi

  local editor="${EDITOR:-${VISUAL:-nano}}"
  cli_info "Opening $env_file in $editor"
  "$editor" "$env_file"
}

# Export configuration
cmd_config_export() {
  local output_file="${1:-}"
  local reveal="${REVEAL:-false}"

  load_env_with_priority

  local env_file=".env"
  if [[ ! -f "$env_file" ]]; then
    cli_error "Configuration file not found: $env_file"
    return 1
  fi

  local timestamp=$(date +%Y%m%d_%H%M%S)
  [[ -z "$output_file" ]] && output_file="config_export_${timestamp}.json"

  printf '{\n  "exported": "%s",\n  "env": "%s",\n  "config": {\n' \
    "$(date -Iseconds 2>/dev/null || date)" "${ENV:-local}" >"$output_file"

  local first=true
  while IFS= read -r line; do
    [[ "$line" =~ ^[[:space:]]*# ]] && continue
    [[ -z "${line// /}" ]] && continue

    local key="${line%%=*}"
    local value="${line#*=}"

    # Redact secrets
    if echo "$key" | grep -qiE "PASSWORD|SECRET|TOKEN|KEY|CREDENTIAL" && [[ "$reveal" != "true" ]]; then
      value="********"
    fi

    [[ "$first" != "true" ]] && printf ",\n" >>"$output_file"
    first=false
    printf '    "%s": "%s"' "$key" "$value" >>"$output_file"
  done <"$env_file"

  printf '\n  }\n}\n' >>"$output_file"

  cli_success "Exported to: $output_file"
  [[ "$reveal" != "true" ]] && cli_info "Secrets redacted. Use --reveal to include."
}

# Import configuration
cmd_config_import() {
  local import_file="$1"

  if [[ -z "$import_file" ]] || [[ ! -f "$import_file" ]]; then
    cli_error "Import file required and must exist"
    printf "Usage: nself config import <file>\n"
    return 1
  fi

  cli_warning "This will overwrite current configuration"
  printf "Continue? (y/N) "
  read -r confirm
  confirm=$(printf "%s" "$confirm" | tr '[:upper:]' '[:lower:]')
  if [[ "$confirm" != "y" ]]; then
    cli_info "Import cancelled"
    return 0
  fi

  # Backup current
  [[ -f ".env" ]] && cp ".env" ".env.bak"

  # Import (simplified JSON parsing)
  if [[ "$import_file" == *.json ]]; then
    grep -o '"[^"]*": *"[^"]*"' "$import_file" | while read -r pair; do
      local key=$(printf "%s" "$pair" | cut -d'"' -f2)
      local value=$(printf "%s" "$pair" | cut -d'"' -f4)
      [[ "$key" == "exported" || "$key" == "env" || "$key" == "config" ]] && continue
      [[ "$value" == "********" ]] && continue
      printf "%s=%s\n" "$key" "$value"
    done >".env.new"

    if [[ -s ".env.new" ]]; then
      mv ".env.new" ".env"
      cli_success "Configuration imported"
    else
      rm -f ".env.new"
      cli_error "No valid configuration found"
      return 1
    fi
  else
    cp "$import_file" ".env"
    cli_success "Configuration imported"
  fi

  cli_info "Run 'nself build && nself restart' to apply"
}

# Sync configuration
cmd_config_sync() {
  local action="${1:-}"

  case "$action" in
    push | pull)
      cli_warning "Config sync requires 'nself deploy sync' command"
      printf "Use: nself deploy sync %s <env>\n" "$action"
      return 1
      ;;
    *)
      cli_error "Unknown sync action: $action"
      printf "Usage: nself config sync <push|pull>\n"
      return 1
      ;;
  esac
}

# ============================================================
# Environment Management (env subcommand)
# ============================================================

cmd_config_env() {
  local subcommand="${1:-list}"
  shift || true

  case "$subcommand" in
    list | ls)
      cli_section "Available Environments"
      if command -v env::list >/dev/null 2>&1; then
        env::list
      else
        # Fallback
        if [[ -d ".environments" ]]; then
          find .environments -maxdepth 1 -type d ! -name ".environments" -exec basename {} \;
        else
          printf "local (default)\n"
        fi
      fi
      ;;

    create)
      local name="${1:-}"
      local template="${2:-local}"

      if [[ -z "$name" ]]; then
        cli_error "Environment name required"
        printf "Usage: nself config env create <name> [template]\n"
        return 1
      fi

      cli_section "Creating Environment: $name"
      if command -v env::create >/dev/null 2>&1; then
        env::create "$name" "$template" "false"
      else
        mkdir -p ".environments/$name"
        touch ".environments/$name/.env"
        cli_success "Environment created: $name"
      fi
      ;;

    switch | use)
      local name="${1:-}"

      if [[ -z "$name" ]]; then
        cli_error "Environment name required"
        printf "Usage: nself config env switch <name>\n"
        return 1
      fi

      if command -v env::switch >/dev/null 2>&1; then
        env::switch "$name"
      else
        export ENV="$name"
        cli_success "Switched to: $name"
      fi
      ;;

    delete | rm)
      local name="${1:-}"

      if [[ -z "$name" ]]; then
        cli_error "Environment name required"
        printf "Usage: nself config env delete <name>\n"
        return 1
      fi

      cli_warning "Delete environment '$name'?"
      printf "Type 'DELETE' to confirm: "
      read -r confirm
      if [[ "$confirm" != "DELETE" ]]; then
        cli_info "Cancelled"
        return 0
      fi

      if command -v env::delete >/dev/null 2>&1; then
        env::delete "$name" "true"
      else
        rm -rf ".environments/$name"
        cli_success "Deleted: $name"
      fi
      ;;

    sync)
      local env_name="${1:-}"
      cli_warning "Environment sync requires 'nself deploy sync' command"
      printf "Use: nself deploy sync pull %s\n" "$env_name"
      ;;

    --help | -h)
      printf "Usage: nself config env <subcommand>\n\n"
      printf "Subcommands:\n"
      printf "  list              List all environments\n"
      printf "  create <name>     Create new environment\n"
      printf "  switch <name>     Switch to environment\n"
      printf "  delete <name>     Delete environment\n"
      printf "  sync <env>        Sync with remote\n"
      ;;

    *)
      cli_error "Unknown env subcommand: $subcommand"
      printf "Run 'nself config env --help' for usage\n"
      return 1
      ;;
  esac
}

# ============================================================
# Secrets Management (secrets subcommand)
# ============================================================

cmd_config_secrets() {
  local subcommand="${1:-list}"
  shift || true

  case "$subcommand" in
    list)
      local env_name="${ENV:-local}"

      while [[ $# -gt 0 ]]; do
        case "$1" in
          --env | -e)
            env_name="$2"
            shift 2
            ;;
          *) shift ;;
        esac
      done

      cli_section "Secrets for: $env_name"

      local secrets_file=".env.secrets"
      if [[ -n "$env_name" ]] && [[ "$env_name" != "local" ]]; then
        secrets_file=".environments/$env_name/.env.secrets"
      fi

      if [[ ! -f "$secrets_file" ]]; then
        cli_warning "No secrets file found: $secrets_file"
        printf "Generate with: nself config secrets generate --env %s\n" "$env_name"
        return 0
      fi

      while IFS= read -r line; do
        [[ "$line" =~ ^[[:space:]]*# ]] && continue
        [[ -z "$line" ]] && continue
        local key="${line%%=*}"
        local value="${line#*=}"
        local length=${#value}
        printf "  %s (%d chars)\n" "$key" "$length"
      done <"$secrets_file"
      ;;

    get)
      local key="${1:-}"
      local reveal="${REVEAL:-false}"

      if [[ -z "$key" ]]; then
        cli_error "Secret key required"
        printf "Usage: nself config secrets get <key>\n"
        return 1
      fi

      local secrets_file=".env.secrets"
      [[ -f "$secrets_file" ]] || secrets_file=".env"

      if ! grep -q "^${key}=" "$secrets_file" 2>/dev/null; then
        cli_error "Secret not found: $key"
        return 1
      fi

      local value=$(grep "^${key}=" "$secrets_file" | cut -d'=' -f2-)

      if [[ "$reveal" == "true" ]]; then
        printf "%s\n" "$value"
      else
        printf "********\n"
        cli_info "Use --reveal to show actual value"
      fi
      ;;

    set)
      local key="${1:-}"
      local value="${2:-}"

      if [[ -z "$key" ]] || [[ -z "$value" ]]; then
        cli_error "Both key and value required"
        printf "Usage: nself config secrets set <key> <value>\n"
        return 1
      fi

      local secrets_file=".env.secrets"
      touch "$secrets_file"
      chmod 600 "$secrets_file"

      if grep -q "^${key}=" "$secrets_file" 2>/dev/null; then
        if [[ "$OSTYPE" == "darwin"* ]]; then
          sed -i '' "s|^${key}=.*|${key}=${value}|" "$secrets_file"
        else
          sed -i "s|^${key}=.*|${key}=${value}|" "$secrets_file"
        fi
        cli_success "Updated secret: $key"
      else
        printf "%s=%s\n" "$key" "$value" >>"$secrets_file"
        cli_success "Added secret: $key"
      fi
      ;;

    delete | rm)
      local key="${1:-}"

      if [[ -z "$key" ]]; then
        cli_error "Secret key required"
        printf "Usage: nself config secrets delete <key>\n"
        return 1
      fi

      local secrets_file=".env.secrets"
      if [[ ! -f "$secrets_file" ]]; then
        cli_error "No secrets file found"
        return 1
      fi

      if grep -q "^${key}=" "$secrets_file"; then
        if [[ "$OSTYPE" == "darwin"* ]]; then
          sed -i '' "/^${key}=/d" "$secrets_file"
        else
          sed -i "/^${key}=/d" "$secrets_file"
        fi
        cli_success "Deleted secret: $key"
      else
        cli_warning "Secret not found: $key"
      fi
      ;;

    rotate)
      local key="${1:-}"
      local rotate_all=false

      while [[ $# -gt 0 ]]; do
        case "$1" in
          --all)
            rotate_all=true
            shift
            ;;
          *)
            key="$1"
            shift
            ;;
        esac
      done

      # Source secrets library
      if [[ -f "$LIB_DIR/security/secrets.sh" ]]; then
        source "$LIB_DIR/security/secrets.sh"
      fi

      if [[ "$rotate_all" == "true" ]]; then
        cli_section "Rotating All Secrets"
        cli_warning "This will generate new values for all secrets"
        printf "Continue? (y/N) "
        read -r confirm
        confirm=$(printf "%s" "$confirm" | tr '[:upper:]' '[:lower:]')
        if [[ "$confirm" != "y" ]]; then
          cli_info "Cancelled"
          return 0
        fi

        # Use secrets library if available
        if command -v secrets::rotate_all >/dev/null 2>&1; then
          secrets::rotate_all ".env.secrets"
        else
          cli_error "Secret rotation not available"
          return 1
        fi
      else
        if [[ -z "$key" ]]; then
          cli_error "Secret key required (or use --all)"
          printf "Usage: nself config secrets rotate <key>\n"
          printf "       nself config secrets rotate --all\n"
          return 1
        fi

        # Use secrets library if available
        if command -v secrets::rotate >/dev/null 2>&1; then
          secrets::rotate "$key" ".env.secrets"
        else
          # Fallback to manual rotation
          local new_value=$(openssl rand -base64 32 | tr -d '/+=' | head -c 32)

          local secrets_file=".env.secrets"
          if grep -q "^${key}=" "$secrets_file" 2>/dev/null; then
            if [[ "$OSTYPE" == "darwin"* ]]; then
              sed -i '' "s|^${key}=.*|${key}=${new_value}|" "$secrets_file"
            else
              sed -i "s|^${key}=.*|${key}=${new_value}|" "$secrets_file"
            fi
            cli_success "Rotated secret: $key"
          else
            printf "%s=%s\n" "$key" "$new_value" >>"$secrets_file"
            cli_success "Created secret: $key"
          fi
        fi
      fi
      ;;

    generate)
      cli_section "Generating Secrets"

      # Source secrets library
      if [[ -f "$LIB_DIR/security/secrets.sh" ]]; then
        source "$LIB_DIR/security/secrets.sh"
      fi

      local key="${1:-}"
      local output_file="${2:-.env.secrets}"

      if [[ -z "$key" ]]; then
        # Generate all secrets
        if command -v secrets::generate_all >/dev/null 2>&1; then
          secrets::generate_all "$output_file"
        else
          cli_error "Secret generation not available"
          return 1
        fi
      else
        # Generate specific secret
        local length="${3:-32}"
        local type="${4:-hex}"

        if command -v secrets::generate_random >/dev/null 2>&1; then
          local value
          value=$(secrets::generate_random "$length" "$type")
          printf "%s=%s\n" "$key" "$value"
        else
          cli_error "Secret generation not available"
          return 1
        fi
      fi
      ;;

    validate)
      cli_section "Validating Secrets"

      # Source secrets library
      if [[ -f "$LIB_DIR/security/secrets.sh" ]]; then
        source "$LIB_DIR/security/secrets.sh"
      fi

      local secrets_file="${1:-.env.secrets}"

      if command -v secrets::validate >/dev/null 2>&1; then
        secrets::validate "$secrets_file"
      else
        cli_error "Secret validation not available"
        return 1
      fi
      ;;

    import)
      local provider="${1:-env}"
      shift || true

      cli_section "Importing Secrets from $provider"

      # Source secrets library
      if [[ -f "$LIB_DIR/security/secrets.sh" ]]; then
        source "$LIB_DIR/security/secrets.sh"
      fi

      case "$provider" in
        env | environment)
          if command -v secrets::import_from_env >/dev/null 2>&1; then
            secrets::import_from_env ".env.secrets" "${1:-NSELF_SECRET_}"
          else
            cli_error "Import from environment not available"
            return 1
          fi
          ;;
        vault)
          if command -v secrets::import_from_vault >/dev/null 2>&1; then
            secrets::import_from_vault "${1:-secret/nself}" ".env.secrets"
          else
            cli_error "Vault import not available"
            cli_info "Install vault CLI: https://www.vaultproject.io/downloads"
            return 1
          fi
          ;;
        aws)
          if command -v secrets::import_from_aws >/dev/null 2>&1; then
            secrets::import_from_aws "${1:-nself/secrets}" ".env.secrets"
          else
            cli_error "AWS import not available"
            cli_info "Install AWS CLI: https://aws.amazon.com/cli/"
            return 1
          fi
          ;;
        *)
          cli_error "Unknown provider: $provider"
          printf "Supported providers: env, vault, aws\n"
          return 1
          ;;
      esac
      ;;

    export)
      local provider="${1:-}"
      shift || true

      if [[ -z "$provider" ]]; then
        cli_error "Provider required"
        printf "Usage: nself config secrets export <provider> [args]\n"
        printf "Providers: vault, aws, azure, gcp\n"
        return 1
      fi

      cli_section "Exporting Secrets to $provider"

      # Source secrets library
      if [[ -f "$LIB_DIR/security/secrets.sh" ]]; then
        source "$LIB_DIR/security/secrets.sh"
      fi

      case "$provider" in
        vault)
          if command -v secrets::export_to_vault >/dev/null 2>&1; then
            secrets::export_to_vault ".env.secrets" "${1:-secret/nself}"
          else
            cli_error "Vault export not available"
            return 1
          fi
          ;;
        aws)
          if command -v secrets::export_to_aws >/dev/null 2>&1; then
            secrets::export_to_aws ".env.secrets" "${1:-nself/secrets}"
          else
            cli_error "AWS export not available"
            return 1
          fi
          ;;
        *)
          cli_error "Unknown provider: $provider"
          printf "Supported providers: vault, aws\n"
          return 1
          ;;
      esac
      ;;

    encrypt)
      cli_section "Encrypting Secrets"

      # Source secrets library
      if [[ -f "$LIB_DIR/security/secrets.sh" ]]; then
        source "$LIB_DIR/security/secrets.sh"
      fi

      local secrets_file="${1:-.env.secrets}"
      local output_file="${2:-${secrets_file}.enc}"

      printf "Enter encryption password: "
      read -s password
      printf "\n"

      if command -v secrets::encrypt >/dev/null 2>&1; then
        secrets::encrypt "$secrets_file" "$output_file" "$password"
      else
        cli_error "Encryption not available"
        cli_info "Install openssl or gpg"
        return 1
      fi
      ;;

    decrypt)
      cli_section "Decrypting Secrets"

      # Source secrets library
      if [[ -f "$LIB_DIR/security/secrets.sh" ]]; then
        source "$LIB_DIR/security/secrets.sh"
      fi

      local encrypted_file="${1:-.env.secrets.enc}"
      local output_file="${2:-.env.secrets}"

      printf "Enter decryption password: "
      read -s password
      printf "\n"

      if command -v secrets::decrypt >/dev/null 2>&1; then
        secrets::decrypt "$encrypted_file" "$output_file" "$password"
      else
        cli_error "Decryption not available"
        return 1
      fi
      ;;

    --help | -h)
      printf "Usage: nself config secrets <subcommand> [options]\n\n"
      printf "Subcommands:\n"
      printf "  list [--env ENV]           List all secrets\n"
      printf "  get <key> [--reveal]       Get secret value\n"
      printf "  set <key> <value>          Set secret value\n"
      printf "  delete <key>               Delete secret\n"
      printf "  rotate [key] [--all]       Rotate secret(s)\n"
      printf "  generate [key]             Generate new secret(s)\n"
      printf "  validate                   Validate secrets strength\n"
      printf "  import <provider> [args]   Import from provider (env, vault, aws)\n"
      printf "  export <provider> [args]   Export to provider (vault, aws)\n"
      printf "  encrypt [file]             Encrypt secrets file\n"
      printf "  decrypt [file]             Decrypt secrets file\n"
      printf "\n"
      printf "Options:\n"
      printf "  --env NAME        Target environment\n"
      printf "  --reveal          Show secret values (use with caution)\n"
      printf "  --all             Apply to all secrets\n"
      printf "\n"
      printf "Examples:\n"
      printf "  nself config secrets list                    # List all secrets\n"
      printf "  nself config secrets generate                # Generate all secrets\n"
      printf "  nself config secrets rotate --all            # Rotate all secrets\n"
      printf "  nself config secrets validate                # Check secret strength\n"
      printf "  nself config secrets import vault            # Import from Vault\n"
      printf "  nself config secrets export aws nself/prod   # Export to AWS\n"
      ;;

    *)
      cli_error "Unknown secrets subcommand: $subcommand"
      printf "Run 'nself config secrets --help' for usage\n"
      return 1
      ;;
  esac
}

# ============================================================
# Vault Integration (vault subcommand)
# ============================================================

cmd_config_vault() {
  local subcommand="${1:-status}"
  shift || true

  case "$subcommand" in
    init)
      cli_section "Initializing Vault"

      if command -v vault_init >/dev/null 2>&1; then
        if ! vault_init; then
          cli_error "Failed to initialize vault"
          return 1
        fi
        cli_success "Vault initialized successfully"
      else
        cli_warning "Vault module not available"
        cli_info "Install vault support or use basic secrets management"
        return 1
      fi
      ;;

    status)
      cli_section "Vault Status"

      if command -v vault_status >/dev/null 2>&1; then
        vault_status
      else
        cli_info "Vault not initialized"
        printf "Run: nself config vault init\n"
      fi
      ;;

    config)
      cli_section "Vault Configuration"

      if command -v vault_configure >/dev/null 2>&1; then
        vault_configure
      else
        cli_warning "Vault configuration not available"
        return 1
      fi
      ;;

    lint)
      # Lint a vault/env file for unquoted special characters that break `source`
      local lint_file="${1:-${HOME}/.claude/vault.env}"
      local lint_errors=0

      cli_section "Vault Lint: ${lint_file}"

      if [[ ! -f "$lint_file" ]]; then
        cli_error "File not found: $lint_file"
        return 1
      fi

      printf "  Scanning for unquoted special characters...\n\n"

      local lineno=0
      while IFS= read -r line; do
        lineno=$((lineno + 1))

        # Skip blank lines and comments
        case "$line" in
          "" | "#"*) continue ;;
        esac

        # Only check KEY=VALUE lines
        case "$line" in
          *=*)
            local key val
            key="${line%%=*}"
            val="${line#*=}"

            # Skip already-quoted values (start with ' or ")
            case "$val" in
              '"'*'"' | "'"*"'") continue ;;
            esac

            # Check for dangerous unquoted characters in the value
            local found_chars=""
            case "$val" in
              *"&"*) found_chars="${found_chars}&" ;;
            esac
            case "$val" in
              *"|"*) found_chars="${found_chars}|" ;;
            esac
            case "$val" in
              *"<"*) found_chars="${found_chars}<" ;;
            esac
            case "$val" in
              *">"*) found_chars="${found_chars}>" ;;
            esac
            case "$val" in
              *";"*) found_chars="${found_chars};" ;;
            esac
            case "$val" in
              *'`'*) found_chars="${found_chars}\`" ;;
            esac

            if [[ -n "$found_chars" ]]; then
              printf "  ${CLI_RED}Line %d${CLI_RESET}: %s contains unquoted char(s): %s\n" \
                "$lineno" "$key" "$found_chars"
              printf "    Value: %s\n" "$val"
              printf "    Fix:   %s=\"%s\"\n" "$key" "$val"
              printf "\n"
              lint_errors=$((lint_errors + 1))
            fi
            ;;
        esac
      done < "$lint_file"

      if [[ $lint_errors -eq 0 ]]; then
        cli_success "No issues found in ${lint_file}"
        return 0
      else
        cli_error "${lint_errors} issue(s) found — quote the values shown above"
        return 1
      fi
      ;;

    --help | -h)
      printf "Usage: nself config vault <subcommand>\n\n"
      printf "Subcommands:\n"
      printf "  init              Initialize vault\n"
      printf "  status            Show vault status\n"
      printf "  config            Configure vault settings\n"
      printf "  lint [file]       Lint vault/env file for unquoted special chars\n"
      printf "                    Default file: ~/.claude/vault.env\n"
      ;;

    *)
      cli_error "Unknown vault subcommand: $subcommand"
      printf "Run 'nself config vault --help' for usage\n"
      return 1
      ;;
  esac
}

# ============================================================
# Validation (validate subcommand)
# ============================================================

cmd_config_validate() {
  local env_name="${ENV:-local}"
  local scope="all"
  local strict=false
  local fix_mode=false

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --strict)
        strict=true
        shift
        ;;
      --fix)
        fix_mode=true
        shift
        ;;
      --security)
        scope="security"
        shift
        ;;
      --config)
        scope="config"
        shift
        ;;
      --deploy)
        scope="deploy"
        shift
        ;;
      --help | -h)
        printf "Usage: nself config validate [env] [options]\n\n"
        printf "Options:\n"
        printf "  --strict          Treat warnings as errors\n"
        printf "  --fix             Attempt automatic fixes\n"
        printf "  --security        Security-only validation\n"
        printf "  --config          Configuration-only validation\n"
        printf "  --deploy          Deployment readiness only\n"
        return 0
        ;;
      *)
        env_name="$1"
        shift
        ;;
    esac
  done

  cli_section "Validating Configuration: $env_name"

  local errors=0
  local warnings=0

  # Load environment file so required-var checks read from .env, not just shell env
  if [[ -f ".env" ]]; then
    set -a
    # shellcheck source=/dev/null
    source ".env" 2>/dev/null || true
    set +a
  elif [[ -f ".env.dev" ]]; then
    set -a
    # shellcheck source=/dev/null
    source ".env.dev" 2>/dev/null || true
    set +a
  fi

  # Basic configuration validation
  if [[ "$scope" == "all" ]] || [[ "$scope" == "config" ]]; then
    cli_info "Checking configuration files..."

    if [[ -f ".env" ]] || [[ -f ".env.dev" ]]; then
      cli_success "Environment file found"
    else
      cli_error "No .env or .env.dev file found"
      errors=$((errors + 1))
    fi

    # Check required variables
    local required_vars=("PROJECT_NAME" "BASE_DOMAIN" "POSTGRES_PASSWORD" "HASURA_GRAPHQL_ADMIN_SECRET")
    for var in "${required_vars[@]}"; do
      if [[ -n "${!var:-}" ]]; then
        cli_success "$var: Set"
      else
        cli_error "$var: Not set"
        errors=$((errors + 1))
      fi
    done
  fi

  # Security validation
  if [[ "$scope" == "all" ]] || [[ "$scope" == "security" ]]; then
    cli_info "Checking security..."

    if command -v security::preflight >/dev/null 2>&1; then
      if ! security::preflight "$env_name" "." "false" 2>/dev/null; then
        errors=$((errors + 1))
      fi
    else
      # Basic security checks
      if [[ "${POSTGRES_PASSWORD:-}" == "postgres" ]]; then
        cli_warning "POSTGRES_PASSWORD: Using insecure default"
        warnings=$((warnings + 1))
      fi
    fi
  fi

  # Deployment readiness
  if [[ "$scope" == "all" ]] || [[ "$scope" == "deploy" ]]; then
    cli_info "Checking deployment readiness..."

    if docker info >/dev/null 2>&1; then
      cli_success "Docker is running"
    else
      cli_error "Docker is not running"
      errors=$((errors + 1))
    fi
  fi

  # Summary
  printf "\n"
  if [[ $errors -eq 0 ]]; then
    cli_success "Validation passed"
    [[ $warnings -gt 0 ]] && cli_warning "$warnings warning(s)"
    return 0
  else
    cli_error "$errors error(s), $warnings warning(s)"

    if [[ "$fix_mode" == "true" ]]; then
      cli_info "Attempting auto-fix..."
      # Auto-fix logic would go here
    fi

    return 1
  fi
}

# ============================================================
# Main Command Handler
# ============================================================

cmd_config() {
  local subcommand="${1:-show}"
  shift || true

  # Parse global options
  local args=()
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --env)
        export ENV="$2"
        shift 2
        ;;
      --reveal)
        export REVEAL=true
        shift
        ;;
      --json)
        export JSON_OUTPUT=true
        shift
        ;;
      --strict)
        export STRICT=true
        shift
        ;;
      --fix)
        export FIX=true
        shift
        ;;
      --no-backup)
        export NO_BACKUP=true
        shift
        ;;
      -h | --help)
        show_config_help
        return 0
        ;;
      *)
        args+=("$1")
        shift
        ;;
    esac
  done

  # Restore positional arguments
  if [[ ${#args[@]} -gt 0 ]]; then
    set -- "${args[@]}"
  else
    set --
  fi

  # Route to subcommand
  case "$subcommand" in
    show) cmd_config_show "$@" ;;
    get) cmd_config_get "$@" ;;
    set) cmd_config_set "$@" ;;
    list | ls) cmd_config_list "$@" ;;
    edit) cmd_config_edit "$@" ;;
    export) cmd_config_export "$@" ;;
    import) cmd_config_import "$@" ;;
    sync) cmd_config_sync "$@" ;;

    # Environment management
    env) cmd_config_env "$@" ;;

    # Secrets management
    secrets) cmd_config_secrets "$@" ;;

    # Vault integration
    vault) cmd_config_vault "$@" ;;

    # Validation
    validate | check) cmd_config_validate "$@" ;;

    # Help
    help | --help | -h) show_config_help ;;

    *)
      cli_error "Unknown subcommand: $subcommand"
      printf "\nRun 'nself config --help' for usage information\n"
      return 1
      ;;
  esac
}

# Export for use as library
export -f cmd_config

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  cmd_config "$@"
  exit $?
fi
