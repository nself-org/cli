#!/usr/bin/env bash
# Purpose: Linux acceptance for `nself db image switch --to pgvector` (EPIC P7-ADOPT D6, D18):
# a started alpine project with a plugin row, functions, a trigger, a sequence at 1000, a grant, a
# default privilege, a policy and a second database moves to the pgvector image with every table
# hash, schema dump, sequence, role, grant, default privilege, owner and policy unchanged.
# Legs: --plan changes nothing; a restore that fails (E511) and a row committed during the restore
# (E510) switch nothing; SIGKILL after env is written -> `start` is refused (E512) -> --resume;
# a second run changes nothing; --rollback; SIGKILL during the restore -> --resume converges.
# Inputs: W shared host/container path, nself/docker/curl/jq on PATH, run from the repo root.
# POSTGRES_VERSION is the Postgres major of the starting alpine image (default 16; the switch
# supports 12 to 17); the switch must land on the pgvector image of the same major (D-0806).
# Outputs: non-vacuous PASS assertions. Constraints: fake credentials only; cleanup owns only the
# e2e-adopt-pgs* containers, volumes and project; shared Docker stays running.
set -euo pipefail
export NSELF_V15=1
PGV="${POSTGRES_VERSION:-16}"
case "$PGV" in [0-9][0-9]) ;; *) echo "FAIL: POSTGRES_VERSION must be a two-digit major, got '$PGV'"; exit 2 ;; esac
unset POSTGRES_VERSION   # the project's env files own it from here; init must not inherit it
command -v openssl >/dev/null || apk add -q openssl >/dev/null
: "${W:?W must be a host-visible work directory}"
REPO="$PWD"
P=e2e-adopt-pgs
DIR="$W/$P"
test ! -e "$DIR" || { echo 'FAIL: fixture path occupied'; exit 1; }
test -z "$(docker ps -aq --filter "name=^${P}_")" || { echo 'FAIL: fixture project already exists'; exit 1; }
mkdir -p "$DIR" "$W/home"
export HOME="$W/home"
export NSELF_PLUGIN_DIR="$W/plugins"
mkdir -p "$NSELF_PLUGIN_DIR/a10-fixture"
cp "$REPO/test/fixtures/import/postgres/plugin/plugin.json" "$NSELF_PLUGIN_DIR/a10-fixture/plugin.json"
LOG="$W/pgvector-switch-e2e.log"
touch "$LOG"; chmod 600 "$LOG"
sha() { if command -v sha256sum >/dev/null 2>&1; then sha256sum | cut -d' ' -f1; else shasum -a 256 | cut -d' ' -f1; fi; }
cleanup() {
  (cd "$DIR" 2>/dev/null && docker compose --env-file .nself/compose.env -f docker-compose.yml down -v --remove-orphans >/dev/null 2>&1) || true
  docker ps -aq --filter "name=^nself-imgswitch-${P}" | xargs -r docker rm -f >/dev/null 2>&1 || true
  docker volume ls -q --filter "name=^${P}_" | xargs -r docker volume rm >/dev/null 2>&1 || true
  docker volume ls -q --filter "name=^nself-imgswitch-${P}" | xargs -r docker volume rm >/dev/null 2>&1 || true
}
trap cleanup EXIT
fail() {
  echo "FAIL: $1"; tail -40 "$LOG"
  docker ps -a --filter "name=${P}" --format '{{.Names}} {{.Image}} {{.Status}}' || true
  exit 1
}
pass() { echo "PASS: $1"; }
# A command that fails under set -e would end the run silently; name it.
set -E
trap 'echo "FAIL: command failed (exit $?) at line $LINENO: $BASH_COMMAND" >&2' ERR
# Only obviously fake credentials, padded to meet the config validators.
F_PG=FAKE-SWITCH-POSTGRES-PASSWORD-00000000000
F_HASURA=FAKE-SWITCH-HASURA-ADMIN-SECRET-0000000000000
F_JWT=FAKE-SWITCH-JWT-KEY-0000000000000000000000000
B=24000
cd "$DIR"
nself init --non-interactive --name "$P" --no-pgvector >>"$LOG" 2>&1 || fail init
cat >>.env.local <<ENV
ENV=dev
POSTGRES_VERSION=${PGV}-alpine
POSTGRES_PASSWORD=$F_PG
HASURA_GRAPHQL_ADMIN_SECRET=$F_HASURA
HASURA_JWT_KEY=$F_JWT
AUTH_SMTP_PASS=FAKE-SWITCH-SMTP-PASS-0000000000
AUTH_EMAIL_SIGNIN_EMAIL_VERIFIED_REQUIRED=false
AUTH_SMTP_HOST=mailpit
MINIO_ENABLED=false
OBJECT_STORAGE_ENABLED=false
POSTGRES_PORT=$((B + 432))
HASURA_PORT=$((B + 480))
AUTH_PORT=$((B + 400))
NGINX_HTTP_PORT=$((B + 481))
NGINX_HTTPS_PORT=$((B + 443))
REDIS_PORT=$((B + 479))
MAILPIT_SMTP_PORT=$((B + 425))
MAILPIT_UI_PORT=$((B + 426))
NSELF_ADMIN_ENABLED=false
ENV
chmod 600 .env.local
nself build --force >>"$LOG" 2>&1 || fail build
nself start --timeout 300 --skip-build >>"$LOG" 2>&1 || fail start
PGC="${P}_postgres"
PGDB="$(cat .env .env.local 2>/dev/null | sed -n 's/^POSTGRES_DB=//p' | tail -1)"
PGDB="${PGDB:-${P//-/_}}"
pg() { docker exec "$PGC" psql -X -U postgres -d "${DB:-$PGDB}" -v ON_ERROR_STOP=1 -At -c "$1"; }
pgs() { DB=postgres pg "$1"; }
test "$(pg "SELECT count(*) FROM public.np_plugins WHERE name='a10-fixture'")" = 1 || fail 'plugin row missing'
image_of() { docker inspect --format '{{.Config.Image}}' "$PGC"; }
case "$(image_of)" in *":${PGV}-alpine"*) ;; *) fail "expected postgres:${PGV}-alpine, got $(image_of)" ;; esac
files_sha() { cat docker-compose.yml nself.lock.yaml .env | sha; }

