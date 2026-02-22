#!/usr/bin/env bash

# Service Health Monitor and Auto-Fixer
# Monitors container health and applies targeted fixes for common issues

# Source utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

if [[ -f "$SCRIPT_DIR/../utils/display.sh" ]]; then
  source "$SCRIPT_DIR/../utils/display.sh"
else
  log_info() { echo "ℹ $1"; }
  log_success() { echo "✓ $1"; }
  log_warning() { echo "⚠ $1"; }
  log_error() { echo "✗ $1"; }
fi

# Track fix attempts to prevent infinite loops
# Using files instead of associative arrays for compatibility
FIX_ATTEMPTS_FILE="/tmp/.nself_fix_attempts_${PROJECT_NAME:-nself}"
LAST_FIX_TIME_FILE="/tmp/.nself_fix_time_${PROJECT_NAME:-nself}"
MAX_FIX_ATTEMPTS=3
FIX_COOLDOWN=300 # 5 minutes between fix attempts

# Track intentionally stopped services
STOPPED_SERVICES_FILE="/tmp/.nself_stopped_services_${PROJECT_NAME:-nself}"

# Service-specific fixes are handled by case statement in apply_service_fix

# Common Docker error patterns are handled inline in generic_fix

# Check if service was intentionally stopped
is_intentionally_stopped() {
  local service="$1"
  [[ -f "$STOPPED_SERVICES_FILE" ]] && grep -q "^$service$" "$STOPPED_SERVICES_FILE"
}

# Mark service as intentionally stopped
mark_as_stopped() {
  local service="$1"
  echo "$service" >>"$STOPPED_SERVICES_FILE"
}

# Remove service from stopped list
mark_as_started() {
  local service="$1"
  if [[ -f "$STOPPED_SERVICES_FILE" ]]; then
    grep -v "^$service$" "$STOPPED_SERVICES_FILE" >"${STOPPED_SERVICES_FILE}.tmp" || true
    mv "${STOPPED_SERVICES_FILE}.tmp" "$STOPPED_SERVICES_FILE"
  fi
}

# Get fix attempts for a service
get_fix_attempts() {
  local service="$1"
  if [[ -f "$FIX_ATTEMPTS_FILE" ]]; then
    grep "^$service:" "$FIX_ATTEMPTS_FILE" 2>/dev/null | cut -d: -f2 || echo "0"
  else
    echo "0"
  fi
}

# Set fix attempts for a service
set_fix_attempts() {
  local service="$1"
  local attempts="$2"
  if [[ -f "$FIX_ATTEMPTS_FILE" ]]; then
    grep -v "^$service:" "$FIX_ATTEMPTS_FILE" >"${FIX_ATTEMPTS_FILE}.tmp" 2>/dev/null || true
    mv "${FIX_ATTEMPTS_FILE}.tmp" "$FIX_ATTEMPTS_FILE"
  fi
  echo "$service:$attempts" >>"$FIX_ATTEMPTS_FILE"
}

# Get last fix time for a service
get_last_fix_time() {
  local service="$1"
  if [[ -f "$LAST_FIX_TIME_FILE" ]]; then
    grep "^$service:" "$LAST_FIX_TIME_FILE" 2>/dev/null | cut -d: -f2 || echo "0"
  else
    echo "0"
  fi
}

# Set last fix time for a service
set_last_fix_time() {
  local service="$1"
  local time="$2"
  if [[ -f "$LAST_FIX_TIME_FILE" ]]; then
    grep -v "^$service:" "$LAST_FIX_TIME_FILE" >"${LAST_FIX_TIME_FILE}.tmp" 2>/dev/null || true
    mv "${LAST_FIX_TIME_FILE}.tmp" "$LAST_FIX_TIME_FILE"
  fi
  echo "$service:$time" >>"$LAST_FIX_TIME_FILE"
}

