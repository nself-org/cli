#!/usr/bin/env bash

# Auto-fix health check issues in docker-compose.yml

fix_healthchecks() {

set -euo pipefail

  local compose_file="${1:-docker-compose.yml}"
  local fixed=false

  if [[ ! -f "$compose_file" ]]; then
    return 0
  fi

  # Create backup in _backup/timestamp structure (only in debug mode)
  if [[ "${DEBUG:-false}" == "true" ]]; then
    local timestamp="$(date +%Y%m%d_%H%M%S)"
    local backup_dir="_backup/${timestamp}"
    mkdir -p "$backup_dir"
    cp "$compose_file" "${backup_dir}/$(basename "$compose_file").healthcheck-backup"
  fi

  # Fix auth service health check (ensure correct endpoint and port)
  if grep -q 'container_name:.*_auth' "$compose_file"; then
    # Fix endpoint - auth uses /healthz not /version
    if grep -q 'http://localhost:4000/version' "$compose_file"; then
      sed -i '' 's|http://localhost:4000/version|http://localhost:4000/healthz|g' "$compose_file"
      fixed=true
    fi
    # Fix if wrong port is used
    if grep -q 'http://localhost:4001/' "$compose_file"; then
      sed -i '' 's|http://localhost:4001/|http://localhost:4000/|g' "$compose_file"
      fixed=true
    fi

    # Fix curl command for auth (nhost/hasura-auth has wget)
    if grep -A 5 'container_name:.*_auth' "$compose_file" | grep -q '"CMD", "curl"'; then
      # Find auth service block and fix its health check
      awk '
        /container_name:.*_auth/ { in_auth=1 }
        in_auth && /healthcheck:/ { in_healthcheck=1 }
        in_healthcheck && /"CMD", "curl"/ {
          gsub(/"CMD", "curl", "-f"/, "\"CMD\", \"wget\", \"--no-verbose\", \"--tries=1\", \"--spider\"")
        }
        in_healthcheck && /retries:/ { in_healthcheck=0 }
        { print }
      ' "$compose_file" >"${compose_file}.tmp" && mv "${compose_file}.tmp" "$compose_file"
      fixed=true
    fi
  fi

  # Fix MLflow health check (curl -> python urllib)
  if grep -q 'container_name:.*_mlflow' "$compose_file"; then
    if grep -A 5 'container_name:.*_mlflow' "$compose_file" | grep -q '"CMD", "curl"'; then
      # Replace curl with python for MLflow
      awk '
        /container_name:.*_mlflow/ { in_mlflow=1 }
        in_mlflow && /healthcheck:/ { in_healthcheck=1 }
        in_healthcheck && /test:.*curl.*http:\/\/localhost:5000/ {
          print "      test: [\"CMD\", \"python\", \"-c\", \"import urllib.request; urllib.request.urlopen('"'"'http://localhost:5000/health'"'"').read()\"]"
          next
        }
        in_healthcheck && /retries:/ { in_healthcheck=0; in_mlflow=0 }
        { print }
      ' "$compose_file" >"${compose_file}.tmp" && mv "${compose_file}.tmp" "$compose_file"
      fixed=true
    fi
  fi

  # Fix custom service health checks (curl -> wget for Node Alpine)
  # This handles service_12, service_23, and any other custom services
  if grep -q 'container_name:.*_service_' "$compose_file"; then
    # Replace curl with wget for all custom services
    awk '
      /container_name:.*_service_[0-9]+/ { in_service=1 }
      in_service && /healthcheck:/ { in_healthcheck=1 }
      in_healthcheck && /"CMD", "curl", "-f"/ {
        gsub(/"CMD", "curl", "-f"/, "\"CMD\", \"wget\", \"--no-verbose\", \"--tries=1\", \"--spider\"")
      }
      in_healthcheck && /retries:/ { in_healthcheck=0; in_service=0 }
      { print }
    ' "$compose_file" >"${compose_file}.tmp" && mv "${compose_file}.tmp" "$compose_file"
    fixed=true
  fi

  # Fix any remaining Node.js services using curl
  # Look for Node images and fix their health checks
  awk '
    /image:.*node.*alpine/ { in_node=1; service_found=1 }
    /image:.*node:/ { in_node=1; service_found=1 }
    service_found && /container_name:/ { container=$0 }
    in_node && /healthcheck:/ { in_healthcheck=1 }
    in_healthcheck && /"CMD", "curl"/ {
      gsub(/"CMD", "curl", "-f"/, "\"CMD\", \"wget\", \"--no-verbose\", \"--tries=1\", \"--spider\"")
    }
    in_healthcheck && /retries:/ { in_healthcheck=0; in_node=0; service_found=0 }
    /^  [a-z_]+:$/ && !/^    / { in_node=0; service_found=0; in_healthcheck=0 }
    { print }
  ' "$compose_file" >"${compose_file}.tmp" && mv "${compose_file}.tmp" "$compose_file"

  # Remove duplicate start_period entries and ensure only one exists
  awk '
    BEGIN { in_service=0; in_healthcheck=0; seen_start_period=0 }
    /^  [a-z_]+:$/ && !/^    / { 
      in_service=1
      in_healthcheck=0
      seen_start_period=0
      print
      next
    }
    in_service && /healthcheck:/ { 
      in_healthcheck=1
      seen_start_period=0
      print
      next
    }
    in_healthcheck && /start_period:/ {
      if (!seen_start_period) {
        seen_start_period=1
        print
      }
      # Skip duplicate start_period lines
      next
    }
    in_healthcheck && /^  [a-z_]+:$/ && !/^    / {
      # New service block, reset state
      in_service=1
      in_healthcheck=0
      seen_start_period=0
      print
      next
    }
    in_healthcheck && !/^      / && !/^        / {
      # Exiting healthcheck block
      in_healthcheck=0
      print
      next
    }
    { print }
  ' "$compose_file" >"${compose_file}.tmp" && mv "${compose_file}.tmp" "$compose_file"

  if [[ "$fixed" == "true" ]]; then
    echo "Fixed health check configurations in docker-compose.yml"
    return 0
  fi

  return 0
}

# Export function for use in other scripts
export -f fix_healthchecks

# Run if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  fix_healthchecks "$@"
fi
