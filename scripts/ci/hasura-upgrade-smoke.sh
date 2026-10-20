#!/usr/bin/env bash
# Purpose: prove the P7-LIVE-23 Hasura move (graphql-engine v2.44.0 -> v2.51.0) on a real
#          stack and print one PASS or FAIL line per assertion.
#          NSELF_V15=1 (upgrade):
#            1. a new v1.5 project is pinned to the hasura-v14 ref (v2.44.0) in
#               nself.lock.yaml (the pin edit stands in for `nself update`, P7-LIVE-10)
#               and starts on it
#            2. a fixture migration (tables and rows) is applied with `nself db migrate`;
#               metadata (tracked tables, a relationship, a select permission, an event
#               trigger to a webhook receiver, a remote schema to a GraphQL receiver) is
#               applied with `nself db hasura metadata apply` and exported with
#               `nself db hasura metadata export`
#            3. the functional checks pass on v2.44.0: rows, permission, mutation,
#               event trigger delivery, remote schema call, subscription event
#            4. a verified backup (nself backup create, verify, verify --restore-test)
#            5. the switch to the `hasura` ref (v2.51.0): the same functional checks, the
#               metadata export equals the pre-switch export (files and normalised JSON),
#               `nself db hasura diff` is clean, apply then export round-trips
#            6. rollback, unsupported path: pin back to v2.44.0 WITHOUT restoring the backup;
#               the observed outcome is asserted (a catalog refusal, or a clean start that
#               passes the functional checks, with the catalog versions in the message)
#            7. rollback, supported path: restore the pre-switch backup over the
#               migrated database (plain restore, no db reset), keep the v2.44.0 pin,
#               start cleanly; rows and metadata are the pre-switch ones, functional checks pass
#          NSELF_V15 unset (baseline): steps 1 to 3 on v2.44.0 plus the metadata
#          apply/export round trip; no switch.
# Usage:   NSELF_V15=1 bash scripts/ci/hasura-upgrade-smoke.sh [path-to-nself-binary]
#          env -u NSELF_V15 bash scripts/ci/hasura-upgrade-smoke.sh [path-to-nself-binary]
# Inputs:  optional binary (built with go into the temp dir when absent); docker with the
#          compose plugin and a reachable engine, curl (7.55+), python3 and openssl on PATH (nself
#          build generates the dev certificate with it; go only when no binary is given). Image refs are read from internal/compose/images.lock.json
#          (hasura, hasura-v14, python), never hardcoded. The stack publishes 127.0.0.1:8080
#          (Hasura), :5432, :80 and :443, so those must be free. TMPDIR must be a path the
#          Docker engine can bind-mount. The webhook and GraphQL receivers are one synthetic
#          container (scripts/ci/testdata/hasura-smoke/receiver.py) on the project network.
# Outputs: one line per assertion, "PASS <name>" or "FAIL <name>" (detail on stderr);
#          the last line is "HASURA-SMOKE: PASS" or "HASURA-SMOKE: FAIL". Exit 1 on any
#          FAIL, when nothing passed, or when a tool is missing. There is no SKIP: an
#          assertion that cannot run is a FAIL. Admin secrets are never printed.
# Constraints: bash 3.2 compatible. The project, HOME and every secret file live in one
#          0700 temp dir; the EXIT trap removes the project's containers, volumes and
#          networks (matched by the unique compose project label or exact container name)
#          and then the dir. Only synthetic rows and runtime-generated secrets are used;
#          the admin secret travels through 0600 files, never argv. No real project,
#          server or remote is touched.
set -uo pipefail

cd "$(dirname "$0")/../.." || exit 1
ROOT="$PWD"
REAL_HOME="$HOME"
FIX="$ROOT/scripts/ci/testdata/hasura-smoke"

PASSED=0
FAILED=0
pass() { printf 'PASS %s\n' "$1"; PASSED=$((PASSED + 1)); }
# Read credentials from private files; no secret values travel through argv.
redact() { python3 "$FIX/redact.py" "${WORK:-}" "${PDIR:-}"; }
fail() {
  printf 'FAIL %s\n' "$1"
  FAILED=$((FAILED + 1))
  if [ -n "${2:-}" ]; then printf '  %s\n' "$2" | redact >&2; fi
}
finish() {
  printf '%s assertions: %s passed, %s failed\n' "$((PASSED + FAILED))" "$PASSED" "$FAILED" >&2
  if [ "$FAILED" -eq 0 ] && [ "$PASSED" -gt 0 ]; then
    echo "HASURA-SMOKE: PASS"
    exit 0
  fi
  echo "HASURA-SMOKE: FAIL"
  exit 1
}
fatal() { fail "$@"; finish; }
info() { printf 'INFO %s\n' "$1"; }
step() { printf '== %s\n' "$1"; }
# check <name> <command...>: PASS when the command exits 0.
check() {
  local name="$1"
  shift
  if "$@" >/dev/null 2>&1; then pass "$name"; else fail "$name"; fi
}

