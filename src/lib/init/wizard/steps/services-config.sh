#!/usr/bin/env bash
set -euo pipefail

# services-config.sh - Wizard step for services configuration
# POSIX-compliant, no Bash 4+ features

# Configure core services
wizard_core_services() {
  local config_array_name="$1"

  clear
  show_wizard_step 3 10 "Core Services"

  echo "🔧 Select Core Services"
  echo ""
  echo "Choose which services to enable for your project:"
  echo ""

  # Hasura GraphQL
  echo "📊 Hasura GraphQL Engine"
  echo "  Instant GraphQL API over your PostgreSQL database"
  echo "  Features: Real-time subscriptions, auth, permissions"
  if confirm_action "Enable Hasura GraphQL?"; then
    eval "$config_array_name+=('HASURA_ENABLED=true')"
    eval "$config_array_name+=('HASURA_PORT=8080')"
  else
    eval "$config_array_name+=('HASURA_ENABLED=false')"
  fi

  echo ""

  # Authentication Service
  echo "🔐 Authentication Service"
  echo "  User management, JWT tokens, social login"
  if confirm_action "Enable Authentication service?"; then
    eval "$config_array_name+=('AUTH_ENABLED=true')"
    eval "$config_array_name+=('AUTH_PORT=4000')"

    echo ""
    echo "Auth provider:"
    local auth_options=(
      "Nhost Auth - Full-featured, production-ready"
      "Supabase Auth - Modern auth with row-level security"
      "Custom - Build your own auth service"
    )
    local selected_auth
    select_option "Select auth provider" auth_options selected_auth

    case $selected_auth in
      0) eval "$config_array_name+=('AUTH_PROVIDER=nhost')" ;;
      1) eval "$config_array_name+=('AUTH_PROVIDER=supabase')" ;;
      2) eval "$config_array_name+=('AUTH_PROVIDER=custom')" ;;
    esac
  else
    eval "$config_array_name+=('AUTH_ENABLED=false')"
  fi

  echo ""

  # MinIO Storage
  echo "📁 MinIO Object Storage"
  echo "  S3-compatible object storage for file uploads"
  if confirm_action "Enable MinIO storage?"; then
    eval "$config_array_name+=('MINIO_ENABLED=true')"
    eval "$config_array_name+=('MINIO_PORT=9000')"
    eval "$config_array_name+=('MINIO_CONSOLE_PORT=9001')"

    echo ""
    local minio_user minio_pass
    prompt_input "MinIO root user" "minioadmin" minio_user
    prompt_input "MinIO root password" "minioadmin" minio_pass
    eval "$config_array_name+=('MINIO_ROOT_USER=$minio_user')"
    eval "$config_array_name+=('MINIO_ROOT_PASSWORD=$minio_pass')"
  else
    eval "$config_array_name+=('MINIO_ENABLED=false')"
  fi

  echo ""

  # Functions (Serverless runtime)
  echo "⚡ Functions Runtime"
  echo "  Serverless functions for custom business logic"
  if confirm_action "Enable Functions service?"; then
    eval "$config_array_name+=('FUNCTIONS_ENABLED=true')"
    eval "$config_array_name+=('FUNCTIONS_PORT=3008')"
  else
    eval "$config_array_name+=('FUNCTIONS_ENABLED=false')"
  fi

  return 0
}

# Configure optional services
wizard_optional_services() {
  local config_array_name="$1"

  clear
  show_wizard_step 6 10 "Optional Services"

  echo "🔌 Additional Services"
  echo ""

  # Redis Cache
  echo "💾 Redis Cache"
  echo "  In-memory data store for caching and sessions"
  if confirm_action "Enable Redis?"; then
    eval "$config_array_name+=('REDIS_ENABLED=true')"
    eval "$config_array_name+=('REDIS_PORT=6379')"

    echo ""
    if confirm_action "Enable Redis persistence?"; then
      eval "$config_array_name+=('REDIS_PERSISTENCE=true')"
    else
      eval "$config_array_name+=('REDIS_PERSISTENCE=false')"
    fi
  else
    eval "$config_array_name+=('REDIS_ENABLED=false')"
  fi

  echo ""

  # Message Queue
  echo "📨 Message Queue"
  echo "  Background jobs, task processing, event streaming"
  if confirm_action "Enable message queue?"; then
    echo ""
    echo "Queue system:"
    local queue_options=(
      "BullMQ - Redis-based, Node.js friendly"
      "RabbitMQ - Enterprise message broker"
      "Kafka - High-throughput event streaming"
      "NATS - Lightweight, cloud-native"
    )
    local selected_queue
    select_option "Select queue system" queue_options selected_queue

    case $selected_queue in
      0)
        eval "$config_array_name+=('QUEUE_ENABLED=true')"
        eval "$config_array_name+=('QUEUE_TYPE=bullmq')"
        if [[ "${REDIS_ENABLED:-false}" != "true" ]]; then
          eval "$config_array_name+=('REDIS_ENABLED=true')"
          eval "$config_array_name+=('REDIS_PORT=6379')"
          echo "  (Redis enabled for BullMQ)"
        fi
        ;;
      1)
        eval "$config_array_name+=('QUEUE_ENABLED=true')"
        eval "$config_array_name+=('QUEUE_TYPE=rabbitmq')"
        eval "$config_array_name+=('RABBITMQ_PORT=5672')"
        ;;
      2)
        eval "$config_array_name+=('QUEUE_ENABLED=true')"
        eval "$config_array_name+=('QUEUE_TYPE=kafka')"
        eval "$config_array_name+=('KAFKA_PORT=9092')"
        ;;
      3)
        eval "$config_array_name+=('QUEUE_ENABLED=true')"
        eval "$config_array_name+=('QUEUE_TYPE=nats')"
        eval "$config_array_name+=('NATS_PORT=4222')"
        ;;
    esac
  else
    eval "$config_array_name+=('QUEUE_ENABLED=false')"
  fi

  echo ""

  # Monitoring
  echo "📈 Monitoring & Observability"
  echo "  Metrics, logs, tracing, alerting"
  if confirm_action "Enable monitoring stack?"; then
    eval "$config_array_name+=('MONITORING_ENABLED=true')"

    echo ""
    local monitoring_services=()

    if confirm_action "Enable Prometheus metrics?"; then
      monitoring_services+=("prometheus")
      eval "$config_array_name+=('PROMETHEUS_ENABLED=true')"
    fi

    if confirm_action "Enable Grafana dashboards?"; then
      monitoring_services+=("grafana")
      eval "$config_array_name+=('GRAFANA_ENABLED=true')"
      eval "$config_array_name+=('GRAFANA_PORT=3001')"
    fi

    if confirm_action "Enable Jaeger tracing?"; then
      monitoring_services+=("jaeger")
      eval "$config_array_name+=('JAEGER_ENABLED=true')"
      eval "$config_array_name+=('JAEGER_PORT=16686')"
    fi

    if confirm_action "Enable Loki log aggregation?"; then
      monitoring_services+=("loki")
      eval "$config_array_name+=('LOKI_ENABLED=true')"
    fi
  else
    eval "$config_array_name+=('MONITORING_ENABLED=false')"
  fi

  return 0
}