# ---- seed -------------------------------------------------------------------------------------
pg "CREATE ROLE appuser LOGIN PASSWORD 'FAKE-APPUSER-PASSWORD-0000'; CREATE SCHEMA app;
CREATE TABLE app.notes(id serial PRIMARY KEY, body text); INSERT INTO app.notes(body) SELECT md5(i::text) FROM generate_series(1,500) i;
SELECT setval('app.notes_id_seq', 1000);
CREATE FUNCTION app.add1(i int) RETURNS int LANGUAGE sql IMMUTABLE AS 'select i+1';
CREATE FUNCTION app.stamp() RETURNS trigger LANGUAGE plpgsql AS 'begin new.body = new.body || chr(33); return new; end';
CREATE TRIGGER notes_stamp BEFORE INSERT ON app.notes FOR EACH ROW EXECUTE FUNCTION app.stamp();
GRANT USAGE ON SCHEMA app TO appuser; GRANT SELECT ON app.notes TO appuser;
ALTER DEFAULT PRIVILEGES IN SCHEMA app GRANT SELECT ON TABLES TO appuser;
ALTER TABLE app.notes ENABLE ROW LEVEL SECURITY; CREATE POLICY p1 ON app.notes FOR SELECT TO appuser USING (id > 0);" >>"$LOG" 2>&1 || fail 'seed'
pg "CREATE ROLE zed LOGIN; CREATE ROLE amy LOGIN; GRANT CONNECT ON DATABASE \"$PGDB\" TO zed, amy; CREATE MATERIALIZED VIEW app.notes_count AS SELECT count(*) AS n FROM app.notes" >>"$LOG" 2>&1 || fail 'seed roles and view'
pgs "CREATE DATABASE second OWNER appuser" >>"$LOG" 2>&1 || fail 'second database'
DB=second pg "CREATE TABLE things(id int PRIMARY KEY, v text); INSERT INTO things SELECT i, 'v' || i FROM generate_series(1,50) i; GRANT SELECT ON things TO appuser" >>"$LOG" 2>&1 || fail 'seed second'

