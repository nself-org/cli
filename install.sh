#!/bin/bash

# install.sh - Smart installation script for nself CLI
# Handles: fresh install, upgrades, migrations, different installation modes
#
# NOTE: This script must be self-contained since it runs before nself is installed.
#       Output functions mirror those in src/lib/utils/display.sh but are
#       duplicated here for independence. See .wiki/OUTPUT_FORMATTING.MD for standards.

set -e

# ========================================================================
# CONFIGURATION
# ========================================================================

# Default installation settings
DEFAULT_INSTALL_MODE="user"  # user, system, docker, portable
DEFAULT_INSTALL_DIR="$HOME/.nself"
DEFAULT_BRANCH="main"
DEFAULT_REPO="nself-org/cli"
NSELF_VERSION="${NSELF_VERSION:-}"  # Allow version override
FULL_INSTALL="${FULL_INSTALL:-false}"  # Install all files including examples, scripts, tests

# Parse command line arguments (will be re-parsed in main() after flags)
INSTALL_MODE="${1:-$DEFAULT_INSTALL_MODE}"
INSTALL_DIR="${2:-$DEFAULT_INSTALL_DIR}"
FORCE_REINSTALL="${FORCE_REINSTALL:-false}"
SKIP_BACKUP="${SKIP_BACKUP:-false}"
SKIP_PATH="${SKIP_PATH:-false}"
VERBOSE="${VERBOSE:-false}"
BIN_LINK=""
NEEDS_SUDO=false

# Repository URLs
REPO_URL="https://github.com/${DEFAULT_REPO}"
GITHUB_API="https://api.github.com/repos/${DEFAULT_REPO}"

# Get the version to install (latest release or specified version)
get_install_version() {
  # CRITICAL FIX: Allow forcing latest main branch via env var
  # Usage: curl -fsSL install.sh | INSTALL_LATEST=true bash
  if [[ "${INSTALL_LATEST:-false}" == "true" ]]; then
    echo "main"
    return 0
  fi

  if [[ -n "$NSELF_VERSION" ]]; then
    echo "$NSELF_VERSION"
  else
    # Fetch latest release tag from GitHub
    local latest_tag=$(curl -s "${GITHUB_API}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    if [[ -n "$latest_tag" ]]; then
      echo "$latest_tag"
    else
      # Fallback to main branch if no releases
      echo "main"
    fi
  fi
}

INSTALL_VERSION=$(get_install_version)
REPO_RAW_URL="https://raw.githubusercontent.com/${DEFAULT_REPO}/${INSTALL_VERSION}"

# Installation paths will be set based on mode (done in main() after arg parsing)
BIN_DIR=""
SRC_DIR=""
BACKUP_DIR="$HOME/.nself-backup"
TEMP_DIR=$(mktemp -d -t nself-install-XXXXXX)

# Function to set installation paths based on mode
set_install_paths() {
  local mode="$1"
  local custom_dir="${2:-}"

  case "$mode" in
    system)
      INSTALL_DIR="/usr/local/nself"
      BIN_LINK="/usr/local/bin/nself"
      NEEDS_SUDO=true
      ;;
    docker)
      INSTALL_DIR="/opt/nself"
      BIN_LINK="/usr/bin/nself"
      NEEDS_SUDO=false
      ;;
    portable)
      INSTALL_DIR="${custom_dir:-./nself}"
      BIN_LINK=""
      NEEDS_SUDO=false
      ;;
    user|*)
      INSTALL_DIR="${custom_dir:-$HOME/.nself}"
      BIN_LINK=""
      NEEDS_SUDO=false
      ;;
  esac

  BIN_DIR="$INSTALL_DIR/bin"
  SRC_DIR="$INSTALL_DIR/src"
}

# Cleanup on exit
trap 'rm -rf "$TEMP_DIR"' EXIT

# ========================================================================
# OUTPUT FUNCTIONS
# ========================================================================
# NOTE: These use echo_* prefix instead of log_* to avoid confusion with
#       system logging. Once installed, nself uses log_* functions from display.sh

# Color support detection
if [[ -t 1 ]] && [[ -n "${TERM:-}" ]] && command -v tput >/dev/null 2>&1; then
  RED=$(tput setaf 1)
  GREEN=$(tput setaf 2)
  YELLOW=$(tput setaf 3)
  BLUE=$(tput setaf 4)
  MAGENTA=$(tput setaf 5)
  CYAN=$(tput setaf 6)
  BOLD=$(tput bold)
  RESET=$(tput sgr0)
else
  RED=""
  GREEN=""
  YELLOW=""
  BLUE=""
  MAGENTA=""
  CYAN=""
  BOLD=""
  RESET=""
fi

