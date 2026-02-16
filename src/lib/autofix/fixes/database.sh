#!/usr/bin/env bash


# Database-related fixes

LAST_FIX_DESCRIPTION=""

set -euo pipefail


get_last_fix_description() {
  echo "$LAST_FIX_DESCRIPTION"
}

fix_postgres_auth_failed() {
  # Reset postgres password
  local password="${POSTGRES_PASSWORD:-postgres}"

  # Update password in .env.local
  sed -i '' "s/POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=$password/" .env.local 2>/dev/null
  if ! grep -q "POSTGRES_PASSWORD=" .env.local 2>/dev/null; then
    echo "POSTGRES_PASSWORD=$password" >>.env.local
  fi

  # Rebuild and restart
  nself build --force >/dev/null 2>&1
  docker compose restart postgres >/dev/null 2>&1
  sleep 3

  LAST_FIX_DESCRIPTION="Reset Postgres password to default"
  return 0
}

fix_database_not_found() {
  local db_name="${DATABASE_NAME:-postgres}"

  # Create the database
  docker exec ${PROJECT_NAME:-nself}_postgres createdb -U postgres "$db_name" 2>/dev/null

  if [[ $? -eq 0 ]]; then
    LAST_FIX_DESCRIPTION="Created database '$db_name'"
    return 0
  fi

  # If that fails, recreate postgres with fresh data
  docker compose stop postgres >/dev/null 2>&1
  docker compose rm -f postgres >/dev/null 2>&1
  docker volume rm ${PROJECT_NAME:-nself}_postgres_data 2>/dev/null
  docker compose up -d postgres >/dev/null 2>&1
  sleep 5

  LAST_FIX_DESCRIPTION="Reset Postgres with fresh database"
  return 0
}
