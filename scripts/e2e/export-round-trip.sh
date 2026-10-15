#!/usr/bin/env bash
# Purpose: Linux acceptance for the round trip: seeded project A -> `nself db export` -> fresh
# project B -> `nself db import nself` with zero mismatches (EPIC D8, D18).
# Inputs: W shared host/container path, nself/docker/curl/jq/psql on PATH, run from the repo root.
# Outputs: non-vacuous PASS assertions. Constraints: fake credentials only; cleanup owns only the
# e2e-adopt-rt-* resources; shared Docker stays running.
set -euo pipefail
export NSELF_V15=1
# Build generates self-signed certs through openssl; the docker CLI image lacks it.
command -v openssl >/dev/null || apk add -q openssl >/dev/null
: "${W:?W must be a host-visible work directory}"
REPO="$PWD"
PA=e2e-adopt-rt-a
PB=e2e-adopt-rt-b
DIRA="$W/$PA"
DIRB="$W/$PB"
for p in "$PA" "$PB"; do
  test ! -e "$W/$p" || { echo 'FAIL: fixture path occupied'; exit 1; }
  test -z "$(docker ps -aq --filter "name=^${p}_")" || { echo 'FAIL: fixture project already exists'; exit 1; }
done
mkdir -p "$DIRA" "$DIRB" "$W/home"
export HOME="$W/home"
# The same offline plugin is installed in both projects (nself build seeds np_plugins from it).
export NSELF_PLUGIN_DIR="$W/plugins"
mkdir -p "$NSELF_PLUGIN_DIR/a10-fixture"
cp "$REPO/test/fixtures/import/postgres/plugin/plugin.json" "$NSELF_PLUGIN_DIR/a10-fixture/plugin.json"
LOG="$W/round-trip-e2e.log"
touch "$LOG"; chmod 600 "$LOG"
owned_a=0
owned_b=0
down() { (cd "$1" && docker compose --env-file .nself/compose.env -f docker-compose.yml down -v --remove-orphans >/dev/null 2>&1) || true; }
cleanup() {
  if [ "$owned_a" = 1 ]; then down "$DIRA"; fi
  if [ "$owned_b" = 1 ]; then down "$DIRB"; fi
}
trap cleanup EXIT
fail() {
  echo "FAIL: $1"; tail -40 "$LOG"
  docker ps -a --filter "name=^e2e-adopt-rt-" --format '{{.Names}} {{.Status}}' || true
  exit 1
}
pass() { echo "PASS: $1"; }
# Only obviously fake credentials, padded to meet the config validators (Postgres >=16, Hasura
# admin secret >=32, JWT key >=32). Signup and signin responses (JWTs) are never printed.
F_PG=FAKE-ROUNDTRIP-POSTGRES-PASSWORD-0000000000
F_HASURA=FAKE-ROUNDTRIP-HASURA-ADMIN-SECRET-0000000000000
F_JWT=FAKE-ROUNDTRIP-JWT-KEY-0000000000000000000000000
F_SMTP=FAKE-ROUNDTRIP-SMTP-PASS-000000000000
F_S3A=FAKE-ROUNDTRIP-S3-ACCESS-KEY-0000
F_S3S=FAKE-ROUNDTRIP-S3-SECRET-KEY-00000000000000000
S3_AUTH="$F_S3A:$F_S3S"
# init_project NAME DIR PORT_BASE: a started project with the same bucket and credentials.
init_project() {
  local name="$1" dir="$2" b="$3"
  cd "$dir"
  nself init --non-interactive --name "$name" --no-pgvector >>"$LOG" 2>&1 || fail "init $name"
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
SEAWEEDFS_PORT=$((b + 333))
S3_ACCESS_KEY=$F_S3A
S3_SECRET_KEY=$F_S3S
POSTGRES_PORT=$((b + 432))
HASURA_PORT=$((b + 480))
AUTH_PORT=$((b + 400))
NGINX_HTTP_PORT=$((b + 481))
NGINX_HTTPS_PORT=$((b + 443))
REDIS_PORT=$((b + 479))
MAILPIT_SMTP_PORT=$((b + 425))
MAILPIT_UI_PORT=$((b + 426))
NSELF_ADMIN_ENABLED=false
ENV
  chmod 600 .env.local
  nself build --force >>"$LOG" 2>&1 || fail "build $name"
}
start_project() {
  nself start --timeout 300 --skip-build >>"$LOG" 2>&1 || fail "start $1"
}
# pgdb NAME: init derives POSTGRES_DB from the project name (dashes become underscores).
pgdb() {
  local db
  db="$(cat .env .env.local 2>/dev/null | sed -n 's/^POSTGRES_DB=//p' | tail -1)"
  echo "${db:-${1//-/_}}"
}
A_BASE=21000
B_BASE=22000
# ---- Project A: seed, export -------------------------------------------------------------
init_project "$PA" "$DIRA" "$A_BASE"
owned_a=1
start_project "$PA"
cd "$DIRA"
PGDB_A="$(pgdb "$PA")"
pga() { docker exec "${PA}_postgres" psql -X -U postgres -d "$PGDB_A" -v ON_ERROR_STOP=1 -At -c "$1"; }
test "$(pga "SELECT count(*) FROM public.np_plugins WHERE name='a10-fixture'")" = 1 || fail 'plugin not installed in A'
pga "CREATE SCHEMA app; CREATE TABLE public.notes(id integer PRIMARY KEY, body text); INSERT INTO public.notes SELECT i, md5(i::text) FROM generate_series(1,1000) i; CREATE TABLE app.extra(id integer PRIMARY KEY, note_id integer REFERENCES public.notes(id), body text); INSERT INTO app.extra VALUES(1, 1, 'fixture'); CREATE TABLE public.np_export_private(id int); CREATE SCHEMA np_export_private; CREATE TABLE np_export_private.private(id int); CREATE SCHEMA app2; CREATE TABLE app2.t(id integer PRIMARY KEY); INSERT INTO app2.t VALUES(1);" >>"$LOG" 2>&1 || fail 'seed A'
for i in 1 2 3; do
  curl -fsS "http://127.0.0.1:$((A_BASE + 400))/signup/email-password" -H 'Content-Type: application/json' \
    -d "{\"email\":\"fake-roundtrip-$i@example.invalid\",\"password\":\"FAKE-USER-PASSWORD-$i\"}" >"$W/signup-$i.json" || fail 'signup A'
  chmod 600 "$W/signup-$i.json"
