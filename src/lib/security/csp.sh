#!/usr/bin/env bash

# csp.sh - Content Security Policy (CSP) generator
# POSIX-compliant, no Bash 4+ features

# Get the directory where this script is located
SECURITY_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

LIB_ROOT="$(dirname "$SECURITY_LIB_DIR")"

# Source dependencies
source "$LIB_ROOT/utils/display.sh" 2>/dev/null || true

# CSP default directives
# SECURITY: Defaults use strict values - no unsafe-inline or unsafe-eval
# To opt in to less restrictive defaults, set CSP_MODE=moderate or CSP_MODE=permissive
CSP_DEFAULT_SRC="${CSP_DEFAULT_SRC:-'self'}"
CSP_SCRIPT_SRC="${CSP_SCRIPT_SRC:-'self'}"
CSP_STYLE_SRC="${CSP_STYLE_SRC:-'self'}"
CSP_IMG_SRC="${CSP_IMG_SRC:-'self' data:}"
CSP_FONT_SRC="${CSP_FONT_SRC:-'self'}"
CSP_CONNECT_SRC="${CSP_CONNECT_SRC:-'self'}"
CSP_MEDIA_SRC="${CSP_MEDIA_SRC:-'self'}"
CSP_OBJECT_SRC="${CSP_OBJECT_SRC:-'none'}"
CSP_FRAME_SRC="${CSP_FRAME_SRC:-'none'}"
CSP_BASE_URI="${CSP_BASE_URI:-'self'}"
CSP_FORM_ACTION="${CSP_FORM_ACTION:-'self'}"
CSP_FRAME_ANCESTORS="${CSP_FRAME_ANCESTORS:-'none'}"
CSP_UPGRADE_INSECURE_REQUESTS="${CSP_UPGRADE_INSECURE_REQUESTS:-true}"

# CSP mode: strict (default), moderate, permissive
# SECURITY HARDENING: Default changed from 'moderate' to 'strict' (V098-P1-013)
# - strict: No unsafe-inline or unsafe-eval (maximum security)
# - moderate: Allows unsafe-inline/unsafe-eval (for compatibility - opt-in only)
# - permissive: Minimal restrictions (development/debugging only)
CSP_MODE="${CSP_MODE:-strict}"

# Generate CSP header based on mode
csp::generate() {
  local mode="${1:-$CSP_MODE}"
  local custom_domains="${2:-}"

  case "$mode" in
    strict)
      csp::generate_strict "$custom_domains"
      ;;
    moderate)
      csp::generate_moderate "$custom_domains"
      ;;
    permissive)
      csp::generate_permissive "$custom_domains"
      ;;
    custom)
      csp::generate_custom "$custom_domains"
      ;;
    *)
      show_error "Invalid CSP mode: $mode (use strict, moderate, permissive, or custom)"
      return 1
      ;;
  esac
}

# Generate strict CSP (maximum security, may break some features)
csp::generate_strict() {
  local custom_domains="${1:-}"

  local csp="default-src 'self'; "
  csp="${csp}script-src 'self'; "
  csp="${csp}style-src 'self'; "
  csp="${csp}img-src 'self' data:; "
  csp="${csp}font-src 'self'; "
  csp="${csp}connect-src 'self'${custom_domains}; "
  csp="${csp}media-src 'self'; "
  csp="${csp}object-src 'none'; "
  csp="${csp}frame-src 'none'; "
  csp="${csp}base-uri 'self'; "
  csp="${csp}form-action 'self'; "
  csp="${csp}frame-ancestors 'none'; "
  csp="${csp}upgrade-insecure-requests"

  printf "%s" "$csp"
}

# Generate moderate CSP (balanced security and compatibility)
csp::generate_moderate() {
  local custom_domains="${1:-}"

  local csp="default-src 'self'; "
  csp="${csp}script-src 'self' 'unsafe-inline' 'unsafe-eval'${custom_domains}; "
  csp="${csp}style-src 'self' 'unsafe-inline'${custom_domains}; "
  csp="${csp}img-src 'self' data: https:; "
  csp="${csp}font-src 'self' data:${custom_domains}; "
  csp="${csp}connect-src 'self'${custom_domains}; "
  csp="${csp}media-src 'self'${custom_domains}; "
  csp="${csp}object-src 'none'; "
  csp="${csp}frame-src 'self'${custom_domains}; "
  csp="${csp}base-uri 'self'; "
  csp="${csp}form-action 'self'; "
  csp="${csp}frame-ancestors 'self'${custom_domains}; "
  csp="${csp}upgrade-insecure-requests"

  printf "%s" "$csp"
}

