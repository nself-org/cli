#!/usr/bin/env bash
# Purpose: prove `nself db generate vector-search` end to end on a default
#          project (P7-ADOPT-07, EPIC ADOPT D7): the generated k-NN function is
#          tracked through Hasura, a filtered role sees only its own rows in
#          distance order, anonymous is denied, the SQL function inlines, and no
#          Postgres role or grant is created.
# Usage:   vector-search.sh        (see EPIC ADOPT D18 for the Linux e2e line)
# Env:     W        work dir that has the same path on the host and in the
#                   container (default: a fresh mktemp dir)
#          E2E_KEEP_STACK=1  leave the stack and work dir for inspection
# Needs:   docker with compose v2, curl, jq, openssl and the nself binary on PATH.
# Outputs: "PASS" or "FAIL: <reason>" on stdout.
# Exit:    0 PASS, 1 FAIL, 3 precondition not met (no Docker).
# Constraints: bash 3.2 compatible. Uses the fixed project name e2e-adopt-vsearch
#          and never touches a project it did not create. The stack, its volumes
#          and the work dir are removed on exit. JWTs are minted here with
#          openssl from the project's own Hasura JWT secret; nothing is printed.
set -euo pipefail

PROJECT="e2e-adopt-vsearch"
export NSELF_V15=1

for tool in nself docker curl jq openssl; do
  command -v "$tool" >/dev/null 2>&1 || { echo "vector-search: $tool is not on PATH" >&2; exit 3; }
done
docker info >/dev/null 2>&1 || { echo "vector-search: Docker is not available" >&2; exit 3; }
docker compose version >/dev/null 2>&1 || { echo "vector-search: Docker compose v2 is not available" >&2; exit 3; }

CLEAN_WORK=0
if [ -z "${W:-}" ]; then
  W="$(mktemp -d "${TMPDIR:-/tmp}/vector-search.XXXXXX")"
  CLEAN_WORK=1
fi
DIR="$W/$PROJECT"
mkdir -p "$DIR"
export HOME="$W/home"
mkdir -p "$HOME"
LOG="$W/vector-search.log"
: >"$LOG"

teardown() {
  if [ "${E2E_KEEP_STACK:-0}" = "1" ]; then
    echo "vector-search: E2E_KEEP_STACK=1, project left in $DIR" >&2
    return
  fi
  if [ -f "$DIR/.env" ]; then
    (cd "$DIR" && nself stop >/dev/null 2>&1) || true
  fi
  # Remove only what this script's project created.
  docker ps -aq --filter "name=^${PROJECT}_" | xargs docker rm -f >/dev/null 2>&1 || true
  docker volume ls -q --filter "name=^${PROJECT}" | xargs docker volume rm -f >/dev/null 2>&1 || true
  docker network ls -q --filter "name=^${PROJECT}" | xargs docker network rm >/dev/null 2>&1 || true
  if [ "$CLEAN_WORK" = "1" ]; then
    rm -rf "$W"
  fi
}
trap teardown EXIT

fail() {
  echo "FAIL: $1"
  tail -n 40 "$LOG" 2>/dev/null || true
  exit 1
}

step() { echo "== $1"; }

# psql runs SQL as the container's own superuser (the CLI's admin path). It is
# used only for the catalog snapshots and EXPLAIN; no app credential exists.
PGC="${PROJECT}_postgres"
psqlc() { docker exec "$PGC" psql -X -U "$PGUSER" -d "$PGDB" -v ON_ERROR_STOP=1 -At -c "$1"; }

cd "$DIR"
step "init and start a default project"
nself init --non-interactive --name "$PROJECT" >"$LOG" 2>&1 || fail "nself init"
nself start >>"$LOG" 2>&1 || fail "nself start"

PGUSER="$(docker inspect "$PGC" --format '{{range .Config.Env}}{{println .}}{{end}}' | sed -n 's/^POSTGRES_USER=//p' | head -n 1)"
PGDB="$(docker inspect "$PGC" --format '{{range .Config.Env}}{{println .}}{{end}}' | sed -n 's/^POSTGRES_DB=//p' | head -n 1)"
[ -n "$PGUSER" ] && [ -n "$PGDB" ] || fail "cannot read the postgres user and database from the container"

