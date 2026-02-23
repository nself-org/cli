#!/usr/bin/env bash
set -euo pipefail

# core-settings.sh - Wizard step for core project settings
# POSIX-compliant, no Bash 4+ features

# Configure core project settings
wizard_core_settings() {
  local config_array_name="$1"

  clear
  show_wizard_step 1 10 "Core Project Settings"

  echo "📋 Basic Configuration"
  echo ""

  # Project Name
  echo "  Used for: Docker containers, database names, resource prefixes"
  echo "  Format: lowercase letters, numbers, hyphens (e.g., my-project)"
  echo "  Requirements: Must start and end with letter/number"
  echo ""
  local project_name
  local default_name="myproject"
  # Try to use current directory name as default if valid
  local dir_name=$(basename "$PWD" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9-]/-/g' | sed 's/^-*//;s/-*$//' | sed 's/--*/-/g')
  if echo "$dir_name" | grep -q '^[a-z0-9][a-z0-9-]*[a-z0-9]$'; then
    default_name="$dir_name"
  fi
  prompt_input "Project name" "$default_name" project_name "^[a-z0-9][a-z0-9-]*[a-z0-9]$"

  # Add to config using eval for Bash 3.2 compatibility
  eval "$config_array_name+=('PROJECT_NAME=$project_name')"

  echo ""

  # Environment Mode
  echo "Environment mode:"
  local env_options=(
    "dev - Development (debug tools, hot reload, verbose logging)"
    "prod - Production (optimized, secure, minimal logging)"
  )
  local selected_env
  select_option "Select environment" env_options selected_env
  local env_mode=$([[ $selected_env -eq 0 ]] && echo "dev" || echo "prod")

  eval "$config_array_name+=('ENV=$env_mode')"

  echo ""

  # Base Domain
  echo "Base domain:"
  echo "  All services will be subdomains of this domain"
  echo ""

  local base_domain
  if [[ "$env_mode" == "dev" ]]; then
    echo "Development domain:"
    local domain_options=(
      "local.nself.org (recommended) - Zero configuration, automatic SSL"
      "localhost - Works everywhere, SSL auto-configured with mkcert and sudo"
      "Custom prefix - e.g., myapp.local.nself.org or myapp.localhost"
    )
    local selected_domain
    select_option "Select domain" domain_options selected_domain

    case $selected_domain in
      0)
        base_domain="local.nself.org"
        ;;
      1)
        base_domain="localhost"
        ;;
      2)
        echo ""
        prompt_input "Enter custom prefix (without .local.nself.org)" "myapp" base_domain "^[a-z][a-z0-9-]*$"
        base_domain="${base_domain}.local.nself.org"
        ;;
    esac
  else
    echo "Production domain (e.g., example.com, api.example.com):"
    prompt_input "Domain" "example.com" base_domain "^[a-z0-9][a-z0-9.-]*[a-z0-9]$"
  fi

  eval "$config_array_name+=('BASE_DOMAIN=$base_domain')"

  # SSL Configuration
  if [[ "$env_mode" == "prod" ]]; then
    echo ""
    local ssl_options=(
      "Let's Encrypt - Automatic free SSL certificates"
      "Self-signed - For testing only"
      "Custom - Bring your own certificates"
    )
    local selected_ssl
    select_option "SSL certificate provider" ssl_options selected_ssl

    case $selected_ssl in
      0)
        eval "$config_array_name+=('SSL_PROVIDER=letsencrypt')"
        echo ""
        local cert_email
        prompt_input "Email for Let's Encrypt notifications" "admin@$base_domain" cert_email
        eval "$config_array_name+=('LETSENCRYPT_EMAIL=$cert_email')"
        ;;
      1)
        eval "$config_array_name+=('SSL_PROVIDER=self-signed')"
        ;;
      2)
        eval "$config_array_name+=('SSL_PROVIDER=custom')"
        echo ""
        echo "Place your certificates in:"
        echo "  ssl/certificates/${base_domain}/fullchain.pem"
        echo "  ssl/certificates/${base_domain}/privkey.pem"
        press_any_key
        ;;
    esac
  else
    eval "$config_array_name+=('SSL_PROVIDER=self-signed')"
  fi

  return 0
}

# Export function
export -f wizard_core_settings
