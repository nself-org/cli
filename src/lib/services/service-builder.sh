#!/usr/bin/env bash

# service-builder.sh - Build custom services from CS_N pattern

# Source utilities
# Use BASH_SOURCE[0] to get the actual file location, not where it's sourced from
SERVICE_BUILDER_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

SCRIPT_DIR="$SERVICE_BUILDER_DIR"

# Only source if not already loaded (avoid re-sourcing in nested calls)
if [[ -z "${DISPLAY_SOURCED:-}" ]]; then
  source "$SCRIPT_DIR/../utils/display.sh"
fi
if [[ -z "${ENV_UTILS_SOURCED:-}" ]]; then
  source "$SCRIPT_DIR/../utils/env.sh"
  export ENV_UTILS_SOURCED=1
fi

# Parse CS_N services
parse_cs_services() {
  # Clear previous parsed services
  PARSED_SERVICES=()

  # Find all CS_N variables (CS_1, CS_2, etc.)
  local service_num=1
  while true; do
    local service_def=$(eval echo "\${CS_${service_num}:-}")

    # Stop if no more services
    if [[ -z "$service_def" ]]; then
      break
    fi

    # Parse CS_N=name:framework[:port][:route]
    IFS=':' read -r name framework port route <<<"$service_def"

    # Trim whitespace
    name=$(echo "$name" | xargs)
    framework=$(echo "$framework" | xargs)
    port=$(echo "${port:-}" | xargs)
    route=$(echo "${route:-}" | xargs)

    # Convert name to uppercase for env var lookups
    local name_upper=$(echo "$name" | tr '[:lower:]' '[:upper:]')

    # Get configuration from CS_N_* variables
    port="${port:-$(eval echo "\${CS_${service_num}_PORT:-}")}"
    route="${route:-$(eval echo "\${CS_${service_num}_ROUTE:-}")}"

    # Auto-assign port if not set
    if [[ -z "$port" ]]; then
      port=$((8000 + service_num))
    fi

    # Don't default route - leave empty if not specified
    # Empty route means internal-only service

    # Get all CS_N_* configuration
    local memory=$(eval echo "\${CS_${service_num}_MEMORY:-256M}")
    local cpu=$(eval echo "\${CS_${service_num}_CPU:-0.25}")
    local replicas=$(eval echo "\${CS_${service_num}_REPLICAS:-1}")
    local table_prefix=$(eval echo "\${CS_${service_num}_TABLE_PREFIX:-}")
    local redis_prefix=$(eval echo "\${CS_${service_num}_REDIS_PREFIX:-}")

    # Auto-determine public based on route presence
    # If route is specified, default to public=true, otherwise false
    local public_default="false"
    if [[ -n "$route" ]]; then
      public_default="true"
    fi
    local public=$(eval echo "\${CS_${service_num}_PUBLIC:-$public_default}")

    local healthcheck=$(eval echo "\${CS_${service_num}_HEALTHCHECK:-/health}")
    local env_vars=$(eval echo "\${CS_${service_num}_ENV:-}")
    local rate_limit=$(eval echo "\${CS_${service_num}_RATE_LIMIT:-}")

    # Get environment-specific domain
    local current_env="${ENV:-dev}"
    local domain=""

    # Only process domain if route is specified
    if [[ -n "$route" ]]; then
      # Check for environment-specific domain override
      if [[ "$current_env" == "prod" ]] || [[ "$current_env" == "production" ]]; then
        domain=$(eval echo "\${CS_${service_num}_PROD_DOMAIN:-}")
      else
        domain=$(eval echo "\${CS_${service_num}_DEV_DOMAIN:-}")
      fi

      # Process route to determine final domain
      if [[ -z "$domain" ]]; then
        # Check if route is a full domain (contains at least one dot and ends with TLD)
        if [[ "$route" =~ ^[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$ ]]; then
          # Full domain provided (e.g., api.example.com or myapi.some.domain.com)
          domain="$route"
        elif [[ "$route" =~ ^[a-zA-Z0-9-]+$ ]]; then
          # Single word - make it a subdomain of BASE_DOMAIN
          domain="${route}.${BASE_DOMAIN}"
        else
          # Anything else, use as-is (could be multi-level like api.v2)
          domain="${route}.${BASE_DOMAIN}"
        fi
      fi
    fi

    # Store parsed service
    PARSED_SERVICES+=("$name|$framework|$domain|$port|$replicas|$memory|$cpu|$env_vars|$healthcheck|$public|$rate_limit|$table_prefix|$redis_prefix")

    # Increment service number
    ((service_num++))
  done

  # Also check for legacy CUSTOM_SERVICES if no CS_N found
  if [[ ${#PARSED_SERVICES[@]} -eq 0 ]] && [[ -n "${CUSTOM_SERVICES:-}" ]]; then
    parse_legacy_custom_services
  fi
}

# Parse legacy CUSTOM_SERVICES format (backward compatibility)
parse_legacy_custom_services() {
  local services_def="${CUSTOM_SERVICES:-}"

  if [[ -z "$services_def" ]]; then
    return 0
  fi

  # Split by comma
  IFS=',' read -ra SERVICES <<<"$services_def"

  for service in "${SERVICES[@]}"; do
    # Trim whitespace
    service=$(echo "$service" | xargs)

    # Parse SERVICE_NAME:LANGUAGE:ROUTING
    IFS=':' read -r name language routing <<<"$service"

    # Default routing to service name if not provided
    routing="${routing:-$name}"

    # Convert to uppercase for env var lookups
    name_upper=$(echo "$name" | tr '[:lower:]' '[:upper:]')

    # Get service-specific configuration
    local port=$(eval echo "\${${name_upper}_PORT:-}")
    local replicas=$(eval echo "\${${name_upper}_REPLICAS:-1}")
    local memory=$(eval echo "\${${name_upper}_MEMORY:-256M}")
    local cpu=$(eval echo "\${${name_upper}_CPU:-0.25}")
    local env_vars=$(eval echo "\${${name_upper}_ENV:-}")
    local healthcheck=$(eval echo "\${${name_upper}_HEALTHCHECK:-/health}")
    local public=$(eval echo "\${${name_upper}_PUBLIC:-true}")
    local rate_limit=$(eval echo "\${${name_upper}_RATE_LIMIT:-}")

    # Auto-assign port if not set
    if [[ -z "$port" ]]; then
      port=$((8000 + ${#PARSED_SERVICES[@]}))
    fi

    # Determine final domain
    local final_domain="${routing}.${BASE_DOMAIN}"

    # Store parsed service
    PARSED_SERVICES+=("$name|$language|$final_domain|$port|$replicas|$memory|$cpu|$env_vars|$healthcheck|$public|$rate_limit||")
  done
}

# Generate docker-compose service definition
generate_service_compose() {
  local service_info="$1"
  IFS='|' read -r name framework domain port replicas memory cpu env_vars healthcheck public rate_limit table_prefix redis_prefix <<<"$service_info"

  cat <<EOF

  ${name}:
    build: 
      context: ./services/${name}
      dockerfile: Dockerfile
      args:
        - SERVICE_NAME=${name}
        - PROJECT_NAME=\${PROJECT_NAME}
        - PORT=${port}
    container_name: \${PROJECT_NAME}_${name}
    restart: unless-stopped
    networks:
      - default
EOF

  # Only expose ports if service is public or explicitly configured
  if [[ "$public" == "true" ]] || [[ -n "$domain" ]]; then
    cat <<EOF
    ports:
      - "${port}:${port}"
EOF
  fi

  cat <<EOF
    environment:
      - SERVICE_NAME=${name}
      - PROJECT_NAME=\${PROJECT_NAME}
      - ENV=\${ENV}
      - PORT=${port}
      - BASE_DOMAIN=\${BASE_DOMAIN}
      - DATABASE_URL=postgresql://\${POSTGRES_USER}:\${POSTGRES_PASSWORD}@postgres:5432/\${POSTGRES_DB}
      - POSTGRES_HOST=postgres
      - POSTGRES_PORT=5432
      - POSTGRES_DB=\${POSTGRES_DB}
      - POSTGRES_USER=\${POSTGRES_USER}
      - POSTGRES_PASSWORD=\${POSTGRES_PASSWORD}
      - REDIS_ENABLED=\${REDIS_ENABLED:-false}
      - REDIS_HOST=redis
      - REDIS_URL=redis://redis:6379
EOF

  # Add BASE_URL only if domain is specified
  if [[ -n "$domain" ]]; then
    echo "      - BASE_URL=https://${domain}"
  fi

  # Add table prefix if specified
  if [[ -n "$table_prefix" ]]; then
    echo "      - TABLE_PREFIX=${table_prefix}"
  fi

  # Add Redis prefix if specified
  if [[ -n "$redis_prefix" ]]; then
    echo "      - REDIS_PREFIX=${redis_prefix}"
  fi

  # Add custom environment variables
  if [[ -n "$env_vars" ]]; then
    IFS=',' read -ra ENVS <<<"$env_vars"
    for env in "${ENVS[@]}"; do
      echo "      - $env"
    done
  fi

  # Add resource limits
  cat <<EOF
    deploy:
      replicas: ${replicas}
      resources:
        limits:
          memory: ${memory}
          cpus: '${cpu}'
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:${port}${healthcheck}"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
    depends_on:
      - postgres
EOF

  # Add Redis dependency if enabled
  if [[ "${REDIS_ENABLED:-false}" == "true" ]]; then
    echo "      - redis"
  fi

  # Add volumes for development
  if [[ "${ENV:-dev}" == "dev" ]]; then
    cat <<EOF
    volumes:
      - ./services/${name}:/app
      - /app/node_modules  # Prevent node_modules from being overwritten
EOF
  fi
}

# Generate Nginx configuration for service
generate_service_nginx() {
  local service_info="$1"
  IFS='|' read -r name framework domain port replicas memory cpu env_vars healthcheck public rate_limit table_prefix redis_prefix <<<"$service_info"

  # Skip if not public or no domain specified
  if [[ "$public" != "true" ]] || [[ -z "$domain" ]]; then
    return
  fi

  cat <<EOF

# ${name} Service (${framework})
server {
    listen 80;
    listen 443 ssl http2;
    server_name ${domain};

    # SSL Configuration
    include /etc/nginx/ssl/ssl.conf;
    ssl_certificate /etc/nginx/ssl/certs/\${BASE_DOMAIN}/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/certs/\${BASE_DOMAIN}/privkey.pem;

    # Security Headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;

EOF

  # Add rate limiting if configured
  if [[ -n "$rate_limit" ]]; then
    cat <<EOF
    # Rate Limiting
    limit_req_zone \$binary_remote_addr zone=${name}_limit:10m rate=${rate_limit}r/m;
    limit_req zone=${name}_limit burst=10 nodelay;
    limit_req_status 429;

EOF
  fi

  cat <<EOF
    # Proxy Configuration
    location / {
        proxy_pass http://${name}:${port};
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_cache_bypass \$http_upgrade;
        
        # Timeouts
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
        
        # Buffering
        proxy_buffering off;
        proxy_request_buffering off;
    }
    
    # Health check endpoint
    location ${healthcheck} {
        access_log off;
        proxy_pass http://${name}:${port}${healthcheck};
    }
}
EOF
}

# Create service from template
create_service_from_template() {
  local service_info="$1"

  # Debug output
  if [[ "${DEBUG:-false}" == "true" ]]; then
    echo "DEBUG: create_service_from_template called with: $service_info" >&2
  fi

  IFS='|' read -r name framework domain port replicas memory cpu env_vars healthcheck public rate_limit table_prefix redis_prefix <<<"$service_info"

  # Map framework aliases to actual template directories
  case "$framework" in
    nodejs | node)
      framework="js"
      ;;
    python)
      framework="py"
      ;;
    golang)
      framework="go"
      ;;
    rust)
      framework="rs"
      ;;
    ruby)
      framework="rb"
      ;;
    dotnet | csharp)
      framework="cs"
      ;;
    typescript)
      framework="ts"
      ;;
    elixir)
      framework="ex"
      ;;
  esac

  local service_dir="./services/${name}"
  local template_dir=""

  # Determine language directory based on framework suffix
  local lang_dir=""
  case "$framework" in
    *-ts | *-js | trpc | bun | deno | node-js | node-ts)
      lang_dir="js"
      ;;
    fastapi | flask | django-rest | celery | ray | agent-*)
      lang_dir="py"
      ;;
    gin | echo | fiber | grpc)
      lang_dir="go"
      ;;
    rails | sinatra)
      lang_dir="ruby"
      ;;
    actix-web)
      lang_dir="rust"
      ;;
    spring-boot)
      lang_dir="java"
      ;;
    aspnet)
      lang_dir="csharp"
      ;;
    laravel)
      lang_dir="php"
      ;;
    phoenix)
      lang_dir="elixir"
      ;;
    ktor)
      lang_dir="kotlin"
      ;;
    vapor)
      lang_dir="swift"
      ;;
    *)
      # Try to find in any language directory
      for dir in js py go ruby rust java csharp php elixir kotlin swift cpp lua zig; do
        if [[ -d "$SERVICE_BUILDER_DIR/../../templates/services/$dir/$framework" ]]; then
          lang_dir="$dir"
          break
        fi
      done
      ;;
  esac

  # Set template directory
  if [[ -n "$lang_dir" ]]; then
    template_dir="$SERVICE_BUILDER_DIR/../../templates/services/${lang_dir}/${framework}"
  fi

  # Debug output
  if [[ "${DEBUG:-false}" == "true" ]]; then
    echo "DEBUG: SERVICE_BUILDER_DIR=$SERVICE_BUILDER_DIR" >&2
    echo "DEBUG: lang_dir=$lang_dir, framework=$framework" >&2
    echo "DEBUG: template_dir=$template_dir" >&2
    echo "DEBUG: Checking if template exists: $template_dir" >&2
    ls -la "$template_dir" 2>&1 >&2 || echo "DEBUG: Template dir does not exist" >&2
  fi

  # Check if template exists
  if [[ ! -d "$template_dir" ]]; then
    log_warning "No template found for framework: ${framework}"
    log_info "Creating generic service directory for ${name}"
    mkdir -p "$service_dir"
    return 1
  fi

  # Create service directory
  if [[ ! -d "$service_dir" ]]; then
    mkdir -p "$service_dir"
    log_info "Creating ${framework} service: ${name}"

    # Debug output
    if [[ "${DEBUG:-false}" == "true" ]]; then
      echo "DEBUG: Copying from $template_dir to $service_dir" >&2
      echo "DEBUG: Template contents:" >&2
      ls -la "$template_dir" >&2
    fi

    # Copy template files
    cp -r "$template_dir"/* "$service_dir/" 2>&1 | while read line; do
      [[ "${DEBUG:-false}" == "true" ]] && echo "DEBUG: cp output: $line" >&2
    done
    cp "$template_dir"/.* "$service_dir/" 2>/dev/null || true

    # Debug: Check what was copied
    if [[ "${DEBUG:-false}" == "true" ]]; then
      echo "DEBUG: After copy, service_dir contents:" >&2
      ls -la "$service_dir" >&2
    fi

    # Process .template files
    for template_file in "$service_dir"/*.template "$service_dir"/**/*.template; do
      if [[ -f "$template_file" ]]; then
        # Debug output
        if [[ "${DEBUG:-false}" == "true" ]]; then
          echo "DEBUG: Processing template: $template_file" >&2
        fi

        # Remove .template extension
        output_file="${template_file%.template}"

        # Debug: Check variables
        if [[ "${DEBUG:-false}" == "true" ]]; then
          echo "DEBUG: name=$name, PROJECT_NAME=$PROJECT_NAME, port=$port, BASE_DOMAIN=$BASE_DOMAIN" >&2
        fi

        # Replace placeholders and save to final file
        # Support both ${VAR} and {{VAR}} syntax
        sed \
          -e "s/\${SERVICE_NAME}/${name}/g" \
          -e "s/{{SERVICE_NAME}}/${name}/g" \
          -e "s/\${PROJECT_NAME}/${PROJECT_NAME}/g" \
          -e "s/{{PROJECT_NAME}}/${PROJECT_NAME}/g" \
          -e "s/\${PORT}/${port}/g" \
          -e "s/{{PORT}}/${port}/g" \
          -e "s/\${BASE_DOMAIN}/${BASE_DOMAIN}/g" \
          -e "s/{{BASE_DOMAIN}}/${BASE_DOMAIN}/g" \
          "$template_file" >"$output_file"

        # Debug: Check output file
        if [[ "${DEBUG:-false}" == "true" ]]; then
          echo "DEBUG: Created $output_file with size: $(wc -c <"$output_file") bytes" >&2
        fi

        # Remove template file
        rm "$template_file"
      fi
    done

    # Replace placeholders in non-template files
    # Support both ${VAR} and {{VAR}} syntax
    find "$service_dir" -type f ! -name "*.template" -exec sed -i.bak \
      -e "s/\${SERVICE_NAME}/${name}/g" \
      -e "s/{{SERVICE_NAME}}/${name}/g" \
      -e "s/\${PROJECT_NAME}/${PROJECT_NAME}/g" \
      -e "s/{{PROJECT_NAME}}/${PROJECT_NAME}/g" \
      -e "s/\${PORT}/${port}/g" \
      -e "s/{{PORT}}/${port}/g" \
      -e "s/\${BASE_DOMAIN}/${BASE_DOMAIN}/g" \
      -e "s/{{BASE_DOMAIN}}/${BASE_DOMAIN}/g" \
      {} \; 2>/dev/null || true

    # Remove backup files
    find "$service_dir" -name "*.bak" -delete 2>/dev/null || true

    # Validate that template variables were replaced
    if grep -r "{{SERVICE_NAME}}\|{{PROJECT_NAME}}\|{{PORT}}\|{{BASE_DOMAIN}}" "$service_dir" --exclude="*.bak" >/dev/null 2>&1; then
      log_warning "Unreplaced template variables found in ${name} service:"
      grep -r "{{SERVICE_NAME}}\|{{PROJECT_NAME}}\|{{PORT}}\|{{BASE_DOMAIN}}" "$service_dir" --exclude="*.bak" | head -5 >&2
    fi

    log_success "Created ${framework} service template for ${name}"
  else
    log_info "Service directory already exists: ${service_dir}"
  fi
}

