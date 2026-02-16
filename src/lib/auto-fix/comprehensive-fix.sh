#!/usr/bin/env bash

# Comprehensive auto-fix for all common issues

# Source display utilities if available
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

if [[ -f "$SCRIPT_DIR/../utils/display.sh" ]]; then
  source "$SCRIPT_DIR/../utils/display.sh"
else
  # Fallback display functions
  log_info() { echo "ℹ $1"; }
  log_success() { echo "✓ $1"; }
  log_warning() { echo "⚠ $1"; }
  log_error() { echo "✗ $1"; }
fi

comprehensive_fix() {
  local project_name="${PROJECT_NAME:-nself}"
  local fixes_applied=0

  log_info "Running comprehensive fixes..."

  # 1. Fix Nginx SSL Configuration Issues
  fix_nginx_ssl() {
    local nginx_fixed=false

    # Check if nginx is having issues
    if docker ps --format "{{.Names}}" | grep -q "${project_name}_nginx"; then
      local nginx_status=$(docker inspect ${project_name}_nginx --format='{{.State.Status}}' 2>/dev/null)

      if [[ "$nginx_status" == "restarting" ]] || [[ "$nginx_status" == "exited" ]]; then
        log_info "Fixing nginx configuration..."

        # Fix SSL include paths in all nginx conf files
        if [[ -d "nginx/conf.d" ]]; then
          # Create ssl.conf if missing
          if [[ ! -f "nginx/conf.d/ssl.conf" ]]; then
            cat >nginx/conf.d/ssl.conf <<'EOF'
# SSL configuration - included from main nginx.conf
# This file prevents include errors
EOF
            nginx_fixed=true
          fi

          # Fix all config files
          for conf_file in nginx/conf.d/*.conf; do
            if [[ -f "$conf_file" ]]; then
              # Fix SSL include path
              sed -i '' 's|include /etc/nginx/ssl/ssl.conf;|include /etc/nginx/conf.d/ssl.conf;|g' "$conf_file" 2>/dev/null ||
                sed -i.bak 's|include /etc/nginx/ssl/ssl.conf;|include /etc/nginx/conf.d/ssl.conf;|g' "$conf_file" 2>/dev/null && rm "$conf_file.bak" 2>/dev/null

              # Fix deprecated http2 directive
              sed -i '' 's|listen 443 ssl http2;|listen 443 ssl;\n    http2 on;|g' "$conf_file" 2>/dev/null ||
                sed -i.bak 's|listen 443 ssl http2;|listen 443 ssl;\n    http2 on;|g' "$conf_file" 2>/dev/null && rm "${conf_file}.bak"

              # Standardize SSL certificate paths
              # Check what certificates actually exist
              if [[ -f "ssl/certificates/localhost/cert.pem" ]]; then
                # Use localhost certificates for localhost domains
                if grep -q "server_name.*localhost" "$conf_file"; then
                  sed -i '' 's|ssl_certificate /etc/nginx/ssl/certs/\${BASE_DOMAIN}/.*|ssl_certificate /etc/nginx/ssl/localhost/cert.pem;|g' "$conf_file" 2>/dev/null ||
                    sed -i.bak 's|ssl_certificate /etc/nginx/ssl/certs/${BASE_DOMAIN}/.*|ssl_certificate /etc/nginx/ssl/localhost/cert.pem;|g' "$conf_file" 2>/dev/null && rm "${conf_file}.bak"

                  sed -i '' 's|ssl_certificate_key /etc/nginx/ssl/certs/\${BASE_DOMAIN}/.*|ssl_certificate_key /etc/nginx/ssl/localhost/key.pem;|g' "$conf_file" 2>/dev/null ||
                    sed -i.bak 's|ssl_certificate_key /etc/nginx/ssl/certs/${BASE_DOMAIN}/.*|ssl_certificate_key /etc/nginx/ssl/localhost/key.pem;|g' "$conf_file" 2>/dev/null && rm "${conf_file}.bak"
                fi
              fi

              if [[ -f "ssl/certificates/nself-org/fullchain.pem" ]]; then
                # Use nself-org certificates for API endpoints
                if grep -q "server_name.*api\|hasura\|auth\|storage" "$conf_file"; then
                  sed -i '' 's|ssl_certificate .*\.pem;|ssl_certificate /etc/nginx/ssl/nself-org/fullchain.pem;|g' "$conf_file" 2>/dev/null ||
                    sed -i.bak 's|ssl_certificate .*\.pem;|ssl_certificate /etc/nginx/ssl/nself-org/fullchain.pem;|g' "$conf_file" 2>/dev/null && rm "${conf_file}.bak"

                  sed -i '' 's|ssl_certificate_key .*\.pem;|ssl_certificate_key /etc/nginx/ssl/nself-org/privkey.pem;|g' "$conf_file" 2>/dev/null ||
                    sed -i.bak 's|ssl_certificate_key .*\.pem;|ssl_certificate_key /etc/nginx/ssl/nself-org/privkey.pem;|g' "$conf_file" 2>/dev/null && rm "${conf_file}.bak"
                fi
              fi

              nginx_fixed=true
            fi
          done
        fi

        # Generate missing SSL certificates if needed
        if [[ ! -f "ssl/certificates/localhost/cert.pem" ]] || [[ ! -f "ssl/certificates/localhost/key.pem" ]]; then
          mkdir -p ssl/certificates/localhost
          openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
            -keyout ssl/certificates/localhost/key.pem \
            -out ssl/certificates/localhost/cert.pem \
            -subj "/C=US/ST=State/L=City/O=Organization/CN=localhost" >/dev/null 2>&1
          nginx_fixed=true
          log_success "Generated localhost SSL certificates"
        fi

        if [[ ! -f "ssl/certificates/nself-org/fullchain.pem" ]] || [[ ! -f "ssl/certificates/nself-org/privkey.pem" ]]; then
          mkdir -p ssl/certificates/nself-org
          openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
            -keyout ssl/certificates/nself-org/privkey.pem \
            -out ssl/certificates/nself-org/fullchain.pem \
            -subj "/C=US/ST=State/L=City/O=Organization/CN=*.local.nself.org" >/dev/null 2>&1
          nginx_fixed=true
          log_success "Generated nself-org SSL certificates"
        fi

        if [[ "$nginx_fixed" == "true" ]]; then
          docker restart ${project_name}_nginx >/dev/null 2>&1
          ((fixes_applied++))
          log_success "Fixed nginx configuration"
        fi
      fi
    fi
  }

  # 2. Fix Database Issues
  fix_databases() {
    local db_fixed=false

    # Check if postgres is running
    if docker ps --format "{{.Names}}" | grep -q "${project_name}_postgres"; then
      # Create missing databases
      local databases_to_create=("mlflow" "auth" "storage")

      for db in "${databases_to_create[@]}"; do
        # Check if database exists
        if ! docker exec ${project_name}_postgres psql -U postgres -lqt 2>/dev/null | cut -d \| -f 1 | grep -qw "$db"; then
          log_info "Creating $db database..."
          docker exec ${project_name}_postgres psql -U postgres -c "CREATE DATABASE $db;" >/dev/null 2>&1
          db_fixed=true
          ((fixes_applied++))
          log_success "Created $db database"
        fi
      done

      # Grant permissions
      if [[ "$db_fixed" == "true" ]]; then
        docker exec ${project_name}_postgres psql -U postgres -c "GRANT ALL PRIVILEGES ON DATABASE mlflow TO postgres;" >/dev/null 2>&1
        docker exec ${project_name}_postgres psql -U postgres -c "GRANT ALL PRIVILEGES ON DATABASE auth TO postgres;" >/dev/null 2>&1
        docker exec ${project_name}_postgres psql -U postgres -c "GRANT ALL PRIVILEGES ON DATABASE storage TO postgres;" >/dev/null 2>&1

        # Restart affected services
        docker restart ${project_name}_mlflow >/dev/null 2>&1 &
        docker restart ${project_name}_auth >/dev/null 2>&1 &
        docker restart ${project_name}_storage >/dev/null 2>&1 &
      fi
    fi
  }

  # 3. Fix Health Check Issues
  fix_healthchecks() {
    if [[ -f "docker-compose.yml" ]]; then
      local healthcheck_fixed=false

      # Fix duplicate start_period entries
      if grep -q "start_period:.*start_period:" docker-compose.yml; then
        sed -i '' '/^      start_period: [0-9]*s$/d' docker-compose.yml 2>/dev/null ||
          sed -i.bak '/^      start_period: [0-9]*s$/d' docker-compose.yml 2>/dev/null && rm docker-compose.yml.bak

        # Re-add single start_period where needed
        awk '
          /healthcheck:/ { in_healthcheck=1 }
          in_healthcheck && /retries:/ {
            print
            if (!has_start_period) {
              print "      start_period: 30s"
            }
            in_healthcheck=0
            has_start_period=0
            next
          }
          in_healthcheck && /start_period:/ { has_start_period=1 }
          /^  [a-z_]+:$/ && !/^    / { in_healthcheck=0; has_start_period=0 }
          { print }
        ' docker-compose.yml >docker-compose.yml.tmp && mv docker-compose.yml.tmp docker-compose.yml

        healthcheck_fixed=true
        ((fixes_applied++))
      fi

      # Fix incorrect ports in health checks
      sed -i '' 's|http://localhost:4000/version|http://localhost:4000/healthz|g' docker-compose.yml 2>/dev/null ||
        sed -i.bak 's|http://localhost:4000/version|http://localhost:4000/healthz|g' docker-compose.yml 2>/dev/null && rm docker-compose.yml.bak
      # Also fix if using wrong port
      sed -i '' 's|http://localhost:4001/|http://localhost:4000/|g' docker-compose.yml 2>/dev/null ||
        sed -i.bak 's|http://localhost:4001/|http://localhost:4000/|g' docker-compose.yml 2>/dev/null && rm docker-compose.yml.bak

      if [[ "$healthcheck_fixed" == "true" ]]; then
        log_success "Fixed health check configurations"
      fi
    fi
  }

  # 4. Fix Monitoring Stack Issues
  fix_monitoring() {
    # Fix Tempo configuration
    if ! [[ -f "monitoring/tempo/tempo.yaml" ]]; then
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

metrics_generator:
  registry:
    external_labels:
      source: tempo
      cluster: docker-compose
  storage:
    path: /var/tempo/generator/wal
    remote_write:
      - url: http://prometheus:9090/api/v1/write
        send_exemplars: true

storage:
  trace:
    backend: local
    wal:
      path: /var/tempo/wal
    local:
      path: /var/tempo/blocks
    pool:
      max_workers: 100
      queue_depth: 10000

overrides:
  metrics_generator_processors: [service-graphs, span-metrics]
EOF
      ((fixes_applied++))
      log_success "Created Tempo configuration"
    fi

    # Fix Loki configuration
    if ! [[ -f "monitoring/loki/local-config.yaml" ]]; then
      mkdir -p monitoring/loki
      cat >monitoring/loki/local-config.yaml <<'EOF'
auth_enabled: false

server:
  http_listen_port: 3100
  grpc_listen_port: 9096

common:
  path_prefix: /loki
  storage:
    filesystem:
      chunks_directory: /loki/chunks
      rules_directory: /loki/rules
  replication_factor: 1
  ring:
    instance_addr: 127.0.0.1
    kvstore:
      store: inmemory

schema_config:
  configs:
    - from: 2020-10-24
      store: tsdb
      object_store: filesystem
      schema: v13
      index:
        prefix: index_
        period: 24h

limits_config:
  allow_structured_metadata: false

ruler:
  alertmanager_url: http://alertmanager:9093

analytics:
  reporting_enabled: false
EOF
      ((fixes_applied++))
      log_success "Created Loki configuration"
    fi

    # Fix Prometheus configuration
    if ! [[ -f "prometheus/prometheus.yml" ]]; then
      mkdir -p prometheus
      cat >prometheus/prometheus.yml <<'EOF'
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'prometheus'
    static_configs:
      - targets: ['localhost:9090']

  - job_name: 'node-exporter'
    static_configs:
      - targets: ['node-exporter:9100']

  - job_name: 'postgres-exporter'
    static_configs:
      - targets: ['postgres-exporter:9187']

  - job_name: 'cadvisor'
    static_configs:
      - targets: ['cadvisor:8080']
EOF
      ((fixes_applied++))
      log_success "Created Prometheus configuration"
    fi
  }

  # 5. Fix Permission Issues
  fix_permissions() {
    # Fix volume permissions for services that need specific user/group
    local services_with_permissions=("tempo" "loki" "prometheus" "grafana")

    for service in "${services_with_permissions[@]}"; do
      if docker ps --format "{{.Names}}" | grep -q "${project_name}_${service}"; then
        case "$service" in
          tempo)
            docker exec ${project_name}_tempo chown -R 10001:10001 /var/tempo 2>/dev/null || true
            ;;
          loki)
            docker exec ${project_name}_loki chown -R 10001:10001 /loki 2>/dev/null || true
            ;;
          prometheus)
            docker exec ${project_name}_prometheus chown -R nobody:nobody /prometheus 2>/dev/null || true
            ;;
          grafana)
            docker exec ${project_name}_grafana chown -R 472:472 /var/lib/grafana 2>/dev/null || true
            ;;
        esac
      fi
    done
  }

  # 6. Fix Environment Variable Issues
  fix_env_vars() {
    if [[ -f ".env.dev" ]]; then
      local env_fixed=false

      # Ensure POSTGRES_PORT is correct (internal port, not external)
      if grep -q "POSTGRES_PORT=543[3-9]" .env.dev; then
        # This is likely an external port mapping, but services need internal port
        log_warning "Detected incorrect POSTGRES_PORT in .env.dev"
        # Don't change it as it might be intentional for external access
      fi

      # Fix missing required variables
      local required_vars=(
        "PROJECT_NAME"
        "BASE_DOMAIN"
        "POSTGRES_DB"
        "POSTGRES_USER"
        "POSTGRES_PASSWORD"
      )

      for var in "${required_vars[@]}"; do
        if ! grep -q "^${var}=" .env.dev; then
          case "$var" in
            PROJECT_NAME)
              echo "PROJECT_NAME=${project_name}" >>.env.dev
              ;;
            BASE_DOMAIN)
              echo "BASE_DOMAIN=localhost" >>.env.dev
              ;;
            POSTGRES_DB)
              echo "POSTGRES_DB=nself" >>.env.dev
              ;;
            POSTGRES_USER)
              echo "POSTGRES_USER=postgres" >>.env.dev
              ;;
            POSTGRES_PASSWORD)
              echo "POSTGRES_PASSWORD=$(openssl rand -base64 32)" >>.env.dev
              ;;
          esac
          env_fixed=true
        fi
      done

      if [[ "$env_fixed" == "true" ]]; then
        ((fixes_applied++))
        log_success "Fixed environment variables"
      fi
    fi
  }

  # Run all fixes
  fix_nginx_ssl
  fix_databases
  fix_healthchecks
  fix_monitoring
  fix_permissions
  fix_env_vars

  # Return number of fixes applied
  if [[ $fixes_applied -gt 0 ]]; then
    log_success "Applied $fixes_applied fixes"
    return 0
  else
    log_info "No fixes needed"
    return 0
  fi
}

# Export for use in other scripts
export -f comprehensive_fix

# Run if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  comprehensive_fix "$@"
fi
