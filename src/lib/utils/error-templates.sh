#!/bin/bash


source "$(dirname "${BASH_SOURCE[0]}")/display.sh"

set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/output-formatter.sh"

# Professional error message templates for common issues

show_docker_not_running_error() {
  local platform="${1:-$PLATFORM}"

  format_error "Docker is not running" ""

  printf "\n%s\n" "${BOLD}Quick Fix:${RESET}"

  case "$platform" in
    macos)
      printf "  %s\n" "${BLUE}→${RESET} Open Docker Desktop: ${BOLD}open -a Docker${RESET}"
      printf "  %s\n" "${BLUE}→${RESET} Or from Spotlight: Press ${BOLD}⌘ + Space${RESET} and type 'Docker'"
      printf "\n%s\n" "${DIM}If Docker Desktop is not installed:${RESET}"
      printf "  %s\n" "${BLUE}→${RESET} Download from: ${UNDERLINE}https://docker.com/products/docker-desktop${RESET}"
      printf "  %s\n" "${BLUE}→${RESET} Or install via Homebrew: ${BOLD}brew install --cask docker${RESET}"
      ;;
    linux)
      printf "  %s\n" "${BLUE}→${RESET} Start Docker service: ${BOLD}sudo systemctl start docker${RESET}"
      printf "  %s\n" "${BLUE}→${RESET} Enable on boot: ${BOLD}sudo systemctl enable docker${RESET}"
      printf "\n%s\n" "${DIM}If Docker is not installed:${RESET}"
      printf "  %s\n" "${BLUE}→${RESET} Install: ${BOLD}curl -fsSL https://get.docker.com | sh${RESET}"
      printf "  %s\n" "${BLUE}→${RESET} Add user to group: ${BOLD}sudo usermod -aG docker \$USER${RESET}"
      ;;
    windows)
      printf "  %s\n" "${BLUE}→${RESET} Open Docker Desktop from Start Menu"
      printf "  %s\n" "${BLUE}→${RESET} Or run: ${BOLD}\"C:\\Program Files\\Docker\\Docker\\Docker Desktop.exe\"${RESET}"
      printf "\n%s\n" "${DIM}If Docker Desktop is not installed:${RESET}"
      printf "  %s\n" "${BLUE}→${RESET} Download from: ${UNDERLINE}https://docker.com/products/docker-desktop${RESET}"
      printf "  %s\n" "${BLUE}→${RESET} Or via winget: ${BOLD}winget install Docker.DockerDesktop${RESET}"
      ;;
  esac

  printf "\n%s\n" "${YELLOW}⚡ Pro Tip:${RESET} After starting Docker, wait 10-15 seconds for it to fully initialize."
}

show_port_conflict_error() {
  local port="$1"
  local service="$2"

  format_error "Port $port is already in use" ""

  printf "\n%s\n" "${BOLD}Conflicting Service:${RESET}"

  # Try to identify what's using the port
  local process_info=""
  if [[ "$PLATFORM" == "macos" ]]; then
    process_info=$(lsof -i :$port -P 2>/dev/null | grep LISTEN | head -1)
  elif [[ "$PLATFORM" == "linux" ]]; then
    process_info=$(ss -tlnp 2>/dev/null | grep ":$port" | head -1)
  fi

  if [[ -n "$process_info" ]]; then
    printf "  %s\n" "${RED}✗${RESET} $process_info"
  fi

  printf "\n%s\n" "${BOLD}Solutions:${RESET}"
  printf "  %s\n" "${BLUE}1.${RESET} Stop the conflicting service:"

  case "$port" in
    5432)
      printf "     %s\n" "${DIM}# PostgreSQL${RESET}"
      [[ "$PLATFORM" == "macos" ]] && printf "     %s\n" "${BOLD}brew services stop postgresql${RESET}"
      [[ "$PLATFORM" == "linux" ]] && printf "     %s\n" "${BOLD}sudo systemctl stop postgresql${RESET}"
      ;;
    6379)
      printf "     %s\n" "${DIM}# Redis${RESET}"
      [[ "$PLATFORM" == "macos" ]] && printf "     %s\n" "${BOLD}brew services stop redis${RESET}"
      [[ "$PLATFORM" == "linux" ]] && printf "     %s\n" "${BOLD}sudo systemctl stop redis${RESET}"
      ;;
    8080)
      printf "     %s\n" "${DIM}# Common web services${RESET}"
      printf "     %s\n" "${BOLD}lsof -ti:$port | xargs kill -9${RESET}"
      ;;
    3000 | 3001)
      printf "     %s\n" "${DIM}# Node.js applications${RESET}"
      printf "     %s\n" "${BOLD}npx kill-port $port${RESET}"
      ;;
  esac

  printf "\n  %s\n" "${BLUE}2.${RESET} Or change the port in your ${BOLD}.env.local${RESET}:"
  printf "     %s\n" "${DIM}# Add to .env.local:${RESET}"

  case "$service" in
    postgres)
      printf "     %s\n" "${BOLD}POSTGRES_PORT=$((port + 1000))${RESET}"
      ;;
    redis)
      printf "     %s\n" "${BOLD}REDIS_PORT=$((port + 1000))${RESET}"
      ;;
    hasura)
      printf "     %s\n" "${BOLD}HASURA_PORT=$((port + 1000))${RESET}"
      ;;
    dashboard)
      printf "     %s\n" "${BOLD}DASHBOARD_PORT=$((port + 1000))${RESET}"
      ;;
  esac

  printf "\n%s\n" "${YELLOW}⚡ Pro Tip:${RESET} Use 'lsof -i :$port' to see what's using the port."
}