# schema_hash DB: hash of the normalised schema-only dump. An empty database (the maintenance database)
# normalises to nothing, so a grep that selects no line is not a failure; a failed dump is retried once
# the server has settled and its error goes to the log.
schema_hash() {
  local i h
  for i in 1 2 3 4 5; do
    if h="$(docker exec "$PGC" pg_dump -U postgres -s -d "$1" 2>>"$LOG" | { grep -v '^--\|^SET \|^\\restrict\|^\\unrestrict\|^$\|set_config' || true; } | sha)"; then echo "$h"; return 0; fi
    sleep 3
  done
  return 1
}
# snap: every fact the switch must keep, from the database itself (not from the report), as one hash.
snap() {
  local c db t out=""
  c="$(docker ps --filter "name=^${PGC}\$" --format '{{.Names}}')"
  test -n "$c" || fail 'postgres is not running'
  for db in $(pgs "SELECT datname FROM pg_database WHERE NOT datistemplate ORDER BY 1"); do
    out+="== $db"$'\n'
    for t in $(DB=$db pg "SELECT quote_ident(n.nspname) || '.' || quote_ident(c.relname) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE c.relkind IN ('r','p') AND n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname !~ '^pg_toast' ORDER BY 1"); do
      case "$t" in
        # Hasura rewrites these two on every boot (instance id, notification): only their row count is stable.
        hdb_catalog.hdb_metadata|hdb_catalog.hdb_schema_notifications) out+="$t $(DB=$db pg "SELECT count(*) FROM $t")"$'\n' ;;
        *) out+="$t $(DB=$db pg "SELECT count(*), coalesce(sum(hashtextextended(t::text, 0)::numeric), 0) FROM $t AS t")"$'\n' ;;
      esac
    done
    out+="$(schema_hash "$db")"$'\n'
    out+="$(DB=$db pg "SELECT schemaname || '.' || sequencename || '=' || last_value FROM pg_sequences ORDER BY 1")"$'\n'
    out+="$(DB=$db pg "SELECT grantee, table_schema, table_name, privilege_type FROM information_schema.role_table_grants WHERE table_schema NOT IN ('pg_catalog','information_schema') ORDER BY 1,2,3,4")"$'\n'
    out+="$(DB=$db pg "SELECT pg_get_userbyid(defaclrole), defaclnamespace::regnamespace::text, defaclobjtype, defaclacl FROM pg_default_acl ORDER BY 1,2,3")"$'\n'
    out+="$(DB=$db pg "SELECT schemaname, tablename, policyname, roles, cmd, qual FROM pg_policies ORDER BY 1,2,3")"$'\n'
    out+="$(DB=$db pg "SELECT n.nspname, c.relname, pg_get_userbyid(c.relowner) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname !~ '^pg_toast' ORDER BY 1,2")"$'\n'
    out+="$(DB=$db pg "SELECT extname, extversion FROM pg_extension ORDER BY 1")"$'\n'
  done
  out+="$(pgs "SELECT rolname, rolsuper, rolinherit, rolcreaterole, rolcreatedb, rolcanlogin, rolreplication, rolbypassrls, rolconnlimit FROM pg_roles ORDER BY 1")"$'\n'
  out+="$(pgs "SELECT d.datname, pg_get_userbyid(d.datdba), d.datcollate, (SELECT coalesce(string_agg(CASE WHEN x.grantee = 0 THEN 'PUBLIC' ELSE pg_get_userbyid(x.grantee) END || ':' || x.privilege_type || ':' || x.is_grantable, ',' ORDER BY CASE WHEN x.grantee = 0 THEN '' ELSE pg_get_userbyid(x.grantee) END, x.privilege_type), '') FROM aclexplode(d.datacl) x) FROM pg_database d ORDER BY 1")"$'\n'
  printf '%s' "$out"
}
writable() { pg "CREATE TABLE IF NOT EXISTS public.rw_probe(x int); INSERT INTO public.rw_probe VALUES (1); DROP TABLE public.rw_probe" >/dev/null 2>&1; }
running_all() { test "$(docker ps --filter "name=^${P}_" --filter status=running --format '{{.Names}}' | wc -l | tr -d ' ')" -ge 4; }
new_volumes() { docker volume ls -q --filter "name=^${P}_postgres_data_pgvector_"; }
wait_for() { # wait_for SECONDS COMMAND...: poll every 0.2 s
  local limit=$(($1 * 5)) i=0; shift
  until "$@"; do i=$((i + 1)); [ "$i" -lt "$limit" ] || return 1; sleep 0.2; done
}
restore_running() { test -n "$(docker ps -q --filter "name=^nself-imgswitch-${P}-restore\$")"; }
phase_is() { grep -q "\"phase\": \"$1\"" .nself/image-switch/state.json 2>/dev/null; }
err_code() { jq -r '.error.code // empty' "$1" 2>/dev/null; }

