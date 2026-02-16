#!/usr/bin/env bash
# trust.sh - OS trust store management for SSL certificates


# Get the directory where this script is located
TRUST_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

# Go up 3 levels: ssl -> lib -> src -> nself root (only if not already set)
NSELF_ROOT="${NSELF_ROOT:-$(cd "$TRUST_LIB_DIR/../../.." && pwd)}"
NSELF_BIN_DIR="${HOME}/.nself/bin"

# Source utilities
source "$TRUST_LIB_DIR/../utils/display.sh" 2>/dev/null || true
source "$TRUST_LIB_DIR/ssl.sh" 2>/dev/null || true

# Install root CA to system trust stores
trust::install_root_ca() {
  local mkcert_cmd

  if ! mkcert_cmd="$(ssl::get_mkcert 2>/dev/null)"; then
    log_error "mkcert not available. Run 'nself ssl bootstrap' first."
    return 1
  fi

  log_info "Installing root CA to system trust stores..."

  # Run mkcert -install
  if $mkcert_cmd -install; then
    log_success "Root CA installed successfully"

    # Platform-specific success messages
    local os="$(uname -s)"
    case "$os" in
      Darwin)
        log_info "✓ Added to macOS Keychain"
        log_info "✓ Added to Firefox (if installed)"
        ;;
      Linux)
        log_info "✓ Added to system certificate store"
        log_info "✓ Added to Firefox/Chrome NSS database (if available)"
        ;;
    esac

    return 0
  else
    log_error "Failed to install root CA"
    return 1
  fi
}

# Install PFX certificate on Windows
trust::install_pfx_windows() {
  local pfx_file="$1"

  if [[ ! -f "$pfx_file" ]]; then
    log_error "PFX file not found: $pfx_file"
    return 1
  fi

  log_info "Installing certificate to Windows certificate store..."

  # Use certutil to import the certificate
  if command -v certutil &>/dev/null; then
    if certutil -user -importpfx "$pfx_file" NoRoot 2>/dev/null; then
      log_success "Certificate imported to Windows store"
      return 0
    fi
  fi

  # Fallback to PowerShell if available
  if command -v powershell &>/dev/null; then
    local ps_script="Import-PfxCertificate -FilePath '$pfx_file' -CertStoreLocation Cert:\CurrentUser\My"
    if powershell -Command "$ps_script" 2>/dev/null; then
      log_success "Certificate imported to Windows store"
      return 0
    fi
  fi

  log_warning "Could not automatically import certificate"
  log_info "Please manually import: $pfx_file"
  log_info "Double-click the file and follow the import wizard"
  return 1
}

# Uninstall root CA from system
trust::uninstall_root_ca() {
  local mkcert_cmd

  if ! mkcert_cmd="$(ssl::get_mkcert 2>/dev/null)"; then
    log_warning "mkcert not available"
    return 1
  fi

  log_info "Uninstalling root CA from system trust stores..."

  if $mkcert_cmd -uninstall; then
    log_success "Root CA uninstalled successfully"
    return 0
  else
    log_error "Failed to uninstall root CA"
    return 1
  fi
}

