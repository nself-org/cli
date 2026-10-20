#!/usr/bin/env bash
# Purpose: prove `nself db import supabase` end to end on Linux: direct DB snapshot without
#          Supabase schemas or policies, gotrue users with bcrypt hashes, storage objects through
#          the storage REST API, RLS translated or reported, shims, sign-in with the old password.
# Usage:   run at the repo root with `nself` on PATH, docker, bash, curl, jq, psql, openssl.
# Env:     W optional scratch dir (default mktemp -d).
# Exit:    0 PASS, 1 FAIL.
# Constraints: every credential is generated here and used only against local fixtures.
set -euo pipefail
umask 022
export NSELF_V15=1
ROOT=$(pwd)
: "${W:=$(mktemp -d)}"
export TMPDIR="$W"
command -v openssl >/dev/null 2>&1 || apk add -q openssl >/dev/null
rand() { openssl rand -hex "$1"; }
b64() { openssl base64 -A | tr '+/' '-_' | tr -d '='; }
jwt() { # role -> HS256 JWT signed with the fixture secret
  local h p s
  h=$(printf '{"alg":"HS256","typ":"JWT"}' | b64)
  p=$(printf '{"role":"%s","iss":"supabase","iat":1700000000,"exp":4102444800}' "$1" | b64)
  s=$(printf '%s.%s' "$h" "$p" | openssl dgst -sha256 -hmac "$FIXTURE_JWT_SECRET" -binary | b64)
  printf '%s.%s.%s' "$h" "$p" "$s"
}
export FIXTURE_PASSWORD FIXTURE_JWT_SECRET FIXTURE_ANON_KEY FIXTURE_SERVICE_KEY
FIXTURE_PASSWORD=$(rand 24)
FIXTURE_JWT_SECRET=$(rand 32)
FIXTURE_ANON_KEY=$(jwt anon)
FIXTURE_SERVICE_KEY=$(jwt service_role)
export FIXTURE_PORT=${FIXTURE_PORT:-$((22000 + RANDOM % 20000))}
export FIXTURE_AUTH_PORT=$((FIXTURE_PORT + 1))
export TARGET_PG_PORT=$((FIXTURE_PORT + 2)) TARGET_HASURA_PORT=$((FIXTURE_PORT + 3)) TARGET_AUTH_PORT=$((FIXTURE_PORT + 4)) TARGET_HTTP_PORT=$((FIXTURE_PORT + 5)) TARGET_HTTPS_PORT=$((FIXTURE_PORT + 6)) TARGET_S3_PORT=$((FIXTURE_PORT + 7))
export FIXTURE_STORAGE_PORT=$((FIXTURE_PORT + 8))
FIXTURE=$ROOT/test/fixtures/import/supabase/compose.yml
PROJECT=a13-$(date +%s)-$RANDOM
TARGET=$W/target
mkdir -p "$TARGET"
owned=0
cleanup() {
  docker compose -p "$PROJECT-src" -f "$FIXTURE" down -v >/dev/null 2>&1 || true
  if [ "$owned" = 1 ]; then (cd "$TARGET" && docker compose down -v >/dev/null 2>&1) || true; fi
}
trap cleanup EXIT
fail() {
  echo "FAIL: $1"
  docker compose -p "$PROJECT-src" -f "$FIXTURE" logs --tail 25 auth storage 2>&1 | sed 's/eyJ[A-Za-z0-9_.-]*/<jwt>/g' | head -60 || true
  exit 1
}
docker compose -p "$PROJECT-src" -f "$FIXTURE" up -d
AUTH=http://127.0.0.1:$FIXTURE_AUTH_PORT
STORAGE_ORIGIN=http://127.0.0.1:$FIXTURE_STORAGE_PORT
STORAGE=$STORAGE_ORIGIN/storage/v1
storage_up() { curl -fsS "$STORAGE/status"; }
SRC=postgresql://postgres:$FIXTURE_PASSWORD@127.0.0.1:$FIXTURE_PORT/postgres
for _ in $(seq 1 90); do
  if curl -fsS "$AUTH/health" >/dev/null 2>&1 && storage_up >/dev/null 2>&1 \
    && [ "$(psql "$SRC" -Atc "SELECT to_regclass('auth.users') IS NOT NULL AND to_regclass('storage.objects') IS NOT NULL" 2>/dev/null || true)" = t ]; then break; fi
  sleep 2
done
curl -fsS "$AUTH/health" >/dev/null || fail 'gotrue not up'
storage_up >/dev/null || fail 'storage-api not up'
for i in 1 2 3; do
  curl -fsS "$AUTH/signup" -H 'Content-Type: application/json' -H "apikey: $FIXTURE_ANON_KEY" \
    -d "{\"email\":\"fake-supabase-$i@example.invalid\",\"password\":\"FAKE-USER-PASSWORD-$i\"}" >"$W/signup-$i.json" || fail signup
  chmod 600 "$W/signup-$i.json"