# ---- plan: changes nothing --------------------------------------------------------------------
BASE_FILES="$(files_sha)"
nself db image switch --to pgvector --plan --json >"$W/plan.json" 2>>"$LOG" || fail 'plan'
jq -e '.data.status=="planned" and (.data.new_image|test("pgvector.*pg'"$PGV"'")) and (.data.new_volume|test("_postgres_data_pgvector_")) and .data.build!=null and ([.data.preflight[].ok]|all) and (.data.databases>=3)' "$W/plan.json" >/dev/null || fail 'plan report'
test "$(files_sha)" = "$BASE_FILES" || fail 'plan changed env, compose or lock'
test -z "$(new_volumes)" && test ! -e .nself/image-switch/state.json || fail 'plan created a volume or a state'
running_all || fail 'plan stopped the stack'
pass '--plan lists the checks, the new volume and the compose and lock change, and changes nothing'

# ---- E511: a restore that fails switches nothing ----------------------------------------------
DB=second pg "CREATE COLLATION a22_de (provider = libc, locale = 'de_DE.utf8')" >>"$LOG" 2>&1 || fail 'plant collation'
if nself db image switch --to pgvector --yes --json >"$W/e511.json" 2>>"$LOG"; then fail 'the failing restore was accepted'; fi
test "$(err_code "$W/e511.json")" = E511 || fail "expected E511, got $(err_code "$W/e511.json")"
test "$(files_sha)" = "$BASE_FILES" || fail 'E511 changed env, compose or lock'
test -z "$(new_volumes)" || fail 'E511 left the new volume'
test ! -e .nself/image-switch/state.json || fail 'E511 left a state'
case "$(image_of)" in *alpine*) ;; *) fail 'E511 changed the image' ;; esac
running_all || fail 'E511 did not restart the stack'
writable || fail 'E511 left the old cluster read-only'
pass 'a failing restore (E511): env, compose and lock unchanged, new volume removed, old stack serving and writable'
DB=second pg "DROP COLLATION a22_de" >>"$LOG" 2>&1 || fail 'drop collation'

# ---- E510: a row committed while the restore runs switches nothing ----------------------------
( if wait_for 150 restore_running; then
    # A session that turns the quiesce setting off for itself (the setting applies from a transaction's start).
    docker exec -e PGOPTIONS=-cdefault_transaction_read_only=off "$PGC" psql -X -U postgres -d "$PGDB" -c "INSERT INTO app.notes(body) VALUES ('committed during the restore')" >/dev/null 2>&1 || true
  fi ) &
LATE=$!
if nself db image switch --to pgvector --yes --json >"$W/e510.json" 2>>"$LOG"; then fail 'the stale dump was accepted'; fi
wait "$LATE" || true
test "$(err_code "$W/e510.json")" = E510 || fail "expected E510, got $(err_code "$W/e510.json")"
test "$(files_sha)" = "$BASE_FILES" || fail 'E510 changed env, compose or lock'
test -z "$(new_volumes)" && test ! -e .nself/image-switch/state.json || fail 'E510 left the new volume or a state'
test "$(pg "SELECT count(*) FROM app.notes WHERE body LIKE 'committed during%'")" = 1 || fail 'the late row is not on the old volume'
running_all && writable || fail 'E510 left the old stack down or read-only'
pass 'a row committed during the restore (E510): nothing switched, the row is on the old volume, read-only reset, stack serving'
pg "DELETE FROM app.notes WHERE body LIKE 'committed during%'; SELECT setval('app.notes_id_seq', 1000)" >>"$LOG" 2>&1 || fail 'remove the late row'