# Generate permissive CSP (minimal restrictions, more compatible)
csp::generate_permissive() {
  local custom_domains="${1:-}"

  local csp="default-src 'self' 'unsafe-inline' 'unsafe-eval' data: https:${custom_domains}; "
  csp="${csp}script-src 'self' 'unsafe-inline' 'unsafe-eval' https:${custom_domains}; "
  csp="${csp}style-src 'self' 'unsafe-inline' https:${custom_domains}; "
  csp="${csp}img-src 'self' data: https:${custom_domains}; "
  csp="${csp}font-src 'self' data: https:${custom_domains}; "
  csp="${csp}connect-src 'self' https:${custom_domains}; "
  csp="${csp}object-src 'none'; "
  csp="${csp}base-uri 'self'; "
  csp="${csp}form-action 'self'"

  printf "%s" "$csp"
}

# Generate custom CSP from environment variables
csp::generate_custom() {
  local custom_domains="${1:-}"

  local csp=""

  # Add each directive if configured
  [[ -n "$CSP_DEFAULT_SRC" ]] && csp="${csp}default-src ${CSP_DEFAULT_SRC}; "
  [[ -n "$CSP_SCRIPT_SRC" ]] && csp="${csp}script-src ${CSP_SCRIPT_SRC}${custom_domains}; "
  [[ -n "$CSP_STYLE_SRC" ]] && csp="${csp}style-src ${CSP_STYLE_SRC}${custom_domains}; "
  [[ -n "$CSP_IMG_SRC" ]] && csp="${csp}img-src ${CSP_IMG_SRC}; "
  [[ -n "$CSP_FONT_SRC" ]] && csp="${csp}font-src ${CSP_FONT_SRC}${custom_domains}; "
  [[ -n "$CSP_CONNECT_SRC" ]] && csp="${csp}connect-src ${CSP_CONNECT_SRC}${custom_domains}; "
  [[ -n "$CSP_MEDIA_SRC" ]] && csp="${csp}media-src ${CSP_MEDIA_SRC}${custom_domains}; "
  [[ -n "$CSP_OBJECT_SRC" ]] && csp="${csp}object-src ${CSP_OBJECT_SRC}; "
  [[ -n "$CSP_FRAME_SRC" ]] && csp="${csp}frame-src ${CSP_FRAME_SRC}${custom_domains}; "
  [[ -n "$CSP_BASE_URI" ]] && csp="${csp}base-uri ${CSP_BASE_URI}; "
  [[ -n "$CSP_FORM_ACTION" ]] && csp="${csp}form-action ${CSP_FORM_ACTION}; "
  [[ -n "$CSP_FRAME_ANCESTORS" ]] && csp="${csp}frame-ancestors ${CSP_FRAME_ANCESTORS}${custom_domains}; "

  if [[ "$CSP_UPGRADE_INSECURE_REQUESTS" == "true" ]]; then
    csp="${csp}upgrade-insecure-requests; "
  fi

  # Remove trailing semicolon and space
  csp="${csp%; }"

  printf "%s" "$csp"
}

# Add custom domain to CSP
csp::add_domain() {
  local domain="$1"
  local current_domains="${CSP_CUSTOM_DOMAINS:-}"

  # Check if domain already exists
  if echo "$current_domains" | grep -q "$domain"; then
    show_info "Domain already in CSP whitelist: $domain"
    return 0
  fi

  # Add domain
  if [[ -z "$current_domains" ]]; then
    export CSP_CUSTOM_DOMAINS=" $domain"
  else
    export CSP_CUSTOM_DOMAINS="$current_domains $domain"
  fi

  show_success "Added domain to CSP whitelist: $domain"
  return 0
}