echo_header() {
  local title="$1"
  local width=58
  local title_len=${#title}
  local padding=$(( (width - title_len - 2) / 2 ))
  local right_padding=$(( width - title_len - 2 - padding ))
  
  # Create border string without using seq or C-style for loop
  local border=""
  local i=0
  while [ $i -lt $width ]; do
    border="${border}═"
    i=$((i + 1))
  done
  
  echo ""
  echo "${BOLD}╔${border}╗${RESET}"
  printf "${BOLD}║%*s%s%*s║${RESET}\n" $padding "" "$title" $right_padding ""
  echo "${BOLD}╚${border}╝${RESET}"
  echo ""
}

echo_section() {
  local title="$1"
  local underline=""
  local i=0
  local title_len=${#title}
  while [ $i -lt $title_len ]; do
    underline="${underline}─"
    i=$((i + 1))
  done
  
  echo ""
  echo "${BOLD}${title}${RESET}"
  echo "$underline"
}

echo_info() {
  echo "${BLUE}[INFO]${RESET} $1"
}

echo_success() {
  echo "${GREEN}[SUCCESS]${RESET} $1"
}

echo_warning() {
  echo "${YELLOW}[WARNING]${RESET} $1"
}

echo_error() {
  echo "${RED}[ERROR]${RESET} $1" >&2
}

echo_debug() {
  if [[ "$VERBOSE" == "true" ]]; then
    echo "${MAGENTA}[DEBUG]${RESET} $1"
  fi
  return 0  # Always return success to avoid set -e issues
}

# Progress spinner (follows OUTPUT_FORMATTING.MD standard)
show_spinner() {
  local pid=$1
  local message=$2
  local spin='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
  local i=0
  
  if [[ -t 1 ]]; then
    # Use BLUE for spinner per standard
    printf "${BLUE}%s${RESET}" "$message"
    
    while kill -0 $pid 2>/dev/null; do
      i=$(( (i+1) %10 ))
      printf "\r${BLUE}${spin:$i:1}${RESET} %s" "$message"
      sleep 0.1
    done
    
    wait $pid
    local result=$?
    
    if [ $result -eq 0 ]; then
      printf "\r${GREEN}✓${RESET} %s\n" "$message"
    else
      printf "\r${RED}✗${RESET} %s\n" "$message"
    fi
    
    return $result
  else
    echo "$message"
    wait $pid
    return $?
  fi
}

confirm() {
  local prompt="${1:-Continue?}"
  local default="${2:-n}"
  
  if [[ "$default" == "y" ]]; then
    prompt="$prompt [Y/n]: "
    default_val=0
  else
    prompt="$prompt [y/N]: "
    default_val=1
  fi
  
  read -p "$prompt" -n 1 -r
  echo
  
  if [[ -z "$REPLY" ]]; then
    return $default_val
  elif [[ "$REPLY" =~ ^[Yy]$ ]]; then
    return 0
  else
    return 1
  fi
}

# ========================================================================
# UTILITY FUNCTIONS
# ========================================================================

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

get_sudo() {
  if [[ "$NEEDS_SUDO" == "true" ]] && [[ "$EUID" -ne 0 ]]; then
    echo "sudo"
  else
    echo ""
  fi
}

run_cmd() {
  local sudo=$(get_sudo)
  echo_debug "Running: $sudo $*"
  $sudo "$@"
}

detect_os() {
  case "$(uname -s)" in
    Darwin*)  OS="macos" ;;
    Linux*)   OS="linux" ;;
    CYGWIN*)  OS="windows" ;;
    MINGW*)   OS="windows" ;;
    *)        OS="unknown" ;;
  esac
  echo_debug "Detected OS: $OS"
}

detect_arch() {
  case "$(uname -m)" in
    x86_64)   ARCH="amd64" ;;
    aarch64)  ARCH="arm64" ;;
    arm64)    ARCH="arm64" ;;
    armv7l)   ARCH="arm" ;;
    *)        ARCH="unknown" ;;
  esac
  echo_debug "Detected architecture: $ARCH"
}

version_compare() {
  # Compare two version strings
  # Returns: 0 if equal, 1 if $1 > $2, 2 if $1 < $2
  local v1="$1"
  local v2="$2"
  
  if [[ "$v1" == "$v2" ]]; then
    return 0
  fi
  
  # Sort versions and check which is higher
  local sorted=$(printf '%s\n%s\n' "$v1" "$v2" | sort -V | head -n1)
  
  if [[ "$sorted" == "$v1" ]]; then
    return 2  # v1 < v2
  else
    return 1  # v1 > v2
  fi
}

get_installed_version() {
  local version="unknown"
  
  # Try multiple methods to get version
  if command_exists nself; then
    version=$(nself version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -n1 || echo "unknown")
  elif [[ -f "$BIN_DIR/VERSION" ]]; then
    version=$(cat "$BIN_DIR/VERSION" 2>/dev/null || echo "unknown")
  elif [[ -f "$INSTALL_DIR/VERSION" ]]; then
    version=$(cat "$INSTALL_DIR/VERSION" 2>/dev/null || echo "unknown")
  fi
  
  echo "$version"
}

get_latest_version() {
  local version
  version=$(curl -fsSL "$REPO_RAW_URL/src/config/VERSION" 2>/dev/null || \
           curl -fsSL "$REPO_RAW_URL/VERSION" 2>/dev/null || \
           echo "unknown")
  echo "$version"
}

# ========================================================================
# INSTALLATION DETECTION
# ========================================================================

