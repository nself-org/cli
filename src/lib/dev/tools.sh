#!/usr/bin/env bash
# tools.sh - Development tools and utilities
# Part of nself v0.7.0 - Sprint 10: DEV-003


# Initialize development environment
dev_init() {

set -euo pipefail

  echo "Setting up development environment..."

  # Create dev directories
  mkdir -p .nself/dev/{fixtures,mocks,profiles}

  # Create dev config
  cat >.nself/dev/config.json <<EOF
{
  "environment": "development",
  "debug": true,
  "hot_reload": true,
  "mock_external_apis": true,
  "performance_profiling": false
}
EOF

  echo "✓ Development environment initialized"
}

# Generate mock data
dev_generate_mocks() {
  local entity_type="$1" # users, posts, products, etc.
  local count="${2:-10}"

  case "$entity_type" in
    users)
      # Generate mock users
      for i in $(seq 1 "$count"); do
        echo "{\"id\":\"$i\",\"name\":\"User $i\",\"email\":\"user$i@example.com\"}"
      done | jq -s '.'
      ;;

    *)
      echo "[]"
      ;;
  esac
}

# Create test fixture
dev_create_fixture() {
  local name="$1"
  local data="$2"

  echo "$data" | jq '.' >".nself/dev/fixtures/$name.json"
  echo "✓ Fixture created: $name"
}

# Load fixture
dev_load_fixture() {
  local name="$1"

  if [[ -f ".nself/dev/fixtures/$name.json" ]]; then
    cat ".nself/dev/fixtures/$name.json"
  else
    echo "null"
  fi
}

# Performance profiling
dev_profile_start() {
  local profile_name="${1:-default}"

  # Record start time and initial metrics
  local start_time=$(date +%s%3N)
  local start_mem=$(ps -o rss= -p $$ 2>/dev/null || echo 0)

  local _profile_dir="${XDG_RUNTIME_DIR:-$HOME/.local/run/nself}"
  mkdir -p "$_profile_dir"
  echo "{\"name\":\"$profile_name\",\"start_time\":$start_time,\"start_mem\":$start_mem}" >"${_profile_dir}/nself_profile_active.json"

  echo "Profiling started: $profile_name"
}

# Stop profiling
dev_profile_stop() {
  local _profile_dir="${XDG_RUNTIME_DIR:-$HOME/.local/run/nself}"
  local profile_file="${_profile_dir}/nself_profile_active.json"

  if [[ ! -f "$profile_file" ]]; then
    echo "No active profile"
    return 1
  fi

  local profile=$(cat "$profile_file")
  local profile_name=$(echo "$profile" | jq -r '.name')
  local start_time=$(echo "$profile" | jq -r '.start_time')
  local start_mem=$(echo "$profile" | jq -r '.start_mem')

  local end_time=$(date +%s%3N)
  local end_mem=$(ps -o rss= -p $$ 2>/dev/null || echo 0)

  local duration=$((end_time - start_time))
  local mem_delta=$((end_mem - start_mem))

  local result=$(jq -n \
    --arg name "$profile_name" \
    --arg duration "$duration" \
    --arg mem_delta "$mem_delta" \
    '{
      name: $name,
      duration_ms: ($duration | tonumber),
      memory_delta_kb: ($mem_delta | tonumber)
    }')

  echo "$result" >".nself/dev/profiles/${profile_name}_$(date +%s).json"

  rm -f "$profile_file"

  echo "✓ Profile completed:"
  echo "$result" | jq '.'
}

# Debug mode toggle
dev_debug() {
  local action="${1:-toggle}"

  case "$action" in
    on)
      export NSELF_DEBUG=1
      echo "✓ Debug mode enabled"
      ;;

    off)
      unset NSELF_DEBUG
      echo "✓ Debug mode disabled"
      ;;

    toggle)
      if [[ -n "${NSELF_DEBUG:-}" ]]; then
        unset NSELF_DEBUG
        echo "✓ Debug mode disabled"
      else
        export NSELF_DEBUG=1
        echo "✓ Debug mode enabled"
      fi
      ;;
  esac
}

# Hot reload watcher (simplified)
dev_watch() {
  local directory="${1:-.}"

  echo "Watching $directory for changes..."

  # Simple file watcher using find + stat
  while true; do
    find "$directory" -type f -name "*.sh" -newer "/tmp/nself_watch_marker" 2>/dev/null | while read -r file; do
      echo "Changed: $file"
      # Would trigger reload here
    done

    touch "/tmp/nself_watch_marker"
    sleep 2
  done
}

# Generate API client
dev_generate_client() {
  local api_spec="$1" # OpenAPI/Swagger spec file
  local language="${2:-typescript}"
  local output_dir="${3:-./generated-client}"

  echo "Generating $language API client from $api_spec..."
  echo "✓ Client generation not implemented yet"
}

export -f dev_init dev_generate_mocks dev_create_fixture dev_load_fixture
export -f dev_profile_start dev_profile_stop dev_debug dev_watch dev_generate_client