# Remove domain from CSP
csp::remove_domain() {
  local domain="$1"
  local current_domains="${CSP_CUSTOM_DOMAINS:-}"

  # Remove domain
  local new_domains
  new_domains=$(echo "$current_domains" | sed "s/ $domain//g" | sed 's/  / /g')

  export CSP_CUSTOM_DOMAINS="$new_domains"

  show_success "Removed domain from CSP whitelist: $domain"
  return 0
}

# List CSP domains
csp::list_domains() {
  local current_domains="${CSP_CUSTOM_DOMAINS:-}"

  if [[ -z "$current_domains" ]]; then
    printf "No custom domains in CSP whitelist\n"
    return 0
  fi

  printf "CSP whitelisted domains:\n"
  for domain in $current_domains; do
    printf "  - %s\n" "$domain"
  done
}

# Generate CSP for specific service
csp::generate_for_service() {
  local service="$1"
  local mode="${2:-moderate}"

  case "$service" in
    hasura)
      # Hasura console requires unsafe-inline/unsafe-eval for its GraphQL IDE
      # This is a known limitation of the Hasura console UI
      # SECURITY NOTE: Only applied to the Hasura admin route, not the main app
      local csp="default-src 'self'; "
      csp="${csp}script-src 'self' 'unsafe-inline' 'unsafe-eval'; "
      csp="${csp}style-src 'self' 'unsafe-inline'; "
      csp="${csp}connect-src 'self' ws: wss:; "
      csp="${csp}img-src 'self' data:; "
      csp="${csp}frame-ancestors 'none'"
      printf "%s" "$csp"
      ;;
    grafana)
      # Grafana requires unsafe-inline/unsafe-eval for dashboard rendering
      # This is a known limitation of the Grafana UI framework
      # SECURITY NOTE: Only applied to the Grafana monitoring route
      local csp="default-src 'self'; "
      csp="${csp}script-src 'self' 'unsafe-inline' 'unsafe-eval'; "
      csp="${csp}style-src 'self' 'unsafe-inline'; "
      csp="${csp}connect-src 'self' https:; "
      csp="${csp}img-src 'self' data: https:; "
      csp="${csp}font-src 'self' data:"
      printf "%s" "$csp"
      ;;
    minio)
      # MinIO console requires unsafe-inline for its React-based UI
      # SECURITY NOTE: Only applied to the MinIO console route
      local csp="default-src 'self'; "
      csp="${csp}script-src 'self' 'unsafe-inline'; "
      csp="${csp}style-src 'self' 'unsafe-inline'; "
      csp="${csp}connect-src 'self' ws: wss:; "
      csp="${csp}img-src 'self' data: blob:"
      printf "%s" "$csp"
      ;;
    *)
      # Use default mode for unknown services (strict by default)
      csp::generate "$mode"
      ;;
  esac
}

# Validate CSP syntax
csp::validate() {
  local csp="$1"

  # Check for common mistakes
  local errors=0

  # Check for trailing semicolon after upgrade-insecure-requests
  if echo "$csp" | grep -q "upgrade-insecure-requests;"; then
    show_warning "upgrade-insecure-requests should not have trailing semicolon"
    errors=$((errors + 1))
  fi

  # Check for missing 'self' in quotes
  if echo "$csp" | grep -q "[^']self[^']"; then
    show_warning "CSP keywords like 'self' should be in single quotes"
    errors=$((errors + 1))
  fi

  # Check for duplicate directives
  local directives="default-src script-src style-src img-src font-src connect-src media-src object-src frame-src base-uri form-action frame-ancestors"
  for directive in $directives; do
    local count
    count=$(echo "$csp" | grep -o "$directive" | wc -l | tr -d ' ')
    if [[ $count -gt 1 ]]; then
      show_error "Duplicate directive found: $directive"
      errors=$((errors + 1))
    fi
  done

  if [[ $errors -eq 0 ]]; then
    show_success "CSP validation passed"
    return 0
  else
    show_error "CSP validation failed with $errors error(s)"
    return 1
  fi
}

