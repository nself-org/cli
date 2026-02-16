#!/usr/bin/env bash
# compose-to-k8s.sh - Docker Compose to Kubernetes manifest converter
# Part of nself v0.4.7 - Infrastructure Everywhere

# Conversion configuration
K8S_API_VERSION_DEPLOYMENT="apps/v1"

set -euo pipefail

K8S_API_VERSION_SERVICE="v1"
K8S_API_VERSION_INGRESS="networking.k8s.io/v1"
K8S_API_VERSION_CONFIGMAP="v1"
K8S_API_VERSION_SECRET="v1"
K8S_API_VERSION_PVC="v1"

# Default resource limits
DEFAULT_CPU_REQUEST="100m"
DEFAULT_CPU_LIMIT="500m"
DEFAULT_MEMORY_REQUEST="128Mi"
DEFAULT_MEMORY_LIMIT="512Mi"
DEFAULT_REPLICAS=2

# Convert Docker Compose to K8s manifests
k8s_convert_compose() {
  local compose_file="${1:-docker-compose.yml}"
  local output_dir="${2:-.nself/k8s/manifests}"
  local namespace="${3:-$(basename "$(pwd)")}"

  if [[ ! -f "$compose_file" ]]; then
    printf "ERROR: Compose file not found: %s\n" "$compose_file" >&2
    return 1
  fi

  mkdir -p "$output_dir"

  printf "Converting %s to Kubernetes manifests...\n" "$compose_file"

  # Generate namespace first
  _generate_namespace "$namespace" "$output_dir"

  # Parse and convert each service
  local services
  services=$(_parse_compose_services "$compose_file")

  local counter=1
  for service in $services; do
    [[ -z "$service" ]] && continue

    printf "  Converting service: %s\n" "$service"

    local padded
    padded=$(printf "%02d" $counter)

    # Extract service configuration
    local image ports env_vars volumes healthcheck

    image=$(_get_service_image "$compose_file" "$service")
    ports=$(_get_service_ports "$compose_file" "$service")
    env_vars=$(_get_service_env "$compose_file" "$service")
    volumes=$(_get_service_volumes "$compose_file" "$service")
    healthcheck=$(_get_service_healthcheck "$compose_file" "$service")

    # Generate deployment
    _generate_deployment "$service" "$namespace" "$image" "$env_vars" "$healthcheck" \
      >"$output_dir/${padded}-${service}-deployment.yaml"

    # Generate service if ports are exposed
    if [[ -n "$ports" ]]; then
      _generate_service "$service" "$namespace" "$ports" \
        >"$output_dir/${padded}-${service}-service.yaml"
    fi

    # Generate PVC if volumes are defined
    if [[ -n "$volumes" ]]; then
      _generate_pvc "$service" "$namespace" "$volumes" \
        >"$output_dir/${padded}-${service}-pvc.yaml"
    fi

    # Generate ConfigMap for environment variables
    if [[ -n "$env_vars" ]]; then
      _generate_configmap "$service" "$namespace" "$env_vars" \
        >"$output_dir/${padded}-${service}-configmap.yaml"
    fi

    ((counter++))
  done

  # Generate ingress if needed
  _generate_ingress "$namespace" "$output_dir" "$compose_file"

  printf "Conversion complete: %d services processed\n" "$((counter - 1))"
}

# Parse services from compose file
_parse_compose_services() {
  local compose_file="$1"

  # Extract service names (top-level keys under 'services:')
  local in_services=0
  local indent_level=0

  while IFS= read -r line; do
    # Check for services section
    if echo "$line" | grep -qE "^services:"; then
      in_services=1
      continue
    fi

    # Check for other top-level sections
    if echo "$line" | grep -qE "^[a-z]+:" && [[ $in_services -eq 1 ]]; then
      if ! echo "$line" | grep -qE "^[[:space:]]"; then
        in_services=0
        continue
      fi
    fi

    # Extract service names (2-space indented keys)
    if [[ $in_services -eq 1 ]]; then
      if echo "$line" | grep -qE "^[[:space:]]{2}[a-zA-Z0-9_-]+:$"; then
        echo "$line" | sed 's/^[[:space:]]*//' | tr -d ':'
      fi
    fi
  done <"$compose_file"
}

