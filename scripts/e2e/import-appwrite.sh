#!/usr/bin/env bash
# Linux e2e for `nself db import appwrite` (P7-ADOPT-16, EPIC D11/D18).
# Seeds the pinned Appwrite fixture through its REST API, imports it into a fresh nSelf target and
# checks zero mismatches, the reset_required users, storage, the relationship and credential redaction.
set -euo pipefail
umask 022
# nself build makes a self-signed certificate; the docker:cli image ships without openssl.
command -v openssl >/dev/null || apk add -q openssl >/dev/null
export NSELF_V15=1
ROOT=$(pwd)
: "${W:=$(mktemp -d)}"
export TMPDIR="$W"
export FIXTURE_PASSWORD
FIXTURE_PASSWORD=$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')
export FIXTURE_PORT=${FIXTURE_PORT:-$((22000 + RANDOM % 20000))}
export TARGET_PG_PORT=$((FIXTURE_PORT + 2)) TARGET_HASURA_PORT=$((FIXTURE_PORT + 3)) TARGET_AUTH_PORT=$((FIXTURE_PORT + 4)) TARGET_HTTP_PORT=$((FIXTURE_PORT + 5)) TARGET_HTTPS_PORT=$((FIXTURE_PORT + 6))
export SEAWEEDFS_PORT=$((FIXTURE_PORT + 7))
FIXTURE=$ROOT/test/fixtures/import/appwrite/compose.yml
PROJECT=a16-$(date +%s)-$RANDOM
export FIXTURE_PROJECT="$PROJECT-src"
TARGET=$W/target
mkdir -p "$TARGET"
cleanup() {
  docker compose -p "$FIXTURE_PROJECT" -f "$FIXTURE" down -v >/dev/null 2>&1 || true
  if [[ -f "$TARGET/docker-compose.yml" ]]; then (cd "$TARGET" && docker compose down -v >/dev/null 2>&1) || true; fi
}
trap cleanup EXIT
docker compose -p "$FIXTURE_PROJECT" -f "$FIXTURE" up -d --wait
export ENDPOINT="http://localhost:$FIXTURE_PORT/v1"
bash "$ROOT/test/fixtures/import/appwrite/seed.sh" "$W/seed"
export AW_KEY
AW_KEY=$(cat "$W/seed/read.key")
cd "$TARGET"
nself init --non-interactive
# Credentials go only into the private target environment, never arguments.
{
  for kv in "PROJECT_NAME=$PROJECT" POSTGRES_DB=nself POSTGRES_USER=postgres POSTGRES_VERSION=16 "POSTGRES_PORT=$TARGET_PG_PORT" "HASURA_PORT=$TARGET_HASURA_PORT" "AUTH_PORT=$TARGET_AUTH_PORT" "NGINX_HTTP_PORT=$TARGET_HTTP_PORT" "NGINX_HTTPS_PORT=$TARGET_HTTPS_PORT" PGVECTOR_ENABLED=false POSTGRES_EXTENSIONS= "POSTGRES_PASSWORD=$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')" "HASURA_GRAPHQL_ADMIN_SECRET=$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')" AUTH_ENABLED=true REDIS_ENABLED=false MAILPIT_ENABLED=false MINIO_ENABLED=false OBJECT_STORAGE_ENABLED=true "SEAWEEDFS_PORT=$SEAWEEDFS_PORT" "S3_ACCESS_KEY=$(head -c 12 /dev/urandom | od -An -tx1 | tr -d ' \n')" "S3_SECRET_KEY=$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')"; do
    printf '\n%s\n' "$kv" >> .env
  done
}
chmod 600 .env
nself start
IMPORT=(nself db import appwrite --endpoint "$ENDPOINT" --project proj1 --api-key-env AW_KEY)
"${IMPORT[@]}" --plan --json > "$W/plan.json"
jq -e '.data.result=="pass" and .data.detection.kind=="appwrite" and .data.detection.source_users==4' "$W/plan.json"
test ! -d .nself/import
"${IMPORT[@]}" --json > "$W/import.json"
jq -e '.data.result=="pass" and (.data.mismatches|length)==0 and (.data.tables|length)==2 and ([.data.tables[].equal]|all)' "$W/import.json"
jq -e '([.data.tables[]|select(.name=="books")]|.[0].source_rows)==25 and ([.data.tables[]|select(.name=="authors")]|.[0].source_rows)==5' "$W/import.json"
jq -e '.data.users.source==4 and .data.users.target==4 and .data.users.equal==true' "$W/import.json"
# The two argon2 users keep no password and are listed with the reset path; bcrypt users keep theirs.
jq -e '(.data.users.reset_required|length)==2 and ([.data.users.reset_required[].email]|sort)==["argon1@fixture.invalid","argon2@fixture.invalid"]' "$W/import.json"
test "$(docker compose exec -T postgres psql -U postgres -d nself -Atc "SELECT count(*) FROM auth.users WHERE password_hash LIKE '\$2%'")" = 2
test "$(docker compose exec -T postgres psql -U postgres -d nself -Atc "SELECT count(*) FROM auth.users WHERE password_hash IS NULL OR password_hash = ''")" = 2
jq -e '.data.storage.source_objects==3 and .data.storage.target_objects==3 and .data.storage.equal==true' "$W/import.json"
# Document permissions are reported, never dropped silently.
jq -e '[.data.exemptions[]|select(.reason|test("permissions"))]|length>=1' "$W/import.json"
# The relationship exists in Hasura (verified by the importer and re-checked here).
jq -e '.data.hasura.equal==true and (.data.hasura.diffs|length)==0' "$W/import.json"
SECRET=$(sed -n 's/^HASURA_GRAPHQL_ADMIN_SECRET=//p' .env | tail -1)
curl -fsS -H "x-hasura-admin-secret: $SECRET" -H 'Content-Type: application/json' -d '{"type":"export_metadata","args":{}}' "http://127.0.0.1:$TARGET_HASURA_PORT/v1/metadata" | jq -e '[.sources[].tables[]|select(.table.name=="books")|.object_relationships[]?|select(.name=="author_rel")]|length==1'
test "$(docker compose exec -T postgres psql -U postgres -d nself -Atc "SELECT count(*) FROM main.books b JOIN main.authors a ON a.id=b.author")" = 25
# A wrong key is refused with a fixed message and no echo.
export BAD_KEY=wrong-fixture-key
if nself db import appwrite --endpoint "$ENDPOINT" --project proj1 --api-key-env BAD_KEY --plan --json > "$W/badkey.json" 2> "$W/badkey.err"; then echo 'FAIL: bad key accepted'; exit 1; fi
grep -q E517 "$W/badkey.json" "$W/badkey.err"
n=0
for f in "$W"/*.json "$W"/*.err "$TARGET"/.nself/import/*/report.json; do
  [[ -f $f ]] || continue
  n=$((n + 1))
  if grep -qF -e "$AW_KEY" -e "$FIXTURE_PASSWORD" -e "$BAD_KEY" "$f"; then echo "FAIL: credential appeared in $f"; exit 1; fi
done
test "$n" -ge 4
echo "PASS Linux appwrite import, zero mismatches, reset_required, storage, relationship, redaction; files read=$n"