# Configure email and search services
wizard_email_search() {
  local config_array_name="$1"

  clear
  show_wizard_step 7 10 "Email & Search"

  echo "📧 Email Service"
  echo ""

  if confirm_action "Enable email functionality?"; then
    echo ""
    echo "Email provider:"
    local email_options=(
      "Mailpit - Local development email catcher"
      "SMTP - External SMTP server"
      "SendGrid - Cloud email service"
      "AWS SES - Amazon email service"
      "Postmark - Transactional email"
    )
    local selected_email
    select_option "Select email provider" email_options selected_email

    case $selected_email in
      0)
        eval "$config_array_name+=('EMAIL_PROVIDER=mailpit')"
        eval "$config_array_name+=('MAILPIT_ENABLED=true')"
        eval "$config_array_name+=('MAILPIT_PORT=1025')"
        eval "$config_array_name+=('MAILPIT_UI_PORT=8025')"
        ;;
      1)
        eval "$config_array_name+=('EMAIL_PROVIDER=smtp')"
        echo ""
        local smtp_host smtp_port smtp_user
        prompt_input "SMTP host" "smtp.gmail.com" smtp_host
        prompt_input "SMTP port" "587" smtp_port "^[0-9]+$"
        prompt_input "SMTP user" "user@example.com" smtp_user
        eval "$config_array_name+=('SMTP_HOST=$smtp_host')"
        eval "$config_array_name+=('SMTP_PORT=$smtp_port')"
        eval "$config_array_name+=('SMTP_USER=$smtp_user')"
        ;;
      2)
        eval "$config_array_name+=('EMAIL_PROVIDER=sendgrid')"
        echo ""
        echo "You'll need to add SENDGRID_API_KEY to .env later"
        press_any_key
        ;;
      3)
        eval "$config_array_name+=('EMAIL_PROVIDER=ses')"
        echo ""
        local ses_region
        prompt_input "AWS region" "us-east-1" ses_region
        eval "$config_array_name+=('AWS_REGION=$ses_region')"
        ;;
      4)
        eval "$config_array_name+=('EMAIL_PROVIDER=postmark')"
        echo ""
        echo "You'll need to add POSTMARK_SERVER_TOKEN to .env later"
        press_any_key
        ;;
    esac
  else
    eval "$config_array_name+=('EMAIL_PROVIDER=none')"
  fi

  echo ""

  # Search Service
  echo "🔍 Search Service"
  echo ""

  if confirm_action "Enable search functionality?"; then
    echo ""
    echo "Search engine:"
    local search_options=(
      "MeiliSearch - Fast, typo-tolerant search"
      "Elasticsearch - Powerful full-text search"
      "Typesense - Lightning fast, typo-tolerant"
      "PostgreSQL FTS - Built-in full-text search"
    )
    local selected_search
    select_option "Select search engine" search_options selected_search

    case $selected_search in
      0)
        eval "$config_array_name+=('SEARCH_ENABLED=true')"
        eval "$config_array_name+=('SEARCH_ENGINE=meilisearch')"
        eval "$config_array_name+=('MEILISEARCH_PORT=7700')"
        ;;
      1)
        eval "$config_array_name+=('SEARCH_ENABLED=true')"
        eval "$config_array_name+=('SEARCH_ENGINE=elasticsearch')"
        eval "$config_array_name+=('ELASTICSEARCH_PORT=9200')"
        ;;
      2)
        eval "$config_array_name+=('SEARCH_ENABLED=true')"
        eval "$config_array_name+=('SEARCH_ENGINE=typesense')"
        eval "$config_array_name+=('TYPESENSE_PORT=8108')"
        ;;
      3)
        eval "$config_array_name+=('SEARCH_ENABLED=true')"
        eval "$config_array_name+=('SEARCH_ENGINE=postgres')"
        echo ""
        echo "PostgreSQL full-text search will be configured"
        ;;
    esac
  else
    eval "$config_array_name+=('SEARCH_ENABLED=false')"
  fi

  return 0
}

# Export functions
export -f wizard_core_services
export -f wizard_optional_services
export -f wizard_email_search