# Get service image
_get_service_image() {
  local compose_file="$1"
  local service="$2"

  local in_service=0
  while IFS= read -r line; do
    if echo "$line" | grep -qE "^[[:space:]]{2}${service}:"; then
      in_service=1
      continue
    fi

    if [[ $in_service -eq 1 ]]; then
      # Check for end of service block
      if echo "$line" | grep -qE "^[[:space:]]{2}[a-zA-Z]" && ! echo "$line" | grep -qE "^[[:space:]]{4}"; then
        break
      fi

      # Extract image
      if echo "$line" | grep -qE "^[[:space:]]+image:"; then
        echo "$line" | sed 's/.*image:[[:space:]]*//' | tr -d '"' | tr -d "'"
        return
      fi
    fi
  done <"$compose_file"

  # Default to service name if no image specified (build context)
  echo "${service}:latest"
}

# Get service ports
_get_service_ports() {
  local compose_file="$1"
  local service="$2"

  local in_service=0
  local in_ports=0
  local ports=""

  while IFS= read -r line; do
    if echo "$line" | grep -qE "^[[:space:]]{2}${service}:"; then
      in_service=1
      continue
    fi

    if [[ $in_service -eq 1 ]]; then
      if echo "$line" | grep -qE "^[[:space:]]{2}[a-zA-Z]" && ! echo "$line" | grep -qE "^[[:space:]]{4}"; then
        break
      fi

      if echo "$line" | grep -qE "^[[:space:]]+ports:"; then
        in_ports=1
        continue
      fi

      if [[ $in_ports -eq 1 ]]; then
        if echo "$line" | grep -qE "^[[:space:]]+- "; then
          local port
          port=$(echo "$line" | sed 's/.*- //' | tr -d '"' | tr -d "'")
          ports="$ports $port"
        elif ! echo "$line" | grep -qE "^[[:space:]]{6}"; then
          in_ports=0
        fi
      fi
    fi
  done <"$compose_file"

  echo "$ports" | tr -s ' '
}

# Get service environment variables
_get_service_env() {
  local compose_file="$1"
  local service="$2"

  local in_service=0
  local in_env=0
  local env_vars=""

  while IFS= read -r line; do
    if echo "$line" | grep -qE "^[[:space:]]{2}${service}:"; then
      in_service=1
      continue
    fi

    if [[ $in_service -eq 1 ]]; then
      if echo "$line" | grep -qE "^[[:space:]]{2}[a-zA-Z]" && ! echo "$line" | grep -qE "^[[:space:]]{4}"; then
        break
      fi

      if echo "$line" | grep -qE "^[[:space:]]+environment:"; then
        in_env=1
        continue
      fi

      if [[ $in_env -eq 1 ]]; then
        if echo "$line" | grep -qE "^[[:space:]]+- " || echo "$line" | grep -qE "^[[:space:]]+[A-Z_]+:"; then
          local env_var
          env_var=$(echo "$line" | sed 's/.*- //' | sed 's/^[[:space:]]*//')
          env_vars="$env_vars|$env_var"
        elif ! echo "$line" | grep -qE "^[[:space:]]{6}"; then
          in_env=0
        fi
      fi
    fi
  done <"$compose_file"

  echo "$env_vars" | sed 's/^|//'
}