# Check if we should attempt a fix
should_attempt_fix() {
  local service="$1"
  local current_time=$(date +%s)

  # Check if service was intentionally stopped
  if is_intentionally_stopped "$service"; then
    return 1
  fi

  # Check fix attempts
  local attempts=$(get_fix_attempts "$service")
  if [[ $attempts -ge $MAX_FIX_ATTEMPTS ]]; then
    # Check if cooldown period has passed
    local last_fix=$(get_last_fix_time "$service")
    local time_diff=$((current_time - last_fix))
    if [[ $time_diff -lt $FIX_COOLDOWN ]]; then
      return 1
    else
      # Reset attempts after cooldown
      set_fix_attempts "$service" "0"
    fi
  fi

  return 0
}

# Record fix attempt
record_fix_attempt() {
  local service="$1"
  local current_time=$(date +%s)
  local attempts=$(get_fix_attempts "$service")
  set_fix_attempts "$service" $((attempts + 1))
  set_last_fix_time "$service" "$current_time"
}

# PostgreSQL specific fixes
fix_postgres() {
  local container="$1"
  local logs=$(docker logs "$container" 2>&1 | tail -20)

  if echo "$logs" | grep -q "database system is ready to accept connections"; then
    return 0 # Healthy
  fi

  if echo "$logs" | grep -q "could not bind IPv4 address"; then
    log_warning "Postgres: Port conflict detected"
    # Find and update port in docker-compose.yml
    local current_port=$(docker inspect "$container" -f '{{range $p, $conf := .NetworkSettings.Ports}}{{if $conf}}{{(index $conf 0).HostPort}}{{end}}{{end}}' | head -1)
    if [[ -n "$current_port" ]]; then
      local new_port=$((current_port + 1))
      log_info "Attempting to use port $new_port"
      # This would need to update docker-compose.yml and recreate
    fi
  fi

  if echo "$logs" | grep -q "FATAL:  data directory .* has wrong ownership"; then
    log_warning "Postgres: Permission issue detected"
    docker exec "$container" chown -R postgres:postgres /var/lib/postgresql/data 2>/dev/null || true
  fi
}

# Hasura specific fixes
fix_hasura() {
  local container="$1"
  local logs=$(docker logs "$container" 2>&1 | tail -20)

  # Check for postgres connection issues
  if echo "$logs" | grep -q "connection to server.*failed"; then
    log_warning "Hasura: Database connection issue"

    # Extract the connection string to check port
    local conn_string=$(echo "$logs" | grep -o "postgres://[^\"]*" | head -1)
    if echo "$conn_string" | grep -q ":543[3-9]"; then
      log_error "Hasura using wrong postgres port (external instead of internal)"
      # Fix: Update DATABASE_URL to use port 5432
      if [[ -f "docker-compose.yml" ]]; then
        sed -i.bak 's/:543[3-9]\//:5432\//g' docker-compose.yml
        docker-compose up -d "$container" 2>/dev/null
      fi
    fi
  fi

  # Check for JWT secret issues
  if echo "$logs" | grep -q "JWT secret"; then
    log_warning "Hasura: JWT configuration issue"
    # Ensure JWT secret is properly formatted
  fi

  # Check for metadata issues
  if echo "$logs" | grep -q "metadata.*inconsistent"; then
    log_warning "Hasura: Metadata inconsistency"
    docker exec "$container" curl -X POST http://localhost:8080/v1/metadata \
      -H "X-Hasura-Admin-Secret: ${HASURA_ADMIN_SECRET:-hasura-admin-secret-dev}" \
      -d '{"type":"reload_metadata","args":{}}' 2>/dev/null || true
  fi
}

# Auth service specific fixes
fix_auth() {
  local container="$1"
  local logs=$(docker logs "$container" 2>&1 | tail -20)

  # Check for database connection
  if echo "$logs" | grep -q "connection.*refused\|no pg_hba.conf entry"; then
    log_warning "Auth: Database connection issue"
    # Ensure auth database exists
    local postgres_container="${PROJECT_NAME:-nself}_postgres"
    docker exec "$postgres_container" psql -U postgres -tc "SELECT 1 FROM pg_database WHERE datname = 'auth'" | grep -q 1 ||
      docker exec "$postgres_container" psql -U postgres -c "CREATE DATABASE auth;" 2>/dev/null
  fi

  # Check for port issues
  if echo "$logs" | grep -q "listen.*address already in use"; then
    log_warning "Auth: Port conflict on 4000"
    # Auth typically runs on 4000, but health checks might expect 4001
  fi

  # Check for missing environment variables
  if echo "$logs" | grep -q "required.*environment\|missing.*config"; then
    log_warning "Auth: Missing configuration"
    # Check for required env vars: HASURA_GRAPHQL_DATABASE_URL, HASURA_GRAPHQL_ADMIN_SECRET, etc.
  fi
}

