#!/usr/bin/env bash
set -euo pipefail
umask 022
export NSELF_V15=1
ROOT=$(pwd)
: "${W:=$(mktemp -d)}"
export TMPDIR="$W"
export FIXTURE_PASSWORD
FIXTURE_PASSWORD=$(python3 -c 'import secrets;print(secrets.token_hex(24))')
export FIXTURE_PORT=${FIXTURE_PORT:-$((22000 + RANDOM % 20000))}
export FIXTURE17_PORT=${FIXTURE17_PORT:-$((FIXTURE_PORT + 1))}
export TARGET_PG_PORT=$((FIXTURE_PORT + 2)) TARGET_HASURA_PORT=$((FIXTURE_PORT + 3)) TARGET_AUTH_PORT=$((FIXTURE_PORT + 4)) TARGET_HTTP_PORT=$((FIXTURE_PORT + 5)) TARGET_HTTPS_PORT=$((FIXTURE_PORT + 6))
FIXTURE=$ROOT/test/fixtures/import/postgres/compose.yml
PROJECT=a10-$(date +%s)-$RANDOM
TARGET=$W/target
mkdir -p "$TARGET"
export NSELF_PLUGIN_DIR="$W/plugins"
mkdir -p "$NSELF_PLUGIN_DIR/a10-fixture"
cp "$ROOT/test/fixtures/import/postgres/plugin/plugin.json" "$NSELF_PLUGIN_DIR/a10-fixture/plugin.json"
cleanup() {
  docker compose -p "$PROJECT-src" -f "$FIXTURE" down -v >/dev/null 2>&1 || true
  if [[ -f "$TARGET/docker-compose.yml" ]]; then (cd "$TARGET" && docker compose down -v >/dev/null 2>&1) || true; fi
}
trap cleanup EXIT
docker compose -p "$PROJECT-src" -f "$FIXTURE" up -d --wait
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
# The offline installed plugin is seeded by nself build, not by manual inserts.
docker compose exec -T postgres psql -U postgres -d nself -Atc "SELECT name FROM public.np_plugins WHERE name='a10-fixture'" | grep -qx a10-fixture
PGPASSWORD="$FIXTURE_PASSWORD" psql -h 127.0.0.1 -p "$FIXTURE_PORT" -U postgres -d postgres -Atc "SELECT current_setting('server_version')" > "$W/source-version.txt"
docker compose exec -T postgres psql -U postgres -d nself -Atc "SELECT table_name,column_name,data_type,is_nullable,column_default IS NOT NULL FROM information_schema.columns WHERE table_schema='auth' AND table_name IN ('users','user_roles') ORDER BY table_name,ordinal_position" > "$W/auth-schema.txt"
nself db import postgres --from-env SRC_URL --plan --json > "$W/plan.json"
jq -e '.data.result=="pass" and (.data.steps|length)>0' "$W/plan.json"
test ! -d .nself/import
nself db import postgres --from-env SRC_URL --json > "$W/import.json"
jq -e '.data.result=="pass" and (.data.mismatches|length)==0 and (.data.tables|length)==5' "$W/import.json"
jq -e '.data.hasura.equal==true and (.data.hasura.diffs|length)==0 and ([.data.tables[].equal]|all)' "$W/import.json"
jq -e '.data.users.source==3 and .data.users.target==3 and .data.users.equal==true' "$W/import.json"
# Both phone-only users keep a NULL email (an empty string would break hasura-auth's unique email).
test "$(docker compose exec -T postgres psql -U postgres -d nself -Atc "SELECT count(*) FROM auth.users WHERE email IS NULL AND phone_number IS NOT NULL")" = 2
docker compose exec -T postgres psql -U postgres -d nself -Atc "SELECT count(*), sum(hashtextextended(t::text,0)::numeric) FROM public.notes t" > "$W/before.txt"
if nself db import postgres --from-env SRC_URL --json > "$W/rerun.json" 2> "$W/rerun.err"; then echo 'FAIL: rerun did not refuse'; exit 1; fi
grep -q E519 "$W/rerun.json" "$W/rerun.err"
docker compose exec -T postgres psql -U postgres -d nself -Atc "SELECT count(*), sum(hashtextextended(t::text,0)::numeric) FROM public.notes t" > "$W/after.txt"
cmp "$W/before.txt" "$W/after.txt"
# Recovery: the documented reset sequence makes the partly or fully loaded target importable again.
nself db reset --yes
nself stop
nself start
# A target that already holds an auth user is refused before any write (E519) and stays unchanged.
docker compose exec -T postgres psql -U postgres -d nself -Atc "INSERT INTO auth.users (id, email, email_verified, created_at, updated_at, disabled, avatar_url, locale, phone_number_verified, default_role, is_anonymous, otp_hash_expires_at, ticket_expires_at, display_name, metadata) VALUES (gen_random_uuid(), 'pre-existing@example.invalid', false, now(), now(), false, '', 'en', false, 'user', false, 'epoch', 'epoch', '', '{}')"
REPORTS_BEFORE=$(ls -d .nself/import/*/report.json | wc -l)
if nself db import postgres --from-env SRC_URL --json > "$W/authused.json" 2> "$W/authused.err"; then echo 'FAIL: used auth.users accepted'; exit 1; fi
grep -q E519 "$W/authused.json" "$W/authused.err"
grep -q 'auth.users is not empty' "$W/authused.json" "$W/authused.err"
test "$(docker compose exec -T postgres psql -U postgres -d nself -Atc "SELECT to_regclass('public.notes') IS NULL")" = t
test "$(docker compose exec -T postgres psql -U postgres -d nself -Atc "SELECT count(*) FROM auth.users")" = 1
# The refusal comes from the target checks, before a run directory exists: no new report, nothing written.
test "$(ls -d .nself/import/*/report.json | wc -l)" = "$REPORTS_BEFORE"
docker compose exec -T postgres psql -U postgres -d nself -Atc "DELETE FROM auth.users WHERE email='pre-existing@example.invalid'"
nself db import postgres --from-env SRC_URL --json > "$W/reimport.json"
jq -e '.data.result=="pass" and (.data.mismatches|length)==0 and .data.users.equal==true and .data.wrote==true' "$W/reimport.json"
# A fresh second target is used for version and unreachable checks.
export SRC17_URL="postgresql://postgres:$FIXTURE_PASSWORD@127.0.0.1:$FIXTURE17_PORT/postgres"
if nself db import postgres --from-env SRC17_URL --plan --json > "$W/newer.json" 2> "$W/newer.err"; then echo 'FAIL: newer source accepted'; exit 1; fi
grep -q E518 "$W/newer.json" "$W/newer.err"
export BAD_SRC_URL="postgresql://postgres:$FIXTURE_PASSWORD@127.0.0.1:1/postgres?connect_timeout=1"
if nself db import postgres --from-env BAD_SRC_URL --plan --json > "$W/unreachable.json" 2> "$W/unreachable.err"; then echo 'FAIL: unreachable source accepted'; exit 1; fi
grep -q E517 "$W/unreachable.json" "$W/unreachable.err"
python3 - "$W" <<'PY'
import os,pathlib,sys
paths=[p for p in pathlib.Path(sys.argv[1]).rglob('*') if p.is_file() and p.suffix in ('.json','.err')]
secret=os.environ['FIXTURE_PASSWORD'].encode()
assert paths
for p in paths: assert secret not in p.read_bytes(), 'credential appeared in output'
print('PASS redaction; files read=',len(paths))
PY
echo 'PASS Linux Postgres import, no-PK hashes, target tracking, rerun refusal, versions, redaction'