done
test "$(psql "$SRC" -Atc 'SELECT count(*) FROM auth.users')" = 3 || fail 'signup count'
test "$(psql "$SRC" -Atc "SELECT count(*) FROM auth.users WHERE encrypted_password LIKE '\$2%'")" = 3 || fail 'gotrue hash is not bcrypt'
psql "$SRC" -v ON_ERROR_STOP=1 -q -f "$ROOT/test/fixtures/import/supabase/seed.sql" || fail seed
curl -fsS -X POST "$STORAGE/bucket" -H "Authorization: Bearer $FIXTURE_SERVICE_KEY" -H 'Content-Type: application/json' -d '{"id":"avatars","name":"avatars","public":false}' >/dev/null || fail bucket
for key in a.png dir/b.png c.txt; do
  printf 'fake object %s\n' "$key" >"$W/obj"
  curl -fsS -X POST "$STORAGE/object/avatars/$key" -H "Authorization: Bearer $FIXTURE_SERVICE_KEY" -H 'Content-Type: application/octet-stream' --data-binary "@$W/obj" >/dev/null || fail upload
done
test "$(psql "$SRC" -Atc 'SELECT count(*) FROM storage.objects')" = 3 || fail 'object count'
export SUPA_DB="$SRC" SUPA_KEY="$FIXTURE_SERVICE_KEY"
cd "$TARGET"
nself init --non-interactive --name "$PROJECT" --no-pgvector >"$W/init.log" 2>&1 || fail init
F_HASURA=FAKE-SUPA-HASURA-ADMIN-SECRET-0000000000000
cat >>.env.local <<ENV
ENV=dev
POSTGRES_PASSWORD=FAKE-SUPA-POSTGRES-PASSWORD-0000000000
HASURA_GRAPHQL_ADMIN_SECRET=$F_HASURA
HASURA_JWT_KEY=FAKE-SUPA-JWT-KEY-00000000000000000000000000
AUTH_SMTP_PASS=FAKE-SUPA-SMTP-PASS-000000000000
AUTH_EMAIL_SIGNIN_EMAIL_VERIFIED_REQUIRED=false
AUTH_SMTP_HOST=mailpit
AUTH_ENABLED=true
MINIO_ENABLED=false
OBJECT_STORAGE_ENABLED=true
OBJECT_STORAGE_BUCKETS=a13-fixture
SEAWEEDFS_PORT=$TARGET_S3_PORT
S3_ACCESS_KEY=FAKE-SUPA-S3-ACCESS-KEY-0000
S3_SECRET_KEY=FAKE-SUPA-S3-SECRET-KEY-000000000000000
POSTGRES_PORT=$TARGET_PG_PORT
HASURA_PORT=$TARGET_HASURA_PORT
AUTH_PORT=$TARGET_AUTH_PORT
NGINX_HTTP_PORT=$TARGET_HTTP_PORT
NGINX_HTTPS_PORT=$TARGET_HTTPS_PORT
PGVECTOR_ENABLED=false
REDIS_ENABLED=false
MAILPIT_ENABLED=false
NSELF_ADMIN_ENABLED=false
ENV
chmod 600 .env.local
nself build --force >"$W/build.log" 2>&1 || { tail -30 "$W/build.log"; fail build; }
owned=1
nself start --timeout 300 --skip-build >"$W/start.log" 2>&1 || { tail -30 "$W/start.log"; fail start; }
PGC="${PROJECT}_postgres"
PGDB="$(cat .env .env.local 2>/dev/null | sed -n 's/^POSTGRES_DB=//p' | tail -1)"
tpg() { docker exec "$PGC" psql -X -U postgres -d "$PGDB" -v ON_ERROR_STOP=1 -At -c "$1"; }
nself db import supabase --db-url-env SUPA_DB --api-url "$STORAGE_ORIGIN" --service-key-env SUPA_KEY --plan --json >"$W/plan.json" || fail plan
jq -e '.data.result=="pass" and (.data.steps|length)>0' "$W/plan.json" >/dev/null || fail 'plan result'
test ! -d .nself/import || fail 'plan wrote a stage'
nself db import supabase --db-url-env SUPA_DB --api-url "$STORAGE_ORIGIN" --service-key-env SUPA_KEY --json >"$W/import.json" 2>"$W/import.err" || { cat "$W/import.err" "$W/import.json" | head -40; fail import; }
jq -e '.data.result=="pass" and (.data.mismatches|length)==0 and (.data.tables|length)==2 and ([.data.tables[].equal]|all)' "$W/import.json" >/dev/null || fail 'tables'
jq -e '.data.users.source==3 and .data.users.target==3 and .data.users.equal==true and (.data.users.reset_required|length)==0' "$W/import.json" >/dev/null || fail users
jq -e '.data.storage.source_objects==3 and .data.storage.target_objects==3 and .data.storage.equal==true' "$W/import.json" >/dev/null || fail storage
jq -e '(.data.untranslated_policies|length)==2 and ([.data.untranslated_policies[].table]|sort)==["public.announcements","storage.objects"]' "$W/import.json" >/dev/null || fail 'untranslated policies'
jq -e '[.data.exemptions[].id]|index("trigger:auth.users#on_auth_user_created")!=null' "$W/import.json" >/dev/null || fail 'auth trigger not reported'
jq -e '(.data.compat_shims|index("auth.uid"))!=null and (.data.compat_shims|index("extensions.uuid_generate_v4"))!=null' "$W/import.json" >/dev/null || fail 'compat shims'
jq -e '.data.hasura.equal==true' "$W/import.json" >/dev/null || fail hasura
# No policy, no row-level-security flag and no Supabase schema reached the target.
test "$(tpg "SELECT count(*) FROM pg_policies WHERE schemaname='public'")" = 0 || fail 'policy restored'
test "$(tpg "SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relrowsecurity")" = 0 || fail 'rls restored'
test "$(tpg "SELECT count(*) FROM pg_namespace WHERE nspname IN ('realtime','supabase_functions','graphql','vault','pgsodium')")" = 0 || fail 'supabase schema restored'
# A user signs in to the target with the password set on Supabase.
curl -fsS "http://127.0.0.1:$TARGET_AUTH_PORT/signin/email-password" -H 'Content-Type: application/json' \
  -d '{"email":"fake-supabase-1@example.invalid","password":"FAKE-USER-PASSWORD-1"}' >"$W/signin.json" || fail signin