# Storage service specific fixes
fix_storage() {
  local container="$1"
  local logs=$(docker logs "$container" 2>&1 | tail -20)

  # Check for MinIO connection
  if echo "$logs" | grep -q "connection.*minio.*refused"; then
    log_warning "Storage: MinIO connection issue"
    # Ensure MinIO is running
    local minio_container="${PROJECT_NAME:-nself}_minio"
    if ! docker ps | grep -q "$minio_container"; then
      docker start "$minio_container" 2>/dev/null
    fi
  fi

  # Check for Hasura connection
  if echo "$logs" | grep -q "dial tcp.*hasura.*no such host"; then
    log_warning "Storage: Cannot connect to Hasura"
    # This often happens when hasura is restarting
    # Wait for hasura to be healthy before restarting storage
    local hasura_container="${PROJECT_NAME:-nself}_hasura"
    local retries=0
    while [[ $retries -lt 30 ]]; do
      if docker exec "$hasura_container" curl -f http://localhost:8080/healthz 2>/dev/null; then
        docker restart "$container" 2>/dev/null
        break
      fi
      sleep 2
      retries=$((retries + 1))
    done
  fi

  # Check for database issues
  if echo "$logs" | grep -q "storage.*database.*not exist"; then
    log_warning "Storage: Database missing"
    local postgres_container="${PROJECT_NAME:-nself}_postgres"
    docker exec "$postgres_container" psql -U postgres -c "CREATE DATABASE storage;" 2>/dev/null || true
  fi
}