# shellcheck source=testdata/hasura-smoke/bootstrap.sh
source "$FIX/bootstrap.sh"
# shellcheck source=testdata/hasura-smoke/helpers.sh
source "$FIX/helpers.sh"

# ---- step 1: new project on the hasura-v14 ref ----------------------------------------
step "1 new project on hasura-v14 ($VER_V14)"
# The stack publishes fixed loopback ports: refuse to start when any is taken.
if ! python3 -c 'import socket,sys
for p in (80, 443, 5432, 8080):
    s = socket.socket()
    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    try:
        s.bind(("127.0.0.1", p))
    except OSError:
        sys.exit("port %d is in use" % p)
    finally:
        s.close()' 2>"$WORK/ports.err"; then
  fatal "ports 80, 443, 5432 and 8080 are free on 127.0.0.1" "$(cat "$WORK/ports.err")"
fi
pass "ports 80, 443, 5432 and 8080 are free on 127.0.0.1"
nsf init --non-interactive --name "$PROJECT" --domain local.nself.org --no-pgvector
[ "$RC" -eq 0 ] || fatal "nself init --non-interactive exits 0" "rc=$RC $(nsf_tail)"
pass "nself init --non-interactive exits 0"
if [ "$(grep -c "^PROJECT_NAME=$PROJECT\$" "$PDIR/.env")" -eq 1 ]; then
  pass "project .env names PROJECT_NAME=$PROJECT"
else
  fatal "project .env names PROJECT_NAME=$PROJECT" "refusing to continue: the project is not the one this run created"
fi
check "project .env leaves HASURA_VERSION at the mode default" grep -q "^HASURA_VERSION=\$" "$PDIR/.env"
(umask 077; sed -n 's/^HASURA_GRAPHQL_ADMIN_SECRET=//p' "$PDIR/.env" | head -n1 | tr -d '\r\n"' > "$WORK/admin.secret")
if [ -s "$WORK/admin.secret" ]; then
  (umask 077; printf 'X-Hasura-Admin-Secret: %s\n' "$(cat "$WORK/admin.secret")" > "$WORK/admin.hdr")
  pass "admin secret read from the project .env into a 0600 file"
else
  fatal "admin secret read from the project .env into a 0600 file"
fi
nsf build
[ "$RC" -eq 0 ] || fatal "nself build exits 0" "rc=$RC $(nsf_tail)"
pass "nself build exits 0"
if [ "$UPGRADE" = 1 ]; then
  # v1.5 pins the new default on first build; this project is an existing v2.44.0 project, so
  # the pin goes back to the hasura-v14 ref before anything starts.
  if [ "$(lock_hasura_pin)" = "$REF_NEW" ]; then pass "v1.5 first build pins hasura to the new ref"; else fail "v1.5 first build pins hasura to the new ref" "got $(lock_hasura_pin)"; fi
  set_pin "$REF_V14"
  nsf build
  [ "$RC" -eq 0 ] || fatal "nself build after the pin edit exits 0" "rc=$RC $(nsf_tail)"
  pass "nself build after the pin edit exits 0"
  if [ "$(compose_hasura_image)" = "$REF_V14" ]; then pass "compose hasura image is the hasura-v14 ref (the pin wins over the v1.5 default)"; else fail "compose hasura image is the hasura-v14 ref" "got $(compose_hasura_image)"; fi
  if [ "$(lock_hasura_pin)" = "$REF_V14" ]; then pass "nself.lock.yaml pins hasura to the hasura-v14 ref"; else fail "nself.lock.yaml pins hasura to the hasura-v14 ref" "got $(lock_hasura_pin)"; fi
  EXPECT_V14="$REF_V14"
else
  if [ "$(compose_hasura_image)" = "$LEGACY_V14" ]; then pass "v1.4 compose hasura image is $LEGACY_V14"; else fail "v1.4 compose hasura image is $LEGACY_V14" "got $(compose_hasura_image)"; fi
  EXPECT_V14="$LEGACY_V14"