done
test "$(pga 'SELECT count(*) FROM auth.users')" = 3 || fail 'signup count A'
meta_a() { curl -fsS "http://127.0.0.1:$((A_BASE + 480))/v1/metadata" -H "X-Hasura-Admin-Secret: $F_HASURA" -H 'Content-Type: application/json' -d "$1"; }
for table in 'public notes' 'app extra'; do
  read -r schema name <<<"$table"
  meta_a "{\"type\":\"pg_track_table\",\"args\":{\"source\":\"default\",\"table\":{\"schema\":\"$schema\",\"name\":\"$name\"}}}" >>"$LOG" || fail track
  meta_a "{\"type\":\"pg_create_select_permission\",\"args\":{\"source\":\"default\",\"table\":{\"schema\":\"$schema\",\"name\":\"$name\"},\"role\":\"user\",\"permission\":{\"columns\":\"*\",\"filter\":{}}}}" >>"$LOG" || fail permission
done
meta_a '{"type":"pg_create_object_relationship","args":{"source":"default","table":{"schema":"app","name":"extra"},"name":"note","using":{"foreign_key_constraint_on":"note_id"}}}' >>"$LOG" || fail relationship
meta_a '{"type":"pg_create_array_relationship","args":{"source":"default","table":{"schema":"public","name":"notes"},"name":"extras","using":{"foreign_key_constraint_on":{"table":{"schema":"app","name":"extra"},"column":"note_id"}}}}' >>"$LOG" || fail relationship
for key in one.txt TWO.txt two.txt nested/four.txt; do
  printf 'fake object %s\n' "$key" >"$W/object"
  curl -fsS --aws-sigv4 'aws:amz:us-east-1:s3' --user "$S3_AUTH" -X PUT --data-binary "@$W/object" "http://127.0.0.1:$((A_BASE + 333))/export-fixture/$key" >>"$LOG" || fail object