# Nginx specific fixes
fix_nginx() {
  local container="$1"
  local logs=$(docker logs "$container" 2>&1 | tail -20)

  # Check for SSL certificate issues
  if echo "$logs" | grep -q "SSL_CTX_use_certificate.*failed\|no such file.*\.pem"; then
    log_warning "Nginx: SSL certificate missing"
    # Generate self-signed certificates if missing
    if [[ ! -f "ssl/certificates/localhost/cert.pem" ]]; then
      mkdir -p ssl/certificates/localhost
      openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
        -keyout ssl/certificates/localhost/key.pem \
        -out ssl/certificates/localhost/cert.pem \
        -subj "/C=US/ST=State/L=City/O=Organization/CN=localhost" 2>/dev/null
    fi
  fi

  # Check for upstream issues
  if echo "$logs" | grep -q "host not found in upstream"; then
    log_warning "Nginx: Upstream service not found"
    # This happens when dependent services aren't running
    # Extract the service name and ensure it's running
    local upstream=$(echo "$logs" | grep -o 'upstream "[^"]*"' | cut -d'"' -f2 | head -1)
    if [[ -n "$upstream" ]]; then
      local service_name=$(echo "$upstream" | cut -d: -f1)
      local service_container="${PROJECT_NAME:-nself}_${service_name}"
      if docker ps -a | grep -q "$service_container"; then
        docker start "$service_container" 2>/dev/null
        sleep 2
        docker restart "$container" 2>/dev/null
      fi
    fi
  fi

  # Check for config syntax errors
  if echo "$logs" | grep -q "nginx:.*emerg.*failed"; then
    log_warning "Nginx: Configuration syntax error"
    # Test nginx config
    docker exec "$container" nginx -t 2>&1 || true
  fi

  # Check for missing includes
  if echo "$logs" | grep -q 'open.*"/etc/nginx/ssl/ssl.conf".*failed'; then
    log_warning "Nginx: Missing SSL config include"
    # Create the missing file
    if [[ -d "nginx/conf.d" ]]; then
      echo "# SSL configuration placeholder" >nginx/conf.d/ssl.conf
      # Fix include paths in all conf files
      for conf in nginx/conf.d/*.conf; do
        sed -i.bak 's|include /etc/nginx/ssl/ssl.conf;|include /etc/nginx/conf.d/ssl.conf;|g' "$conf" 2>/dev/null && rm "$conf.bak" 2>/dev/null ||
          if [[ "$OSTYPE" == "darwin"* ]]; then
            sed -i '' 's|include /etc/nginx/ssl/ssl.conf;|include /etc/nginx/conf.d/ssl.conf;|g' "$conf" 2>/dev/null
          else
            sed -i 's|include /etc/nginx/ssl/ssl.conf;|include /etc/nginx/conf.d/ssl.conf;|g' "$conf" 2>/dev/null
          fi
      done
      docker restart "$container" 2>/dev/null
    fi
  fi
}

# MLflow specific fixes
fix_mlflow() {
  local container="$1"
  local logs=$(docker logs "$container" 2>&1 | tail -50)
  local postgres_container="${PROJECT_NAME:-nself}_postgres"

  # Check for command parsing issues (multi-line command problem)
  if echo "$logs" | grep -q "sh:.*--.*not found\|command not found"; then
    log_warning "MLflow: Command parsing issue detected"
    # This is a docker-compose.yml issue - the command needs to be on one line
    # We'll need to recreate the container with fixed command
    if [[ -f "docker-compose.yml" ]]; then
      # Fix the multi-line mlflow server command to be single line
      sed -i.bak '/mlflow server$/,/--workers.*$/c\
        mlflow server --backend-store-uri postgresql://postgres:${POSTGRES_PASSWORD}@postgres:5432/mlflow --default-artifact-root s3://mlflow-artifacts/ --host 0.0.0.0 --port 5000 --serve-artifacts --workers 4' docker-compose.yml 2>/dev/null ||
        if [[ "$OSTYPE" == "darwin"* ]]; then
          sed -i '' '/mlflow server$/,/--workers.*$/c\
        mlflow server --backend-store-uri postgresql://postgres:${POSTGRES_PASSWORD}@postgres:5432/mlflow --default-artifact-root s3://mlflow-artifacts/ --host 0.0.0.0 --port 5000 --serve-artifacts --workers 4' docker-compose.yml 2>/dev/null
        else
          sed -i '/mlflow server$/,/--workers.*$/c\
        mlflow server --backend-store-uri postgresql://postgres:${POSTGRES_PASSWORD}@postgres:5432/mlflow --default-artifact-root s3://mlflow-artifacts/ --host 0.0.0.0 --port 5000 --serve-artifacts --workers 4' docker-compose.yml 2>/dev/null
        fi

      # Recreate the container
      docker stop "$container" 2>/dev/null
      docker rm "$container" 2>/dev/null
      docker-compose up -d mlflow 2>/dev/null
      return 0
    fi
  fi

  # Check for database schema issues (metrics table missing)
  if echo "$logs" | grep -q "relation.*metrics.*does not exist\|mlflow.*database.*does not exist\|UndefinedTable"; then
    log_warning "MLflow: Database schema missing"

    # First ensure database exists
    if ! docker exec "$postgres_container" psql -U postgres -lqt 2>/dev/null | cut -d \| -f 1 | grep -qw "mlflow"; then
      log_info "Creating MLflow database..."
      docker exec "$postgres_container" psql -U postgres -c "CREATE DATABASE mlflow;" 2>/dev/null || true
    fi

    # Get database credentials from container environment
    local db_user=$(docker inspect "$container" --format '{{range .Config.Env}}{{println .}}{{end}}' | grep -E "^POSTGRES_USER=" | cut -d= -f2 || echo "postgres")
    local db_pass=$(docker inspect "$container" --format '{{range .Config.Env}}{{println .}}{{end}}' | grep -E "^POSTGRES_PASSWORD=" | cut -d= -f2 || echo "postgres")

    # Run MLflow database migrations to create schema
    log_info "Initializing MLflow database schema..."

    # Method 1: Try using mlflow db upgrade inside the container
    docker exec "$container" sh -c "pip install -q psycopg2-binary 2>/dev/null; mlflow db upgrade postgresql://${db_user}:${db_pass}@postgres:5432/mlflow" 2>/dev/null

    if [[ $? -ne 0 ]]; then
      # Method 2: Run a temporary container to initialize the database
      log_info "Running MLflow database migration via temporary container..."
      docker run --rm \
        --network "${PROJECT_NAME:-nself}_default" \
        -e MLFLOW_BACKEND_STORE_URI="postgresql://${db_user}:${db_pass}@postgres:5432/mlflow" \
        ghcr.io/mlflow/mlflow:v2.9.2 \
        sh -c "pip install -q psycopg2-binary && mlflow db upgrade postgresql://${db_user}:${db_pass}@postgres:5432/mlflow" 2>/dev/null
    fi

    # Restart MLflow after database initialization
    docker restart "$container" 2>/dev/null
    log_success "MLflow database initialized"
    return 0
  fi

  # Check for S3/MinIO connection issues
  if echo "$logs" | grep -q "S3.*Connection.*Error\|Unable to connect to S3\|botocore.*exceptions"; then
    log_warning "MLflow: S3/MinIO connection issue"
    # Ensure MinIO is running and accessible
    local minio_container="${PROJECT_NAME:-nself}_minio"
    if ! docker ps | grep -q "$minio_container"; then
      docker start "$minio_container" 2>/dev/null
      sleep 3
    fi

    # Create mlflow-artifacts bucket if it doesn't exist
    docker exec "$minio_container" sh -c "mc alias set local http://localhost:9000 minioadmin minioadmin 2>/dev/null; mc mb local/mlflow-artifacts 2>/dev/null" || true

    docker restart "$container" 2>/dev/null
  fi

  # Check for port binding issues
  if echo "$logs" | grep -q "bind.*address already in use.*5000"; then
    log_warning "MLflow: Port 5000 already in use"
    # Check what's using port 5000
    local port_user=$(lsof -i :5000 2>/dev/null | grep LISTEN | awk '{print $2}' | head -1)
    if [[ -n "$port_user" ]]; then
      log_error "Port 5000 is used by PID $port_user"
    fi
  fi
}

# Tempo specific fixes
fix_tempo() {
  local container="$1"
  local logs=$(docker logs "$container" 2>&1 | tail -20)

  # Check for config file issues
  if echo "$logs" | grep -q "failed to read configFile.*tempo.yaml"; then
    log_warning "Tempo: Configuration file missing"
    # Create minimal tempo config
    mkdir -p monitoring/tempo
    cat >monitoring/tempo/tempo.yaml <<'EOF'
server:
  http_listen_port: 3200

distributor:
  receivers:
    otlp:
      protocols:
        http:
        grpc:

ingester:
  trace_idle_period: 10s
  max_block_bytes: 1_000_000
  max_block_duration: 5m

compactor:
  compaction:
    compaction_window: 1h
    max_block_bytes: 100_000_000
    block_retention: 1h
    compacted_block_retention: 10m

storage:
  trace:
    backend: local
    wal:
      path: /var/tempo/wal
    local:
      path: /var/tempo/blocks
EOF
    docker restart "$container" 2>/dev/null
  fi

  # Check for invalid config fields
  if echo "$logs" | grep -q "field.*not found in type"; then
    log_warning "Tempo: Invalid configuration fields"
    # Remove problematic fields from config
  fi

  # Check for permission issues
  if echo "$logs" | grep -q "permission denied.*tempo"; then
    log_warning "Tempo: Permission issue"
    docker exec "$container" chown -R 10001:10001 /var/tempo 2>/dev/null || true
  fi
}

# Loki specific fixes
fix_loki() {
  local container="$1"
  local logs=$(docker logs "$container" 2>&1 | tail -20)

  # Check for config issues
  if echo "$logs" | grep -q "error loading config"; then
    log_warning "Loki: Configuration error"
    # Create default loki config if missing
  fi

  # Check for storage issues
  if echo "$logs" | grep -q "permission denied.*/loki"; then
    log_warning "Loki: Storage permission issue"
    docker exec "$container" chown -R 10001:10001 /loki 2>/dev/null || true
  fi

  # Check for schema version issues
  if echo "$logs" | grep -q "schema.*version"; then
    log_warning "Loki: Schema version issue"
    # Update schema config to v13
  fi
}

