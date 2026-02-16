#!/usr/bin/env bash
# audit-log.sh - Comprehensive audit logging
# Part of nself v0.6.0 - Phase 2


audit_init() {

set -euo pipefail

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE SCHEMA IF NOT EXISTS audit;
CREATE TABLE IF NOT EXISTS audit.events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_type TEXT NOT NULL,
  actor_type TEXT NOT NULL,
  actor_id UUID,
  resource_type TEXT,
  resource_id UUID,
  action TEXT NOT NULL,
  result TEXT NOT NULL,
  metadata JSONB DEFAULT '{}',
  ip_address INET,
  user_agent TEXT,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit.events(actor_id);
CREATE INDEX IF NOT EXISTS idx_audit_resource ON audit.events(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_audit_created ON audit.events(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_event_type ON audit.events(event_type);
EOSQL
}

audit_log() {
  local event_type="$1"
  local actor_id="${2:-}"
  local action="$3"
  local result="${4:-success}"
  local metadata="${5:-{}}"

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  local actor_clause="NULL"
  [[ -n "$actor_id" ]] && actor_clause="'$actor_id'"
  local meta=$(echo "$metadata" | sed "s/'/''/g")

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO audit.events (event_type, actor_type, actor_id, action, result, metadata)
     VALUES ('$event_type', 'user', $actor_clause, '$action', '$result', '$meta'::jsonb);" >/dev/null 2>&1
}

audit_query() {
  local filters="${1:-{}}"
  local limit="${2:-100}"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local where="WHERE 1=1"
  local event_type=$(echo "$filters" | jq -r '.event_type // empty')
  local actor_id=$(echo "$filters" | jq -r '.actor_id // empty')
  [[ -n "$event_type" ]] && where="$where AND event_type = '$event_type'"
  [[ -n "$actor_id" ]] && where="$where AND actor_id = '$actor_id'"

  local events=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(e) FROM (
       SELECT id, event_type, actor_id, action, result, metadata, created_at
       FROM audit.events $where ORDER BY created_at DESC LIMIT $limit
     ) e;" 2>/dev/null | xargs)

  [[ -z "$events" || "$events" == "null" ]] && echo "[]" || echo "$events"
}

export -f audit_init audit_log audit_query