fi
nsf start
[ "$RC" -eq 0 ] || fatal "nself start exits 0" "rc=$RC $(nsf_tail)"
pass "nself start exits 0"
wait_hasura || fatal "Hasura healthy on hasura-v14"
pass "Hasura healthy on hasura-v14"
if [ "$(running_hasura_image)" = "$EXPECT_V14" ]; then pass "running Hasura container uses the hasura-v14 ref"; else fail "running Hasura container uses the hasura-v14 ref" "got $(running_hasura_image)"; fi
if [ "$(running_hasura_version)" = "$VER_V14" ]; then pass "Hasura reports version $VER_V14"; else fail "Hasura reports version $VER_V14" "got $(running_hasura_version)"; fi

# ---- step 2: data and metadata on hasura-v14 ------------------------------------------
step "2 migration and metadata on hasura-v14"
mkdir -p "$PDIR/hasura/migrations/default"
cp "$FIX/migrations/"*.sql "$PDIR/hasura/migrations/default/"
cp "$FIX/metadata.json" "$PDIR/hasura/metadata.json"
nsf db migrate up
if [ "$RC" -eq 0 ] && grep -q 'Applied 1 migration' "$WORK/out"; then pass "nself db migrate applies the fixture migration"; else fail "nself db migrate applies the fixture migration" "rc=$RC $(nsf_tail)"; fi
if [ "$(dbq 'select count(*) from public.smoke_items;')" = 3 ]; then pass "fixture rows are in postgres (3 smoke_items)"; else fail "fixture rows are in postgres (3 smoke_items)"; fi
if start_receiver; then pass "receiver container (webhook :9100, remote schema :9101) is up on the project network"; else fatal "receiver container is up on the project network" "$(redact < "$WORK/rcv.err" | tail -n 3 | tr '\n' '|')"; fi
nsf db hasura metadata apply
if [ "$RC" -eq 0 ]; then pass "nself db hasura metadata apply exits 0"; else fatal "nself db hasura metadata apply exits 0" "rc=$RC $(nsf_tail)"; fi
if export_tree "$WORK/meta-pre" && export_json "$WORK/meta-pre.json"; then pass "metadata exported before any switch (tree and API JSON)"; else fatal "metadata exported before any switch" "rc=$RC $(nsf_tail)"; fi
check "exported metadata tracks both tables, the event trigger and the remote schema" python3 -c 'import json,sys
m = json.load(open(sys.argv[1]))
t = {x["table"]["name"]: x for x in m["sources"][0]["tables"]}
assert set(t) == {"smoke_items", "smoke_tags"}
assert t["smoke_items"]["event_triggers"][0]["name"] == "smoke_item_events"
assert t["smoke_items"]["select_permissions"][0]["role"] == "user"
assert m["remote_schemas"][0]["name"] == "smoke_remote"' "$WORK/meta-pre.json"
nsf db hasura metadata apply
if [ "$RC" -eq 0 ] && export_tree "$WORK/meta-rt" && cmp -s "$WORK/meta-pre.txt" "$WORK/meta-rt.txt"; then
  pass "round trip on hasura-v14: apply the exported tree, export again, identical"
else
  fail "round trip on hasura-v14: apply the exported tree, export again, identical" "rc=$RC"
fi
CAT_V14="$(catalog_version)"
info "hasura catalog version on $VER_V14: $CAT_V14"
functional v14 3
export_json "$WORK/meta-v14-after.json" || fail "metadata export through the API"
if cmp -s "$WORK/meta-pre.json" "$WORK/meta-v14-after.json"; then pass "v14: API metadata is unchanged by the functional checks"; else fail "v14: API metadata is unchanged by the functional checks"; fi

if [ "$UPGRADE" != 1 ]; then
  finish
fi

# ---- step 3: verified backup -----------------------------------------------------------
step "3 verified backup"
NROWS_BACKUP="$(dbq 'select count(*) from public.smoke_items;')"
nsf backup create
if [ "$RC" -eq 0 ]; then pass "nself backup create exits 0"; else fail "nself backup create exits 0" "rc=$RC $(nsf_tail)"; fi
nsf backup list --format json
BID="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["data"][0]["id"])' "$WORK/out" 2>/dev/null)"
if [ -n "$BID" ] && [ -f "$PDIR/backups/$BID.dump" ]; then
  pass "backup recorded: id $BID, path backups/$BID.dump ($NROWS_BACKUP smoke_items rows)"
else
  fatal "backup recorded (id and file)" "id='$BID' $(find "$PDIR/backups" -type f 2>&1 | head -3 | tr '\n' ' ')"
