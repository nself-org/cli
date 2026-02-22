#!/usr/bin/env bash

# supabase.sh - Migrate from Supabase to nself
# Mission: Help users escape vendor lock-in
# v0.4.8

set -euo pipefail

# Import utilities
SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$SCRIPT_DIR/../utils/display.sh" 2>/dev/null || true

# Colors
: "${COLOR_GREEN:=\033[0;32m}"
: "${COLOR_YELLOW:=\033[0;33m}"
: "${COLOR_RED:=\033[0;31m}"
: "${COLOR_CYAN:=\033[0;36m}"
: "${COLOR_RESET:=\033[0m}"
: "${COLOR_DIM:=\033[2m}"

# Validate Supabase connection
validate_supabase_connection() {
  local supabase_url="$1"
  local supabase_key="$2"

  if [[ -z "$supabase_url" ]] || [[ -z "$supabase_key" ]]; then
    log_error "Supabase URL and API key are required"
    return 1
  fi

  # Test connection
  log_info "Testing Supabase connection..."

  if command -v curl >/dev/null 2>&1; then
    local response=$(curl -s -o /dev/null -w "%{http_code}" \
      -H "apikey: $supabase_key" \
      -H "Authorization: Bearer $supabase_key" \
      "$supabase_url/rest/v1/" 2>/dev/null)

    if [[ "$response" == "200" ]]; then
      log_success "Supabase connection validated"
      return 0
    else
      log_error "Failed to connect to Supabase (HTTP $response)"
      return 1
    fi
  else
    log_warning "curl not found - skipping connection test"
    return 0
  fi
}

# Export Supabase database schema
export_supabase_schema() {
  local db_host="$1"
  local db_port="$2"
  local db_name="$3"
  local db_user="$4"
  local db_pass="$5"
  local output_dir="$6"

  log_info "Exporting Supabase database schema..."

  mkdir -p "$output_dir/schema"

  # Set password for pg_dump
  export PGPASSWORD="$db_pass"

  # Export schema only
  local schema_file="$output_dir/schema/schema.sql"

  if pg_dump -h "$db_host" -p "$db_port" -U "$db_user" -d "$db_name" \
    --schema-only \
    --no-owner \
    --no-acl \
    --exclude-schema=storage \
    --exclude-schema=auth \
    >"$schema_file" 2>/dev/null; then
    log_success "Schema exported: $schema_file"
  else
    log_error "Failed to export schema"
    log_info "Make sure PostgreSQL client tools are installed"
    log_info "macOS: brew install postgresql"
    log_info "Linux: apt install postgresql-client"
    unset PGPASSWORD
    return 1
  fi

  # Export auth schema separately
  local auth_schema_file="$output_dir/schema/auth-schema.sql"
  if pg_dump -h "$db_host" -p "$db_port" -U "$db_user" -d "$db_name" \
    --schema-only \
    --schema=auth \
    --no-owner \
    --no-acl \
    >"$auth_schema_file" 2>/dev/null; then
    log_success "Auth schema exported: $auth_schema_file"
  fi

  # Export storage schema separately
  local storage_schema_file="$output_dir/schema/storage-schema.sql"
  if pg_dump -h "$db_host" -p "$db_port" -U "$db_user" -d "$db_name" \
    --schema-only \
    --schema=storage \
    --no-owner \
    --no-acl \
    >"$storage_schema_file" 2>/dev/null; then
    log_success "Storage schema exported: $storage_schema_file"
  fi

  unset PGPASSWORD
  return 0
}

# Export Supabase database data
export_supabase_data() {
  local db_host="$1"
  local db_port="$2"
  local db_name="$3"
  local db_user="$4"
  local db_pass="$5"
  local output_dir="$6"
  local tables="${7:-all}"

  log_info "Exporting Supabase database data..."

  mkdir -p "$output_dir/data"

  # Set password
  export PGPASSWORD="$db_pass"

  # Export data
  local data_file="$output_dir/data/data.sql"

  if [[ "$tables" == "all" ]]; then
    # Export all data
    if pg_dump -h "$db_host" -p "$db_port" -U "$db_user" -d "$db_name" \
      --data-only \
      --no-owner \
      --no-acl \
      --exclude-schema=storage \
      --exclude-schema=auth \
      >"$data_file" 2>/dev/null; then
      log_success "Data exported: $data_file"
    else
      log_error "Failed to export data"
      unset PGPASSWORD
      return 1
    fi
  else
    # Export specific tables
    if pg_dump -h "$db_host" -p "$db_port" -U "$db_user" -d "$db_name" \
      --data-only \
      --no-owner \
      --no-acl \
      -t "$tables" \
      >"$data_file" 2>/dev/null; then
      log_success "Data exported: $data_file"
    else
      log_error "Failed to export data"
      unset PGPASSWORD
      return 1
    fi
  fi

  # Export auth data
  local auth_data_file="$output_dir/data/auth-data.sql"
  if pg_dump -h "$db_host" -p "$db_port" -U "$db_user" -d "$db_name" \
    --data-only \
    --schema=auth \
    --no-owner \
    --no-acl \
    >"$auth_data_file" 2>/dev/null; then
    log_success "Auth data exported: $auth_data_file"
  fi

  # Get data statistics
  local row_count=$(grep -c "INSERT INTO" "$data_file" 2>/dev/null || echo "0")
  log_info "Exported approximately $row_count data rows"

  unset PGPASSWORD
  return 0
}

