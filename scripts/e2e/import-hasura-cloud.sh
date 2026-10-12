#!/usr/bin/env bash
# Purpose: Linux acceptance for `nself db import hasura-cloud` (P7-ADOPT-11).
# Inputs: W shared host/container path; nself, docker, curl, jq, psql on PATH.
# Outputs: non-vacuous PASS assertions. Constraints: fake credentials only; cleanup owns only the
# resources this run created (compose projects named a11hc-*).
set -euo pipefail
umask 022
export NSELF_V15=1
# Build generates self-signed certs through openssl; the docker CLI image lacks openssl and python3.
command -v openssl >/dev/null || apk add -q openssl >/dev/null
command -v python3 >/dev/null || apk add -q python3 >/dev/null
ROOT=$(pwd)
: "${W:=$(mktemp -d)}"
export TMPDIR="$W"
FIXTURE=$ROOT/test/fixtures/import/hasura-cloud
export FIXTURE_PASSWORD FIXTURE_ADMIN_SECRET
FIXTURE_PASSWORD=$(python3 -c 'import secrets;print(secrets.token_hex(24))')
FIXTURE_ADMIN_SECRET=$(python3 -c 'import secrets;print(secrets.token_hex(24))')
export FIXTURE_PORT=${FIXTURE_PORT:-$((22000 + RANDOM % 20000))}
export FIXTURE_HASURA_PORT=$((FIXTURE_PORT + 1))
export TARGET_PG_PORT=$((FIXTURE_PORT + 2)) TARGET_HASURA_PORT=$((FIXTURE_PORT + 3)) TARGET_AUTH_PORT=$((FIXTURE_PORT + 4)) TARGET_HTTP_PORT=$((FIXTURE_PORT + 5)) TARGET_HTTPS_PORT=$((FIXTURE_PORT + 6))
PROJECT=a11hc-$(date +%s)-$RANDOM
TARGET=$W/target
mkdir -p "$TARGET"
cleanup() {
  docker compose -p "$PROJECT-src" -f "$FIXTURE/compose.yml" down -v >/dev/null 2>&1 || true
  if [[ -f "$TARGET/docker-compose.yml" ]]; then (cd "$TARGET" && docker compose down -v >/dev/null 2>&1) || true; fi
}
trap cleanup EXIT
trap 'echo "FAIL at line $LINENO"; cat "$W/last.json" 2>/dev/null || true' ERR
docker compose -p "$PROJECT-src" -f "$FIXTURE/compose.yml" up -d --wait
SRC_HASURA="http://127.0.0.1:$FIXTURE_HASURA_PORT"
curl -fsS -H "X-Hasura-Admin-Secret: $FIXTURE_ADMIN_SECRET" -H 'Content-Type: application/json' --data @"$FIXTURE/metadata.json" "$SRC_HASURA/v1/metadata" > "$W/source-metadata-apply.json"
export SRC_URL="postgresql://postgres:$FIXTURE_PASSWORD@127.0.0.1:$FIXTURE_PORT/postgres"
cd "$TARGET"
nself init --non-interactive
# Credentials go only into the private target environment, never arguments.
python3 - "$PROJECT" <<'PY'
import os, pathlib, secrets, sys
p=pathlib.Path('.env')
with p.open('a') as f:
    for k,v in {'PROJECT_NAME':sys.argv[1], 'POSTGRES_DB':'nself', 'POSTGRES_USER':'postgres', 'POSTGRES_VERSION':'16', 'POSTGRES_PORT':os.environ['TARGET_PG_PORT'], 'HASURA_PORT':os.environ['TARGET_HASURA_PORT'], 'AUTH_PORT':os.environ['TARGET_AUTH_PORT'], 'NGINX_HTTP_PORT':os.environ['TARGET_HTTP_PORT'], 'NGINX_HTTPS_PORT':os.environ['TARGET_HTTPS_PORT'], 'PGVECTOR_ENABLED':'false', 'POSTGRES_EXTENSIONS':'', 'POSTGRES_PASSWORD':secrets.token_hex(24), 'HASURA_GRAPHQL_ADMIN_SECRET':secrets.token_hex(24), 'AUTH_ENABLED':'true', 'REDIS_ENABLED':'false', 'MAILPIT_ENABLED':'false'}.items():
        f.write(f'\n{k}={v}\n')
