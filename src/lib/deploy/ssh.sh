#!/usr/bin/env bash

# ssh.sh - SSH connection management for deployments
# POSIX-compliant, no Bash 4+ features

# Get the directory where this script is located
DEPLOY_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

LIB_ROOT="$(dirname "$DEPLOY_LIB_DIR")"

# Source dependencies
source "$LIB_ROOT/utils/display.sh" 2>/dev/null || true
source "$LIB_ROOT/utils/platform-compat.sh" 2>/dev/null || true

# SSH connection timeout (seconds)
SSH_CONNECT_TIMEOUT="${SSH_CONNECT_TIMEOUT:-10}"
SSH_COMMAND_TIMEOUT="${SSH_COMMAND_TIMEOUT:-300}"

# Test SSH connection to a server
# Returns 0 on success, 1 on failure
# On failure, prints actionable error messages to stderr
ssh::test_connection() {
  local host="$1"
  local user="${2:-root}"
  local port="${3:-22}"
  local key_file="${4:-}"
  local timeout="${5:-$SSH_CONNECT_TIMEOUT}"

  # Build SSH options
  local ssh_opts="-o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=$timeout"

  if [[ -n "$key_file" ]]; then
    # Expand tilde
    local expanded_key="${key_file/#\~/$HOME}"
    if [[ ! -f "$expanded_key" ]]; then
      log_error "SSH key file not found: $key_file"
      printf "  Tried key: %s\n" "$key_file" >&2
      printf "  Specify a valid key with: --key /path/to/private_key\n" >&2
      return 1
    fi
    ssh_opts="$ssh_opts -i $expanded_key"
  fi

  # Test connection (capture stderr for diagnostics)
  local ssh_err_file=""
  ssh_err_file=$(mktemp 2>/dev/null || mktemp -t nself_ssh_err)
  if ssh $ssh_opts -p "$port" "${user}@${host}" "echo 'connection_ok'" 2>"$ssh_err_file" | grep -q "connection_ok"; then
    rm -f "$ssh_err_file"
    return 0
  else
    local ssh_err=""
    ssh_err=$(cat "$ssh_err_file" 2>/dev/null)
    rm -f "$ssh_err_file"

    # Provide actionable guidance based on the error
    local ssh_err_lower=""
    ssh_err_lower=$(printf "%s" "$ssh_err" | tr '[:upper:]' '[:lower:]')
    case "$ssh_err_lower" in
      *"permission denied"*|*"publickey"*|*"no more authentication"*)
        log_error "SSH authentication failed for ${user}@${host}:${port}"
        if [[ -n "$key_file" ]]; then
          printf "  Key tried: %s\n" "$key_file" >&2
        else
          printf "  No key specified (using SSH agent/defaults)\n" >&2
        fi
        printf "  Fix: ssh-copy-id -i <key> -p %s %s@%s\n" "$port" "$user" "$host" >&2
        printf "  Or:  nself deploy server init %s --key /path/to/key\n" "$host" >&2
        ;;
      *"connection refused"*)
        log_error "SSH connection refused at ${host}:${port}"
        printf "  Ensure SSH is running and port %s is open\n" "$port" >&2
        ;;
      *"timed out"*|*"no route"*)
        log_error "SSH connection timed out for ${host}:${port}"
        printf "  Verify the server is running and the address is correct\n" >&2
        ;;
      *)
        log_error "SSH connection failed for ${user}@${host}:${port}"
        if [[ -n "$ssh_err" ]]; then
          printf "  SSH error: %s\n" "$ssh_err" >&2
        fi
        ;;
    esac
    return 1
  fi
}

# Execute command on remote server
ssh::exec() {
  local host="$1"
  local command="$2"
  local user="${3:-root}"
  local port="${4:-22}"
  local key_file="${5:-}"

  # Build SSH options
  local ssh_opts="-o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=$SSH_CONNECT_TIMEOUT"

  if [[ -n "$key_file" ]]; then
    local expanded_key="${key_file/#\~/$HOME}"
    ssh_opts="$ssh_opts -i $expanded_key"
  fi

  # Execute command
  ssh $ssh_opts -p "$port" "${user}@${host}" "$command"
}

