#!/usr/bin/env bash

# Auto-fix common restart loop issues

fix_restart_loops() {

set -euo pipefail

  local project_name="${PROJECT_NAME:-nself}"
  local fixed_any=false

  # Check for services in restart loops
  local restarting_services=$(docker ps --filter "label=com.docker.compose.project=$project_name" --format "{{.Names}} {{.Status}}" | grep -i "restarting" | awk '{print $1}')

  if [[ -z "$restarting_services" ]]; then
    return 0
  fi

  echo "Fixing restart loops for services..."

  for service in $restarting_services; do
    local service_name="${service#${project_name}_}"

    # Get last error from logs
    local last_error=$(docker logs "$service" 2>&1 | tail -10)

    case "$service_name" in
      nginx)
        # Fix nginx SSL and config issues
        if echo "$last_error" | grep -q "open() \"/etc/nginx/ssl/ssl.conf\" failed"; then
          # Create minimal ssl.conf that won't conflict
          if [[ ! -f "nginx/conf.d/ssl.conf" ]]; then
            cat >nginx/conf.d/ssl.conf <<'EOF'
# SSL configuration handled in main nginx.conf
# This file exists to prevent include errors
EOF
            fixed_any=true
            echo "  - Created nginx/conf.d/ssl.conf"
          fi
        fi

        if echo "$last_error" | grep -q "listen ... http2.*deprecated"; then
          # Fix deprecated http2 directive in custom-services.conf
          if [[ -f "nginx/conf.d/custom-services.conf" ]]; then
            sed -i '' 's/listen 443 ssl http2;/listen 443 ssl;\n    http2 on;/g' nginx/conf.d/custom-services.conf 2>/dev/null ||
              sed -i.bak 's/listen 443 ssl http2;/listen 443 ssl;\n    http2 on;/g' nginx/conf.d/custom-services.conf 2>/dev/null && rm nginx/conf.d/custom-services.conf.bak
            fixed_any=true
            echo "  - Fixed deprecated http2 directive"
          fi
        fi

        if echo "$last_error" | grep -q "cannot load certificate.*cert.pem"; then
          # Generate missing SSL certificates
          if [[ ! -f "ssl/certificates/localhost/cert.pem" ]]; then
            mkdir -p ssl/certificates/localhost
            openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
              -keyout ssl/certificates/localhost/key.pem \
              -out ssl/certificates/localhost/cert.pem \
              -subj "/C=US/ST=State/L=City/O=Organization/CN=localhost" >/dev/null 2>&1
            fixed_any=true
            echo "  - Generated SSL certificates"
          fi
        fi

        if echo "$last_error" | grep -q "/etc/nginx/ssl/certs/\${BASE_DOMAIN}"; then
          # Fix certificate paths with environment variables
          if [[ -f "nginx/conf.d/custom-services.conf" ]]; then
            sed -i '' 's|/etc/nginx/ssl/certs/${BASE_DOMAIN}/|/etc/nginx/ssl/localhost/|g' nginx/conf.d/custom-services.conf 2>/dev/null ||
              sed -i.bak 's|/etc/nginx/ssl/certs/${BASE_DOMAIN}/|/etc/nginx/ssl/localhost/|g' nginx/conf.d/custom-services.conf 2>/dev/null && rm nginx/conf.d/custom-services.conf.bak
            fixed_any=true
            echo "  - Fixed SSL certificate paths"
          fi
        fi
        ;;

      tempo)
        # Fix Tempo configuration issues
        if echo "$last_error" | grep -q "failed to read configFile /etc/tempo.yaml"; then
          # Create tempo config if missing
          if [[ ! -f "monitoring/tempo/tempo.yaml" ]]; then
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
            fixed_any=true
            echo "  - Created monitoring/tempo/tempo.yaml"
          fi
        fi

        if echo "$last_error" | grep -q "permission denied"; then
          # Fix tempo volume permissions
          docker exec "$service" chown -R 10001:10001 /var/tempo 2>/dev/null || true
          fixed_any=true
          echo "  - Fixed Tempo permissions"
        fi
        ;;

      loki)
        # Fix Loki configuration issues
        if echo "$last_error" | grep -q "schema v13 is required\|tsdb.*is required"; then
          # Update Loki config for new schema
          if [[ -f "monitoring/loki/local-config.yaml" ]]; then
            # Backup original
            cp monitoring/loki/local-config.yaml monitoring/loki/local-config.yaml.bak

            # Update schema version and store type
            sed -i '' 's/schema: v11/schema: v13/g' monitoring/loki/local-config.yaml 2>/dev/null ||
              sed -i.bak 's/schema: v11/schema: v13/g' monitoring/loki/local-config.yaml 2>/dev/null && rm monitoring/loki/local-config.yaml.bak

            sed -i '' 's/store: boltdb-shipper/store: tsdb/g' monitoring/loki/local-config.yaml 2>/dev/null ||
              sed -i.bak 's/store: boltdb-shipper/store: tsdb/g' monitoring/loki/local-config.yaml 2>/dev/null && rm monitoring/loki/local-config.yaml.bak

            # Add limits config if missing
            if ! grep -q "limits_config:" monitoring/loki/local-config.yaml; then
              echo "" >>monitoring/loki/local-config.yaml
              echo "limits_config:" >>monitoring/loki/local-config.yaml
              echo "  allow_structured_metadata: false" >>monitoring/loki/local-config.yaml
            fi

            fixed_any=true
            echo "  - Updated Loki configuration"
          fi
        fi
        ;;

      auth)
        # Fix auth service issues
        if echo "$last_error" | grep -q "connection refused.*4000\|4001"; then
          # Auth service port mismatch - ensure health check uses correct port
          if [[ -f "docker-compose.yml" ]]; then
            # The service runs on 4000, ensure health check uses same port
            sed -i '' 's|http://localhost:4001/|http://localhost:4000/|g' docker-compose.yml 2>/dev/null ||
              sed -i.bak 's|http://localhost:4001/|http://localhost:4000/|g' docker-compose.yml 2>/dev/null && rm docker-compose.yml.bak
            fixed_any=true
            echo "  - Fixed auth service port"
          fi
        fi
        ;;

      *)
        # Generic fixes for other services
        if echo "$last_error" | grep -q "exec.*curl.*not found"; then
          # Replace curl with wget in health checks
          if [[ -f "docker-compose.yml" ]]; then
            # Find the service block and replace curl with wget
            awk -v svc="$service_name" '
              $1 == svc":" { in_service=1 }
              in_service && /healthcheck:/ { in_healthcheck=1 }
              in_healthcheck && /"CMD", "curl"/ {
                gsub(/"CMD", "curl", "-f"/, "\"CMD\", \"wget\", \"--no-verbose\", \"--tries=1\", \"--spider\"")
              }
              in_healthcheck && /retries:/ { in_healthcheck=0; in_service=0 }
              /^[a-z_]+:$/ && !/^  / { in_service=0; in_healthcheck=0 }
              { print }
            ' docker-compose.yml >docker-compose.yml.tmp && mv docker-compose.yml.tmp docker-compose.yml
            fixed_any=true
            echo "  - Fixed health check for $service_name"
          fi
        fi
        ;;
    esac

    # If we fixed something for this service, restart it
    if [[ "$fixed_any" == "true" ]]; then
      docker restart "$service" >/dev/null 2>&1
    fi
  done

  if [[ "$fixed_any" == "true" ]]; then
    echo "Restart loop fixes applied"
    return 0
  else
    echo "Unable to auto-fix restart loops - manual intervention may be needed"
    return 1
  fi
}

# Export for use in other scripts
export -f fix_restart_loops

# Run if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  fix_restart_loops "$@"
fi