done
FINGER_A="$(pga 'SELECT count(*), sum(hashtextextended(t::text,0)::numeric) FROM public.notes t')"
nself db export --out "$W/bundle" --schemas public,app --json >"$W/summary.json" 2>>"$LOG" || fail export
jq -e '.data.tables==2 and .data.users==3 and .data.objects==4' "$W/summary.json" >/dev/null || fail summary
pass 'seeded project A exported (2 tables, 3 users, 4 objects)'
# A second bundle holds only app2.t (a table B will not have) and the four objects, and no users
# (A is discarded, so its users go first): only the object store check can refuse it.
pga 'DELETE FROM auth.users' >>"$LOG" 2>&1 || fail 'delete users'
nself db export --out "$W/bundle2" --schemas app2 --json >"$W/summary2.json" 2>>"$LOG" || fail 'export app2'
jq -e '.data.tables==1 and .data.objects==4 and .data.users==0' "$W/summary2.json" >/dev/null || fail 'summary app2'
# A is no longer needed; stopping it keeps the host small while B starts.
nself stop >>"$LOG" 2>&1 || fail 'stop A'
# ---- Project B: fresh, same plugin, import -----------------------------------------------
init_project "$PB" "$DIRB" "$B_BASE"
owned_b=1
start_project "$PB"
cd "$DIRB"
PGDB_B="$(pgdb "$PB")"
pgb() { docker exec "${PB}_postgres" psql -X -U postgres -d "$PGDB_B" -v ON_ERROR_STOP=1 -At -c "$1"; }
test "$(pgb "SELECT count(*) FROM public.np_plugins WHERE name='a10-fixture'")" = 1 || fail 'plugin not installed in B'
test "$(pgb "SELECT to_regclass('public.notes') IS NULL")" = t || fail 'B is not fresh'
nself db import nself "$W/bundle" --plan --json >"$W/plan.json" 2>>"$LOG" || fail plan
jq -e '.data.result=="pass" and .data.detection.kind=="nself" and (.data.steps|length)>0' "$W/plan.json" >/dev/null || fail 'plan report'
test ! -d .nself/import || fail 'plan wrote a stage'
test "$(pgb "SELECT to_regclass('public.notes') IS NULL")" = t || fail 'plan changed B'
nself db import nself "$W/bundle" --json >"$W/import.json" 2>>"$LOG" || fail import
jq -e '.data.source=="nself" and .data.result=="pass" and (.data.mismatches|length)==0 and .data.wrote==true' "$W/import.json" >/dev/null || fail 'import result'
jq -e '(.data.tables|length)==2 and ([.data.tables[].equal]|all) and ([.data.tables[].source_rows]|add)==1001' "$W/import.json" >/dev/null || fail 'table hashes'
jq -e '.data.users.source==3 and .data.users.target==3 and .data.users.equal==true and (.data.users.reset_required|length)==0' "$W/import.json" >/dev/null || fail users
jq -e '.data.storage.source_objects==4 and .data.storage.target_objects==4 and .data.storage.equal==true' "$W/import.json" >/dev/null || fail storage
jq -e '.data.hasura.equal==true and (.data.hasura.diffs|length)==0' "$W/import.json" >/dev/null || fail hasura
pass 'import reports zero mismatches: tables with hashes, users, objects, Hasura metadata'
# Independent checks, not taken from the report.
test "$(pgb 'SELECT count(*), sum(hashtextextended(t::text,0)::numeric) FROM public.notes t')" = "$FINGER_A" || fail 'notes fingerprint'
test "$(pgb 'SELECT count(*) FROM app.extra WHERE note_id=1')" = 1 || fail 'extra row'
test "$(pgb 'SELECT count(*) FROM auth.users')" = 3 || fail 'user count'
test "$(pgb "SELECT count(*) FROM public.np_plugins WHERE name='a10-fixture'")" = 1 || fail 'B plugin row lost'
test "$(pgb "SELECT to_regclass('public.np_export_private') IS NULL")" = t || fail 'plugin data crossed over'
meta_b() { curl -fsS "http://127.0.0.1:$((B_BASE + 480))/v1/metadata" -H "X-Hasura-Admin-Secret: $F_HASURA" -H 'Content-Type: application/json' -d "$1"; }
meta_b '{"type":"export_metadata","version":2,"args":{}}' >"$W/meta-b.json" || fail 'export metadata B'
jq -e '[.metadata.sources[0].tables[] | select(.table.name=="notes" or .table.name=="extra")] | length==2 and all(.select_permissions|length==1)' "$W/meta-b.json" >/dev/null || fail 'tracked tables or permissions'
jq -e '[.metadata.sources[0].tables[] | select(.table.name=="extra") | .object_relationships[]?.name]==["note"]' "$W/meta-b.json" >/dev/null || fail 'object relationship'
jq -e '[.metadata.sources[0].tables[] | select(.table.name=="notes") | .array_relationships[]?.name]==["extras"]' "$W/meta-b.json" >/dev/null || fail 'array relationship'
for key in one.txt TWO.txt two.txt nested/four.txt; do
  printf 'fake object %s\n' "$key" | sha256sum | cut -d' ' -f1 >"$W/want"
  curl -fsS --aws-sigv4 'aws:amz:us-east-1:s3' --user "$S3_AUTH" "http://127.0.0.1:$((B_BASE + 333))/export-fixture/$key" | sha256sum | cut -d' ' -f1 >"$W/got"
  cmp "$W/want" "$W/got" || fail "object $key differs"
done
pass 'rows, relationships, permissions and objects (by sha256) read back from B'
for i in 1 2 3; do
  curl -fsS "http://127.0.0.1:$((B_BASE + 400))/signin/email-password" -H 'Content-Type: application/json' \
    -d "{\"email\":\"fake-roundtrip-$i@example.invalid\",\"password\":\"FAKE-USER-PASSWORD-$i\"}" >"$W/signin-$i.json" || fail "signin $i"
  chmod 600 "$W/signin-$i.json"
  jq -e '.session.accessToken|type=="string" and length>0' "$W/signin-$i.json" >/dev/null || fail "no session for user $i"