HC="${PROJECT}_hasura"
HENV="$(docker inspect "$HC" --format '{{range .Config.Env}}{{println .}}{{end}}')"
ADMIN_SECRET="$(printf '%s\n' "$HENV" | sed -n 's/^HASURA_GRAPHQL_ADMIN_SECRET=//p' | head -n 1)"
JWT_JSON="$(printf '%s\n' "$HENV" | sed -n 's/^HASURA_GRAPHQL_JWT_SECRET=//p' | head -n 1)"
[ -n "$ADMIN_SECRET" ] && [ -n "$JWT_JSON" ] || fail "cannot read the Hasura secrets from the container"
JWT_TYPE="$(printf '%s' "$JWT_JSON" | jq -r '.type')"
JWT_KEY="$(printf '%s' "$JWT_JSON" | jq -r '.key')"
[ "$JWT_TYPE" = "HS256" ] || fail "the project JWT type is $JWT_TYPE, this script mints HS256 tokens"
HASURA_PORT="$(grep -E '^HASURA_PORT=' .env | head -n 1 | cut -d= -f2 || true)"
HASURA="http://localhost:${HASURA_PORT:-8080}"

step "wait for Hasura"
n=0
until curl -fsS "$HASURA/healthz" >/dev/null 2>&1; do
  n=$((n + 1))
  [ "$n" -le 90 ] || fail "Hasura did not become healthy at $HASURA"
  sleep 2
done

meta() { # meta <json body>: Hasura metadata API as admin
  curl -fsS -X POST "$HASURA/v1/metadata" -H "X-Hasura-Admin-Secret: $ADMIN_SECRET" -H 'Content-Type: application/json' -d "$1"
}

DOCS='{"schema":"public","name":"docs"}'
sel_perm() { # sel_perm <role> <columns json> [filter json]: (re)create a select permission on docs for <role> (filter default {})
  local flt="${3:-}"
  [ -n "$flt" ] || flt='{}'
  meta '{"type":"pg_drop_select_permission","args":{"source":"default","table":'"$DOCS"',"role":"'"$1"'"}}' >/dev/null 2>&1 || true
  meta '{"type":"pg_create_select_permission","args":{"source":"default","table":'"$DOCS"',"role":"'"$1"'","permission":{"columns":'"$2"',"filter":'"$flt"'}}}' >>"$LOG"
}
drop_perm() { meta '{"type":"pg_drop_select_permission","args":{"source":"default","table":'"$DOCS"',"role":"'"$1"'"}}' >>"$LOG"; }
live_funcs() { meta '{"type":"export_metadata","args":{}}' | jq '[.sources[].functions[]?] | length'; }

b64url() { openssl base64 -A | tr '+/' '-_' | tr -d '='; }

mint() { # mint <user-id> <role>: HS256 JWT with a single allowed role
  local header payload sig now
  now="$(date +%s)"
  header="$(printf '{"alg":"HS256","typ":"JWT"}' | b64url)"
  payload="$(jq -nc --arg uid "$1" --arg role "$2" --argjson now "$now" \
    '{sub:$uid, iat:$now, exp:($now+3600),
      "https://hasura.io/jwt/claims": {"x-hasura-allowed-roles":[$role], "x-hasura-default-role":$role, "x-hasura-user-id":$uid}}' | b64url)"
  sig="$(printf '%s.%s' "$header" "$payload" | openssl dgst -sha256 -hmac "$JWT_KEY" -binary | b64url)"
  printf '%s.%s.%s' "$header" "$payload" "$sig"
}

gql() { # gql <jwt> <query>: GraphQL over HTTP, always prints the JSON body
  local body
  body="$(jq -nc --arg q "$2" '{query:$q}')"
  curl -sS -X POST "$HASURA/v1/graphql" -H "Authorization: Bearer $1" -H 'Content-Type: application/json' -d "$body"
}

QUERY='query { docs_nearest_embedding(args: {query: "[1,0,0]"}, limit: 2) { id owner embedding } }'

