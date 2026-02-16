#!/usr/bin/env bash

# detection.sh - Project detection and analysis

# Global variables for detection results
DETECTED_FRAMEWORK=""

set -euo pipefail

DETECTED_LANGUAGE=""
DETECTED_DATABASE=""
DETECTED_SERVICES=()

# Detect project framework
detect_project_framework() {
  DETECTED_FRAMEWORK=""
  DETECTED_LANGUAGE=""

  # JavaScript/TypeScript detection
  if [[ -f "package.json" ]]; then
    DETECTED_LANGUAGE="JavaScript/TypeScript"

    # Read package.json
    if command -v jq >/dev/null 2>&1; then
      local deps=$(jq -r '.dependencies // {} | keys[]' package.json 2>/dev/null)
      local devDeps=$(jq -r '.devDependencies // {} | keys[]' package.json 2>/dev/null)
      local allDeps="$deps $devDeps"
    else
      # Fallback to grep
      local allDeps=$(grep -E '"(next|react|vue|angular|express|fastify|gatsby|nuxt)"' package.json)
    fi

    # Check for specific frameworks
    if echo "$allDeps" | grep -q "next"; then
      DETECTED_FRAMEWORK="Next.js"
    elif echo "$allDeps" | grep -q "@angular"; then
      DETECTED_FRAMEWORK="Angular"
    elif echo "$allDeps" | grep -q "vue"; then
      DETECTED_FRAMEWORK="Vue.js"
      if echo "$allDeps" | grep -q "nuxt"; then
        DETECTED_FRAMEWORK="Nuxt.js"
      fi
    elif echo "$allDeps" | grep -q "react"; then
      DETECTED_FRAMEWORK="React"
      if echo "$allDeps" | grep -q "gatsby"; then
        DETECTED_FRAMEWORK="Gatsby"
      fi
    elif echo "$allDeps" | grep -q "express"; then
      DETECTED_FRAMEWORK="Express.js"
    elif echo "$allDeps" | grep -q "fastify"; then
      DETECTED_FRAMEWORK="Fastify"
    fi
  fi

  # Python detection
  if [[ -f "requirements.txt" ]] || [[ -f "Pipfile" ]] || [[ -f "pyproject.toml" ]]; then
    DETECTED_LANGUAGE="Python"

    if [[ -f "manage.py" ]] || grep -q "django" requirements.txt 2>/dev/null; then
      DETECTED_FRAMEWORK="Django"
    elif grep -q "flask" requirements.txt 2>/dev/null; then
      DETECTED_FRAMEWORK="Flask"
    elif grep -q "fastapi" requirements.txt 2>/dev/null; then
      DETECTED_FRAMEWORK="FastAPI"
    fi
  fi

  # Ruby detection
  if [[ -f "Gemfile" ]]; then
    DETECTED_LANGUAGE="Ruby"

    if grep -q "rails" Gemfile 2>/dev/null; then
      DETECTED_FRAMEWORK="Ruby on Rails"
    elif grep -q "sinatra" Gemfile 2>/dev/null; then
      DETECTED_FRAMEWORK="Sinatra"
    fi
  fi

  # PHP detection
  if [[ -f "composer.json" ]]; then
    DETECTED_LANGUAGE="PHP"

    if grep -q "laravel" composer.json 2>/dev/null; then
      DETECTED_FRAMEWORK="Laravel"
    elif grep -q "symfony" composer.json 2>/dev/null; then
      DETECTED_FRAMEWORK="Symfony"
    fi
  fi

  # Go detection
  if [[ -f "go.mod" ]]; then
    DETECTED_LANGUAGE="Go"

    if grep -q "gin-gonic" go.mod 2>/dev/null; then
      DETECTED_FRAMEWORK="Gin"
    elif grep -q "fiber" go.mod 2>/dev/null; then
      DETECTED_FRAMEWORK="Fiber"
    elif grep -q "echo" go.mod 2>/dev/null; then
      DETECTED_FRAMEWORK="Echo"
    fi
  fi

  # .NET detection
  if ls *.csproj >/dev/null 2>&1 || ls *.sln >/dev/null 2>&1; then
    DETECTED_LANGUAGE=".NET"
    DETECTED_FRAMEWORK="ASP.NET Core"
  fi

  # Java detection
  if [[ -f "pom.xml" ]] || [[ -f "build.gradle" ]]; then
    DETECTED_LANGUAGE="Java"

    if grep -q "spring" pom.xml 2>/dev/null || grep -q "spring" build.gradle 2>/dev/null; then
      DETECTED_FRAMEWORK="Spring Boot"
    fi
  fi
}

# Detect existing Docker setup
detect_docker_setup() {
  local has_docker=false
  local has_compose=false

  if [[ -f "Dockerfile" ]]; then
    has_docker=true
  fi

  if [[ -f "docker-compose.yml" ]] || [[ -f "docker-compose.yaml" ]]; then
    has_compose=true
  fi

  if $has_docker && $has_compose; then
    echo "existing_docker_full"
  elif $has_docker; then
    echo "existing_docker_partial"
  else
    echo "no_docker"
  fi
}