# Get service volumes
_get_service_volumes() {
  local compose_file="$1"
  local service="$2"

  local in_service=0
  local in_volumes=0
  local volumes=""

  while IFS= read -r line; do
    if echo "$line" | grep -qE "^[[:space:]]{2}${service}:"; then
      in_service=1
      continue
    fi

    if [[ $in_service -eq 1 ]]; then
      if echo "$line" | grep -qE "^[[:space:]]{2}[a-zA-Z]" && ! echo "$line" | grep -qE "^[[:space:]]{4}"; then
        break
      fi

      if echo "$line" | grep -qE "^[[:space:]]+volumes:"; then
        in_volumes=1
        continue
      fi

      if [[ $in_volumes -eq 1 ]]; then
        if echo "$line" | grep -qE "^[[:space:]]+- "; then
          local vol
          vol=$(echo "$line" | sed 's/.*- //' | tr -d '"' | tr -d "'")
          volumes="$volumes $vol"
        elif ! echo "$line" | grep -qE "^[[:space:]]{6}"; then
          in_volumes=0
        fi
      fi
    fi
  done <"$compose_file"

  echo "$volumes" | tr -s ' '
}

# Get service healthcheck
_get_service_healthcheck() {
  local compose_file="$1"
  local service="$2"

  # Simplified - just check if healthcheck exists
  if grep -A 10 "^[[:space:]]*${service}:" "$compose_file" | grep -q "healthcheck:"; then
    echo "true"
  else
    echo ""
  fi
}

# Generate namespace manifest
_generate_namespace() {
  local namespace="$1"
  local output_dir="$2"

  cat >"$output_dir/00-namespace.yaml" <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: ${namespace}
  labels:
    app.kubernetes.io/managed-by: nself
    app.kubernetes.io/part-of: ${namespace}
EOF
}

# Generate deployment manifest
_generate_deployment() {
  local service="$1"
  local namespace="$2"
  local image="$3"
  local env_vars="$4"
  local healthcheck="$5"

  cat <<EOF
apiVersion: ${K8S_API_VERSION_DEPLOYMENT}
kind: Deployment
metadata:
  name: ${service}
  namespace: ${namespace}
  labels:
    app: ${service}
    app.kubernetes.io/name: ${service}
    app.kubernetes.io/component: service
    app.kubernetes.io/managed-by: nself
spec:
  replicas: ${DEFAULT_REPLICAS}
  selector:
    matchLabels:
      app: ${service}
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  template:
    metadata:
      labels:
        app: ${service}
        app.kubernetes.io/name: ${service}
    spec:
      containers:
      - name: ${service}
        image: ${image}
        imagePullPolicy: IfNotPresent
        resources:
          requests:
            cpu: "${DEFAULT_CPU_REQUEST}"
            memory: "${DEFAULT_MEMORY_REQUEST}"
          limits:
            cpu: "${DEFAULT_CPU_LIMIT}"
            memory: "${DEFAULT_MEMORY_LIMIT}"
EOF

  # Add environment variables from ConfigMap
  if [[ -n "$env_vars" ]]; then
    cat <<EOF
        envFrom:
        - configMapRef:
            name: ${service}-config
EOF
  fi

  # Add liveness probe if healthcheck exists
  if [[ -n "$healthcheck" ]]; then
    cat <<EOF
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
EOF
  fi

  # Close container and pod spec
  cat <<EOF
      restartPolicy: Always
      terminationGracePeriodSeconds: 30
EOF
}

# Generate service manifest
_generate_service() {
  local service="$1"
  local namespace="$2"
  local ports="$3"

  cat <<EOF
apiVersion: ${K8S_API_VERSION_SERVICE}
kind: Service
metadata:
  name: ${service}
  namespace: ${namespace}
  labels:
    app: ${service}
    app.kubernetes.io/name: ${service}
    app.kubernetes.io/managed-by: nself
spec:
  selector:
    app: ${service}
  ports:
EOF

  # Parse and add ports
  for port_spec in $ports; do
    local host_port container_port

    if echo "$port_spec" | grep -q ":"; then
      host_port=$(echo "$port_spec" | cut -d':' -f1)
      container_port=$(echo "$port_spec" | cut -d':' -f2)
    else
      host_port="$port_spec"
      container_port="$port_spec"
    fi

    # Handle port/protocol format
    container_port=$(echo "$container_port" | cut -d'/' -f1)

    cat <<EOF
  - name: port-${container_port}
    port: ${host_port}
    targetPort: ${container_port}
    protocol: TCP
EOF
  done

  cat <<EOF
  type: ClusterIP
EOF
}

