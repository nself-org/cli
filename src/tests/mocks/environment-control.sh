#!/usr/bin/env bash
set -uo pipefail
#
# Environment Control Mocks
# Provides infrastructure to control environment for branch testing
#

# Mock command availability
mock_command_exists() {
  local cmd="$1"
  local exists="$2"

  if [[ "$exists" == "true" ]]; then
    # Create a mock function that succeeds
    eval "${cmd}() { echo \"[MOCK] $cmd called with args: \$*\"; return 0; }"
    # Also mock command -v to return success
    if [[ "$cmd" == "timeout" ]] || [[ "$cmd" == "gtimeout" ]]; then
      eval "command() {
        if [[ \"\${2:-}\" == \"$cmd\" ]]; then
          echo \"/usr/bin/$cmd\"
          return 0
        fi
        builtin command \"\$@\"
      }"
    fi
  else
    # Create a mock function that returns "command not found"
    eval "${cmd}() { return 127; }"
    # Mock command -v to return failure
    if [[ "$cmd" == "timeout" ]] || [[ "$cmd" == "gtimeout" ]]; then
      eval "command() {
        if [[ \"\${2:-}\" == \"$cmd\" ]]; then
          return 1
        fi
        builtin command \"\$@\"
      }"
    fi
  fi
}

# Mock platform/OS
mock_platform() {
  local platform="$1"

  case "$platform" in
    macos|darwin)
      export OSTYPE="darwin22.0"
      export PLATFORM="darwin"
      ;;
    linux)
      export OSTYPE="linux-gnu"
      export PLATFORM="linux"
      ;;
    wsl)
      export OSTYPE="linux-gnu"
      export WSL_DISTRO_NAME="Ubuntu"
      export PLATFORM="linux"
      ;;
    *)
      echo "[MOCK] Unknown platform: $platform" >&2
      return 1
      ;;
  esac
}

# Mock file existence
mock_file_exists() {
  local file="$1"
  local exists="$2"
  local content="${3:-}"

  if [[ "$exists" == "true" ]]; then
    mkdir -p "$(dirname "$file")"
    if [[ -n "$content" ]]; then
      echo "$content" > "$file"
    else
      touch "$file"
    fi
  else
    rm -f "$file" 2>/dev/null || true
  fi
}

# Mock directory existence
mock_directory_exists() {
  local dir="$1"
  local exists="$2"

  if [[ "$exists" == "true" ]]; then
    mkdir -p "$dir"
  else
    rm -rf "$dir" 2>/dev/null || true
  fi
}

# Mock Docker state
mock_docker_running() {
  local running="$1"

  if [[ "$running" == "true" ]]; then
    docker() {
      case "${1:-}" in
        ps)
          echo "CONTAINER ID   IMAGE     COMMAND   CREATED   STATUS    PORTS     NAMES"
          echo "abc123         nginx     nginx     1 min     Up        80/tcp    nginx"
          return 0
          ;;
        info|version)
          echo "Docker version 24.0.0"
          return 0
          ;;
        *)
          echo "[MOCK] Docker running: $*"
          return 0
          ;;
      esac
    }

    docker-compose() {
      echo "[MOCK] Docker Compose: $*"
      return 0
    }
  else
    docker() {
      echo "Cannot connect to the Docker daemon" >&2
      return 1
    }

    docker-compose() {
      echo "Cannot connect to the Docker daemon" >&2
      return 1
    }
  fi
}

# Mock environment variable
mock_env_var() {
  local var="$1"
  local value="$2"

  export "${var}=${value}"
}

# Mock user input
mock_user_input() {
  local input="$1"

  # Override read to return predefined input
  read() {
    eval "$1='$input'"
    return 0
  }
}

# Mock network connectivity
mock_network_available() {
  local available="$1"

  if [[ "$available" == "true" ]]; then
    ping() {
      echo "PING example.com (93.184.216.34): 56 data bytes"
      echo "64 bytes from 93.184.216.34: icmp_seq=0 ttl=56 time=10.1 ms"
      return 0
    }

    curl() {
      if [[ "${*}" == *"--head"* ]] || [[ "${*}" == *"-I"* ]]; then
        echo "HTTP/1.1 200 OK"
        return 0
      else
        echo "[MOCK] curl response"
        return 0
      fi
    }
  else
    ping() {
      echo "ping: cannot resolve example.com: Unknown host" >&2
      return 1
    }

    curl() {
      echo "curl: (6) Could not resolve host" >&2
      return 6
    }
  fi
}

# Mock git repository
mock_git_repo() {
  local is_repo="$1"
  local branch="${2:-main}"

  if [[ "$is_repo" == "true" ]]; then
    git() {
      case "${1:-}" in
        rev-parse)
          if [[ "${2:-}" == "--git-dir" ]]; then
            echo ".git"
            return 0
          fi
          ;;
        branch|symbolic-ref)
          echo "$branch"
          return 0
          ;;
        status)
          echo "On branch $branch"
          echo "nothing to commit, working tree clean"
          return 0
          ;;
        *)
          echo "[MOCK] git $*"
          return 0
          ;;
      esac
    }
  else
    git() {
      echo "fatal: not a git repository" >&2
      return 128
    }
  fi
}

# Mock PostgreSQL availability
mock_postgres_available() {
  local available="$1"

  if [[ "$available" == "true" ]]; then
    psql() {
      echo "[MOCK] PostgreSQL query result"
      return 0
    }

    pg_isready() {
      echo "accepting connections"
      return 0
    }
  else
    psql() {
      echo "psql: could not connect to server" >&2
      return 2
    }

    pg_isready() {
      echo "no response"
      return 1
    }
  fi
}