# Check trust status
trust::status() {
  local mkcert_cmd

  echo "Trust Store Status:"
  echo "==================="
  echo

  # Check mkcert root CA
  if mkcert_cmd="$(ssl::get_mkcert 2>/dev/null)"; then
    echo "mkcert Root CA:"

    local ca_root="$($mkcert_cmd -CAROOT 2>/dev/null)"
    if [[ -n "$ca_root" ]]; then
      echo "  Location: $ca_root"

      if [[ -f "$ca_root/rootCA.pem" ]]; then
        echo "  ✓ Root CA certificate exists"

        # Check if installed
        if $mkcert_cmd -install -check 2>/dev/null; then
          echo "  ✓ Installed in system trust store"
        else
          echo "  ✗ Not installed in system (run 'nself trust')"
        fi

        # Show certificate details
        local ca_subject=$(openssl x509 -in "$ca_root/rootCA.pem" -noout -subject 2>/dev/null | sed 's/subject=//')
        local ca_expiry=$(openssl x509 -in "$ca_root/rootCA.pem" -noout -enddate 2>/dev/null | cut -d= -f2)
        echo "  Subject: $ca_subject"
        echo "  Expires: $ca_expiry"
      else
        echo "  ✗ Root CA certificate not found"
      fi
    else
      echo "  ✗ mkcert not initialized"
    fi
  else
    echo "  ✗ mkcert not installed"
  fi
  echo

  # Platform-specific trust store info
  local os="$(uname -s)"
  case "$os" in
    Darwin)
      echo "Platform: macOS"
      echo "  Trust stores: System Keychain, Firefox NSS"
      # Check if certificate is in keychain
      if security find-certificate -c "mkcert" &>/dev/null; then
        echo "  ✓ mkcert certificate found in Keychain"
      fi
      ;;
    Linux)
      echo "Platform: Linux"
      echo "  Trust stores: /usr/local/share/ca-certificates, Firefox/Chrome NSS"
      # Check common certificate locations
      if [[ -f "/usr/local/share/ca-certificates/mkcert-rootCA.crt" ]] ||
        [[ -f "/etc/pki/ca-trust/source/anchors/mkcert-rootCA.pem" ]]; then
        echo "  ✓ mkcert certificate found in system store"
      fi
      ;;
  esac
  echo

  # Check for PFX files
  local pfx_file="$NSELF_ROOT/templates/certs/nself-org/wildcard.pfx"
  if [[ -f "$pfx_file" ]]; then
    echo "Windows Certificate (PFX):"
    echo "  ✓ PFX bundle available at: $pfx_file"
    echo "  Import manually on Windows for *.local.nself.org"
  fi
}

# Update /etc/hosts if needed
trust::update_hosts() {
  local project_name="${PROJECT_NAME:-app}"
  local needs_update=false
  local hosts_entries=()

  # Build list of required hosts entries
  hosts_entries+=(
    "127.0.0.1       api.localhost"
    "127.0.0.1       auth.localhost"
    "127.0.0.1       storage.localhost"
    "127.0.0.1       functions.localhost"
    "127.0.0.1       dashboard.localhost"
    "127.0.0.1       console.localhost"
  )

  # Add project-specific entries
  if [[ -n "$project_name" ]]; then
    hosts_entries+=("127.0.0.1       ${project_name}.localhost")
    [[ "$project_name" == "nchat" ]] && hosts_entries+=("127.0.0.1       chat.localhost")
    [[ "$project_name" == "admin" ]] && hosts_entries+=("127.0.0.1       admin.localhost")
  fi

  # Check which entries are missing
  local missing_entries=()
  for entry in "${hosts_entries[@]}"; do
    local domain=$(echo "$entry" | awk '{print $2}')
    if ! grep -q "$domain" /etc/hosts 2>/dev/null; then
      missing_entries+=("$entry")
      needs_update=true
    fi
  done

  if [[ "$needs_update" == "true" ]]; then
    log_info "Some domains need to be added to /etc/hosts for proper resolution"
    echo
    echo "The following entries are missing:"
    for entry in "${missing_entries[@]}"; do
      echo "  $entry"
    done
    echo

    # Ask for permission
    read -p "Add these entries to /etc/hosts? (requires sudo) [y/N]: " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      for entry in "${missing_entries[@]}"; do
        echo "$entry" | sudo tee -a /etc/hosts >/dev/null
      done
      log_success "Added ${#missing_entries[@]} entries to /etc/hosts"
    else
      log_warning "Skipped /etc/hosts update. Some domains may not resolve correctly."
    fi
  else
    log_info "All required domains are already in /etc/hosts"
  fi
}

