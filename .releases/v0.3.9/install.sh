#!/usr/bin/env bash
#
# nself Installation Script
# Version: 0.3.9
# 
# Usage:
#   curl -sSL https://install.nself.org | bash
#   wget -qO- https://install.nself.org | bash
#

set -e

# Configuration
NSELF_VERSION="0.3.9"
NSELF_REPO="nself-org/cli"
INSTALL_DIR="${NSELF_HOME:-/usr/local/nself}"
BIN_DIR="/usr/local/bin"
GITHUB_URL="https://github.com/${NSELF_REPO}"
RELEASE_URL="${GITHUB_URL}/releases/download/v${NSELF_VERSION}/nself-v${NSELF_VERSION}.tar.gz"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
RESET='\033[0m'

# Helper functions
log_info() { echo -e "${BLUE}ℹ${RESET} $1"; }
log_success() { echo -e "${GREEN}✓${RESET} $1"; }
log_warning() { echo -e "${YELLOW}⚠${RESET} $1"; }
log_error() { echo -e "${RED}✗${RESET} $1" >&2; }

# Detect OS
detect_os() {
  case "$(uname -s)" in
    Linux*)     OS="linux";;
    Darwin*)    OS="macos";;
    *)          OS="unknown";;
  esac
  
  case "$(uname -m)" in
    x86_64)     ARCH="amd64";;
    aarch64)    ARCH="arm64";;
    arm64)      ARCH="arm64";;
    *)          ARCH="unknown";;
  esac
}

# Check prerequisites
check_prerequisites() {
  local missing=()
  
  command -v docker >/dev/null 2>&1 || missing+=("docker")
  command -v docker-compose >/dev/null 2>&1 || {
    docker compose version >/dev/null 2>&1 || missing+=("docker-compose")
  }
  command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1 || missing+=("curl or wget")
  
  if [ ${#missing[@]} -gt 0 ]; then
    log_error "Missing required dependencies: ${missing[*]}"
    log_info "Please install the missing dependencies and try again."
    
    if [ "$OS" = "linux" ]; then
      log_info "On Ubuntu/Debian: sudo apt-get install docker.io docker-compose"
      log_info "On RHEL/CentOS: sudo yum install docker docker-compose"
    elif [ "$OS" = "macos" ]; then
      log_info "On macOS: brew install docker docker-compose"
    fi
    exit 1
  fi
}

# Download and extract nself
download_nself() {
  local temp_dir=$(mktemp -d)
  local archive_path="$temp_dir/nself.tar.gz"
  
  log_info "Downloading nself v${NSELF_VERSION}..."
  
  if command -v curl >/dev/null 2>&1; then
    curl -sSL "$RELEASE_URL" -o "$archive_path" || {
      # Fallback to GitHub archive if release not found
      curl -sSL "${GITHUB_URL}/archive/v${NSELF_VERSION}.tar.gz" -o "$archive_path"
    }
  else
    wget -qO "$archive_path" "$RELEASE_URL" || {
      # Fallback to GitHub archive if release not found
      wget -qO "$archive_path" "${GITHUB_URL}/archive/v${NSELF_VERSION}.tar.gz"
    }
  fi
  
  log_info "Extracting nself..."
  tar -xzf "$archive_path" -C "$temp_dir"
  
  # Find the extracted directory
  local extracted_dir=$(find "$temp_dir" -type d -name "nself*" | head -1)
  
  # Create installation directory
  if [ -d "$INSTALL_DIR" ]; then
    log_warning "Existing installation found at $INSTALL_DIR"
    read -p "Do you want to overwrite it? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
      log_info "Installation cancelled"
      exit 0
    fi
    sudo rm -rf "$INSTALL_DIR"
  fi
  
  # Install nself
  sudo mkdir -p "$INSTALL_DIR"
  sudo cp -r "$extracted_dir"/* "$INSTALL_DIR/"
  
  # Clean up
  rm -rf "$temp_dir"
}

# Create symlink
create_symlink() {
  log_info "Creating nself command..."
  
  # Create wrapper script
  sudo tee "$BIN_DIR/nself" > /dev/null << 'EOF'
#!/bin/bash
export NSELF_HOME="/usr/local/nself"
exec "/usr/local/nself/bin/nself" "$@"
EOF
  
  sudo chmod +x "$BIN_DIR/nself"
  sudo chmod +x "$INSTALL_DIR/bin/nself"
  
  log_success "nself command installed to $BIN_DIR/nself"
}

# Install shell completions
install_completions() {
  # Bash completions
  if [ -d /etc/bash_completion.d ] && [ -f "$INSTALL_DIR/completions/nself.bash" ]; then
    sudo cp "$INSTALL_DIR/completions/nself.bash" /etc/bash_completion.d/
    log_success "Bash completions installed"
  fi
  
  # Zsh completions
  if [ -d /usr/share/zsh/site-functions ] && [ -f "$INSTALL_DIR/completions/_nself" ]; then
    sudo cp "$INSTALL_DIR/completions/_nself" /usr/share/zsh/site-functions/
    log_success "Zsh completions installed"
  fi
}

# Verify installation
verify_installation() {
  if $BIN_DIR/nself version >/dev/null 2>&1; then
    local version=$($BIN_DIR/nself version)
    log_success "nself $version installed successfully!"
  else
    log_error "Installation verification failed"
    exit 1
  fi
}

# Main installation
main() {
  echo ""
  echo "╔══════════════════════════════════════════════════════════╗"
  echo "║           nself Installation Script v${NSELF_VERSION}            ║"
  echo "╚══════════════════════════════════════════════════════════╝"
  echo ""
  
  detect_os
  log_info "Detected OS: $OS ($ARCH)"
  
  check_prerequisites
  download_nself
  create_symlink
  install_completions
  verify_installation
  
  echo ""
  echo "╔══════════════════════════════════════════════════════════╗"
  echo "║                  Installation Complete! 🚀                ║"
  echo "╚══════════════════════════════════════════════════════════╝"
  echo ""
  echo "Quick Start:"
  echo "  mkdir myproject && cd myproject"
  echo "  nself init"
  echo "  nself build"
  echo "  nself start"
  echo ""
  echo "Documentation: https://github.com/nself-org/cli/docs/"
  echo "Report issues: https://github.com/nself-org/cli/issues"
  echo ""
}

# Run installation
main "$@"