# Mock Redis availability
mock_redis_available() {
  local available="$1"

  if [[ "$available" == "true" ]]; then
    redis-cli() {
      case "${1:-}" in
        ping)
          echo "PONG"
          return 0
          ;;
        get|set)
          echo "[MOCK] Redis: $*"
          return 0
          ;;
        *)
          echo "[MOCK] Redis: $*"
          return 0
          ;;
      esac
    }
  else
    redis-cli() {
      echo "Could not connect to Redis" >&2
      return 1
    }
  fi
}

# Mock disk space
mock_disk_space() {
  local available_gb="$1"
  local total_gb="${2:-100}"

  df() {
    local used_gb=$((total_gb - available_gb))
    local used_pct=$((used_gb * 100 / total_gb))

    echo "Filesystem     1G-blocks  Used Available Use% Mounted on"
    echo "/dev/disk1     ${total_gb}G      ${used_gb}G    ${available_gb}G  ${used_pct}% /"
    return 0
  }
}

# Mock process running
mock_process_running() {
  local process="$1"
  local running="$2"

  if [[ "$running" == "true" ]]; then
    pgrep() {
      if [[ "${1:-}" == "$process" ]] || [[ "${2:-}" == "$process" ]]; then
        echo "12345"
        return 0
      fi
      return 1
    }

    ps() {
      echo "PID   COMMAND"
      echo "12345 $process"
      return 0
    }
  else
    pgrep() {
      return 1
    }

    ps() {
      echo "PID   COMMAND"
      return 0
    }
  fi
}

# Mock port availability
mock_port_available() {
  local port="$1"
  local available="$2"

  if [[ "$available" == "true" ]]; then
    lsof() {
      # Port is available (no process using it)
      return 1
    }

    nc() {
      # Cannot connect (port not in use)
      return 1
    }
  else
    lsof() {
      echo "COMMAND   PID USER   FD   TYPE DEVICE SIZE/OFF NODE NAME"
      echo "nginx   12345 root    6u  IPv4  0x123      0t0  TCP *:${port} (LISTEN)"
      return 0
    }

    nc() {
      # Can connect (port in use)
      return 0
    }
  fi
}

# Mock file permissions
mock_file_permissions() {
  local file="$1"
  local perms="$2"

  # Ensure file exists
  if [[ ! -f "$file" ]]; then
    touch "$file"
  fi

  chmod "$perms" "$file"
}

# Save original environment
save_environment() {
  # Save to temp file
  local env_save="/tmp/nself-test-env-$$.save"

  # Save critical variables
  {
    echo "OSTYPE=$OSTYPE"
    echo "PATH=$PATH"
    echo "HOME=$HOME"
    echo "USER=$USER"
    echo "SHELL=$SHELL"
  } > "$env_save"

  echo "$env_save"
}

# Restore environment
restore_environment() {
  local env_save="${1:-}"

  if [[ -z "$env_save" ]] || [[ ! -f "$env_save" ]]; then
    echo "[WARN] Cannot restore environment - save file not found" >&2
    return 1
  fi

  # Source the saved environment
  while IFS='=' read -r key value; do
    export "${key}=${value}"
  done < "$env_save"

  # Clean up
  rm -f "$env_save"
}

# Setup test environment
setup_test_environment() {
  local env_type="${1:-minimal}"

  case "$env_type" in
    ci)
      # Minimal CI environment
      mock_platform "linux"
      mock_command_exists "timeout" "false"
      mock_command_exists "docker" "true"
      mock_command_exists "docker-compose" "true"
      ;;

    macos)
      # macOS environment
      mock_platform "macos"
      mock_command_exists "timeout" "false"
      mock_command_exists "gtimeout" "true"
      mock_command_exists "docker" "true"
      ;;

    linux)
      # Full Linux environment
      mock_platform "linux"
      mock_command_exists "timeout" "true"
      mock_command_exists "docker" "true"
      mock_command_exists "docker-compose" "true"
      ;;

    minimal)
      # Bare minimum environment
      mock_platform "linux"
      mock_command_exists "docker" "false"
      mock_command_exists "timeout" "false"
      ;;

    *)
      echo "[ERROR] Unknown environment type: $env_type" >&2
      return 1
      ;;
  esac
}

# Check if we're in a mocked environment
is_mocked_environment() {
  # Check if any mock functions are defined
  if declare -f docker >/dev/null 2>&1; then
    # Check if it's our mock
    if declare -f docker | grep -q "\[MOCK\]"; then
      return 0
    fi
  fi
  return 1
}

# Clean up all mocks
cleanup_mocks() {
  # Unset mock functions
  local mocks=(
    "docker"
    "docker-compose"
    "git"
    "psql"
    "pg_isready"
    "redis-cli"
    "df"
    "pgrep"
    "ps"
    "lsof"
    "nc"
    "ping"
    "curl"
    "read"
  )

  for mock in "${mocks[@]}"; do
    unset -f "$mock" 2>/dev/null || true
  done
}

# Export functions
export -f mock_command_exists
export -f mock_platform
export -f mock_file_exists
export -f mock_directory_exists
export -f mock_docker_running
export -f mock_env_var
export -f mock_user_input
export -f mock_network_available
export -f mock_git_repo
export -f mock_postgres_available
export -f mock_redis_available
export -f mock_disk_space
export -f mock_process_running
export -f mock_port_available
export -f mock_file_permissions
export -f save_environment
export -f restore_environment
export -f setup_test_environment
export -f is_mocked_environment
export -f cleanup_mocks