# Main health check function
check_service_health() {
  local container="$1"
  local service_type="$2"

  # Get container status
  local container_status=$(docker inspect "$container" --format='{{.State.Status}}' 2>/dev/null)
  local exit_code=$(docker inspect "$container" --format='{{.State.ExitCode}}' 2>/dev/null)
  local restart_count=$(docker inspect "$container" --format='{{.RestartCount}}' 2>/dev/null)

  # Check if service needs attention
  case "$container_status" in
    "running")
      # Check if healthy
      local health_status=$(docker inspect "$container" --format='{{.State.Health.Status}}' 2>/dev/null)
      if [[ "$health_status" == "unhealthy" ]]; then
        return 1
      fi
      return 0
      ;;
    "restarting")
      # Service is in restart loop
      if [[ $restart_count -gt 3 ]]; then
        return 2 # Needs intervention
      fi
      return 1
      ;;
    "exited" | "dead")
      # Check if it was intentional
      if is_intentionally_stopped "$container"; then
        return 0 # Don't fix
      fi
      return 2 # Needs restart
      ;;
    *)
      return 1
      ;;
  esac
}

# Apply fix for a specific service
apply_service_fix() {
  local container="$1"
  local service_type="$2"

  if ! should_attempt_fix "$container"; then
    return 1
  fi

  record_fix_attempt "$container"

  # Apply service-specific fix
  case "$service_type" in
    postgres) fix_postgres "$container" ;;
    hasura) fix_hasura "$container" ;;
    auth) fix_auth "$container" ;;
    storage) fix_storage "$container" ;;
    nginx) fix_nginx "$container" ;;
    mlflow) fix_mlflow "$container" ;;
    tempo) fix_tempo "$container" ;;
    loki) fix_loki "$container" ;;
    *) generic_fix "$container" ;;
  esac
}

