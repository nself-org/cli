#!/usr/bin/env bash
# Purpose: Linux acceptance for portable export using an isolated fake project.
# Inputs: W shared host/container path, nself/docker/curl/jq/psql on PATH.
# Outputs: non-vacuous PASS assertions. Constraints: fake credentials only;
# cleanup owns only e2e-adopt-export resources; shared Docker stays running.
set -euo pipefail
export NSELF_V15=1
# Build generates self-signed certs through openssl; the docker CLI image lacks it.
command -v openssl >/dev/null || apk add -q openssl >/dev/null
PROJECT=e2e-adopt-export
: "${W:?W must be a host-visible work directory}"
REPO="$PWD"
DIR="$W/$PROJECT"
test ! -e "$DIR" || { echo 'FAIL: fixture path occupied'; exit 1; }
test -z "$(docker ps -aq --filter "name=^${PROJECT}_")" || { echo 'FAIL: fixture project already exists'; exit 1; }
mkdir -p "$DIR" "$W/home"
export HOME="$W/home"
LOG="$W/export-e2e.log"
touch "$LOG"; chmod 600 "$LOG"
owned=0
cleanup() {
  if [ "$owned" = 1 ]; then
    (cd "$DIR" && docker compose --env-file .nself/compose.env -f docker-compose.yml down -v --remove-orphans >/dev/null 2>&1) || true
  fi
}
trap cleanup EXIT
fail() {
  echo "FAIL: $1"; tail -40 "$LOG"
  docker ps -a --filter "name=^${PROJECT}_" --format '{{.Names}} {{.Status}}' || true
  docker logs --tail 30 "${PROJECT}_postgres" 2>&1 || true
  exit 1
}
pass() { echo "PASS: $1"; }
cd "$DIR"
nself init --non-interactive --name "$PROJECT" --no-pgvector >"$LOG" 2>&1 || fail init
# Only obviously fake credentials, padded to meet the config validators
# (Postgres >=16, Hasura admin secret >=32, JWT key >=32). Do not print signup
# responses (JWTs).
F_PG=FAKE-EXPORT-POSTGRES-PASSWORD-0000000000
F_HASURA=FAKE-EXPORT-HASURA-ADMIN-SECRET-0000000000000
F_JWT=FAKE-EXPORT-JWT-KEY-0000000000000000000000000
F_SMTP=FAKE-EXPORT-SMTP-PASS-000000000000
F_S3A=FAKE-EXPORT-S3-ACCESS-KEY-0000
F_S3S=FAKE-EXPORT-S3-SECRET-KEY-00000000000000000
S3_AUTH="$F_S3A:$F_S3S"
cat >>.env.local <<ENV
ENV=dev
POSTGRES_PASSWORD=$F_PG
HASURA_GRAPHQL_ADMIN_SECRET=$F_HASURA
HASURA_JWT_KEY=$F_JWT
AUTH_SMTP_PASS=$F_SMTP
AUTH_EMAIL_SIGNIN_EMAIL_VERIFIED_REQUIRED=false
AUTH_SMTP_HOST=mailpit
MINIO_ENABLED=false
OBJECT_STORAGE_ENABLED=true
OBJECT_STORAGE_BUCKETS=export-fixture
SEAWEEDFS_PORT=19333
S3_ACCESS_KEY=$F_S3A
S3_SECRET_KEY=$F_S3S
POSTGRES_PORT=19432
HASURA_PORT=19480
AUTH_PORT=19400
NGINX_HTTP_PORT=19481
NGINX_HTTPS_PORT=19443
REDIS_PORT=19479
MAILPIT_SMTP_PORT=19425
MAILPIT_UI_PORT=19426
NSELF_ADMIN_ENABLED=false
ENV
chmod 600 .env.local
nself build --force >>"$LOG" 2>&1 || fail build
owned=1
nself start --timeout 300 --skip-build >>"$LOG" 2>&1 || fail start
PGC="${PROJECT}_postgres"
pg() { docker exec "$PGC" psql -X -U postgres -d nself -v ON_ERROR_STOP=1 -At -c "$1"; }
pg 'CREATE SCHEMA app; CREATE TABLE public.notes(id integer PRIMARY KEY, body text); INSERT INTO public.notes SELECT i, md5(i::text) FROM generate_series(1,1000) i; CREATE TABLE app.extra(id integer PRIMARY KEY, body text); INSERT INTO app.extra VALUES(1, '\''fixture'\''); CREATE TABLE public.np_export_private(id int); CREATE SCHEMA np_export_private; CREATE TABLE np_export_private.private(id int);' >>"$LOG" 2>&1 || fail seed
for i in 1 2 3; do
  curl -fsS 'http://127.0.0.1:19400/signup/email-password' -H 'Content-Type: application/json' \
    -d "{\"email\":\"fake-export-$i@example.invalid\",\"password\":\"FAKE-USER-PASSWORD-$i\"}" >"$W/signup-$i.json" || fail signup
  chmod 600 "$W/signup-$i.json"