show_memory_warning() {
  local available_gb="$1"
  local required_gb="${2:-4}"

  format_warning "Low system memory: ${available_gb}GB available (${required_gb}GB recommended)" ""

  printf "\n%s\n" "${BOLD}Impact:${RESET}"
  printf "  %s\n" "${YELLOW}⚠${RESET} Services may run slowly"
  printf "  %s\n" "${YELLOW}⚠${RESET} Builds might fail with out-of-memory errors"
  printf "  %s\n" "${YELLOW}⚠${RESET} Database operations could timeout"

  printf "\n%s\n" "${BOLD}Solutions:${RESET}"

  case "$PLATFORM" in
    macos)
      printf "  %s\n" "${BLUE}→${RESET} Close unnecessary applications"
      printf "  %s\n" "${BLUE}→${RESET} Increase Docker Desktop memory allocation:"
      printf "     %s\n" "${DIM}Docker Desktop → Settings → Resources → Memory${RESET}"
      ;;
    linux)
      printf "  %s\n" "${BLUE}→${RESET} Check memory usage: ${BOLD}free -h${RESET}"
      printf "  %s\n" "${BLUE}→${RESET} Clear cache: ${BOLD}sudo sh -c 'echo 3 > /proc/sys/vm/drop_caches'${RESET}"
      printf "  %s\n" "${BLUE}→${RESET} Add swap space if needed"
      ;;
    windows)
      printf "  %s\n" "${BLUE}→${RESET} Close unnecessary applications"
      printf "  %s\n" "${BLUE}→${RESET} Increase Docker Desktop memory in Settings"
      printf "  %s\n" "${BLUE}→${RESET} Consider WSL2 memory configuration"
      ;;
  esac

  printf "\n%s\n" "${YELLOW}⚡ Pro Tip:${RESET} You can run with reduced services for development:"
  printf "  %s\n" "${BOLD}REDIS_ENABLED=false BULLMQ_ENABLED=false nself start${RESET}"
}