# Generic fixes for unknown services
generic_fix() {
  local container="$1"
  local logs=$(docker logs "$container" 2>&1 | tail -20)

  # Check common error patterns
  for pattern in "connection refused" "no such host" "permission denied" "cannot allocate memory" "address already in use" "no such file or directory" "command not found" "exit code 127" "exit code 137" "exit code 143"; do
    if echo "$logs" | grep -qi "$pattern"; then
      case "$pattern" in
        "connection refused")
          log_info "Connection refused - checking dependencies"
          # Restart in dependency order
          ;;
        "no such host")
          log_info "DNS/Network issue detected"
          docker restart "$container" 2>/dev/null
          ;;
        "permission denied")
          log_info "Permission issue detected"
          # Try to fix common permission issues
          ;;
        "address already in use")
          log_error "Port conflict detected - manual intervention needed"
          ;;
        "no such file or directory")
          log_warning "Missing file detected"
          # Try to create missing files
          ;;
        "command not found" | "exit code 127")
          log_error "Command issue - check docker-compose.yml"
          ;;
        "exit code 137")
          log_error "Out of memory - increase Docker memory limits"
          ;;
        "exit code 143")
          log_info "Service was terminated (SIGTERM)"
          ;;
      esac
      break
    fi
  done
}

# Monitor all services
monitor_all_services() {
  local project_name="${PROJECT_NAME:-nself}"
  local unhealthy_count=0
  local fixed_count=0

  # Get all containers for this project
  local containers=$(docker ps -a --filter "label=com.docker.compose.project=$project_name" --format "{{.Names}}")

  for container in $containers; do
    # Extract service type from container name
    local service_type=$(echo "$container" | sed "s/${project_name}_//")

    if ! check_service_health "$container" "$service_type"; then
      unhealthy_count=$((unhealthy_count + 1))
      log_warning "Service $service_type needs attention"

      if apply_service_fix "$container" "$service_type"; then
        fixed_count=$((fixed_count + 1))
        log_success "Applied fix for $service_type"
      fi
    fi
  done

  if [[ $unhealthy_count -eq 0 ]]; then
    log_success "All services healthy"
  else
    log_info "Found $unhealthy_count unhealthy services, fixed $fixed_count"
  fi

  return $unhealthy_count
}

# Export functions for use in other scripts
export -f monitor_all_services
export -f check_service_health
export -f apply_service_fix
export -f mark_as_stopped
export -f mark_as_started

# Run if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  monitor_all_services "$@"
fi