# Show current CSP configuration
csp::show() {
  local mode="${CSP_MODE:-moderate}"
  local custom_domains="${CSP_CUSTOM_DOMAINS:-}"

  printf "${COLOR_CYAN}Current CSP Configuration${COLOR_RESET}\n"
  printf "  Mode: %s\n" "$mode"

  if [[ -n "$custom_domains" ]]; then
    printf "  Custom domains:%s\n" "$custom_domains"
  fi

  printf "\n${COLOR_CYAN}Generated CSP Header:${COLOR_RESET}\n"
  local csp
  csp=$(csp::generate "$mode" "$custom_domains")
  printf "%s\n" "$csp"

  # Validate
  printf "\n"
  csp::validate "$csp"
}

# Export CSP to nginx format
csp::export_nginx() {
  local mode="${1:-$CSP_MODE}"
  local custom_domains="${CSP_CUSTOM_DOMAINS:-}"
  local output_file="${2:-nginx/includes/csp.conf}"

  local csp
  csp=$(csp::generate "$mode" "$custom_domains")

  # Create directory if needed
  mkdir -p "$(dirname "$output_file")" 2>/dev/null || true

  # Write nginx format
  cat >"$output_file" <<EOF
# Content Security Policy (CSP)
# Generated by nself security headers
# Mode: $mode

add_header Content-Security-Policy "$csp" always;
EOF

  show_success "Exported CSP to $output_file"
  return 0
}

# Interactive CSP configuration
csp::configure() {
  printf "${COLOR_CYAN}CSP Configuration Wizard${COLOR_RESET}\n\n"

  # Select mode
  printf "Select CSP mode:\n"
  printf "  1) Strict (maximum security, no unsafe-inline/unsafe-eval) [recommended]\n"
  printf "  2) Moderate (allows unsafe-inline/unsafe-eval for compatibility)\n"
  printf "  3) Permissive (minimal restrictions - development/debugging only)\n"
  printf "  4) Custom (configure each directive manually)\n"
  printf "\nChoice [1]: "
  read -r mode_choice

  local mode="strict"
  case "${mode_choice:-1}" in
    1) mode="strict" ;;
    2) mode="moderate" ;;
    3) mode="permissive" ;;
    4) mode="custom" ;;
    *) mode="strict" ;;
  esac

  # Ask for custom domains
  printf "\nAdd custom domains to whitelist? (y/n) [n]: "
  read -r add_domains

  local custom_domains=""
  if [[ "$(echo "$add_domains" | tr '[:upper:]' '[:lower:]')" == "y" ]]; then
    printf "Enter domains (space-separated, e.g., cdn.example.com api.example.com): "
    read -r domains

    for domain in $domains; do
      custom_domains="$custom_domains $domain"
    done
  fi

  # Generate and show CSP
  printf "\n${COLOR_CYAN}Generated CSP:${COLOR_RESET}\n"
  local csp
  csp=$(csp::generate "$mode" "$custom_domains")
  printf "%s\n\n" "$csp"

  # Validate
  csp::validate "$csp"

  # Ask to save
  printf "\nSave this configuration? (y/n) [y]: "
  read -r save_choice

  if [[ "$(echo "${save_choice:-y}" | tr '[:upper:]' '[:lower:]')" == "y" ]]; then
    # Update .env
    if [[ -f ".env" ]]; then
      # Remove old CSP settings
      grep -v "^CSP_MODE=" .env >.env.tmp 2>/dev/null || true
      grep -v "^CSP_CUSTOM_DOMAINS=" .env.tmp >.env 2>/dev/null || true
      rm -f .env.tmp

      # Add new settings
      printf "\n# Content Security Policy\n" >>.env
      printf "CSP_MODE=%s\n" "$mode" >>.env
      if [[ -n "$custom_domains" ]]; then
        printf "CSP_CUSTOM_DOMAINS=%s\n" "$custom_domains" >>.env
      fi

      show_success "CSP configuration saved to .env"
    fi

    # Export to nginx
    csp::export_nginx "$mode" "$custom_domains"
  fi
}

# Export functions
export -f csp::generate
export -f csp::generate_strict
export -f csp::generate_moderate
export -f csp::generate_permissive
export -f csp::generate_custom
export -f csp::add_domain
export -f csp::remove_domain
export -f csp::list_domains
export -f csp::generate_for_service
export -f csp::validate
export -f csp::show
export -f csp::export_nginx
export -f csp::configure