# Export Supabase Auth users via API
export_supabase_auth_api() {
  local supabase_url="$1"
  local service_role_key="$2"
  local output_dir="$3"

  log_info "Exporting Supabase Auth users via API..."

  mkdir -p "$output_dir/auth"

  # Note: This requires Supabase service_role key (not anon key)
  local auth_endpoint="${supabase_url}/auth/v1/admin/users"
  local output_file="$output_dir/auth/users.json"

  if command -v curl >/dev/null 2>&1; then
    if curl -s \
      -H "apikey: $service_role_key" \
      -H "Authorization: Bearer $service_role_key" \
      "$auth_endpoint" \
      >"$output_file" 2>/dev/null; then

      local user_count=$(command -v jq >/dev/null 2>&1 && jq '. | length' "$output_file" 2>/dev/null || echo "unknown")
      log_success "Exported $user_count users via API: $output_file"
      return 0
    else
      log_warning "API export failed - using database export instead"
      rm -f "$output_file"
      return 1
    fi
  else
    log_warning "curl not available - skipping API export"
    return 1
  fi
}

# Export Supabase Storage buckets and files
export_supabase_storage() {
  local supabase_url="$1"
  local service_role_key="$2"
  local output_dir="$3"

  log_info "Exporting Supabase Storage..."

  mkdir -p "$output_dir/storage"

  # List buckets
  local buckets_endpoint="${supabase_url}/storage/v1/bucket"
  local buckets_file="$output_dir/storage/buckets.json"

  if ! command -v curl >/dev/null 2>&1; then
    log_warning "curl not available - skipping storage export"
    return 1
  fi

  # Get bucket list
  if curl -s \
    -H "apikey: $service_role_key" \
    -H "Authorization: Bearer $service_role_key" \
    "$buckets_endpoint" \
    >"$buckets_file" 2>/dev/null; then

    log_success "Storage buckets exported: $buckets_file"

    # Parse buckets and list files
    if command -v jq >/dev/null 2>&1; then
      local buckets=$(jq -r '.[].id' "$buckets_file" 2>/dev/null)

      for bucket in $buckets; do
        log_info "Exporting bucket: $bucket"

        local files_endpoint="${supabase_url}/storage/v1/object/list/$bucket"
        local bucket_dir="$output_dir/storage/$bucket"
        mkdir -p "$bucket_dir"

        # List files in bucket
        local files=$(curl -s \
          -H "apikey: $service_role_key" \
          -H "Authorization: Bearer $service_role_key" \
          "$files_endpoint" | jq -r '.[].name' 2>/dev/null)

        local file_count=0
        for file in $files; do
          # Download file
          local file_url="${supabase_url}/storage/v1/object/$bucket/$file"
          local file_path="$bucket_dir/$file"
          local file_dir=$(dirname "$file_path")

          mkdir -p "$file_dir"

          if curl -s \
            -H "apikey: $service_role_key" \
            -H "Authorization: Bearer $service_role_key" \
            "$file_url" \
            -o "$file_path" 2>/dev/null; then
            file_count=$((file_count + 1))
          fi
        done

        log_success "Downloaded $file_count files from bucket: $bucket"
      done
    else
      log_warning "jq not available - cannot list files in buckets"
    fi

    return 0
  else
    log_error "Failed to export storage buckets"
    return 1
  fi
}

# Import schema to nself PostgreSQL
import_supabase_schema_to_nself() {
  local schema_file="$1"
  local db_host="${2:-localhost}"
  local db_port="${3:-5432}"
  local db_name="${4:-nhost}"
  local db_user="${5:-postgres}"
  local db_pass="${6:-}"

  log_info "Importing schema to nself PostgreSQL..."

  export PGPASSWORD="$db_pass"

  if psql -h "$db_host" -p "$db_port" -U "$db_user" -d "$db_name" \
    -f "$schema_file" >/dev/null 2>&1; then
    log_success "Schema imported successfully"
    unset PGPASSWORD
    return 0
  else
    log_error "Failed to import schema"
    unset PGPASSWORD
    return 1
  fi
}

# Import data to nself PostgreSQL
import_supabase_data_to_nself() {
  local data_file="$1"
  local db_host="${2:-localhost}"
  local db_port="${3:-5432}"
  local db_name="${4:-nhost}"
  local db_user="${5:-postgres}"
  local db_pass="${6:-}"

  log_info "Importing data to nself PostgreSQL..."

  export PGPASSWORD="$db_pass"

  if psql -h "$db_host" -p "$db_port" -U "$db_user" -d "$db_name" \
    -f "$data_file" >/dev/null 2>&1; then
    log_success "Data imported successfully"
    unset PGPASSWORD
    return 0
  else
    log_error "Failed to import data"
    unset PGPASSWORD
    return 1
  fi
}

