#!/usr/bin/env bash
set -euo pipefail

# services.sh - Service generation for build

# Source platform compatibility functions
source "$(dirname "${BASH_SOURCE[0]}")/../utils/platform-compat.sh" 2>/dev/null || true

# Generate all services
generate_all_services() {
  local force="${1:-false}"

  # Legacy template services - disabled by default
  # Only enable if explicitly set (for backward compatibility)
  if [[ "${TEMPLATES_ENABLED:-false}" == "true" ]]; then
    generate_template_services "$force"
  fi

  # Generate custom services
  generate_custom_services "$force"

  # Generate microservices
  if [[ "${MICROSERVICES_ENABLED:-false}" == "true" ]]; then
    generate_microservices "$force"
  fi

  return 0
}

# Generate services (compatibility alias)
generate_services() {
  generate_all_services "$@"
}

# Generate frontend service
generate_frontend_service() {
  local service_name="${1:-frontend}"
  local port="${2:-3000}"
  local force="${3:-false}"

  local service_file="services/${service_name}.yml"

  if [[ "$force" != "true" ]] && [[ -f "$service_file" ]]; then
    return 0
  fi

  mkdir -p services 2>/dev/null || true

  cat >"$service_file" <<EOF
  ${service_name}:
    build:
      context: ./${service_name}
      dockerfile: Dockerfile
    container_name: \${PROJECT_NAME}_${service_name}
    restart: unless-stopped
    ports:
      - "${port}:${port}"
    environment:
      - NODE_ENV=\${ENV:-development}
      - PORT=${port}
    volumes:
      - ./${service_name}:/app
      - /app/node_modules
    networks:
      - nself_network
EOF

  CREATED_FILES+=("Service: ${service_name}")
}

# Generate backend service
generate_backend_service() {
  local service_name="${1:-backend}"
  local port="${2:-4000}"
  local force="${3:-false}"

  local service_file="services/${service_name}.yml"

  if [[ "$force" != "true" ]] && [[ -f "$service_file" ]]; then
    return 0
  fi

  mkdir -p services 2>/dev/null || true

  cat >"$service_file" <<EOF
  ${service_name}:
    build:
      context: ./${service_name}
      dockerfile: Dockerfile
    container_name: \${PROJECT_NAME}_${service_name}
    restart: unless-stopped
    ports:
      - "${port}:${port}"
    environment:
      - NODE_ENV=\${ENV:-development}
      - PORT=${port}
      - DATABASE_URL=postgresql://\${POSTGRES_USER}:\${POSTGRES_PASSWORD}@postgres:5432/\${POSTGRES_DB}
    volumes:
      - ./${service_name}:/app
      - /app/node_modules
    depends_on:
      - postgres
    networks:
      - nself_network
EOF

  CREATED_FILES+=("Service: ${service_name}")
}

