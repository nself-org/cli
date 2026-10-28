#!/usr/bin/env bash
# Purpose: Linux acceptance for the isolation guard (P7-ADOPT-05, EPIC D4): on the P7-ADOPT-04 multi-app
# project `nself doctor --deep --only isolation --json` reports zero failures; after the admin metadata API
# replaces the notes select filter with {}, the same command exits non-zero and names public.notes, user,
# select and ISO-02. Setup is the one in scripts/e2e/multi-app-isolation.sh (init, template overlay, build,
# start, migrate, reconcile --apply), copied because that script owns its own teardown trap.
# Inputs: W shared host/container path, nself/docker/curl/jq on PATH, run from the repo root.
# Outputs: non-vacuous PASS assertions. Exit 0 on success, 1 on any failure.
# Constraints: fake credentials only; bash 3.2 compatible. The admin secret travels only in a 0600 header
#   file under $W (curl -H @file), never on a command line. Read-only against the stack except the one
#   deliberate permission change, which lives in the throwaway e2e-isolation-guard project. Cleanup owns
#   only that project.
set -euo pipefail
export NSELF_V15=1
: "${W:?W must be a host-visible work directory}"
NAME=e2e-isolation-guard
DIR="$W/p"
test ! -e "$DIR" || { echo 'FAIL: fixture path occupied'; exit 1; }
test -z "$(docker ps -aq --filter "name=^${NAME}_")" || { echo 'FAIL: fixture project already exists'; exit 1; }
mkdir -p "$DIR" "$W/home"
export HOME="$W/home"
LOG="$W/isolation-guard-e2e.log"
touch "$LOG"; chmod 600 "$LOG"
owned=0
cleanup() {
  if [ "$owned" = 1 ]; then
    (cd "$DIR" && nself stop >/dev/null 2>&1) || true
    (cd "$DIR" && docker compose --env-file .nself/compose.env -f docker-compose.yml down -v --remove-orphans >/dev/null 2>&1) || true
  fi
}
trap cleanup EXIT
fail() {
  echo "FAIL: $1"; tail -40 "$LOG"
  exit 1
}
pass() { echo "PASS: $1"; }

F_PG=FAKE-ISOGUARD-POSTGRES-PASSWORD-00000000
F_HASURA=FAKE-ISOGUARD-HASURA-ADMIN-SECRET-000000000
B=$((24000 + RANDOM % 2000))
HASURA="http://127.0.0.1:$((B + 480))"

# ---- project: init, template overlay, env ----------------------------------------------------
cd "$DIR"
nself init --non-interactive --name "$NAME" --no-pgvector >>"$LOG" 2>&1 || fail 'init'
owned=1
nself init --template multi-app . --force >"$W/overlay.out" 2>>"$LOG" || fail 'template overlay'
for f in README.md manifest.json nself.yml .env.example migrations/001_multi_app.sql hasura/metadata/databases/default/tables/tables.yaml; do
  test -f "$f" || fail "overlay did not write $f"
done
test ! -e flutter || fail 'overlay wrote a flutter/ folder'
# One step per line: the flag audit reads a flag against the nearest `nself` word on its line.
steps=(
  'nself build'
  'nself start'
  'nself db migrate up --migration-dir migrations'
  'nself db reconcile --apply'
)
for step in "${steps[@]}"; do
  grep -qx "    $step" "$W/overlay.out" || fail "overlay next steps lack: $step"
done
pass 'template overlay wrote the multi-app files and printed its four next steps'
{
  cat <<ENV
ENV=dev
POSTGRES_PASSWORD=$F_PG
HASURA_GRAPHQL_ADMIN_SECRET=$F_HASURA
AUTH_EMAIL_SIGNIN_EMAIL_VERIFIED_REQUIRED=false
AUTH_SMTP_HOST=mailpit
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
  # The shipped template line that maps user metadata to the x-hasura-source-account-id claim.
  grep -E '^(AUTH_JWT_CUSTOM_CLAIMS|ENV_ALLOWLIST)=' .env.example
} >>.env.local
chmod 600 .env.local

