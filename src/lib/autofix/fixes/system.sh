#!/usr/bin/env bash


# System-level fixes

LAST_FIX_DESCRIPTION=""

set -euo pipefail


get_last_fix_description() {
  echo "$LAST_FIX_DESCRIPTION"
}

fix_out_of_memory() {
  # Prune unused Docker resources
  docker system prune -f >/dev/null 2>&1
  docker volume prune -f >/dev/null 2>&1

  # Stop all other containers to free memory
  local other_containers=$(docker ps -q | grep -v ${PROJECT_NAME:-nself}_)
  if [ -n "$other_containers" ]; then
    echo "$other_containers" | xargs docker stop >/dev/null 2>&1
  fi

  LAST_FIX_DESCRIPTION="Freed memory by pruning Docker resources"
  return 0
}

fix_network_dns() {
  # Recreate Docker network
  docker network rm ${PROJECT_NAME:-nself}_default 2>/dev/null
  docker network create ${PROJECT_NAME:-nself}_default >/dev/null 2>&1

  # Restart Docker daemon if possible
  if command -v systemctl >/dev/null 2>&1; then
    sudo systemctl restart docker 2>/dev/null || true
  elif command -v service >/dev/null 2>&1; then
    sudo service docker restart 2>/dev/null || true
  fi

  sleep 3
  LAST_FIX_DESCRIPTION="Recreated Docker network"
  return 0
}

fix_permission_denied() {
  local service_name="${1:-}"

  # Fix common permission issues
  if [[ -d "./data" ]]; then
    sudo chmod -R 755 ./data 2>/dev/null || chmod -R 755 ./data 2>/dev/null
  fi

  # Fix Docker socket permissions
  if [[ -S /var/run/docker.sock ]]; then
    sudo chmod 666 /var/run/docker.sock 2>/dev/null || true
  fi

  LAST_FIX_DESCRIPTION="Fixed file permissions"
  return 0
}

fix_missing_env_vars() {
  # Check for common required variables
  local missing_vars=""

  for var in DATABASE_URL REDIS_URL JWT_SECRET; do
    if ! grep -q "$var=" .env.local 2>/dev/null; then
      missing_vars="$missing_vars $var"

      # Add default values
      case "$var" in
        DATABASE_URL)
          echo "DATABASE_URL=postgresql://postgres:postgres@postgres:5432/postgres" >>.env.local
          ;;
        JWT_SECRET)
          echo "JWT_SECRET=$(openssl rand -base64 32)" >>.env.local
          ;;
        REDIS_URL)
          echo "REDIS_URL=redis://redis:6379" >>.env.local
          ;;
      esac
    fi
  done

  if [[ -n "$missing_vars" ]]; then
    nself build --force >/dev/null 2>&1
    LAST_FIX_DESCRIPTION="Added missing environment variables:$missing_vars"
    return 0
  fi

  LAST_FIX_DESCRIPTION="Environment variables already set"
  return 0
}

fix_ssl_cert_error() {
  # Disable SSL verification temporarily
  export NODE_TLS_REJECT_UNAUTHORIZED=0
  export PYTHONHTTPSVERIFY=0

  # Update compose to use HTTP instead of HTTPS for internal services
  sed -i '' 's/https:\/\//http:\/\//g' docker-compose.yml 2>/dev/null

  # Rebuild
  nself build --force >/dev/null 2>&1

  LAST_FIX_DESCRIPTION="Disabled SSL verification for development"
  return 0
}