# ---- autovacuum is active on the old cluster while the switch reads it (R4) -------------------
# The dump and the captures read every table, which moves pg_stat counters; the recheck must not
# compare them. Dead tuples plus a 1 s autovacuum naptime keep autovacuum working through the switch.
pgs "ALTER SYSTEM SET autovacuum_naptime = '1s'" >>"$LOG" 2>&1 && pgs "SELECT pg_reload_conf()" >>"$LOG" 2>&1 || fail 'autovacuum settings'
pg "ALTER TABLE app.notes SET (autovacuum_vacuum_threshold = 5, autovacuum_vacuum_scale_factor = 0); UPDATE app.notes SET body = body WHERE id <= 200" >>"$LOG" 2>&1 || fail 'dead tuples'
# ---- the switch, killed after env is written ---------------------------------------------------
snap >"$W/snap.before"
OLDVOL="$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/var/lib/postgresql/data"}}{{.Name}}{{end}}{{end}}' "$PGC")"
cp docker-compose.yml "$W/compose.alpine"
nself db image switch --to pgvector --yes >"$W/switch1.out" 2>&1 &
SW=$!
wait_for 240 phase_is switched || { kill "$SW" 2>/dev/null || true; fail 'the switch never reached the switched phase'; }
kill -9 "$SW"; wait "$SW" 2>/dev/null || true
phase_is switched || fail 'state is not switched after the kill'
if nself start --skip-build >"$W/start-blocked.out" 2>&1; then fail 'start ran while a switch was incomplete'; fi
grep -q E512 "$W/start-blocked.out" && grep -q -- '--resume' "$W/start-blocked.out" && grep -q -- '--rollback' "$W/start-blocked.out" || fail 'start was not refused with E512 naming --resume and --rollback'
pass 'SIGKILL after env was written: start is refused with E512, which names both ways out'
nself db image switch --to pgvector --yes >"$W/switch-again.out" 2>&1 && fail 'a second run was accepted while incomplete'
grep -q E512 "$W/switch-again.out" || fail 'a second run did not say E512'
nself db image switch --resume --json >"$W/resume.json" 2>>"$LOG" || fail 'resume'
jq -e '.data.status=="switched"' "$W/resume.json" >/dev/null || fail 'resume report'

# ---- after the switch -------------------------------------------------------------------------
wait_for 120 running_all || fail 'the stack is not running after the resume'
case "$(image_of)" in *pgvector*"pg${PGV}"*) ;; *) fail "postgres is not the pg${PGV} pgvector image: $(image_of)" ;; esac
NEWVOL="$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/var/lib/postgresql/data"}}{{.Name}}{{end}}{{end}}' "$PGC")"
test "$NEWVOL" != "$OLDVOL" && test "$NEWVOL" = "$(sed -n 's/^POSTGRES_DATA_VOLUME=//p' .env | tail -1 | tr -d '"')" || fail 'postgres is not on the new volume named in .env'
grep -q "name: $NEWVOL" docker-compose.yml || fail 'compose does not name the new volume'
wait_for 120 writable || fail 'the new cluster is not writable'
snap >"$W/snap.after"
cmp "$W/snap.before" "$W/snap.after" || { diff "$W/snap.before" "$W/snap.after" | head -20 || true; fail 'the data differs after the switch'; }
pass 'tables, hashes, schema dumps, sequences, roles, grants, default privileges, owners and policies equal the pre-switch capture'
pg "CREATE EXTENSION IF NOT EXISTS vector" >>"$LOG" 2>&1 || fail 'CREATE EXTENSION vector'
test "$(pg "SELECT count(*) FROM public.np_plugins WHERE name='a10-fixture'")" = 1 || fail 'plugin row lost'
test "$(pg "SELECT id FROM app.notes ORDER BY id DESC LIMIT 1")" = 500 || fail 'last row'
test "$(pg "WITH i AS (INSERT INTO app.notes(body) VALUES ('x') RETURNING id) SELECT id FROM i")" = 1001 || fail 'the sequence did not continue at 1001'
pg "DELETE FROM app.notes WHERE id = 1001; SELECT setval('app.notes_id_seq', 1000)" >>"$LOG" 2>&1 || fail 'undo the probe row'
pass 'vector works, the plugin row is there, the sequence continues at 1001'
# The old volume still has its data: copy it from a read-only mount and count in a throwaway server.
OLDIMG="$(grep -m1 '^        image:' "$W/compose.alpine" | sed 's/.*image: *//')"
SCRATCH="nself-imgswitch-${P}-oldcheck"
docker volume create "$SCRATCH" >/dev/null
docker run --rm -v "$OLDVOL":/src:ro -v "$SCRATCH":/dst "$OLDIMG" sh -c 'cp -a /src/. /dst/ && chown -R 70:70 /dst' >>"$LOG" 2>&1 || fail 'copy the old volume'
OC="nself-imgswitch-${P}-oldcheck"
docker run -d --name "$OC" --user 70:70 -v "$SCRATCH":/var/lib/postgresql/data "$OLDIMG" >/dev/null || fail 'start the copy'
wait_for 60 docker exec "$OC" pg_isready -U postgres >/dev/null 2>&1 || fail 'the old data did not start'
test "$(docker exec "$OC" psql -X -U postgres -d "$PGDB" -At -c 'SELECT count(*) FROM app.notes')" = 500 || fail 'the old volume lost rows'
docker rm -f "$OC" >/dev/null; docker volume rm "$SCRATCH" >/dev/null
pass 'the old volume still holds the data (counted on a copy of a read-only mount)'
AFTER_SWITCH_FILES="$(files_sha)"
nself db image switch --to pgvector --yes >"$W/again.out" 2>>"$LOG" || fail 'second run'
grep -q 'already on pgvector' "$W/again.out" || fail 'second run did not say it is already pgvector'
test "$(files_sha)" = "$AFTER_SWITCH_FILES" || fail 'the second run changed the project'
pass 'a second run prints that postgres is already pgvector and changes nothing'