step "create docs(id, owner, embedding vector(3)) with 6 rows through a migration"
mkdir -p migrations
cat >migrations/20260101000000_docs.sql <<'SQL'
CREATE EXTENSION IF NOT EXISTS vector;
CREATE TABLE public.docs (
  id serial PRIMARY KEY,
  owner text NOT NULL,
  embedding vector(3) NOT NULL
);
-- alice's rows are all farther from [1,0,0] than bob's rows.
INSERT INTO public.docs (owner, embedding) VALUES
  ('alice', '[0,1,0]'),
  ('alice', '[0.2,0.8,0]'),
  ('alice', '[0.5,0.5,0]'),
  ('bob',   '[1,0,0]'),
  ('bob',   '[0.9,0.1,0]'),
  ('bob',   '[0.8,0.2,0]');
SQL
nself db migrate up --migration-dir migrations >>"$LOG" 2>&1 || fail "applying the docs migration"

step "track docs with a user select filter, then export the metadata to disk"
meta '{"type":"pg_track_table","args":{"source":"default","table":{"schema":"public","name":"docs"}}}' >>"$LOG" || fail "tracking docs"
meta '{"type":"pg_create_select_permission","args":{"source":"default","table":{"schema":"public","name":"docs"},"role":"user","permission":{"columns":["id","owner","embedding"],"filter":{"owner":{"_eq":"X-Hasura-User-Id"}}}}}' >>"$LOG" || fail "user select permission"
nself db hasura metadata export >>"$LOG" 2>&1 || fail "nself db hasura metadata export"

TABLES_BEFORE="$(meta '{"type":"export_metadata","args":{}}' | jq '[.sources[].tables[]] | length')"
ROLES_BEFORE="$(psqlc 'SELECT count(*) FROM pg_roles')"
GRANTS_BEFORE="$(psqlc "SELECT coalesce(relacl::text,'') || '|' || relowner::regrole::text FROM pg_class WHERE oid = 'public.docs'::regclass")"
GRANT_ROWS_BEFORE="$(psqlc "SELECT grantee || ':' || privilege_type FROM information_schema.role_table_grants WHERE table_schema='public' AND table_name='docs' ORDER BY 1")"

step "E513: a text column and an untracked table write nothing"
before_files="$(ls migrations | wc -l | tr -d ' ')"
set +e
nself db generate vector-search public.docs.owner >"$W/e513a.out" 2>&1
code_a=$?
psqlc 'CREATE TABLE public.notes (id serial PRIMARY KEY, embedding vector(3))' >/dev/null
nself db generate vector-search public.notes.embedding >"$W/e513b.out" 2>&1
code_b=$?
set -e
[ "$code_a" != "0" ] && grep -q "E513" "$W/e513a.out" || fail "text column was not refused with E513 (exit $code_a)"
[ "$code_b" != "0" ] && grep -q "E513" "$W/e513b.out" || fail "untracked table was not refused with E513 (exit $code_b)"
[ "$(ls migrations | wc -l | tr -d ' ')" = "$before_files" ] || fail "a refused run wrote a migration"
psqlc 'DROP TABLE public.notes' >/dev/null

step "generate, migrate, then track"
nself db generate vector-search public.docs.embedding >"$W/gen.out" 2>&1 || { cat "$W/gen.out"; fail "generate vector-search"; }
FILE="$(ls migrations/*_vector_search_docs_embedding.sql 2>/dev/null | head -n 1)"
[ -n "$FILE" ] || fail "no vector-search migration was written"
grep -q 'RETURNS SETOF "public"."docs"' "$FILE" || fail "the function does not return SETOF the table"
if grep -qi 'LIMIT\|SECURITY DEFINER' "$FILE"; then fail "the migration holds a LIMIT or SECURITY DEFINER"; fi
nself db migrate up --migration-dir migrations >>"$LOG" 2>&1 || fail "applying the vector-search migration"

step "E514: a select role that hides the vector column blocks --track and changes nothing"
sel_perm staff '["id","owner"]' || fail "staff select permission"
set +e
nself db generate vector-search public.docs.embedding --track >"$W/e514hidden.out" 2>&1
code=$?
set -e
[ "$code" != "0" ] && grep -q "E514" "$W/e514hidden.out" && grep -q "staff" "$W/e514hidden.out" \
  || { cat "$W/e514hidden.out"; fail "a role hiding the column was not refused with E514 naming it (exit $code)"; }
