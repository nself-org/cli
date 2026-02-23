#!/usr/bin/env bash

# hosts-helper.sh - Helper for /etc/hosts configuration

# Check if domain resolves to localhost
check_domain_resolution() {

set -euo pipefail

  local domain="$1"

  # Skip for standard domains
  if [[ "$domain" == "localhost" ]] || [[ "$domain" == "local.nself.org" ]]; then
    return 0
  fi

  # Check if domain resolves
  local resolved_ip
  resolved_ip=$(getent hosts "$domain" 2>/dev/null | awk '{print $1}' | head -1)

  if [[ -z "$resolved_ip" ]]; then
    return 1 # Domain doesn't resolve
  elif [[ "$resolved_ip" == "127.0.0.1" ]] || [[ "$resolved_ip" == "::1" ]]; then
    return 0 # Already resolves to localhost
  else
    return 2 # Resolves to different IP
  fi
}

# Generate /etc/hosts entries
generate_hosts_entries() {
  local base_domain="$1"
  local services="${2:-api hasura storage auth admin}"

  echo "# nself entries for $base_domain"
  echo "127.0.0.1 $base_domain"

  for service in $services; do
    echo "127.0.0.1 $service.$base_domain"
  done
}

# Check and update /etc/hosts
update_hosts_file() {
  local base_domain="$1"

  # Skip for domains that auto-resolve
  if [[ "$base_domain" == "localhost" ]] ||
    [[ "$base_domain" =~ \.localhost$ ]] ||
    [[ "$base_domain" == "local.nself.org" ]]; then
    return 0
  fi

  # Check if entries exist
  if grep -q "$base_domain" /etc/hosts 2>/dev/null; then
    log_info "/etc/hosts already contains $base_domain entries"
    return 0
  fi

  # Generate entries
  local entries
  entries=$(generate_hosts_entries "$base_domain")

  echo "The following entries need to be added to /etc/hosts:"
  echo ""
  echo "$entries"
  echo ""

  printf "%s" "Add these entries to /etc/hosts now? (requires sudo) (Y/n): "
  local add_entries
  read add_entries
  add_entries="${add_entries:-y}"

  if [[ "$add_entries" == "y" ]] || [[ "$add_entries" == "Y" ]]; then
    # Create temp file with entries
    local temp_file
    temp_file=$(mktemp /tmp/nself-hosts.XXXXXX)
    trap "rm -f '$temp_file'" EXIT
    echo "" >"$temp_file"
    echo "$entries" >>"$temp_file"

    # Append to /etc/hosts
    if sudo sh -c "cat '$temp_file' >> /etc/hosts"; then
      log_success "Added entries to /etc/hosts"
      rm -f "$temp_file"
      return 0
    else
      log_error "Failed to update /etc/hosts"
      echo "You can manually add these entries later:"
      echo "$entries"
      rm -f "$temp_file"
      return 1
    fi
  else
    echo "You'll need to manually add these entries to /etc/hosts:"
    echo "$entries"
    return 0
  fi
}

# Check if running on macOS and offer to use dnsmasq
check_macos_dnsmasq() {
  local base_domain="$1"

  if [[ "$(uname)" != "Darwin" ]]; then
    return 0
  fi

  # Check if domain ends with .local
  if [[ "$base_domain" == *.local ]]; then
    echo ""
    log_info "macOS Tip: For .local domains, you can use dnsmasq"
    echo "This will automatically resolve all *.local domains to localhost"
    echo ""
    echo "To set up dnsmasq:"
    echo "  brew install dnsmasq"
    echo "  echo 'address=/.local/127.0.0.1' > /usr/local/etc/dnsmasq.conf"
    echo "  sudo brew services start dnsmasq"
    echo ""
  fi
}

# Validate domain for local development
validate_local_domain() {
  local domain="$1"

  # Check for problematic TLDs
  case "$domain" in
    *.com | *.org | *.net | *.io)
      log_warning "Using a real TLD for local development"
      echo "Consider using .local or .test instead"
      ;;
    *.dev)
      log_warning ".dev is a real TLD owned by Google"
      echo "It requires HTTPS and may not work locally"
      echo "Consider using .local or .test instead"
      ;;
    *.app)
      log_warning ".app requires HTTPS everywhere"
      echo "Consider using .local or .test instead"
      ;;
  esac
}

# Automated SSL setup for build process
setup_ssl_automatically() {
  local base_domain="$1"
  local cert_dir="${2:-.certs}"

  # Skip for domains with built-in SSL
  if [[ "$base_domain" == "local.nself.org" ]]; then
    log_info "Using pre-configured SSL for local.nself.org"
    return 0
  fi

  echo "Setting up SSL certificates automatically..."

  # Install mkcert if needed (macOS)
  if ! command -v mkcert >/dev/null 2>&1; then
    if [[ "$(uname)" == "Darwin" ]]; then
      echo "Installing mkcert (this happens once)..."
      brew install mkcert 2>/dev/null || {
        log_info "mkcert will be installed during build"
      }
    fi
  fi

  # The actual certificate generation happens during build
  log_info "SSL certificates will be generated during 'nself build'"
  echo "  • You may be prompted for sudo password (once)"
  echo "  • This sets up the local certificate authority"
  echo "  • All services will use HTTPS automatically"

  return 0
}

# Export functions
export -f check_domain_resolution
export -f generate_hosts_entries
export -f update_hosts_file
export -f check_macos_dnsmasq
export -f validate_local_domain
export -f setup_ssl_automatically