# Execute command with timeout
ssh::exec_with_timeout() {
  local host="$1"
  local command="$2"
  local timeout="${3:-$SSH_COMMAND_TIMEOUT}"
  local user="${4:-root}"
  local port="${5:-22}"
  local key_file="${6:-}"

  # Build SSH options
  local ssh_opts="-o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=$SSH_CONNECT_TIMEOUT"

  if [[ -n "$key_file" ]]; then
    local expanded_key="${key_file/#\~/$HOME}"
    ssh_opts="$ssh_opts -i $expanded_key"
  fi

  # Execute with timeout
  if command -v timeout >/dev/null 2>&1; then
    timeout "$timeout" ssh $ssh_opts -p "$port" "${user}@${host}" "$command"
  elif command -v gtimeout >/dev/null 2>&1; then
    gtimeout "$timeout" ssh $ssh_opts -p "$port" "${user}@${host}" "$command"
  else
    # No timeout available, run without
    ssh $ssh_opts -p "$port" "${user}@${host}" "$command"
  fi
}

# Copy file to remote server
ssh::copy_to() {
  local local_path="$1"
  local remote_path="$2"
  local host="$3"
  local user="${4:-root}"
  local port="${5:-22}"
  local key_file="${6:-}"

  # Build SCP options
  local scp_opts="-o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=$SSH_CONNECT_TIMEOUT"

  if [[ -n "$key_file" ]]; then
    local expanded_key="${key_file/#\~/$HOME}"
    scp_opts="$scp_opts -i $expanded_key"
  fi

  # Copy file
  scp $scp_opts -P "$port" "$local_path" "${user}@${host}:${remote_path}"
}

# Copy file from remote server
ssh::copy_from() {
  local remote_path="$1"
  local local_path="$2"
  local host="$3"
  local user="${4:-root}"
  local port="${5:-22}"
  local key_file="${6:-}"

  # Build SCP options
  local scp_opts="-o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=$SSH_CONNECT_TIMEOUT"

  if [[ -n "$key_file" ]]; then
    local expanded_key="${key_file/#\~/$HOME}"
    scp_opts="$scp_opts -i $expanded_key"
  fi

  # Copy file
  scp $scp_opts -P "$port" "${user}@${host}:${remote_path}" "$local_path"
}