# Generate PVC manifest
_generate_pvc() {
  local service="$1"
  local namespace="$2"
  local volumes="$3"

  local pvc_counter=1
  for vol in $volumes; do
    # Only process named volumes (not bind mounts)
    if ! echo "$vol" | grep -q "^[./]"; then
      local vol_name
      vol_name=$(echo "$vol" | cut -d':' -f1)

      cat <<EOF
---
apiVersion: ${K8S_API_VERSION_PVC}
kind: PersistentVolumeClaim
metadata:
  name: ${service}-${vol_name}
  namespace: ${namespace}
  labels:
    app: ${service}
    app.kubernetes.io/name: ${service}
    app.kubernetes.io/managed-by: nself
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
  storageClassName: standard
EOF
      ((pvc_counter++))
    fi
  done
}

# Generate ConfigMap manifest
_generate_configmap() {
  local service="$1"
  local namespace="$2"
  local env_vars="$3"

  cat <<EOF
apiVersion: ${K8S_API_VERSION_CONFIGMAP}
kind: ConfigMap
metadata:
  name: ${service}-config
  namespace: ${namespace}
  labels:
    app: ${service}
    app.kubernetes.io/name: ${service}
    app.kubernetes.io/managed-by: nself
data:
EOF

  # Parse environment variables
  echo "$env_vars" | tr '|' '\n' | while IFS= read -r env_line; do
    [[ -z "$env_line" ]] && continue

    local key value
    if echo "$env_line" | grep -q "="; then
      key=$(echo "$env_line" | cut -d'=' -f1)
      value=$(echo "$env_line" | cut -d'=' -f2-)
    else
      key=$(echo "$env_line" | cut -d':' -f1)
      value=$(echo "$env_line" | cut -d':' -f2- | sed 's/^[[:space:]]*//')
    fi

    # Skip if value references a secret or is empty
    if [[ -z "$value" ]] || echo "$value" | grep -qE '^\$\{'; then
      continue
    fi

    # Clean up value
    value=$(echo "$value" | tr -d '"' | tr -d "'")

    printf "  %s: \"%s\"\n" "$key" "$value"
  done
}

# Generate ingress manifest
_generate_ingress() {
  local namespace="$1"
  local output_dir="$2"
  local compose_file="$3"

  # Check if any services expose web ports (80, 443, 8080, 3000, etc)
  local has_web_services=0
  local web_services=""

  local services
  services=$(_parse_compose_services "$compose_file")

  for service in $services; do
    local ports
    ports=$(_get_service_ports "$compose_file" "$service")

    for port in $ports; do
      local container_port
      container_port=$(echo "$port" | cut -d':' -f2 | cut -d'/' -f1)

      case "$container_port" in
        80 | 443 | 8080 | 3000 | 5000 | 8000)
          has_web_services=1
          web_services="$web_services $service:$container_port"
          ;;
      esac
    done
  done

  if [[ $has_web_services -eq 1 ]]; then
    cat >"$output_dir/99-ingress.yaml" <<EOF
apiVersion: ${K8S_API_VERSION_INGRESS}
kind: Ingress
metadata:
  name: ${namespace}-ingress
  namespace: ${namespace}
  labels:
    app.kubernetes.io/managed-by: nself
  annotations:
    kubernetes.io/ingress.class: nginx
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
spec:
  ingressClassName: nginx
  rules:
EOF

    for service_port in $web_services; do
      local svc port
      svc=$(echo "$service_port" | cut -d':' -f1)
      port=$(echo "$service_port" | cut -d':' -f2)

      cat >>"$output_dir/99-ingress.yaml" <<EOF
  - host: ${svc}.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: ${svc}
            port:
              number: ${port}
EOF
    done

    printf "  Generated ingress for %d web services\n" "$(echo "$web_services" | wc -w | tr -d ' ')"
  fi
}

# Export functions
export -f k8s_convert_compose