show_disk_space_error() {
  local available_gb="$1"
  local required_gb="${2:-10}"

  format_error "Insufficient disk space: ${available_gb}GB available (${required_gb}GB required)" ""

  printf "\n%s\n" "${BOLD}Free up space:${RESET}"
  printf "  %s\n" "${BLUE}1.${RESET} Clean Docker resources:"
  printf "     %s\n" "${BOLD}docker system prune -a --volumes${RESET}"
  printf "     %s\n" "${DIM}This will remove all unused containers, images, and volumes${RESET}"

  printf "\n  %s\n" "${BLUE}2.${RESET} Remove old nself builds:"
  printf "     %s\n" "${BOLD}rm -rf ./generated ./node_modules ./services/*/node_modules${RESET}"

  case "$PLATFORM" in
    macos)
      printf "\n  %s\n" "${BLUE}3.${RESET} macOS specific cleanup:"
      printf "     %s\n" "${BOLD}rm -rf ~/Library/Caches/*${RESET}"
      printf "     %s\n" "${BOLD}brew cleanup${RESET}"
      ;;
    linux)
      printf "\n  %s\n" "${BLUE}3.${RESET} Linux specific cleanup:"
      printf "     %s\n" "${BOLD}sudo apt-get clean${RESET} ${DIM}(or yum/dnf clean all)${RESET}"
      printf "     %s\n" "${BOLD}journalctl --vacuum-size=100M${RESET}"
      ;;
    windows)
      printf "\n  %s\n" "${BLUE}3.${RESET} Windows specific cleanup:"
      printf "     %s\n" "Run Disk Cleanup utility"
      printf "     %s\n" "${BOLD}cleanmgr /sageset:1${RESET}"
      ;;
  esac

  printf "\n%s\n" "${YELLOW}⚡ Pro Tip:${RESET} Check disk usage with: ${BOLD}df -h${RESET} (Unix) or ${BOLD}dir${RESET} (Windows)"
}

show_permission_error() {
  local path="$1"

  format_error "Permission denied: $path" ""

  printf "\n%s\n" "${BOLD}Solutions:${RESET}"

  case "$PLATFORM" in
    macos | linux)
      printf "  %s\n" "${BLUE}1.${RESET} Fix ownership:"
      printf "     %s\n" "${BOLD}sudo chown -R \$(whoami) $path${RESET}"
      printf "\n  %s\n" "${BLUE}2.${RESET} Fix permissions:"
      printf "     %s\n" "${BOLD}chmod -R 755 $path${RESET}"

      if [[ "$path" =~ docker ]]; then
        printf "\n  %s\n" "${BLUE}3.${RESET} Add user to docker group:"
        printf "     %s\n" "${BOLD}sudo usermod -aG docker \$USER${RESET}"
        printf "     %s\n" "${DIM}Then log out and back in${RESET}"
      fi
      ;;
    windows)
      printf "  %s\n" "${BLUE}1.${RESET} Run as Administrator"
      printf "  %s\n" "${BLUE}2.${RESET} Check file properties → Security tab"
      printf "  %s\n" "${BLUE}3.${RESET} For WSL issues, check Windows Defender settings"
      ;;
  esac

  printf "\n%s\n" "${YELLOW}⚡ Pro Tip:${RESET} Always run 'nself init' in a directory you own."
}

show_network_error() {
  local service="$1"
  local url="${2:-}"

  format_error "Network connection failed: $service" ""

  printf "\n%s\n" "${BOLD}Check:${RESET}"
  printf "  %s\n" "${BLUE}✓${RESET} Internet connection: ${BOLD}ping -c 1 google.com${RESET}"
  printf "  %s\n" "${BLUE}✓${RESET} DNS resolution: ${BOLD}nslookup $service${RESET}"
  printf "  %s\n" "${BLUE}✓${RESET} Proxy settings: ${BOLD}echo \$HTTP_PROXY${RESET}"

  if [[ -n "$url" ]]; then
    printf "  %s\n" "${BLUE}✓${RESET} Service status: ${BOLD}curl -I $url${RESET}"
  fi

  printf "\n%s\n" "${BOLD}Common fixes:${RESET}"
  printf "  %s\n" "${BLUE}→${RESET} Behind corporate proxy? Set proxy environment variables"
  printf "  %s\n" "${BLUE}→${RESET} Using VPN? Try disconnecting temporarily"
  printf "  %s\n" "${BLUE}→${RESET} Firewall blocking? Check firewall rules"

  printf "\n%s\n" "${YELLOW}⚡ Pro Tip:${RESET} For Docker Hub issues, try: ${BOLD}docker pull hello-world${RESET}"
}