# Import storage to MinIO
import_supabase_storage_to_minio() {
  local storage_dir="$1"
  local minio_endpoint="${2:-http://localhost:9000}"
  local minio_access_key="${3:-minioadmin}"
  local minio_secret_key="${4:-minioadmin}"

  log_info "Uploading Supabase storage to MinIO..."

  if ! command -v mc >/dev/null 2>&1; then
    log_warning "MinIO Client (mc) not found"
    log_info "Install: brew install minio/stable/mc (macOS)"
    log_info "Alternatively, use the MinIO Console to upload files manually"
    log_info "Files are located in: $storage_dir"
    return 1
  fi

  # Configure MinIO client
  local alias_name="nself-supabase-migration"
  mc alias set "$alias_name" "$minio_endpoint" "$minio_access_key" "$minio_secret_key" >/dev/null 2>&1

  # Upload each bucket
  for bucket_path in "$storage_dir"/*; do
    if [[ -d "$bucket_path" ]]; then
      local bucket_name=$(basename "$bucket_path")

      # Create bucket in MinIO
      if ! mc ls "$alias_name/$bucket_name" >/dev/null 2>&1; then
        mc mb "$alias_name/$bucket_name" 2>&1 | grep -v "Bucket created" || true
        log_success "Created bucket: $bucket_name"
      fi

      # Upload files
      mc cp --recursive "$bucket_path/" "$alias_name/$bucket_name/" 2>&1 | grep -E "^(Total|✓)" || true
      log_success "Uploaded bucket: $bucket_name"
    fi
  done

  # Remove alias
  mc alias remove "$alias_name" >/dev/null 2>&1

  log_success "Storage migration completed"
  return 0
}

# Main migration orchestrator
migrate_from_supabase() {
  local supabase_url="$1"
  local service_role_key="$2"
  local db_host="$3"
  local db_port="${4:-5432}"
  local db_name="${5:-postgres}"
  local db_user="${6:-postgres}"
  local db_pass="$7"
  local output_dir="${8:-./supabase-migration-$(date +%Y%m%d-%H%M%S)}"

  printf "${COLOR_CYAN}╔════════════════════════════════════════╗${COLOR_RESET}\n"
  printf "${COLOR_CYAN}║   Supabase → nself Migration Tool     ║${COLOR_RESET}\n"
  printf "${COLOR_CYAN}╚════════════════════════════════════════╝${COLOR_RESET}\n"
  echo ""

  log_info "Mission: Help you escape vendor lock-in"
  echo ""

  # Validate connection
  if ! validate_supabase_connection "$supabase_url" "$service_role_key"; then
    return 1
  fi

  mkdir -p "$output_dir"

  # Step 1: Export from Supabase
  printf "${COLOR_CYAN}➞ Step 1: Export from Supabase${COLOR_RESET}\n"
  echo ""

  # Export schema
  export_supabase_schema "$db_host" "$db_port" "$db_name" "$db_user" "$db_pass" "$output_dir"
  echo ""

  # Export data
  export_supabase_data "$db_host" "$db_port" "$db_name" "$db_user" "$db_pass" "$output_dir" "all"
  echo ""

  # Export storage
  export_supabase_storage "$supabase_url" "$service_role_key" "$output_dir"
  echo ""

  # Step 2: Instructions for import
  printf "${COLOR_CYAN}➞ Step 2: Import to nself${COLOR_RESET}\n"
  echo ""

  log_info "To complete migration, run these commands:"
  echo ""
  echo "  # Import database schema"
  echo "  nself migrate from supabase import-schema \"$output_dir/schema/schema.sql\""
  echo ""
  echo "  # Import database data"
  echo "  nself migrate from supabase import-data \"$output_dir/data/data.sql\""
  echo ""
  echo "  # Import auth schema and data"
  echo "  nself migrate from supabase import-schema \"$output_dir/schema/auth-schema.sql\""
  echo "  nself migrate from supabase import-data \"$output_dir/data/auth-data.sql\""
  echo ""
  echo "  # Import storage to MinIO"
  echo "  nself migrate from supabase import-storage \"$output_dir/storage\""
  echo ""

  log_success "Supabase data exported to: $output_dir"
  echo ""
  log_info "Migration files:"
  printf "  ${COLOR_DIM}Schema:  $output_dir/schema/${COLOR_RESET}\n"
  printf "  ${COLOR_DIM}Data:    $output_dir/data/${COLOR_RESET}\n"
  printf "  ${COLOR_DIM}Storage: $output_dir/storage/${COLOR_RESET}\n"

  return 0
}

# Export functions
export -f validate_supabase_connection
export -f export_supabase_schema
export -f export_supabase_data
export -f export_supabase_auth_api
export -f export_supabase_storage
export -f import_supabase_schema_to_nself
export -f import_supabase_data_to_nself
export -f import_supabase_storage_to_minio
export -f migrate_from_supabase