chmod 600 "$W/signin.json"
TOKEN=$(jq -r '.session.accessToken' "$W/signin.json")
UID1=$(jq -r '.session.user.id' "$W/signin.json")
test -n "$TOKEN" -a "$TOKEN" != null || fail 'no access token'
gql() { curl -fsS "http://127.0.0.1:$TARGET_HASURA_PORT/v1/graphql" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d "$1"; }
gql '{"query":"{ notes { id user_id } }"}' >"$W/notes.json" || fail 'notes query'
jq -e --arg u "$UID1" '(.data.notes|length)==1 and .data.notes[0].user_id==$u' "$W/notes.json" >/dev/null || fail 'select permission is not owner-filtered'
# The auth.uid() default fills the owner column with the caller's id.
gql '{"query":"mutation { insert_notes_one(object: {title: \"from hasura\"}) { user_id } }"}' >"$W/insert.json" || fail 'insert request'
jq -e --arg u "$UID1" '.data.insert_notes_one.user_id==$u' "$W/insert.json" >/dev/null || fail 'insert did not fill user_id from auth.uid()'
# Cross-user mutations must be rejected or affect zero rows.
for operation in update delete; do
  if [ "$operation" = update ]; then
    mutation="mutation { update_notes(where: {user_id: {_neq: \"$UID1\"}}, _set: {title: \"forbidden\"}) { affected_rows } }"
  else
    mutation="mutation { delete_notes(where: {user_id: {_neq: \"$UID1\"}}) { affected_rows } }"
  fi
  gql "$(jq -nc --arg q "$mutation" '{query:$q}')" >"$W/$operation-denied.json" || fail "$operation request"
  jq -e --arg field "${operation}_notes" '.errors!=null or .data[$field].affected_rows==0' "$W/$operation-denied.json" >/dev/null || fail "cross-user $operation allowed"
done
# The untranslated policy leaves the table closed to the user role.
gql '{"query":"{ announcements { id } }"}' >"$W/announcements.json" || true
jq -e '.errors!=null' "$W/announcements.json" >/dev/null || fail 'untranslated policy table is open'
if nself db import supabase --db-url-env SUPA_DB --api-url "$STORAGE_ORIGIN" --service-key-env SUPA_KEY --json >"$W/rerun.json" 2>"$W/rerun.err"; then fail 'rerun did not refuse'; fi
grep -q E519 "$W/rerun.json" "$W/rerun.err" || fail 'rerun refusal is not E519'
# Redaction: no credential and no password hash in any output or report.
HASHES=$(psql "$SRC" -Atc 'SELECT encrypted_password FROM auth.users')
count=0
for f in "$W"/plan.json "$W"/import.json "$W"/import.err "$W"/rerun.json "$W"/rerun.err "$TARGET"/.nself/import/*/report.json; do
  [ -f "$f" ] || continue
  count=$((count + 1))
  for secret in "$FIXTURE_PASSWORD" "$FIXTURE_SERVICE_KEY" "$FIXTURE_JWT_SECRET"; do
    if grep -qF -- "$secret" "$f"; then fail "credential appeared in $f"; fi
  done
  while read -r h; do
    [ -n "$h" ] || continue
    if grep -qF -- "$h" "$f"; then fail "password hash appeared in $f"; fi
  done <<<"$HASHES"
done
test "$count" -ge 4 || fail 'redaction scan read too few files'
echo "PASS redaction; files read=$count"
echo 'PASS Linux Supabase import: snapshot without policies, bcrypt users sign in, 3 objects, 2 listed policies, shims, auth.uid() default'