# Main trust installation function
trust::install() {
  log_info "Setting up certificate trust..."

  # Install mkcert root CA
  if ! trust::install_root_ca; then
    return 1
  fi

  # Update /etc/hosts if needed (for localhost subdomains)
  if [[ "${BASE_DOMAIN:-localhost}" == "localhost" ]]; then
    trust::update_hosts
  fi

  # Check for Windows and PFX
  if [[ "$OSTYPE" == "msys" ]] || [[ "$OSTYPE" == "cygwin" ]]; then
    local pfx_file="$NSELF_ROOT/templates/certs/nself-org/wildcard.pfx"
    if [[ -f "$pfx_file" ]]; then
      log_info "Found PFX certificate for Windows"
      trust::install_pfx_windows "$pfx_file"
    fi
  fi

  echo
  log_success "Certificate trust setup complete!"
  log_info "Your browser will now trust locally-generated certificates"

  # Show what domains are trusted
  echo
  echo "Trusted domains:"

  if [[ "${BASE_DOMAIN:-localhost}" == "localhost" ]]; then
    echo "  • localhost"
    echo "  • api.localhost, auth.localhost, storage.localhost"
    [[ -n "${PROJECT_NAME:-}" ]] && echo "  • ${PROJECT_NAME}.localhost"
    [[ "${PROJECT_NAME:-}" == "nchat" ]] && echo "  • chat.localhost"
  else
    echo "  • ${BASE_DOMAIN}, *.${BASE_DOMAIN}"
  fi

  echo "  • 127.0.0.1, ::1"

  if [[ -f "$NSELF_ROOT/templates/certs/nself-org/fullchain.pem" ]]; then
    local issuer=$(openssl x509 -in "$NSELF_ROOT/templates/certs/nself-org/fullchain.pem" -noout -issuer 2>/dev/null | sed 's/issuer=//')
    if [[ "$issuer" != *"Let's Encrypt"* ]]; then
      echo "  • local.nself.org, *.local.nself.org"
    fi
  fi

  echo
  log_info "You may need to restart your browser for changes to take effect"

  return 0
}

# Check if trust is needed (returns 0 if trust IS needed, 1 if already trusted)
trust::needs_install() {
  local mkcert_cmd

  # Get mkcert command
  if ! mkcert_cmd="$(ssl::get_mkcert 2>/dev/null)"; then
    return 1 # No mkcert, can't check
  fi

  # Check if root CA is installed
  if $mkcert_cmd -install -check 2>/dev/null; then
    return 1 # Already installed
  fi

  return 0 # Needs installation
}

# Auto-install trust if needed (silent mode for automation)
trust::auto_install() {
  local silent="${1:-false}"

  # Check if auto-trust is disabled
  if [[ "${SSL_AUTO_TRUST:-true}" == "false" ]]; then
    return 0
  fi

  # Check if trust is needed
  if ! trust::needs_install; then
    return 0 # Already trusted
  fi

  local mkcert_cmd
  if ! mkcert_cmd="$(ssl::get_mkcert 2>/dev/null)"; then
    return 1
  fi

  if [[ "$silent" != "true" ]]; then
    log_info "Installing SSL root CA to system trust store..."
  fi

  # Try to install - may prompt for password
  if $mkcert_cmd -install 2>/dev/null; then
    if [[ "$silent" != "true" ]]; then
      log_success "Root CA installed - browsers will trust local certificates"
    fi
    return 0
  else
    if [[ "$silent" != "true" ]]; then
      log_warning "Could not auto-install trust (may need password)"
      log_info "Run 'nself trust' manually to enable trusted HTTPS"
    fi
    return 1
  fi
}

# Export functions
export -f trust::install_root_ca
export -f trust::install_pfx_windows
export -f trust::uninstall_root_ca
export -f trust::status
export -f trust::install
export -f trust::needs_install
export -f trust::auto_install