# ---- build, start, migrate, reconcile --------------------------------------------------------
nself build --force >>"$LOG" 2>&1 || fail 'build'
nself start --timeout 300 --skip-build >>"$LOG" 2>&1 || fail 'start'
PGDB="$(cat .env .env.local 2>/dev/null | sed -n 's/^POSTGRES_DB=//p' | tail -1)"
PGDB="${PGDB:-${NAME//-/_}}"
pg() { docker exec "${NAME}_postgres" psql -X -U postgres -d "$PGDB" -v ON_ERROR_STOP=1 -At -c "$1"; }
nself db migrate up --migration-dir migrations >>"$LOG" 2>&1 || fail 'migrate'
nself db migrate up --migration-dir migrations >>"$LOG" 2>&1 || fail 'migrate is not re-runnable'
test "$(pg "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('app_members','notes','projects')")" = 3 || fail 'tables missing'
nself db reconcile >"$W/reconcile-plan.out" 2>>"$LOG" || fail 'reconcile dry run'
for t in notes projects app_members; do
  grep -q "public.$t" "$W/reconcile-plan.out" || fail "reconcile plan does not mention $t"
done
grep -q 'Dry run' "$W/reconcile-plan.out" || fail 'reconcile without --apply is not a dry run'
nself db reconcile --apply >>"$LOG" 2>&1 || fail 'reconcile --apply'
nself db reconcile >"$W/reconcile-again.out" 2>>"$LOG" || fail 'second reconcile'
grep -q 'Nothing to reconcile' "$W/reconcile-again.out" || fail 'live metadata still differs after reconcile'
pass 'migrate and targeted reconcile applied; a second reconcile is a no-op'

# ---- the guard: clean template, then a leaking permission -------------------------------------------
umask 077
printf 'X-Hasura-Admin-Secret: %s\n' "$F_HASURA" >"$W/admin.hdr"
# Doctor output: v1.5 prints an envelope {data:{checks}}, v1.4 the bare report; accept both.
D='def ch: (.data // .).checks;'
doctor_iso() { # OUTFILE: runs the guard in the project, returns its exit status
  (cd "$DIR" && nself doctor --deep --only isolation --json >"$1" 2>>"$LOG")
}
rc=0; doctor_iso "$W/iso-clean.json" || rc=$?
test "$rc" = 0 || fail "doctor --only isolation exited $rc on the clean template"
test "$(jq "$D"' [ch[]] | length' "$W/iso-clean.json")" -ge 4 || fail 'clean run: fewer than ISO-01..04 results'
test "$(jq "$D"' [ch[] | select(.name | startswith("[isolation] ") | not)] | length' "$W/iso-clean.json")" = 0 || fail 'clean run: a check outside section isolation'
test "$(jq "$D"' [ch[] | select(.status == "fail")] | length' "$W/iso-clean.json")" = 0 || fail 'clean run: isolation failures on the template'
for r in ISO-01 ISO-02 ISO-03 ISO-04; do
  test "$(jq --arg r "$r" "$D"' [ch[] | select(.name | contains($r)) | select(.status == "pass")] | length' "$W/iso-clean.json")" = 1 || fail "clean run: $r did not pass"
done
pass 'clean multi-app template: only section isolation, ISO-01..04 pass, zero failures'

# The admin metadata API swaps the notes select filter for {} (the leak the guard exists to catch).
cat >"$W/leak.json" <<'JSON'
{"type":"bulk","args":[
 {"type":"pg_drop_select_permission","args":{"source":"default","table":{"schema":"public","name":"notes"},"role":"user"}},
 {"type":"pg_create_select_permission","args":{"source":"default","table":{"schema":"public","name":"notes"},"role":"user","permission":{"columns":["id","source_account_id","title","created_at"],"filter":{}}}}
]}
JSON
curl -fsS "$HASURA/v1/metadata" -H 'Content-Type: application/json' -H "@$W/admin.hdr" --data-binary "@$W/leak.json" >"$W/leak.out" 2>>"$LOG" || fail 'metadata API refused the filter change'
jq -e 'type == "array"' "$W/leak.out" >/dev/null || fail 'metadata API did not apply the filter change'
rc=0; doctor_iso "$W/iso-leak.json" || rc=$?
test "$rc" != 0 || fail 'doctor --only isolation exited 0 with a leaking notes filter'
test "$(jq "$D"' [ch[] | select(.status == "fail") | select(.name | contains("ISO-02")) | select(.message | contains("public.notes") and contains("role=user") and contains("op=select"))] | length' "$W/iso-leak.json")" = 1 || fail 'leak run: no ISO-02 failure naming public.notes, user, select'
test "$(jq "$D"' [ch[] | select(.status == "fail")] | length' "$W/iso-leak.json")" = 1 || fail 'leak run: more than the one expected failure'
pass 'notes select filter replaced by {}: exit non-zero, ISO-02 fail names public.notes, user, select'