# ---- rollback after success -------------------------------------------------------------------
if nself db image switch --rollback </dev/null >"$W/rollback-noyes.out" 2>&1; then fail 'rollback after success without --yes was accepted'; fi
grep -q E403 "$W/rollback-noyes.out" || fail 'rollback without --yes did not ask for confirmation (E403)'
nself db image switch --rollback --yes --json >"$W/rollback.json" 2>>"$LOG" || fail 'rollback'
jq -e --arg v "$NEWVOL" '.data.status=="rolled-back" and (.data.next_steps|join(" ")|contains($v))' "$W/rollback.json" >/dev/null || fail 'rollback report'
wait_for 120 running_all || fail 'the stack is not running after the rollback'
case "$(image_of)" in *alpine*) ;; *) fail "rollback did not restore the alpine image: $(image_of)" ;; esac
test "$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/var/lib/postgresql/data"}}{{.Name}}{{end}}{{end}}' "$PGC")" = "$OLDVOL" || fail 'rollback is not on the old volume'
diff -q "$W/compose.alpine" docker-compose.yml >/dev/null || fail 'compose is not what it was before the switch'
docker volume ls -q | grep -qx "$NEWVOL" || fail 'rollback removed the new volume'
wait_for 60 writable || fail 'the old cluster is still read-only after the rollback'
snap >"$W/snap.rolledback"
cmp "$W/snap.before" "$W/snap.rolledback" || { diff "$W/snap.before" "$W/snap.rolledback" | head -20 || true; fail 'the data differs after the rollback'; }
pass 'rollback restores env, compose and lock, starts the old volume (writable), keeps and names the new volume'

# ---- SIGKILL during the restore, then --resume ------------------------------------------------
nself db image switch --to pgvector --yes >"$W/switch2.out" 2>&1 &
SW=$!
wait_for 240 restore_running || { kill "$SW" 2>/dev/null || true; fail 'the restore one-shot never appeared'; }
kill -9 "$SW"; wait "$SW" 2>/dev/null || true
if nself start --skip-build >"$W/start-blocked2.out" 2>&1; then fail 'start ran during an incomplete restore'; fi
grep -q E512 "$W/start-blocked2.out" || fail 'start was not refused with E512 after a kill during the restore'
nself db image switch --resume --json >"$W/resume2.json" 2>>"$LOG" || fail 'resume after a kill during the restore'
jq -e '.data.status=="switched"' "$W/resume2.json" >/dev/null || fail 'resume report 2'
wait_for 120 running_all || fail 'the stack is not running after the second resume'
case "$(image_of)" in *pgvector*) ;; *) fail 'the second switch is not on pgvector' ;; esac
wait_for 120 writable || fail 'the new cluster is not writable after the second resume'
snap >"$W/snap.after2"
cmp "$W/snap.before" "$W/snap.after2" || { diff "$W/snap.before" "$W/snap.after2" | head -20 || true; fail 'the data differs after a kill during the restore and a resume'; }
test "$(docker ps -aq --filter "name=^nself-imgswitch-${P}" | wc -l | tr -d ' ')" = 0 || fail 'a one-shot container was left behind'
pass 'SIGKILL during the restore: --resume converges to the same data, no one-shot container is left'
echo 'PASS: pgvector-switch e2e'