# Sync directory to remote using rsync
ssh::rsync_to() {
  local local_path="$1"
  local remote_path="$2"
  local host="$3"
  local user="${4:-root}"
  local port="${5:-22}"
  local key_file="${6:-}"
  local exclude="${7:-}"

  # Check rsync availability
  if ! command -v rsync >/dev/null 2>&1; then
    log_error "rsync is not installed"
    return 1
  fi

  # SECURITY: Build rsync command using arrays instead of eval to prevent injection
  local rsync_args=("-avz" "--delete")

  # Build SSH command for rsync
  local ssh_cmd="ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=$SSH_CONNECT_TIMEOUT -p $port"

  if [[ -n "$key_file" ]]; then
    local expanded_key="${key_file/#\~/$HOME}"
    ssh_cmd="$ssh_cmd -i $expanded_key"
  fi

  # Add excludes safely using array
  if [[ -n "$exclude" ]]; then
    # Split by comma
    local IFS=','
    for pattern in $exclude; do
      # Validate exclude pattern - reject suspicious characters
      case "$pattern" in
        *\;*|*\&*|*\|*|*\`*|*\$\(*)
          log_warning "Skipping suspicious exclude pattern: $pattern"
          continue
          ;;
      esac
      rsync_args+=("--exclude=$pattern")
    done
  fi

  # Default excludes for nself projects
  rsync_args+=("--exclude=.git" "--exclude=node_modules" "--exclude=.env.local" "--exclude=.env.secrets")

  # Execute rsync directly using arrays (no eval needed)
  rsync "${rsync_args[@]}" -e "$ssh_cmd" "$local_path" "${user}@${host}:${remote_path}"
}

# Get server info
ssh::get_server_info() {
  local host="$1"
  local user="${2:-root}"
  local port="${3:-22}"
  local key_file="${4:-}"

  local info_script='
    echo "hostname=$(hostname)"
    echo "os=$(cat /etc/os-release 2>/dev/null | grep "^ID=" | cut -d= -f2 | tr -d \")"
    echo "os_version=$(cat /etc/os-release 2>/dev/null | grep "^VERSION_ID=" | cut -d= -f2 | tr -d \")"
    echo "kernel=$(uname -r)"
    echo "arch=$(uname -m)"
    echo "cpu_cores=$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 1)"
    echo "memory_mb=$(free -m 2>/dev/null | awk "/^Mem:/ {print \$2}" || echo 0)"
    echo "disk_gb=$(df -BG / 2>/dev/null | awk "NR==2 {print \$4}" | tr -d G || echo 0)"
    echo "docker_version=$(docker --version 2>/dev/null | cut -d" " -f3 | tr -d , || echo "not installed")"
    echo "docker_compose_version=$(docker compose version 2>/dev/null | cut -d" " -f4 || docker-compose --version 2>/dev/null | cut -d" " -f3 | tr -d , || echo "not installed")"
  '

  ssh::exec "$host" "$info_script" "$user" "$port" "$key_file"
}

# Check if Docker is installed and running on remote
ssh::check_docker() {
  local host="$1"
  local user="${2:-root}"
  local port="${3:-22}"
  local key_file="${4:-}"

  local check_script='
    if command -v docker >/dev/null 2>&1; then
      if docker info >/dev/null 2>&1; then
        echo "docker_ok"
      else
        echo "docker_not_running"
      fi
    else
      echo "docker_not_installed"
    fi
  '

  local result
  result=$(ssh::exec "$host" "$check_script" "$user" "$port" "$key_file" 2>/dev/null)

  case "$result" in
    docker_ok)
      return 0
      ;;
    docker_not_running)
      log_error "Docker is installed but not running on $host"
      return 2
      ;;
    docker_not_installed)
      log_error "Docker is not installed on $host"
      return 3
      ;;
    *)
      log_error "Failed to check Docker status on $host"
      return 1
      ;;
  esac
}

# Install Docker on remote server (if needed)
ssh::install_docker() {
  local host="$1"
  local user="${2:-root}"
  local port="${3:-22}"
  local key_file="${4:-}"

  log_info "Installing Docker on $host..."

  local install_script='
    # Detect OS
    if [ -f /etc/debian_version ]; then
      # Debian/Ubuntu
      apt-get update
      apt-get install -y docker.io docker-compose-plugin
      systemctl enable docker
      systemctl start docker
    elif [ -f /etc/redhat-release ]; then
      # RHEL/CentOS/Fedora
      yum install -y docker docker-compose-plugin
      systemctl enable docker
      systemctl start docker
    else
      echo "Unsupported OS"
      exit 1
    fi

    # Verify installation
    docker --version
    docker compose version
  '

  ssh::exec "$host" "$install_script" "$user" "$port" "$key_file"
}

# Create deployment directory on remote
ssh::create_deploy_dir() {
  local host="$1"
  local deploy_path="$2"
  local user="${3:-root}"
  local port="${4:-22}"
  local key_file="${5:-}"

  ssh::exec "$host" "mkdir -p '$deploy_path'" "$user" "$port" "$key_file"
}

# Check if path exists on remote
ssh::path_exists() {
  local host="$1"
  local path="$2"
  local user="${3:-root}"
  local port="${4:-22}"
  local key_file="${5:-}"

  ssh::exec "$host" "test -e '$path' && echo 'exists'" "$user" "$port" "$key_file" 2>/dev/null | grep -q "exists"
}

# Get connection string for display
ssh::connection_string() {
  local host="$1"
  local user="${2:-root}"
  local port="${3:-22}"

  if [[ "$port" == "22" ]]; then
    printf "%s@%s" "$user" "$host"
  else
    printf "%s@%s:%s" "$user" "$host" "$port"
  fi
}

# Export functions
export -f ssh::test_connection
export -f ssh::exec
export -f ssh::exec_with_timeout
export -f ssh::copy_to
export -f ssh::copy_from
export -f ssh::rsync_to
export -f ssh::get_server_info
export -f ssh::check_docker
export -f ssh::install_docker
export -f ssh::create_deploy_dir
export -f ssh::path_exists
export -f ssh::connection_string