p.chmod(0o600)
PY
nself start
TARGET_SECRET=$(sed -n 's/^HASURA_GRAPHQL_ADMIN_SECRET=//p' .env | tail -1)
TARGET_HASURA="http://127.0.0.1:$TARGET_HASURA_PORT"
gql() { # role, user id, query
  curl -fsS -H "X-Hasura-Admin-Secret: $TARGET_SECRET" -H "X-Hasura-Role: $1" -H "X-Hasura-User-Id: $2" -H 'Content-Type: application/json' --data "$(jq -cn --arg q "$3" '{query:$q}')" "$TARGET_HASURA/v1/graphql" | tee "$W/last.json"
}
# Refusals happen before any write: a wrong admin secret, and plain http to a non-loopback host.
export SRC_SECRET=wrong-secret-for-the-refusal-check
if nself db import hasura-cloud --db-url-env SRC_URL --hasura-url "$SRC_HASURA" --admin-secret-env SRC_SECRET --plan --json > "$W/badsecret.json" 2> "$W/badsecret.err"; then echo 'FAIL: wrong admin secret accepted'; exit 1; fi
grep -q E517 "$W/badsecret.json" "$W/badsecret.err"
export SRC_SECRET=$FIXTURE_ADMIN_SECRET
if nself db import hasura-cloud --db-url-env SRC_URL --hasura-url http://example.com --admin-secret-env SRC_SECRET --plan --json > "$W/plainhttp.json" 2> "$W/plainhttp.err"; then echo 'FAIL: plain http to a remote host accepted'; exit 1; fi
grep -q E517 "$W/plainhttp.json" "$W/plainhttp.err"
test ! -d .nself/import
nself db import hasura-cloud --db-url-env SRC_URL --hasura-url "$SRC_HASURA/v1/graphql" --admin-secret-env SRC_SECRET --plan --json > "$W/plan.json"
jq -e '.data.result=="pass" and (.data.steps|length)>0 and .data.detection.kind=="hasura-cloud"' "$W/plan.json"
test ! -d .nself/import
nself db import hasura-cloud --db-url-env SRC_URL --hasura-url "$SRC_HASURA" --admin-secret-env SRC_SECRET --keep-stage --json > "$W/import.json"
jq -e '.data.result=="pass" and (.data.mismatches|length)==0 and (.data.tables|length)==2 and ([.data.tables[].equal]|all)' "$W/import.json"
jq -e '.data.hasura.equal==true and (.data.hasura.diffs|length)==0' "$W/import.json"
jq -e '.data.users.source==0 and .data.users.target==0 and .data.users.equal==true and .data.storage.source_objects==0' "$W/import.json"
BUNDLE=$(ls -d .nself/import/*/stage | head -1)
# The bundle holds the metadata subset, with counts and without connection strings.
jq -e '.producer.source=="hasura-cloud" and .hasura.tables==2 and .hasura.relationships==2 and .hasura.permissions==5 and .hasura.functions==1' "$BUNDLE/manifest.json"
test ! -e "$BUNDLE/auth/users.jsonl"
if grep -R -q -e 'database_url' -e "$FIXTURE_PASSWORD" "$BUNDLE/hasura"; then echo 'FAIL: connection details in exported metadata'; exit 1; fi
# Zero mismatches is not enough: the target Hasura must behave like the source for each role.
OWNER=c0000000-0000-4000-8000-000000000001
gql user "$OWNER" '{ posts(order_by: {id: asc}) { id title author { name } } }' | jq -e '(.errors|not) and (.data.posts|length)==2 and .data.posts[0].author.name=="Ada"'
# The array relationship obeys the posts permission: each author shows only the owner's posts.
gql user "$OWNER" '{ authors(order_by: {name: asc}) { name posts { id } } }' | jq -e '(.errors|not) and (.data.authors|map(.posts|length))==[1,1]'
gql editor "$OWNER" '{ posts { id title } }' | jq -e '(.errors|not) and (.data.posts|length)==4'
# The editor role may not read the body column: permissions came across column for column.
gql editor "$OWNER" '{ posts { body } }' | jq -e '.errors|length>0'
# The SQL function came across with the schema. Tracking it in Hasura is not part of the load yet.
T_PGPASS=$(sed -n 's/^POSTGRES_PASSWORD=//p' .env | tail -1)
test "$(PGPASSWORD="$T_PGPASS" psql -h 127.0.0.1 -p "$TARGET_PG_PORT" -U postgres -d nself -X -At -c "SELECT count(*) FROM public.search_posts('engine')")" = 2
# A second import is refused (E519) and leaves the target unchanged.
if nself db import hasura-cloud --db-url-env SRC_URL --hasura-url "$SRC_HASURA" --admin-secret-env SRC_SECRET --json > "$W/rerun.json" 2> "$W/rerun.err"; then echo 'FAIL: rerun did not refuse'; exit 1; fi
grep -q E519 "$W/rerun.json" "$W/rerun.err"
# A source with auth.users is an nhost project: this source says so instead of reading it as users.
PGPASSWORD="$FIXTURE_PASSWORD" psql -h 127.0.0.1 -p "$FIXTURE_PORT" -U postgres -d postgres -qc 'CREATE SCHEMA auth; CREATE TABLE auth.users (id uuid PRIMARY KEY)'
if nself db import hasura-cloud --db-url-env SRC_URL --hasura-url "$SRC_HASURA" --admin-secret-env SRC_SECRET --plan --json > "$W/nhostlike.json" 2> "$W/nhostlike.err"; then echo 'FAIL: auth.users source accepted'; exit 1; fi
grep -q 'db import nhost' "$W/nhostlike.json" "$W/nhostlike.err"
python3 - "$W" <<'PY'
import os,pathlib,sys
paths=[p for p in pathlib.Path(sys.argv[1]).rglob('*') if p.is_file() and p.suffix in ('.json','.err')]
secrets=[os.environ['FIXTURE_PASSWORD'].encode(), os.environ['FIXTURE_ADMIN_SECRET'].encode()]
assert paths
for p in paths:
    data=p.read_bytes()
    if p.parts[-2] == 'stage' or 'stage' in p.parts: continue
    for s in secrets: assert s not in data, 'credential appeared in output ' + p.name
print('PASS redaction; files read=',len(paths))
PY
echo 'PASS Linux hasura-cloud import: tables, relationships, role permissions, function, refusals, redaction'