# Build all custom services
build_custom_services() {
  # Debug output
  if [[ "${DEBUG:-false}" == "true" ]]; then
    echo "DEBUG: build_custom_services called" >&2
  fi

  # Parse CS_N services
  parse_cs_services

  if [[ ${#PARSED_SERVICES[@]} -eq 0 ]]; then
    return 0
  fi

  log_info "Building ${#PARSED_SERVICES[@]} custom service(s)..."

  # Generate docker-compose.custom.yml
  {
    echo "# Custom Services Configuration"
    echo "# Auto-generated by nself from CS_N variables"
    echo ""
    echo "services:"

    for service_info in "${PARSED_SERVICES[@]}"; do
      generate_service_compose "$service_info"
    done

    echo ""
    echo "networks:"
    echo "  default:"
    echo "    external: true"
    echo "    name: \${PROJECT_NAME}_network"
  } >docker-compose.custom.yml

  # Generate nginx configurations
  local nginx_config=""
  for service_info in "${PARSED_SERVICES[@]}"; do
    nginx_config+=$(generate_service_nginx "$service_info")
  done

  # Write to nginx custom services config
  if [[ -n "$nginx_config" ]]; then
    mkdir -p ./nginx/conf.d
    echo "$nginx_config" >./nginx/conf.d/custom-services.conf
    log_success "Generated Nginx configuration for custom services"
  fi

  # Create service templates
  for service_info in "${PARSED_SERVICES[@]}"; do
    create_service_from_template "$service_info"
  done

  log_success "Custom services configuration complete"

  # Show summary
  echo ""
  echo "Custom Services Summary:"
  echo "========================"
  for service_info in "${PARSED_SERVICES[@]}"; do
    IFS='|' read -r name framework domain port replicas memory cpu env_vars healthcheck public rate_limit table_prefix redis_prefix <<<"$service_info"
    if [[ -n "$domain" ]]; then
      echo "  • ${name} (${framework}) → https://${domain}"
    else
      # Worker services without public routes
      echo "  • ${name} (${framework}) → Internal service on port ${port}"
    fi
  done
  echo ""

  echo "Next steps:"
  echo "  1. Edit service code in ./services/<name>/"
  echo "  2. Run 'nself build' to rebuild containers"
  echo "  3. Run 'nself start' to start services"
}

# Export functions
export -f parse_cs_services
export -f build_custom_services