done
test "$(pg 'SELECT count(*) FROM auth.users')" = 3 || fail 'signup count'
meta() { curl -fsS 'http://127.0.0.1:19480/v1/metadata' -H "X-Hasura-Admin-Secret: $F_HASURA" -H 'Content-Type: application/json' -d "$1"; }
for table in 'public notes' 'app extra'; do
  read -r schema name <<<"$table"
  meta "{\"type\":\"pg_track_table\",\"args\":{\"source\":\"default\",\"table\":{\"schema\":\"$schema\",\"name\":\"$name\"}}}" >>"$LOG" || fail track
  meta "{\"type\":\"pg_create_select_permission\",\"args\":{\"source\":\"default\",\"table\":{\"schema\":\"$schema\",\"name\":\"$name\"},\"role\":\"user\",\"permission\":{\"columns\":\"*\",\"filter\":{}}}}" >>"$LOG" || fail permission
done
for key in one.txt TWO.txt two.txt nested/four.txt; do
  printf 'fake object %s\n' "$key" >"$W/object"
  curl -fsS --aws-sigv4 'aws:amz:us-east-1:s3' --user "$S3_AUTH" -X PUT --data-binary "@$W/object" "http://127.0.0.1:19333/export-fixture/$key" >>"$LOG" || fail object
done
nself db export --out "$W/bundle" --json >"$W/summary.json" 2>>"$LOG" || fail export
jq -e '.data.tables==2 and .data.users==3 and .data.objects==4' "$W/summary.json" >/dev/null || fail summary
apk add -q python3 py3-jsonschema >/dev/null
python3 - "$REPO/schemas/portable-export.v1.schema.json" "$W/bundle/manifest.json" <<'PY'
import json, sys, jsonschema
jsonschema.validate(json.load(open(sys.argv[2])), json.load(open(sys.argv[1])))
PY
pass 'manifest validates against portable-export.v1.schema.json'
(cd "$W/bundle" && jq -r '.files[] | .sha256+"  "+.path' manifest.json | sha256sum -c) >>"$LOG" || fail sha256
pg_restore --list "$W/bundle/db/data.dump" >"$W/toc"
grep -q 'TABLE public notes' "$W/toc" && grep -q 'TABLE app extra' "$W/toc" || fail tables
! grep -q np_export "$W/toc" || fail 'plugin data exported'
test "$(wc -l <"$W/bundle/auth/users.jsonl" | tr -d ' ')" = 3 || fail users
jq -se 'all(.[]; .hash_algorithm=="bcrypt")' "$W/bundle/auth/users.jsonl" >/dev/null || fail bcrypt
test "$(find "$W/bundle/storage" -type f | wc -l | tr -d ' ')" = 4 || fail objects
test "$(stat -c %a "$W/bundle")" = 700 || fail 'directory mode'
test -z "$(find "$W/bundle" -type f ! -perm 600 -print)" || fail 'file modes'
test -z "$(find "$W/bundle" -type d ! -perm 700 -print)" || fail 'subdirectory modes'
grep -qx HASURA_GRAPHQL_ADMIN_SECRET "$W/bundle/config/keys.txt" || fail 'key name'
! grep -rF "$F_SMTP" "$W/bundle" --exclude=data.dump || fail secret
pass 'counts, checksums, archive, users, objects, exclusions and modes'
sleep 2
nself db export --out "$W/bundle2" --json >"$W/summary2.json" 2>>"$LOG" || fail 'second export'
jq -S 'del(.created_at)' "$W/bundle/manifest.json" >"$W/manifest1"
jq -S 'del(.created_at)' "$W/bundle2/manifest.json" >"$W/manifest2"
cmp "$W/manifest1" "$W/manifest2" || fail deterministic
pass 'unchanged project manifests identical except created_at'
expect_refusal() {
  local out="$1" code="$2"
  if nself db export --out "$out" --json >"$W/refusal.json" 2>"$W/refusal.err"; then fail "accepted $code"; fi
  grep -q "$code" "$W/refusal.json" "$W/refusal.err" || fail "missing $code"
  ! grep -q "$F_SMTP" "$W/refusal.json" "$W/refusal.err" || fail 'secret in error'
}
bundle_digest() { (cd "$W/bundle" && find . -type f -exec sha256sum {} \; | sort | sha256sum); }
before=$(bundle_digest)
expect_refusal "$W/bundle" E527
test "$(bundle_digest)" = "$before" || fail 'E527 changed output bytes'
test -f "$W/bundle/manifest.json" || fail 'E527 changed output'
pass 'nonempty output refused without changing it'
pg "COMMENT ON TABLE public.notes IS '$F_SMTP';" >>"$LOG"
expect_refusal "$W/comment-bundle" E526
test ! -e "$W/comment-bundle" || fail 'comment bundle remains'
grep -q 'db/schema.sql' "$W/refusal.json" "$W/refusal.err" || fail 'comment file unnamed'
pg 'COMMENT ON TABLE public.notes IS NULL;' >>"$LOG"
pg "UPDATE public.notes SET body='$F_SMTP' WHERE id=1;" >>"$LOG"
expect_refusal "$W/row-bundle" E526
test ! -e "$W/row-bundle" || fail 'row bundle remains'
grep -q 'db/data.dump' "$W/refusal.json" "$W/refusal.err" || fail 'row file unnamed'
test -z "$(find "$W" -maxdepth 1 -name '.nself-export-*' -print)" || fail 'private stage remains'
pass 'comment and archive-only secret rejected with full cleanup'
echo 'EXPORT E2E PASS'