# Generate template services
generate_template_services() {
  local force="${1:-false}"
  local templates_dir="${TEMPLATES_DIR:-src/templates/services}"

  if [[ ! -d "$templates_dir" ]]; then
    return 0
  fi

  # Process service templates
  for template in "$templates_dir"/*.template; do
    if [[ -f "$template" ]]; then
      process_service_template "$template" "$force"
    fi
  done
}

# Process a service template
process_service_template() {
  local template="$1"
  local force="${2:-false}"
  local service_name=$(basename "$template" .template)
  local output_file="services/${service_name}"

  # Check if service should be generated
  local service_var=$(echo "${service_name}_ENABLED" | tr '[:lower:]' '[:upper:]')
  service_var="${service_var//-/_}"

  # Use eval for Bash 3.2 compatibility
  eval "local enabled=\${$service_var:-false}"
  if [[ "$enabled" != "true" ]]; then
    return 0
  fi

  # Create services directory
  mkdir -p services 2>/dev/null || true

  # Check if already exists
  if [[ "$force" != "true" ]] && [[ -f "$output_file" ]]; then
    show_info "Service $service_name already exists (use --force to regenerate)"
    return 0
  fi

  # Process template with variable substitution
  process_template "$template" "$output_file"

  CREATED_FILES+=("Service: $service_name")
}

# Process template with variable substitution
process_template() {
  local template="$1"
  local output="$2"
  local temp_file=$(mktemp)

  # Read template and substitute variables
  while IFS= read -r line || [[ -n "$line" ]]; do
    # Replace ${VAR} with actual values
    while [[ "$line" =~ \$\{([A-Z_][A-Z0-9_]*)\} ]]; do
      local var_name="${BASH_REMATCH[1]}"
      # Use eval for Bash 3.2 compatibility
      eval "local var_value=\${$var_name:-}"
      line="${line//\$\{${var_name}\}/$var_value}"
    done
    echo "$line"
  done <"$template" >"$temp_file"

  # Move to final location
  mv "$temp_file" "$output"
}

# Generate a single custom service from template
generate_custom_service() {
  local service_name="$1"
  local force="${2:-false}"

  # Look for CS_N environment variables
  local cs_num=1
  while true; do
    local cs_var="CS_${cs_num}"
    local cs_value="${!cs_var:-}"

    if [[ -z "$cs_value" ]]; then
      break
    fi

    # Parse CS_N format: service_name:template_type:port
    IFS=':' read -r configured_name template_type port <<<"$cs_value"

    # Match by name or by CS number
    if [[ "$configured_name" == "$service_name" ]]; then
      # Copy template to services directory if it doesn't exist
      local service_dir="services/$configured_name"

      if [[ "$force" != "true" ]] && [[ -d "$service_dir" ]] && [[ -n "$(ls -A "$service_dir" 2>/dev/null)" ]]; then
        # Skip if directory exists and has files (user may have customized)
        echo "Service '$configured_name' already exists, skipping template copy"
        return 0
      fi

      # Find the template
      local template_root="${NSELF_ROOT:-/usr/local/lib/nself}"
      local template_dir="$template_root/src/templates/services"
      local template_found=false

      # Look for template in language directories
      for lang_dir in js python go rust; do
        if [[ -d "$template_dir/$lang_dir/$template_type" ]]; then
          # Found the template, copy it
          echo "Copying template '$template_type' to services/$configured_name"
          mkdir -p "$service_dir"
          cp -r "$template_dir/$lang_dir/$template_type"/* "$service_dir/" 2>/dev/null || true

          # Replace placeholders in copied files
          find "$service_dir" -type f \( -name "*.js" -o -name "*.ts" -o -name "*.py" -o -name "*.go" -o -name "*.json" -o -name "*.yml" -o -name "*.yaml" -o -name "Dockerfile" \) | while read file; do
            safe_sed_inline "$file" \
              -e "s/{{SERVICE_NAME}}/$configured_name/g" \
              -e "s/{{SERVICE_PORT}}/$port/g" \
              -e "s/{{PROJECT_NAME}}/${PROJECT_NAME}/g"
          done

          template_found=true
          echo "Created service '$configured_name' from template '$template_type'"
          break
        fi
      done

      if [[ "$template_found" != "true" ]]; then
        echo "Warning: Template '$template_type' not found for service '$configured_name'"
      fi

      return 0
    fi

    cs_num=$((cs_num + 1))
  done
}

# Generate custom service from template
generate_custom_service_from_template() {
  local service_name="$1"
  local template_type="$2"
  local port="$3"
  local force="${4:-false}"

  # Skip if directory exists and has files (user may have customized)
  local service_dir="services/$service_name"
  if [[ "$force" != "true" ]] && [[ -d "$service_dir" ]] && [[ -n "$(ls -A "$service_dir" 2>/dev/null)" ]]; then
    [[ "${VERBOSE:-false}" == "true" ]] && echo "Service '$service_name' already exists, skipping template copy"
    return 0
  fi

  # Find the template
  local template_root="${NSELF_ROOT:-/usr/local/lib/nself}"
  local template_dir="$template_root/src/templates/services"
  local template_found=false

  # Look for template in language directories
  for lang_dir in js py go rust; do
    if [[ -d "$template_dir/$lang_dir/$template_type" ]]; then
      # Found the template, copy it
      [[ "${VERBOSE:-false}" == "true" ]] && echo "Copying template '$template_type' to services/$service_name"
      mkdir -p "$service_dir"
      cp -r "$template_dir/$lang_dir/$template_type"/* "$service_dir/" 2>/dev/null || true

      # Replace placeholders in all template files
      find "$service_dir" -type f -name "*.template" | while read template_file; do
        # Replace placeholders
        safe_sed_inline "$template_file" \
          -e "s/{{SERVICE_NAME}}/$service_name/g" \
          -e "s/{{SERVICE_PORT}}/$port/g" \
          -e "s/{{PORT}}/$port/g" \
          -e "s/{{PROJECT_NAME}}/${PROJECT_NAME}/g" \
          -e "s/{{BASE_DOMAIN}}/${BASE_DOMAIN:-localhost}/g"

        # Rename file to remove .template extension
        base_name="${template_file%.template}"
        mv "$template_file" "$base_name"
      done

      template_found=true
      [[ "${VERBOSE:-false}" == "true" ]] && echo "Created service '$service_name' from template '$template_type'"
      break
    fi
  done

  if [[ "$template_found" != "true" ]]; then
    echo "Warning: Template '$template_type' not found"
    return 1
  fi

  return 0
}

# Generate custom services
generate_custom_services() {
  local force="${1:-false}"

  # Check for CS_N environment variables
  local cs_num=1
  while true; do
    local cs_var="CS_${cs_num}"
    local cs_value="${!cs_var:-}"

    if [[ -z "$cs_value" ]]; then
      break
    fi

    # Parse CS_N format: service_name:template_type:port
    IFS=':' read -r service_name template_type port <<<"$cs_value"

    # Generate the service from template
    generate_custom_service_from_template "$service_name" "$template_type" "$port" "$force"

    cs_num=$((cs_num + 1))
  done

  # Check for Next.js app
  if [[ -f "package.json" ]] && grep -q "next" package.json 2>/dev/null; then
    generate_nextjs_service "$force"
  fi

  # Check for React app
  if [[ -f "package.json" ]] && grep -q "react" package.json 2>/dev/null; then
    if ! grep -q "next" package.json 2>/dev/null; then
      generate_react_service "$force"
    fi
  fi

  # Check for Node.js app
  if [[ -f "package.json" ]] && [[ -f "index.js" || -f "server.js" || -f "app.js" ]]; then
    generate_nodejs_service "$force"
  fi

  # Check for Python app
  if [[ -f "requirements.txt" ]] || [[ -f "pyproject.toml" ]]; then
    generate_python_service "$force"
  fi

  # Check for Go app
  if [[ -f "go.mod" ]]; then
    generate_go_service "$force"
  fi
}

# Generate Next.js service
generate_nextjs_service() {
  local force="${1:-false}"
  local service_file="services/nextjs.yml"

  if [[ "$force" != "true" ]] && [[ -f "$service_file" ]]; then
    return 0
  fi

  mkdir -p services 2>/dev/null || true

  cat >"$service_file" <<EOF
  nextjs:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: \${PROJECT_NAME}_nextjs
    restart: unless-stopped
    ports:
      - "\${NEXTJS_PORT:-3000}:3000"
    environment:
      NODE_ENV: \${NODE_ENV:-development}
      DATABASE_URL: postgres://\${POSTGRES_USER:-postgres}:\${POSTGRES_PASSWORD:-postgres}@postgres:5432/\${POSTGRES_DB:-\${PROJECT_NAME}}
    volumes:
      - ./:/app
      - /app/node_modules
      - /app/.next
    networks:
      - nself_network
    depends_on:
      - postgres
EOF

  # Create Dockerfile if not exists
  if [[ ! -f "Dockerfile" ]]; then
    create_nextjs_dockerfile
  fi

  CREATED_FILES+=("Service: Next.js")
}

# Create Next.js Dockerfile
create_nextjs_dockerfile() {
  cat >Dockerfile <<'EOF'
FROM node:18-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json* yarn.lock* pnpm-lock.yaml* ./
RUN \
  if [ -f yarn.lock ]; then yarn install --frozen-lockfile; \
  elif [ -f package-lock.json ]; then npm ci; \
  elif [ -f pnpm-lock.yaml ]; then yarn global add pnpm && pnpm i; \
  else echo "Warning: Lockfile not found." && npm i; \
  fi

FROM node:18-alpine AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npm run build

FROM node:18-alpine AS runner
WORKDIR /app
ENV NODE_ENV production
COPY --from=builder /app/public ./public
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static

EXPOSE 3000
ENV PORT 3000
CMD ["node", "server.js"]
EOF
}

# Generate React service
generate_react_service() {
  local force="${1:-false}"
  local service_file="services/react.yml"

  if [[ "$force" != "true" ]] && [[ -f "$service_file" ]]; then
    return 0
  fi

  mkdir -p services 2>/dev/null || true

  cat >"$service_file" <<EOF
  react:
    build:
      context: .
      dockerfile: Dockerfile.react
    container_name: \${PROJECT_NAME}_react
    restart: unless-stopped
    ports:
      - "\${REACT_PORT:-3000}:80"
    volumes:
      - ./build:/usr/share/nginx/html:ro
    networks:
      - nself_network
EOF

  # Create Dockerfile if not exists
  if [[ ! -f "Dockerfile.react" ]]; then
    create_react_dockerfile
  fi

  CREATED_FILES+=("Service: React")
}

# Create React Dockerfile
create_react_dockerfile() {
  cat >Dockerfile.react <<'EOF'
FROM node:18-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/build /usr/share/nginx/html
COPY nginx.conf /etc/nginx/nginx.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
EOF
}

# Generate Node.js service
generate_nodejs_service() {
  local force="${1:-false}"
  local service_file="services/nodejs.yml"

  if [[ "$force" != "true" ]] && [[ -f "$service_file" ]]; then
    return 0
  fi

  mkdir -p services 2>/dev/null || true

  # Detect main file
  local main_file="index.js"
  [[ -f "server.js" ]] && main_file="server.js"
  [[ -f "app.js" ]] && main_file="app.js"

  cat >"$service_file" <<EOF
  nodejs:
    build:
      context: .
      dockerfile: Dockerfile.node
    container_name: \${PROJECT_NAME}_nodejs
    restart: unless-stopped
    ports:
      - "\${NODE_PORT:-3000}:3000"
    environment:
      NODE_ENV: \${NODE_ENV:-development}
      DATABASE_URL: postgres://\${POSTGRES_USER:-postgres}:\${POSTGRES_PASSWORD:-postgres}@postgres:5432/\${POSTGRES_DB:-\${PROJECT_NAME}}
    volumes:
      - ./:/app
      - /app/node_modules
    networks:
      - nself_network
    command: node ${main_file}
EOF

  CREATED_FILES+=("Service: Node.js")
}

# Generate Python service
generate_python_service() {
  local force="${1:-false}"
  local service_file="services/python.yml"

  if [[ "$force" != "true" ]] && [[ -f "$service_file" ]]; then
    return 0
  fi

  mkdir -p services 2>/dev/null || true

  # Detect Python framework
  local framework="flask"
  local main_file="app.py"

  if grep -q "django" requirements.txt 2>/dev/null; then
    framework="django"
    main_file="manage.py"
  elif grep -q "fastapi" requirements.txt 2>/dev/null; then
    framework="fastapi"
    main_file="main.py"
  fi

  cat >"$service_file" <<EOF
  python:
    build:
      context: .
      dockerfile: Dockerfile.python
    container_name: \${PROJECT_NAME}_python
    restart: unless-stopped
    ports:
      - "\${PYTHON_PORT:-8000}:8000"
    environment:
      DATABASE_URL: postgres://\${POSTGRES_USER:-postgres}:\${POSTGRES_PASSWORD:-postgres}@postgres:5432/\${POSTGRES_DB:-\${PROJECT_NAME}}
    volumes:
      - ./:/app
    networks:
      - nself_network
EOF

  CREATED_FILES+=("Service: Python")
}

# Generate Go service
generate_go_service() {
  local force="${1:-false}"
  local service_file="services/go.yml"

  if [[ "$force" != "true" ]] && [[ -f "$service_file" ]]; then
    return 0
  fi

  mkdir -p services 2>/dev/null || true

  cat >"$service_file" <<EOF
  go:
    build:
      context: .
      dockerfile: Dockerfile.go
    container_name: \${PROJECT_NAME}_go
    restart: unless-stopped
    ports:
      - "\${GO_PORT:-8080}:8080"
    environment:
      DATABASE_URL: postgres://\${POSTGRES_USER:-postgres}:\${POSTGRES_PASSWORD:-postgres}@postgres:5432/\${POSTGRES_DB:-\${PROJECT_NAME}}
    networks:
      - nself_network
EOF

  CREATED_FILES+=("Service: Go")
}

# Generate microservices
generate_microservices() {
  local force="${1:-false}"

  if [[ -n "${MICROSERVICES:-}" ]]; then
    IFS=',' read -ra SERVICES <<<"$MICROSERVICES"
    for service in "${SERVICES[@]}"; do
      generate_microservice "$service" "$force"
    done
  fi
}

# Generate individual microservice
generate_microservice() {
  local service_name="$1"
  local force="${2:-false}"
  local service_file="services/${service_name}.yml"

  if [[ "$force" != "true" ]] && [[ -f "$service_file" ]]; then
    return 0
  fi

  mkdir -p services 2>/dev/null || true

  local service_upper=$(echo "$service_name" | tr '[:lower:]' '[:upper:]')
  local port_var="${service_upper}_PORT"
  # Use eval for Bash 3.2 compatibility
  eval "local port=\${$port_var:-3000}"

  cat >"$service_file" <<EOF
  ${service_name}:
    image: \${${service_upper}_IMAGE:-${service_name}:latest}
    container_name: \${PROJECT_NAME}_${service_name}
    restart: unless-stopped
    ports:
      - "\${${service_upper}_PORT:-${port}}:${port}"
    environment:
      NODE_ENV: \${NODE_ENV:-development}
      DATABASE_URL: postgres://\${POSTGRES_USER:-postgres}:\${POSTGRES_PASSWORD:-postgres}@postgres:5432/\${POSTGRES_DB:-\${PROJECT_NAME}}
    networks:
      - nself_network
EOF

  CREATED_FILES+=("Microservice: $service_name")
}

# Export functions
export -f generate_all_services
export -f generate_template_services
export -f generate_all_services
export -f generate_services
export -f generate_frontend_service
export -f generate_backend_service
export -f generate_template_services
export -f process_service_template
export -f process_template
export -f generate_custom_service
export -f generate_custom_services
export -f generate_nextjs_service
export -f generate_react_service
export -f generate_nodejs_service
export -f generate_python_service
export -f generate_go_service
export -f generate_microservices
export -f generate_microservice