[ "$(live_funcs)" = "0" ] || fail "a refused --track changed live metadata"
sel_perm staff '["id","owner","embedding"]' '{"owner":{"_eq":"bob"}}' || fail "fixing the staff permission"
nself db hasura metadata export >>"$LOG" 2>&1 || fail "nself db hasura metadata export (after the fix)"

nself db generate vector-search public.docs.embedding --track --json >"$W/track1.json" 2>"$W/track1.err" || { cat "$W/track1.err"; fail "generate --track"; }
jq -e '.data.tracked == true and .data.disk_metadata == "updated"' "$W/track1.json" >/dev/null \
  || { cat "$W/track1.json"; fail "first --track did not track and mirror the entry"; }
# A rerun changes nothing.
nself db generate vector-search public.docs.embedding --track --json >"$W/track2.json" 2>/dev/null || fail "second --track"
jq -e '.data.tracked == true and .data.disk_metadata == "unchanged" and .data.migration_written == false' "$W/track2.json" >/dev/null \
  || { cat "$W/track2.json"; fail "second --track was not a no-op"; }
[ "$(ls migrations/*_vector_search_docs_embedding.sql | wc -l | tr -d ' ')" = "1" ] || fail "reruns wrote extra migration files"

step "inference: staff (full columns, not in --roles) is served by its own select filter"
STAFF="$(mint carol staff)"
OUT="$(gql "$STAFF" 'query { docs_nearest_embedding(args: {query: "[1,0,0]"}, limit: 100) { owner } }')"
printf '%s\n' "$OUT" >>"$LOG"
printf '%s' "$OUT" | jq -e '.data.docs_nearest_embedding | length == 3 and all(.owner == "bob")' >/dev/null \
  || fail "staff did not get exactly the rows its own select filter allows: $OUT"

step "the filtered role sees only its own rows, nearest first"
ALICE="$(mint alice user)"
BOB="$(mint bob user)"
OUT="$(gql "$ALICE" "$QUERY")"
printf '%s\n' "$OUT" >>"$LOG"
printf '%s' "$OUT" | jq -e '.data.docs_nearest_embedding | map(.owner) == ["alice","alice"]' >/dev/null \
  || fail "alice did not get exactly her own 2 rows: $OUT"
printf '%s' "$OUT" | jq -e '.data.docs_nearest_embedding | map(.embedding) == ["[0.5,0.5,0]","[0.2,0.8,0]"]' >/dev/null \
  || fail "alice's rows are not in distance order: $OUT"
OUT="$(gql "$BOB" "$QUERY")"
printf '%s' "$OUT" | jq -e '.data.docs_nearest_embedding | map(.embedding) == ["[1,0,0]","[0.9,0.1,0]"]' >/dev/null \
  || fail "bob did not get his own 2 nearest rows: $OUT"
ALL="$(gql "$ALICE" 'query { docs_nearest_embedding(args: {query: "[1,0,0]"}, limit: 100) { owner } }')"
printf '%s' "$ALL" | jq -e '.data.docs_nearest_embedding | length == 3 and all(.owner == "alice")' >/dev/null \
  || fail "a large limit leaked rows of another owner: $ALL"

step "anonymous is denied"
ANON="$(mint nobody anonymous)"
OUT="$(gql "$ANON" "$QUERY")"
printf '%s' "$OUT" | jq -e '.errors[0].message | test("not found in type")' >/dev/null \
  || fail "anonymous did not get a field-not-found error: $OUT"
OUT="$(curl -sS -X POST "$HASURA/v1/graphql" -H 'Content-Type: application/json' -d "$(jq -nc --arg q "$QUERY" '{query:$q}')")"
printf '%s' "$OUT" | jq -e '(.errors | length) > 0 and (.data == null)' >/dev/null \
  || fail "a request with no credentials returned data: $OUT"