# Detect database from code
detect_database_usage() {
  DETECTED_DATABASE=""

  # Check for PostgreSQL
  if grep -r "postgres\|postgresql\|pg" --include="*.js" --include="*.ts" --include="*.py" \
    --include="*.rb" --include="*.go" --include="*.php" --include="*.java" . 2>/dev/null | head -1 >/dev/null; then
    DETECTED_DATABASE="postgres"
    return
  fi

  # Check for MySQL/MariaDB
  if grep -r "mysql\|mariadb" --include="*.js" --include="*.ts" --include="*.py" \
    --include="*.rb" --include="*.go" --include="*.php" --include="*.java" . 2>/dev/null | head -1 >/dev/null; then
    DETECTED_DATABASE="mysql"
    return
  fi

  # Check for MongoDB
  if grep -r "mongodb\|mongoose" --include="*.js" --include="*.ts" --include="*.py" \
    --include="*.rb" --include="*.go" --include="*.php" --include="*.java" . 2>/dev/null | head -1 >/dev/null; then
    DETECTED_DATABASE="mongodb"
    return
  fi

  # Check for Redis
  if grep -r "redis\|ioredis" --include="*.js" --include="*.ts" --include="*.py" \
    --include="*.rb" --include="*.go" --include="*.php" --include="*.java" . 2>/dev/null | head -1 >/dev/null; then
    DETECTED_SERVICES+=("redis")
  fi
}

# Detect existing services from docker-compose
detect_existing_services() {
  DETECTED_SERVICES=()

  if [[ -f "docker-compose.yml" ]] || [[ -f "docker-compose.yaml" ]]; then
    local compose_file="docker-compose.yml"
    [[ -f "docker-compose.yaml" ]] && compose_file="docker-compose.yaml"

    # Check for common services
    if grep -q "postgres\|postgresql" "$compose_file" 2>/dev/null; then
      DETECTED_SERVICES+=("PostgreSQL")
    fi

    if grep -q "mysql\|mariadb" "$compose_file" 2>/dev/null; then
      DETECTED_SERVICES+=("MySQL/MariaDB")
    fi

    if grep -q "redis" "$compose_file" 2>/dev/null; then
      DETECTED_SERVICES+=("Redis")
    fi

    if grep -q "mongo" "$compose_file" 2>/dev/null; then
      DETECTED_SERVICES+=("MongoDB")
    fi

    if grep -q "elasticsearch" "$compose_file" 2>/dev/null; then
      DETECTED_SERVICES+=("Elasticsearch")
    fi

    if grep -q "rabbitmq\|amqp" "$compose_file" 2>/dev/null; then
      DETECTED_SERVICES+=("RabbitMQ")
    fi

    if grep -q "kafka" "$compose_file" 2>/dev/null; then
      DETECTED_SERVICES+=("Kafka")
    fi

    if grep -q "minio\|s3" "$compose_file" 2>/dev/null; then
      DETECTED_SERVICES+=("MinIO/S3")
    fi
  fi

  # Also check from code
  detect_database_usage
}

# Analyze project size
analyze_project_size() {
  local file_count=0
  local line_count=0

  # Count source files
  file_count=$(find . -type f \( -name "*.js" -o -name "*.ts" -o -name "*.py" \
    -o -name "*.rb" -o -name "*.go" -o -name "*.php" -o -name "*.java" \
    -o -name "*.cs" \) 2>/dev/null | wc -l)

  # Rough line count (sample)
  if command -v wc >/dev/null 2>&1; then
    line_count=$(find . -type f \( -name "*.js" -o -name "*.ts" -o -name "*.py" \
      -o -name "*.rb" -o -name "*.go" -o -name "*.php" -o -name "*.java" \
      -o -name "*.cs" \) -exec wc -l {} \; 2>/dev/null | awk '{sum+=$1} END {print sum}')
  fi

  # Determine project size
  if [[ $file_count -lt 10 ]]; then
    echo "small"
  elif [[ $file_count -lt 50 ]]; then
    echo "medium"
  elif [[ $file_count -lt 200 ]]; then
    echo "large"
  else
    echo "enterprise"
  fi
}

# Recommend services based on detection
recommend_services() {
  local framework="$1"
  local recommendations=()

  case "$framework" in
    "Next.js" | "Nuxt.js" | "Gatsby")
      recommendations+=("PostgreSQL" "Redis" "MinIO")
      ;;
    "Express.js" | "Fastify")
      recommendations+=("PostgreSQL" "Redis" "RabbitMQ")
      ;;
    "Django" | "Rails" | "Laravel")
      recommendations+=("PostgreSQL" "Redis" "Sidekiq/Celery")
      ;;
    "FastAPI" | "Flask" | "Gin" | "Fiber")
      recommendations+=("PostgreSQL" "Redis")
      ;;
    *)
      recommendations+=("PostgreSQL")
      ;;
  esac

  echo "${recommendations[@]}"
}

# Export functions
export -f detect_project_framework
export -f detect_docker_setup
export -f detect_database_usage
export -f detect_existing_services
export -f analyze_project_size
export -f recommend_services