show_dependency_missing_error() {
  local dep="$1"

  format_error "Required dependency not found: $dep" ""

  printf "\n%s\n" "${BOLD}Install $dep:${RESET}"

  case "$dep" in
    node | nodejs)
      printf "  %s\n" "${BLUE}→${RESET} Via nvm: ${BOLD}curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.0/install.sh | bash${RESET}"
      printf "  %s\n" "${BLUE}→${RESET} Then: ${BOLD}nvm install --lts${RESET}"
      [[ "$PKG_MANAGER" == "brew" ]] && printf "  %s\n" "${BLUE}→${RESET} Or via Homebrew: ${BOLD}brew install node${RESET}"
      [[ "$PKG_MANAGER" == "apt" ]] && printf "  %s\n" "${BLUE}→${RESET} Or via apt: ${BOLD}sudo apt-get install nodejs npm${RESET}"
      ;;
    go | golang)
      printf "  %s\n" "${BLUE}→${RESET} Download: ${UNDERLINE}https://go.dev/dl/${RESET}"
      [[ "$PKG_MANAGER" == "brew" ]] && printf "  %s\n" "${BLUE}→${RESET} Or via Homebrew: ${BOLD}brew install go${RESET}"
      [[ "$PKG_MANAGER" == "apt" ]] && printf "  %s\n" "${BLUE}→${RESET} Or via apt: ${BOLD}sudo apt-get install golang${RESET}"
      ;;
    python | python3)
      [[ "$PKG_MANAGER" == "brew" ]] && printf "  %s\n" "${BLUE}→${RESET} Via Homebrew: ${BOLD}brew install python3${RESET}"
      [[ "$PKG_MANAGER" == "apt" ]] && printf "  %s\n" "${BLUE}→${RESET} Via apt: ${BOLD}sudo apt-get install python3 python3-pip${RESET}"
      printf "  %s\n" "${BLUE}→${RESET} Or via pyenv for version management"
      ;;
    git)
      [[ "$PKG_MANAGER" == "brew" ]] && printf "  %s\n" "${BLUE}→${RESET} Via Homebrew: ${BOLD}brew install git${RESET}"
      [[ "$PKG_MANAGER" == "apt" ]] && printf "  %s\n" "${BLUE}→${RESET} Via apt: ${BOLD}sudo apt-get install git${RESET}"
      [[ "$PLATFORM" == "windows" ]] && printf "  %s\n" "${BLUE}→${RESET} Download: ${UNDERLINE}https://git-scm.com/download/win${RESET}"
      ;;
  esac

  printf "\n%s\n" "${YELLOW}⚡ Pro Tip:${RESET} Dependencies will run in Docker even if not installed locally."
}

show_success_banner() {
  local message="$1"

  echo
  printf "%s\n" "${GREEN}╔══════════════════════════════════════════════════════════╗${RESET}"
  printf "%s\n" "${GREEN}║${RESET}                                                          ${GREEN}║${RESET}"
  printf "%s\n" "${GREEN}║${RESET}   ${GREEN}✨ SUCCESS ✨${RESET}                                        ${GREEN}║${RESET}"
  printf "${GREEN}║${RESET}   %-54s${GREEN}║${RESET}\n" "$message"
  printf "%s\n" "${GREEN}║${RESET}                                                          ${GREEN}║${RESET}"
  printf "%s\n" "${GREEN}╚══════════════════════════════════════════════════════════╝${RESET}"
  echo
}

show_welcome_message() {
  echo
  printf "%s\n" "${BLUE}╔══════════════════════════════════════════════════════════╗${RESET}"
  printf "%s\n" "${BLUE}║${RESET}                                                          ${BLUE}║${RESET}"
  printf "%s\n" "${BLUE}║${RESET}   ${BOLD}Welcome to nself${RESET} - Modern Full-Stack Platform       ${BLUE}║${RESET}"
  printf "%s\n" "${BLUE}║${RESET}                                                          ${BLUE}║${RESET}"
  printf "%s\n" "${BLUE}║${RESET}   ${DIM}Build production-ready applications with ease${RESET}       ${BLUE}║${RESET}"
  printf "%s\n" "${BLUE}║${RESET}                                                          ${BLUE}║${RESET}"
  printf "%s\n" "${BLUE}╚══════════════════════════════════════════════════════════╝${RESET}"
  echo
}

export -f show_docker_not_running_error
export -f show_port_conflict_error
export -f show_memory_warning
export -f show_disk_space_error
export -f show_permission_error
export -f show_network_error
export -f show_dependency_missing_error
export -f show_success_banner
export -f show_welcome_message