fi
nsf backup verify "$BID"
if [ "$RC" -eq 0 ]; then pass "nself backup verify exits 0"; else fail "nself backup verify exits 0" "rc=$RC $(nsf_tail)"; fi
nsf backup verify "$BID" --restore-test
if [ "$RC" -eq 0 ] && grep -q 'restore-test' "$WORK/out"; then pass "nself backup verify --restore-test exits 0"; else fail "nself backup verify --restore-test exits 0" "rc=$RC $(nsf_tail)"; fi

# ---- step 4: switch to the hasura ref --------------------------------------------------
step "4 switch to the hasura ref ($VER_NEW)"
set_pin "$REF_NEW"
# nself update is the official pin mover (P7-LIVE-10): the pin edit above is its stand-in.
nsf build
if [ "$RC" -eq 0 ]; then pass "switch: nself build exits 0"; else fail "switch: nself build exits 0" "rc=$RC $(nsf_tail)"; fi
if [ "$(compose_hasura_image)" = "$REF_NEW" ]; then pass "switch: compose hasura image is the hasura ref"; else fail "switch: compose hasura image is the hasura ref" "got $(compose_hasura_image)"; fi
if [ "$(lock_hasura_pin)" = "$REF_NEW" ]; then pass "switch: nself.lock.yaml pins the hasura ref"; else fail "switch: nself.lock.yaml pins the hasura ref" "got $(lock_hasura_pin)"; fi
nsf start
if [ "$RC" -eq 0 ]; then pass "switch: nself start exits 0"; else fail "switch: nself start exits 0" "rc=$RC $(nsf_tail)"; fi
if wait_hasura; then pass "switch: Hasura healthy"; else fail "switch: Hasura healthy" "$(docker logs --tail 5 "${PROJECT}_hasura" 2>&1 | redact | tr '\n' '|')"; fi
if [ "$(running_hasura_image)" = "$REF_NEW" ]; then pass "switch: running Hasura container uses the hasura ref"; else fail "switch: running Hasura container uses the hasura ref" "got $(running_hasura_image)"; fi
if [ "$(running_hasura_version)" = "$VER_NEW" ]; then pass "switch: Hasura reports version $VER_NEW"; else fail "switch: Hasura reports version $VER_NEW" "got $(running_hasura_version)"; fi

# ---- step 5: after the switch ----------------------------------------------------------
step "5 after the switch"
nsf db hasura diff
if [ "$RC" -eq 0 ]; then pass "nself db hasura diff exits 0 (live metadata equals the on-disk export)"; else fail "nself db hasura diff exits 0" "rc=$RC $(nsf_tail)"; fi
if export_json "$WORK/meta-post.json" && cmp -s "$WORK/meta-pre.json" "$WORK/meta-post.json"; then
  pass "metadata export (API JSON, normalised) equals the pre-switch export"
else
  fail "metadata export (API JSON, normalised) equals the pre-switch export" "$(diff "$WORK/meta-pre.json" "$WORK/meta-post.json" 2>&1 | head -n 8 | tr '\n' '|')"
fi
if export_tree "$WORK/meta-post" && cmp -s "$WORK/meta-pre.txt" "$WORK/meta-post.txt"; then
  pass "nself db hasura metadata export tree equals the pre-switch export"
else
  fail "nself db hasura metadata export tree equals the pre-switch export" "$(diff "$WORK/meta-pre.txt" "$WORK/meta-post.txt" 2>&1 | head -n 8 | tr '\n' '|')"
fi

nsf db hasura metadata apply
if [ "$RC" -eq 0 ] && export_tree "$WORK/meta-rt2" && cmp -s "$WORK/meta-pre.txt" "$WORK/meta-rt2.txt"; then
  pass "v2.51 round trip: apply the exported tree, export again, identical to the pre-switch export"
else
  fail "v2.51 round trip: apply the exported tree, export again, identical to the pre-switch export" "rc=$RC $(nsf_tail)"
fi
functional v251 "$NROWS_BACKUP"
NROWS_AFTER="$(dbq 'select count(*) from public.smoke_items;')"
CAT_NEW="$(catalog_version)"
info "hasura catalog version on $VER_NEW: $CAT_NEW (on $VER_V14 it was $CAT_V14)"