done
pass 'users sign in to B with their original passwords'
# Importing the same bundle again is refused (E519) and changes nothing.
objs_b() {
  for key in one.txt TWO.txt two.txt nested/four.txt; do
    curl -fsS --aws-sigv4 'aws:amz:us-east-1:s3' --user "$S3_AUTH" "http://127.0.0.1:$((B_BASE + 333))/export-fixture/$key" | sha256sum
  done | sha256sum
}
meta_hash() { meta_b '{"type":"export_metadata","version":2,"args":{}}' | jq -S '.metadata' | sha256sum; }
# state_b: rows, users, objects' bytes and Hasura metadata in B as one comparable line. Each part is
# read on its own so a failing read stops the run instead of making two lines equal.
state_b() {
  local rows objs meta
  rows="$(pgb 'SELECT (SELECT count(*) FROM public.notes), (SELECT sum(hashtextextended(t::text,0)::numeric) FROM public.notes t), (SELECT count(*) FROM auth.users), (SELECT count(*) FROM app.extra), (SELECT count(*) FROM information_schema.schemata WHERE schema_name='"'"'app2'"'"')')" || fail 'read rows'
  objs="$(objs_b)" || fail 'read objects'
  meta="$(meta_hash)" || fail 'read metadata'
  test -n "$rows" && test -n "$objs" && test -n "$meta" || fail 'empty state'
  echo "$rows|$objs|$meta"
}
before="$(state_b)"
if nself db import nself "$W/bundle" --json >"$W/rerun.json" 2>"$W/rerun.err"; then fail 'rerun accepted'; fi
jq -e '.error.code=="E519"' "$W/rerun.json" >/dev/null || fail 'rerun without E519'
test "$(state_b)" = "$before" || fail 'rerun changed B'
pass 'second import refused with E519, B unchanged'
# A bundle whose tables B does not have still cannot overwrite B's objects (D-0723): the object
# store check refuses it, in --plan as well as in the real run, before anything is written.
for mode in --plan ''; do
  if nself db import nself "$W/bundle2" $mode --json >"$W/objects.json" 2>"$W/objects.err"; then fail "object collision accepted ($mode)"; fi
  jq -e '.error.code=="E519" and (.error.message|test("object store"))' "$W/objects.json" >/dev/null || fail "object collision without E519 ($mode)"
done
test "$(state_b)" = "$before" || fail 'object collision changed B'
pass 'bundle that would overwrite existing objects refused (plan and run), B unchanged'
# A damaged or foreign bundle is refused before anything is read into B, on a used target too.
stages_before="$(find "$DIRB/.nself/import" -mindepth 2 -maxdepth 2 -name stage -print | wc -l)"
cp -r "$W/bundle" "$W/flipped"
chmod -R u+w "$W/flipped"
dump="$W/flipped/db/data.dump"
printf '\377' | dd of="$dump" bs=1 seek=0 count=1 conv=notrunc 2>/dev/null
if nself db import nself "$W/flipped" --json >"$W/flipped.json" 2>"$W/flipped.err"; then fail 'flipped byte accepted'; fi
grep -q E516 "$W/flipped.json" "$W/flipped.err" || fail 'flipped byte without E516'
cp -r "$W/bundle" "$W/v2"
chmod -R u+w "$W/v2"
sed -i 's/"schema_version": "[^"]*"/"schema_version": "2"/' "$W/v2/manifest.json"
if nself db import nself "$W/v2" --json >"$W/v2.json" 2>"$W/v2.err"; then fail 'schema_version 2 accepted'; fi
grep -q E515 "$W/v2.json" "$W/v2.err" || fail 'schema_version 2 without E515'
test "$(state_b)" = "$before" || fail 'refused bundles changed B'
test "$(find "$DIRB/.nself/import" -mindepth 2 -maxdepth 2 -name stage -print | wc -l)" = "$stages_before" || fail 'a refused bundle left a stage'
pass 'flipped byte refused with E516, schema_version 2 with E515, nothing staged'
python3 --version >/dev/null 2>&1 || apk add -q python3 >/dev/null
python3 - "$W" "$F_PG" "$F_HASURA" "$F_JWT" "$F_SMTP" "$F_S3S" <<'PY'
import pathlib, sys
root, secrets = pathlib.Path(sys.argv[1]), [s.encode() for s in sys.argv[2:]]
paths = [p for p in root.glob('*') if p.is_file() and p.suffix in ('.json', '.err', '.log')]
paths += list(root.glob('e2e-adopt-rt-b/.nself/import/*/report.json'))
assert paths
for p in paths:
    for s in secrets:
        assert s not in p.read_bytes(), 'credential appeared in output'
print('PASS: no credential in reports or logs; files read =', len(paths))
PY
echo 'ROUND TRIP E2E PASS'