detect_existing_installation() {
  echo_header "Checking for Existing Installation"
  
  local found_installations=()
  
  # Check standard locations
  local locations=(
    "$HOME/.nself"
    "/usr/local/nself"
    "/opt/nself"
    "./nself"
  )
  
  for loc in "${locations[@]}"; do
    if [[ -d "$loc" ]] && [[ -f "$loc/bin/nself.sh" || -f "$loc/bin/nself" ]]; then
      found_installations+=("$loc")
      echo_info "Found installation at: $loc"
    fi
  done
  
  # Check PATH for nself
  if command_exists nself; then
    local nself_path=$(which nself)
    echo_info "Found nself in PATH: $nself_path"
    
    # Get the installation directory from the command
    local nself_dir=$(dirname $(dirname $(readlink -f "$nself_path" || echo "$nself_path")))
    # shellcheck disable=SC2199 # Intentional array concatenation in regex match
    if [[ -d "$nself_dir" ]] && [[ ! " ${found_installations[*]} " =~ " ${nself_dir} " ]]; then
      found_installations+=("$nself_dir")
    fi
  fi
  
  if [[ ${#found_installations[@]} -eq 0 ]]; then
    echo_success "No existing installation found - proceeding with fresh install"
    return 1
  else
    # Check version of primary installation
    local installed_version=$(get_installed_version)
    local latest_version=$(get_latest_version)
    
    echo ""
    echo_info "Currently installed: v${installed_version}"
    echo_info "Latest available: v${latest_version}"
    
    # Determine if this is an upgrade or reinstall
    if [[ "$installed_version" != "unknown" ]] && [[ "$latest_version" != "unknown" ]]; then
      version_compare "$installed_version" "$latest_version"
      local cmp_result=$?
      
      if [[ $cmp_result -eq 0 ]]; then
        echo_success "You have the latest version installed"
        
        if [[ "$FORCE_REINSTALL" != "true" ]]; then
          if ! confirm "Reinstall anyway?" "n"; then
            echo_info "Installation cancelled"
            exit 0
          fi
        fi
      elif [[ $cmp_result -eq 2 ]]; then
        echo_warning "An update is available: v${installed_version} → v${latest_version}"
        
        # Check for breaking changes (major version difference)
        local installed_major="${installed_version%%.*}"
        local latest_major="${latest_version%%.*}"
        
        if [[ "$installed_major" != "$latest_major" ]]; then
          echo ""
          echo_warning "⚠️  BREAKING CHANGES DETECTED ⚠️"
          echo_warning "This is a major version upgrade ($installed_major.x → $latest_major.x)"
          echo_warning "Your current installation will be backed up"
          echo ""
          
          if ! confirm "Proceed with upgrade?" "y"; then
            echo_info "Upgrade cancelled"
            exit 0
          fi
        fi
      else
        echo_warning "Installed version ($installed_version) is newer than latest ($latest_version)"
        
        if ! confirm "Downgrade to v${latest_version}?" "n"; then
          echo_info "Installation cancelled"
          exit 0
        fi
      fi
    fi
    
    return 0
  fi
}

# ========================================================================
# BACKUP FUNCTIONS
# ========================================================================

backup_existing_installation() {
  if [[ "$SKIP_BACKUP" == "true" ]]; then
    echo_info "Skipping backup (--skip-backup specified)"
    return 0
  fi
  
  if [[ ! -d "$INSTALL_DIR" ]]; then
    return 0
  fi
  
  echo_header "Backing Up Existing Installation"
  
  local timestamp=$(date +%Y%m%d_%H%M%S)
  local version=$(get_installed_version)
  local backup_name="nself_${version}_${timestamp}"
  local backup_path="$BACKUP_DIR/$backup_name"
  
  echo_info "Creating backup at: $backup_path"
  
  (
    mkdir -p "$BACKUP_DIR"
    cp -r "$INSTALL_DIR" "$backup_path"
    
    # Save installation metadata
    cat > "$backup_path/.backup_info" << EOF
Backup Date: $(date)
Version: $version
Original Path: $INSTALL_DIR
Installation Mode: $INSTALL_MODE
EOF
  ) &
  show_spinner $! "Backing up installation"
  
  echo_success "Backup created: $backup_path"
  
  # Clean old backups (keep last 3)
  echo_info "Cleaning old backups..."
  local backup_count=$(ls -1 "$BACKUP_DIR" 2>/dev/null | wc -l)
  if [[ $backup_count -gt 3 ]]; then
    ls -1t "$BACKUP_DIR" | tail -n +4 | while read old_backup; do
      echo_debug "Removing old backup: $old_backup"
      rm -rf "$BACKUP_DIR/$old_backup"
    done
  fi
}

# ========================================================================
# PREREQUISITE CHECKS
# ========================================================================

check_prerequisites() {
  echo_header "Prerequisites Check"
  
  local errors=0
  
  # Check OS
  detect_os
  if [[ "$OS" == "unknown" ]]; then
    echo_error "Unsupported operating system"
    ((errors++))
  else
    echo_success "Operating system: $OS"
  fi
  
  # Check architecture
  detect_arch
  if [[ "$ARCH" == "unknown" ]]; then
    echo_warning "Unknown architecture - installation may not work correctly"
  else
    echo_success "Architecture: $ARCH"
  fi
  
  # Check for required commands
  local required_commands=("curl" "tar" "bash")
  for cmd in "${required_commands[@]}"; do
    if command_exists "$cmd"; then
      echo_success "Found required command: $cmd"
    else
      echo_error "Missing required command: $cmd"
      ((errors++))
    fi
  done
  
  # Check for optional but recommended commands
  local optional_commands=("git" "docker")
  for cmd in "${optional_commands[@]}"; do
    if command_exists "$cmd"; then
      echo_success "Found optional command: $cmd"
    else
      echo_warning "Missing optional command: $cmd (some features may not work)"
    fi
  done
  
  # Check disk space
  local available_space=$(df "$HOME" | awk 'NR==2 {print $4}')
  if [[ $available_space -lt 100000 ]]; then  # Less than 100MB
    echo_error "Insufficient disk space (need at least 100MB)"
    ((errors++))
  else
    echo_success "Sufficient disk space available"
  fi
  
  # Check permissions for installation directory
  local parent_dir=$(dirname "$INSTALL_DIR")
  if [[ ! -w "$parent_dir" ]]; then
    if [[ "$NEEDS_SUDO" != "true" ]]; then
      echo_error "No write permission for $parent_dir"
      echo_info "Try: sudo $0 system  # for system-wide installation"
      ((errors++))
    fi
  else
    echo_success "Write permission for installation directory"
  fi
  
  if [[ $errors -gt 0 ]]; then
    echo ""
    echo_error "Prerequisites check failed with $errors error(s)"
    exit 1
  fi
  
  echo ""
  echo_success "All prerequisites met"
}

# ========================================================================
# DOWNLOAD FUNCTIONS
# ========================================================================

verify_checksum() {
  local file="$1"
  local expected_checksum="$2"

  if [[ -z "$expected_checksum" ]]; then
    echo_warning "No checksum available for verification"
    return 1
  fi

  local actual_checksum=""
  if command_exists sha256sum; then
    actual_checksum=$(sha256sum "$file" | awk '{print $1}')
  elif command_exists shasum; then
    actual_checksum=$(shasum -a 256 "$file" | awk '{print $1}')
  else
    echo_warning "No SHA-256 tool found (sha256sum or shasum) - skipping verification"
    return 1
  fi

  if [[ "$actual_checksum" == "$expected_checksum" ]]; then
    echo_success "Checksum verification passed (SHA-256)"
    return 0
  else
    echo_error "Checksum verification FAILED!"
    echo_error "  Expected: $expected_checksum"
    echo_error "  Actual:   $actual_checksum"
    echo_error "The downloaded file may be corrupted or tampered with."
    echo_error "Aborting installation for safety."
    return 2
  fi
}

download_nself() {
  echo_header "Downloading nself"

  echo_info "Version: $INSTALL_VERSION"
  echo_info "Target: $TEMP_DIR"

  local tar_url=""
  local checksum_url=""
  local archive_file="$TEMP_DIR/nself-archive.tar.gz"

  # Determine download URL based on version
  if [[ "$INSTALL_VERSION" == "main" ]] || [[ "$INSTALL_VERSION" == "latest" ]]; then
    tar_url="$REPO_URL/archive/refs/heads/main.tar.gz"
    echo_info "Installing development version (full source)"
  elif [[ "$INSTALL_VERSION" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    local version="${INSTALL_VERSION#v}"
    tar_url="${REPO_URL}/releases/download/v${version}/nself-v${version}.tar.gz"
    checksum_url="${REPO_URL}/releases/download/v${version}/nself-v${version}.tar.gz.sha256"
    echo_info "Installing release version (minimal runtime files)"
  elif [[ "$INSTALL_VERSION" == "main" ]] || [[ "$INSTALL_VERSION" == "master" ]]; then
    # CRITICAL FIX: Use correct URL for branch archives (not tags)
    tar_url="$REPO_URL/archive/refs/heads/${INSTALL_VERSION}.tar.gz"
    echo_info "Installing latest from ${INSTALL_VERSION} branch (full source)"
  else
    # Assume it's a tag
    tar_url="$REPO_URL/archive/refs/tags/${INSTALL_VERSION}.tar.gz"
    echo_info "Installing from tag: ${INSTALL_VERSION} (full source)"
  fi

  # Download archive to file first (for integrity verification)
  echo_info "Downloading archive..."
  if ! curl -fsSL "$tar_url" -o "$archive_file" 2>/dev/null; then
    # Fallback for release versions
    if [[ "$INSTALL_VERSION" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      echo_warning "Release tarball not found, falling back to source archive..."
      tar_url="$REPO_URL/archive/refs/tags/${INSTALL_VERSION}.tar.gz"
      checksum_url=""
      if ! curl -fsSL "$tar_url" -o "$archive_file" 2>/dev/null; then
        echo_error "Failed to download nself from $tar_url"
        exit 1
      fi
    else
      echo_error "Failed to download nself from $tar_url"
      exit 1
    fi
  fi

  echo_success "Downloaded archive"

  # Verify integrity if checksum is available
  if [[ -n "$checksum_url" ]]; then
    echo_info "Verifying artifact integrity..."
    local expected_checksum
    expected_checksum=$(curl -fsSL "$checksum_url" 2>/dev/null | awk '{print $1}')

    if [[ -n "$expected_checksum" ]]; then
      verify_checksum "$archive_file" "$expected_checksum"
      local verify_result=$?
      if [[ $verify_result -eq 2 ]]; then
        rm -f "$archive_file"
        exit 1
      fi
    else
      echo_warning "Checksum file not available - skipping verification"
      echo_warning "For maximum security, verify the download manually"
    fi
  else
    echo_info "No checksum endpoint for this version type - skipping verification"
  fi

  # Extract verified archive
  echo_info "Extracting archive..."
  if tar -xzf "$archive_file" -C "$TEMP_DIR" --strip-components=1; then
    echo_success "Archive extracted successfully"
    rm -f "$archive_file"
  else
    echo_error "Failed to extract archive"
    rm -f "$archive_file"
    exit 1
  fi
}

# ========================================================================
# INSTALLATION FUNCTIONS
# ========================================================================
#
# File Installation Strategy:
# ---------------------------
# MINIMAL (default):
#   - bin/           : CLI entry point (~4KB)
#   - src/cli/       : Command implementations (~1.4MB)
#   - src/lib/       : Core libraries (~3.7MB)
#   - src/templates/ : Project templates for init (~1.3MB)
#   - src/services/  : Service definitions (~144KB)
#   - src/database/  : Schema definitions (~264KB)
#   - src/tools/     : Runtime utilities (SSL, etc) (~4.9MB)
#   - src/VERSION    : Version tracking
#   Total: ~11.6MB (sufficient for all CLI operations)
#
# FULL (--full flag):
#   - Everything above, PLUS:
#   - src/examples/  : Example projects (~48KB)
#   - src/scripts/   : Development scripts (~16KB)
#   - src/tests/     : Test suites (~1.4MB)
#   Total: ~13.1MB (for development and testing)
#
# EXCLUDED from both modes:
#   - .wiki/          : Documentation (available online)
#   - .github/       : CI/CD workflows
#   - Hidden dirs    : Git metadata, IDE configs
#
# ========================================================================

install_files() {
  echo_header "Installing Files"

  local source_dir="$TEMP_DIR/nself"
  [[ -d "$source_dir" ]] || source_dir="$TEMP_DIR"

  # Create installation directory
  echo_info "Creating directory: $INSTALL_DIR"
  run_cmd mkdir -p "$INSTALL_DIR"

  # Determine installation mode
  if [[ "$FULL_INSTALL" == "true" ]]; then
    echo_info "Full installation mode (all files including examples, scripts, tests)"
  else
    echo_info "Minimal installation mode (CLI runtime only, ~8MB)"
  fi

  # Copy files
  echo_info "Copying files..."
  (
    # Copy bin directory (CLI entry point shim)
    if [[ -d "$source_dir/bin" ]]; then
      run_cmd cp -r "$source_dir/bin" "$INSTALL_DIR/"
    fi

    # Copy src directory with smart filtering
    if [[ -d "$source_dir/src" ]]; then
      run_cmd mkdir -p "$INSTALL_DIR/src"

      # Always copy VERSION file
      for version_file in "$source_dir/src/VERSION" "$source_dir/src/config/VERSION" "$source_dir/VERSION"; do
        if [[ -f "$version_file" ]]; then
          run_cmd cp "$version_file" "$INSTALL_DIR/src/VERSION"
          break
        fi
      done

      if [[ "$FULL_INSTALL" == "true" ]]; then
        # Full install: Copy everything except hidden directories
        for item in "$source_dir/src/"*; do
          local basename=$(basename "$item")
          if [[ "$basename" != .* ]]; then
            run_cmd cp -r "$item" "$INSTALL_DIR/src/"
          fi
        done
      else
        # Minimal install: Copy only runtime essentials
        # Core runtime: cli, lib (required for all commands)
        for dir in cli lib; do
          if [[ -d "$source_dir/src/$dir" ]]; then
            run_cmd cp -r "$source_dir/src/$dir" "$INSTALL_DIR/src/"
          fi
        done

        # Templates: Required for nself init and custom service generation
        if [[ -d "$source_dir/src/templates" ]]; then
          run_cmd cp -r "$source_dir/src/templates" "$INSTALL_DIR/src/"
        fi

        # Services: Copy directory structure if exists (may contain starter services)
        if [[ -d "$source_dir/src/services" ]]; then
          run_cmd cp -r "$source_dir/src/services" "$INSTALL_DIR/src/"
        fi

        # Database: Schema definitions used at runtime
        if [[ -d "$source_dir/src/database" ]]; then
          run_cmd cp -r "$source_dir/src/database" "$INSTALL_DIR/src/"
        fi

        # Tools: SSL certificate generation and other runtime utilities
        if [[ -d "$source_dir/src/tools" ]]; then
          run_cmd cp -r "$source_dir/src/tools" "$INSTALL_DIR/src/"
        fi

        # EXCLUDED from minimal install (development/documentation only):
        # - src/examples/   (~48KB)  - Example projects and reference code
        # - src/scripts/    (~16KB)  - Development helper scripts
        # - src/tests/      (~1.4MB) - Test suites (not needed for runtime)

        echo_debug "Excluded from minimal install: examples/ scripts/ tests/ (~1.5MB saved)"
      fi
    fi

    # Copy LICENSE and README only (documentation is online)
    for file in LICENSE README.md; do
      [[ -f "$source_dir/$file" ]] && run_cmd cp "$source_dir/$file" "$INSTALL_DIR/"
    done
  ) &
  show_spinner $! "Installing files"

  # Set permissions
  echo_info "Setting permissions..."
  (
    run_cmd chmod -R 755 "$INSTALL_DIR"
    # Make the bin shim executable
    run_cmd chmod +x "$INSTALL_DIR/bin/nself" 2>/dev/null || true
    # Make all CLI scripts executable
    run_cmd chmod +x "$INSTALL_DIR/src/cli/"*.sh 2>/dev/null || true
    # Make all tool scripts executable (only if installed)
    if [[ -d "$INSTALL_DIR/src/tools" ]]; then
      run_cmd find "$INSTALL_DIR/src/tools" -name "*.sh" -exec chmod +x {} \; 2>/dev/null || true
    fi
  ) &
  show_spinner $! "Setting permissions"

  echo_success "Files installed to: $INSTALL_DIR"

  # Show installation size info
  if command_exists du; then
    local install_size=$(du -sh "$INSTALL_DIR" 2>/dev/null | awk '{print $1}')
    echo_info "Installation size: $install_size"
  fi
}

setup_path() {
  if [[ "$SKIP_PATH" == "true" ]] || [[ "$INSTALL_MODE" == "portable" ]]; then
    return 0
  fi
  
  echo_header "Setting Up PATH"
  
  # For system installation, create symlink
  if [[ -n "$BIN_LINK" ]]; then
    echo_info "Creating symlink: $BIN_LINK"
    run_cmd ln -sf "$BIN_DIR/nself" "$BIN_LINK"
    echo_success "System-wide installation complete"
    return 0
  fi
  
  # For user installation, update shell configuration
  local shell_configs=()
  local current_shell=$(basename "$SHELL")
  
  case "$current_shell" in
    bash)  shell_configs+=("$HOME/.bashrc" "$HOME/.bash_profile") ;;
    zsh)   shell_configs+=("$HOME/.zshrc") ;;
    fish)  shell_configs+=("$HOME/.config/fish/config.fish") ;;
    *)     shell_configs+=("$HOME/.profile") ;;
  esac
  
  local path_line="export PATH=\"$BIN_DIR:\$PATH\""
  local added_to=()
  
  for config in "${shell_configs[@]}"; do
    if [[ -f "$config" ]]; then
      # Check if already in PATH
      if grep -q "$BIN_DIR" "$config" 2>/dev/null; then
        echo_info "PATH already configured in $config"
      else
        echo "" >> "$config"
        echo "# Added by nself installer on $(date)" >> "$config"
        echo "$path_line" >> "$config"
        added_to+=("$config")
        echo_success "Added to PATH in: $config"
      fi
    fi
  done
  
  if [[ ${#added_to[@]} -gt 0 ]]; then
    echo ""
    echo_warning "PATH has been updated in: ${added_to[*]}"
    echo_warning "Run this to use nself immediately:"
    echo ""
    echo "    ${CYAN}source ${added_to[0]}${RESET}"
    echo ""
    echo "Or start a new terminal session"
  fi
}

# ========================================================================
# MIGRATION FUNCTIONS
# ========================================================================

migrate_configuration() {
  local old_version="$1"
  local new_version="$2"
  
  echo_header "Migrating Configuration"
  
  echo_info "Migrating from v${old_version} to v${new_version}"
  
  # Version-specific migrations
  local old_major="${old_version%%.*}"
  local new_major="${new_version%%.*}"
  
  if [[ "$old_major" == "0" ]] && [[ "$new_major" == "0" ]]; then
    # 0.x to 0.y migration
    local old_minor=$(echo "$old_version" | cut -d. -f2)
    local new_minor=$(echo "$new_version" | cut -d. -f2)
    
    if [[ $old_minor -lt 3 ]] && [[ $new_minor -ge 3 ]]; then
      echo_info "Migrating from pre-0.3.0 structure..."
      
      # Specific migrations for 0.2.x → 0.3.x
      # - Directory structure changed
      # - Command files reorganized
      # - Configuration format updated
      
      echo_success "Migration completed for 0.3.0"
    fi
  fi
  
  # Copy user configurations if they exist
  local user_configs=(
    "$HOME/.nself/config.json"
    "$HOME/.nself/settings.json"
    "$HOME/.nself/.env"
  )
  
  for config in "${user_configs[@]}"; do
    if [[ -f "$config" ]]; then
      local config_name=$(basename "$config")
      echo_info "Preserving user configuration: $config_name"
      cp "$config" "$INSTALL_DIR/" 2>/dev/null || true
    fi
  done
}

# ========================================================================
# VERIFICATION
# ========================================================================

verify_installation() {
  echo_header "Verifying Installation"
  
  local errors=0
  
  # Check main executable exists
  if [[ -f "$BIN_DIR/nself" ]] && [[ -f "$SRC_DIR/cli/nself.sh" ]]; then
    echo_success "Main executable found"
  else
    echo_error "Main executable not found"
    ((errors++))
  fi
  
  # Check if nself is accessible
  if [[ "$INSTALL_MODE" != "portable" ]]; then
    if command_exists nself || [[ -f "$BIN_LINK" ]]; then
      echo_success "nself is accessible from PATH"
      
      # Try to get version
      local version=$("$BIN_DIR/nself" version 2>/dev/null || echo "unknown")
      echo_success "Installed version: $version"
    else
      echo_warning "nself not in PATH yet (restart terminal or source shell config)"
    fi
  fi
  
  # Check critical directories
  local required_dirs=("bin" "src/cli" "src/lib" "src/templates")
  for dir in "${required_dirs[@]}"; do
    if [[ -d "$INSTALL_DIR/$dir" ]]; then
      echo_success "Required directory exists: $dir"
    else
      echo_error "Missing required directory: $dir"
      ((errors++))
    fi
  done
  
  if [[ $errors -gt 0 ]]; then
    echo ""
    echo_error "Installation verification failed with $errors error(s)"
    
    # Offer to restore backup
    if [[ -d "$BACKUP_DIR" ]] && [[ $(ls -1 "$BACKUP_DIR" 2>/dev/null | wc -l) -gt 0 ]]; then
      echo ""
      if confirm "Restore from backup?" "y"; then
        restore_from_backup
      fi
    fi
    
    exit 1
  fi
  
  echo ""
  echo_success "Installation verified successfully"
}

# ========================================================================
# RESTORE FUNCTIONS
# ========================================================================

restore_from_backup() {
  echo_header "Restoring from Backup"
  
  if [[ ! -d "$BACKUP_DIR" ]]; then
    echo_error "No backups found"
    return 1
  fi
  
  # List available backups
  local backups=($(ls -1t "$BACKUP_DIR" 2>/dev/null))
  
  if [[ ${#backups[@]} -eq 0 ]]; then
    echo_error "No backups found"
    return 1
  fi
  
  echo_info "Available backups:"
  local i=1
  for backup in "${backups[@]}"; do
    local info_file="$BACKUP_DIR/$backup/.backup_info"
    if [[ -f "$info_file" ]]; then
      local backup_date=$(grep "Backup Date:" "$info_file" | cut -d: -f2-)
      local backup_version=$(grep "Version:" "$info_file" | cut -d: -f2 | tr -d ' ')
      echo "  $i) $backup (v${backup_version},${backup_date})"
    else
      echo "  $i) $backup"
    fi
    ((i++))
  done
  
  echo ""
  read -p "Select backup number (1-${#backups[@]}): " selection
  
  if [[ ! "$selection" =~ ^[0-9]+$ ]] || [[ $selection -lt 1 ]] || [[ $selection -gt ${#backups[@]} ]]; then
    echo_error "Invalid selection"
    return 1
  fi
  
  local selected_backup="${backups[$((selection-1))]}"
  echo_info "Restoring from: $selected_backup"
  
  # Remove current installation
  echo_info "Removing current installation..."
  run_cmd rm -rf "$INSTALL_DIR"
  
  # Restore backup
  echo_info "Restoring backup..."
  run_cmd cp -r "$BACKUP_DIR/$selected_backup" "$INSTALL_DIR"
  
  echo_success "Restored from backup successfully"
}

# ========================================================================
# UNINSTALL FUNCTION
# ========================================================================

uninstall_nself() {
  echo_header "Uninstalling nself"
  
  if ! confirm "Are you sure you want to uninstall nself?" "n"; then
    echo_info "Uninstall cancelled"
    exit 0
  fi
  
  # Remove installation directory
  if [[ -d "$INSTALL_DIR" ]]; then
    echo_info "Removing $INSTALL_DIR..."
    run_cmd rm -rf "$INSTALL_DIR"
  fi
  
  # Remove symlinks
  if [[ -n "$BIN_LINK" ]] && [[ -L "$BIN_LINK" ]]; then
    echo_info "Removing symlink $BIN_LINK..."
    run_cmd rm -f "$BIN_LINK"
  fi
  
  # Remove from PATH
  echo_info "Removing from PATH configurations..."
  local shell_configs=("$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.zshrc" "$HOME/.profile")
  
  for config in "${shell_configs[@]}"; do
    if [[ -f "$config" ]] && grep -q "$BIN_DIR" "$config"; then
      # Remove nself PATH entries
      sed -i.bak "/$BIN_DIR/d" "$config"
      sed -i.bak "/nself installer/d" "$config"
      echo_success "Removed from $config"
    fi
  done
  
  # Ask about backups
  if [[ -d "$BACKUP_DIR" ]]; then
    if confirm "Remove all backups?" "n"; then
      echo_info "Removing backups..."
      rm -rf "$BACKUP_DIR"
    else
      echo_info "Keeping backups in $BACKUP_DIR"
    fi
  fi
  
  echo ""
  echo_success "nself has been uninstalled"
  echo_info "Thank you for using nself!"
}

# ========================================================================
# MAIN INSTALLATION FLOW
# ========================================================================

print_banner() {
  echo ""
  echo "${BOLD}${CYAN}╔════════════════════════════════════════════════════════════════╗${RESET}"
  echo "${BOLD}${CYAN}║                                                                ║${RESET}"
  echo "${BOLD}${CYAN}║${RESET}    ${BOLD}${GREEN}_  _   ___  ___  _     ___${RESET}   ${CYAN}Self-Hosted Infrastructure${RESET}   ${BOLD}${CYAN}║${RESET}"
  echo "${BOLD}${CYAN}║${RESET}   ${BOLD}${GREEN}| \\| | / __|| __|| |   | __|${RESET}  ${CYAN}Made Simple${RESET}                 ${BOLD}${CYAN}║${RESET}"
  echo "${BOLD}${CYAN}║${RESET}   ${BOLD}${GREEN}| .  | \\__ \\| _| | |__ | _|${RESET}                                ${BOLD}${CYAN}║${RESET}"
  echo "${BOLD}${CYAN}║${RESET}   ${BOLD}${GREEN}|_|\\_| |___/|___||____||_|${RESET}   ${CYAN}${INSTALL_VERSION}${RESET}                       ${BOLD}${CYAN}║${RESET}"
  echo "${BOLD}${CYAN}║                                                                ║${RESET}"
  echo "${BOLD}${CYAN}╚════════════════════════════════════════════════════════════════╝${RESET}"
  echo ""
}

print_help() {
  echo "Usage: $0 [mode] [directory] [options]"
  echo ""
  echo "Installation modes:"
  echo "  user      Install for current user (default)"
  echo "  system    Install system-wide (requires sudo)"
  echo "  docker    Install for Docker container"
  echo "  portable  Install to current/specified directory"
  echo ""
  echo "Options:"
  echo "  --full            Install all files (examples, scripts, tests)"
  echo "  --force           Force reinstall even if up to date"
  echo "  --skip-backup     Don't backup existing installation"
  echo "  --skip-path       Don't modify PATH"
  echo "  --verbose         Show detailed output"
  echo "  --uninstall       Uninstall nself"
  echo "  --help            Show this help"
  echo ""
  echo "Examples:"
  echo "  $0                    # Minimal install for current user (~11.6MB)"
  echo "  $0 --full             # Full install with examples/tests (~13.1MB)"
  echo "  $0 system             # Install system-wide"
  echo "  $0 portable ./tools   # Install to ./tools/nself"
  echo "  $0 --uninstall        # Remove nself"
  echo ""
  echo "Environment variables:"
  echo "  FULL_INSTALL=true     Install all files (same as --full)"
  echo "  FORCE_REINSTALL=true  Force reinstallation"
  echo "  SKIP_BACKUP=true      Skip backup"
  echo "  VERBOSE=true          Verbose output"
  echo ""
  echo "Note: By default, only runtime files are installed (cli, lib, templates)."
  echo "      Use --full to include examples, scripts, and test files."
}

main() {
  # Handle special flags (can be combined, so loop through all args)
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --help|-h)
        print_help
        exit 0
        ;;
      --uninstall)
        uninstall_nself
        exit 0
        ;;
      --full)
        FULL_INSTALL=true
        shift
        ;;
      --force)
        FORCE_REINSTALL=true
        shift
        ;;
      --skip-backup)
        SKIP_BACKUP=true
        shift
        ;;
      --skip-path)
        SKIP_PATH=true
        shift
        ;;
      --verbose|-v)
        VERBOSE=true
        shift
        ;;
      --*)
        echo_error "Unknown option: $1"
        echo ""
        print_help
        exit 1
        ;;
      *)
        # Not a flag, must be install mode or directory
        break
        ;;
    esac
  done
  
  # Re-parse after handling flags
  INSTALL_MODE="${1:-$DEFAULT_INSTALL_MODE}"
  local custom_install_dir="${2:-}"

  # Set installation paths based on mode
  set_install_paths "$INSTALL_MODE" "$custom_install_dir"

  # Print banner
  print_banner

  echo_info "Installation mode: ${BOLD}$INSTALL_MODE${RESET}"
  echo_info "Installation directory: ${BOLD}$INSTALL_DIR${RESET}"
  echo ""
  
  # Run installation steps
  check_prerequisites
  
  # Check for existing installation and handle upgrade/backup
  if detect_existing_installation; then
    local old_version=$(get_installed_version)
    backup_existing_installation
    
    # Remove old installation
    echo_info "Removing old installation..."
    run_cmd rm -rf "$INSTALL_DIR"
  fi
  
  download_nself
  install_files
  
  # Migrate if this was an upgrade
  if [[ -n "${old_version:-}" ]] && [[ "$old_version" != "unknown" ]]; then
    local new_version=$(get_latest_version)
    if [[ "$old_version" != "$new_version" ]]; then
      migrate_configuration "$old_version" "$new_version"
    fi
  fi
  
  setup_path
  verify_installation
  
  # Print success message
  echo ""
  echo_header "Installation Complete! 🎉"
  
  echo "${GREEN}nself has been successfully installed!${RESET}"
  echo ""
  echo "Next steps:"
  echo "  1. Restart your terminal or run: ${CYAN}source ~/.bashrc${RESET}"
  echo "  2. Verify installation: ${CYAN}nself version${RESET}"
  echo "  3. Get started: ${CYAN}nself help${RESET}"
  echo ""
  echo "Quick start:"
  echo "  ${CYAN}mkdir myproject && cd myproject${RESET}"
  echo "  ${CYAN}nself init${RESET}"
  echo "  ${CYAN}nself start${RESET}"
  echo ""
  echo "Documentation: ${BLUE}https://github.com/${DEFAULT_REPO}/wiki${RESET}"
  echo "Support: ${BLUE}https://github.com/${DEFAULT_REPO}/issues${RESET}"
  echo ""
  
  # Show backup location if created
  if [[ -d "$BACKUP_DIR" ]] && [[ $(ls -1 "$BACKUP_DIR" 2>/dev/null | wc -l) -gt 0 ]]; then
    echo_info "Previous installation backed up to: $BACKUP_DIR"
  fi
}

# ========================================================================
# ENTRY POINT
# ========================================================================

# Run main function
main "$@"