# ---- step 6: rollback, unsupported (pin back, no restore) -----------------------------
step "6 pin back to hasura-v14 without restoring"
set_pin "$REF_V14"
# nself update is the official pin mover (P7-LIVE-10): the pin edit above is its stand-in.
# The CLI does not guard a pin-back: the outcome is whatever the older engine does with the
# catalog the newer one left, and it is asserted as observed. Hasura is either refused with a
# catalog message in its log (PASS), or it starts and passes every functional check (PASS,
# with the catalog versions in the message); anything else is a FAIL.
nsf build
if [ "$RC" -ne 0 ]; then
  fail "pin-back-without-restore-observed" "nself build failed, rc=$RC $(nsf_tail)"
else
  nsf start
  START_RC="$RC"
  if wait_hasura 90 && [ "$(running_hasura_version)" = "$VER_V14" ] && [ "$(running_hasura_image)" = "$REF_V14" ]; then
    CAT_BACK="$(catalog_version)"
    pass "pin-back-without-restore-observed: Hasura $VER_V14 starts on the database $VER_NEW migrated (start rc=$START_RC; catalog version $CAT_V14 on $VER_V14, $CAT_NEW on $VER_NEW, $CAT_BACK after the pin-back)"
    functional pinback "$NROWS_AFTER"
  else
    WHY="$(docker logs --tail 40 "${PROJECT}_hasura" 2>&1 | grep -i -m1 -o 'catalog[^"]*\(version\|newer\|downgrade\)[^"]*\|[^"]*downgrade[^"]*' | cut -c1-160)"
    STATE="$(docker inspect -f '{{.State.Status}} restarts={{.RestartCount}}' "${PROJECT}_hasura" 2>/dev/null)"
    if [ -n "$WHY" ]; then
      pass "pin-back-without-restore-observed: Hasura $VER_V14 refuses the migrated database ($STATE; start rc=$START_RC; log: $WHY)"
    else
      fail "pin-back-without-restore-observed" "Hasura is not healthy ($STATE; start rc=$START_RC) but the log shows no catalog refusal: $(docker logs --tail 5 "${PROJECT}_hasura" 2>&1 | redact | cut -c1-160 | tr '\n' '|')"
    fi
  fi
fi

# ---- step 7: rollback, supported (restore the pre-switch backup) ---------------------
step "7 restore the pre-switch backup, keep hasura-v14"
if [ "$(lock_hasura_pin)" = "$REF_V14" ]; then pass "restore: lock still pins the hasura-v14 ref"; else fail "restore: lock still pins the hasura-v14 ref" "got $(lock_hasura_pin)"; fi
# Rows right before the restore: the pin-back step inserted more after NROWS_AFTER was read.
NROWS_BEFORE_RESTORE="$(dbq 'select count(*) from public.smoke_items;')"
# Stop only Hasura (postgres must stay up for the restore).
nsf stop hasura
if [ "$RC" -eq 0 ]; then pass "restore: nself stop hasura exits 0"; else fail "restore: nself stop hasura exits 0" "rc=$RC $(nsf_tail)"; fi
nsf backup restore "$BID" --only pg --yes
if [ "$RC" -eq 0 ]; then
  pass "restore: nself backup restore --only pg --yes exits 0 on the migrated database"
else
  fail "restore: nself backup restore --only pg --yes exits 0 on the migrated database" "rc=$RC $(nsf_tail)"
fi
build_start "restore" || true
if [ "$(compose_hasura_image)" = "$REF_V14" ] && [ "$(running_hasura_image)" = "$REF_V14" ]; then
  pass "restore: compose and container run the hasura-v14 ref"
else
  fail "restore: compose and container run the hasura-v14 ref" "compose=$(compose_hasura_image) running=$(running_hasura_image)"
fi
if [ "$(running_hasura_version)" = "$VER_V14" ]; then pass "restore: Hasura reports version $VER_V14"; else fail "restore: Hasura reports version $VER_V14" "got $(running_hasura_version)"; fi
if [ "$(dbq 'select count(*) from public.smoke_items;')" = "$NROWS_BACKUP" ]; then pass "restore: smoke_items has the $NROWS_BACKUP rows of the backup (the $((NROWS_BEFORE_RESTORE - NROWS_BACKUP)) rows added since are gone)"; else fail "restore: smoke_items has the $NROWS_BACKUP rows of the backup" "got $(dbq 'select count(*) from public.smoke_items;')"; fi
if export_json "$WORK/meta-restored.json" && cmp -s "$WORK/meta-pre.json" "$WORK/meta-restored.json"; then
  pass "restore: metadata export equals the pre-switch export"
else
  fail "restore: metadata export equals the pre-switch export" "$(diff "$WORK/meta-pre.json" "$WORK/meta-restored.json" 2>&1 | head -n 8 | tr '\n' '|')"
fi
functional restored "$NROWS_BACKUP"

finish