step "the function inlines (no Function Scan)"
EXPL="$(curl -sS -X POST "$HASURA/v1/graphql/explain" -H "X-Hasura-Admin-Secret: $ADMIN_SECRET" -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg q "$QUERY" '{query:{query:$q}, user:{"x-hasura-role":"user","x-hasura-user-id":"alice"}}')")"
printf '%s\n' "$EXPL" >>"$LOG"
printf '%s' "$EXPL" | jq -e '(.[0].plan | length) > 0' >/dev/null || fail "Hasura returned no plan: $EXPL"
if printf '%s' "$EXPL" | grep -q "Function Scan"; then fail "the Hasura plan has a Function Scan: $EXPL"; fi
printf '%s' "$EXPL" | grep -q "docs" || fail "the plan does not scan docs: $EXPL"

step "E514 for a role without select, and from the on-disk metadata"
set +e
nself db generate vector-search public.docs.embedding --track --roles guest >"$W/e514.out" 2>&1
code=$?
set -e
[ "$code" != "0" ] && grep -q "E514" "$W/e514.out" || fail "a role without select was not refused with E514 (exit $code)"
sel_perm viewer '["id","owner"]' || fail "viewer select permission"
nself db hasura metadata export >>"$LOG" 2>&1 || fail "nself db hasura metadata export (viewer)"
files_before="$(ls migrations | wc -l | tr -d ' ')"
set +e
nself db generate vector-search public.docs.embedding >"$W/e514disk.out" 2>&1
code=$?
set -e
[ "$code" != "0" ] && grep -q "E514" "$W/e514disk.out" && grep -q "viewer" "$W/e514disk.out" \
  || { cat "$W/e514disk.out"; fail "the on-disk hidden column was not refused with E514 naming viewer (exit $code)"; }
[ "$(ls migrations | wc -l | tr -d ' ')" = "$files_before" ] || fail "the E514 run wrote a migration"
drop_perm viewer || fail "dropping viewer"
nself db hasura metadata export >>"$LOG" 2>&1 || fail "nself db hasura metadata export (viewer removed)"

step "a drifted disk is left alone"
sel_perm auditor '"*"' || fail "auditor select permission"
SUM_BEFORE="$(cat hasura/metadata/*.yaml | openssl dgst -sha256 | awk '{print $NF}')"
nself db generate vector-search public.docs.embedding --track --roles user,auditor --json >"$W/track3.json" 2>/dev/null || fail "--track with a drifted disk"
jq -e '.data.tracked == true and .data.disk_metadata == "skipped" and (.data.note | length) > 0' "$W/track3.json" >/dev/null \
  || { cat "$W/track3.json"; fail "a drifted disk was not skipped with a note"; }
SUM_AFTER="$(cat hasura/metadata/*.yaml | openssl dgst -sha256 | awk '{print $NF}')"
[ "$SUM_BEFORE" = "$SUM_AFTER" ] || fail "on-disk metadata changed although it had drifted"
meta '{"type":"export_metadata","args":{}}' | jq -e '[.sources[].functions[]? | select(.function.name=="docs_nearest_embedding") | .permissions[].role] | sort == ["auditor","user"]' >/dev/null \
  || fail "live metadata does not hold both roles after the drifted run"

step "live metadata and the catalog are unchanged apart from the function"
TABLES_AFTER="$(meta '{"type":"export_metadata","args":{}}' | jq '[.sources[].tables[]] | length')"
[ "$TABLES_BEFORE" = "$TABLES_AFTER" ] || fail "tracked table count changed: $TABLES_BEFORE -> $TABLES_AFTER"
ROLES_AFTER="$(psqlc 'SELECT count(*) FROM pg_roles')"
GRANTS_AFTER="$(psqlc "SELECT coalesce(relacl::text,'') || '|' || relowner::regrole::text FROM pg_class WHERE oid = 'public.docs'::regclass")"
GRANT_ROWS_AFTER="$(psqlc "SELECT grantee || ':' || privilege_type FROM information_schema.role_table_grants WHERE table_schema='public' AND table_name='docs' ORDER BY 1")"
[ "$ROLES_BEFORE" = "$ROLES_AFTER" ] || fail "pg_roles count changed: $ROLES_BEFORE -> $ROLES_AFTER"
[ "$GRANTS_BEFORE" = "$GRANTS_AFTER" ] || fail "the ACL of public.docs changed"
[ "$GRANT_ROWS_BEFORE" = "$GRANT_ROWS_AFTER" ] || fail "the grant list of public.docs changed"

echo "PASS"
