#!/usr/bin/env bash
# Purpose: Linux acceptance for the multi-app isolation template (P7-ADOPT-04, EPIC D3 review F10):
# init + template overlay + build + start + migrate + `nself db reconcile`, then prove the
# source_account_id wall with real hasura-auth users and forged tokens.
# Inputs: W shared host/container path, nself/docker/curl/jq/openssl on PATH, run from the repo root.
# Outputs: non-vacuous PASS assertions. Exit 0 on success, 1 on any failure.
# Constraints: fake credentials only; bash 3.2 compatible. The JWT key is read with
#   `nself config get HASURA_JWT_KEY --reveal`, held in one shell variable and never echoed,
#   never put on a command line (the HMAC below feeds it through pipes) and never written to disk.
#   Admin secret and bearer tokens travel only in 0600 header files under $W (curl -H @file).
#   Cleanup owns only the e2e-multi-app-* project.
set -euo pipefail
export NSELF_V15=1
command -v openssl >/dev/null || apk add -q openssl >/dev/null
: "${W:?W must be a host-visible work directory}"
NAME=e2e-multi-app-iso
DIR="$W/p"
test ! -e "$DIR" || { echo 'FAIL: fixture path occupied'; exit 1; }
test -z "$(docker ps -aq --filter "name=^${NAME}_")" || { echo 'FAIL: fixture project already exists'; exit 1; }
mkdir -p "$DIR" "$W/home"
export HOME="$W/home"
LOG="$W/multi-app-e2e.log"
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

F_PG=FAKE-MULTIAPP-POSTGRES-PASSWORD-00000000
F_HASURA=FAKE-MULTIAPP-HASURA-ADMIN-SECRET-000000000
F_JWT=FAKE-MULTIAPP-JWT-KEY-000000000000000000000000
B=$((24000 + RANDOM % 2000))
HASURA="http://127.0.0.1:$((B + 480))"
AUTH="http://127.0.0.1:$((B + 400))"

# ---- project: init, template overlay, env ----------------------------------------------------
cd "$DIR"
nself init --non-interactive --name "$NAME" --no-pgvector >>"$LOG" 2>&1 || fail 'init'
owned=1
nself init --template multi-app . --force >"$W/overlay.out" 2>>"$LOG" || fail 'template overlay'
for f in README.md manifest.json nself.yml .env.example migrations/001_multi_app.sql hasura/metadata.json hasura/metadata/databases/default/tables/tables.yaml; do
  test -f "$f" || fail "overlay did not write $f"
done
test ! -e flutter || fail 'overlay wrote a flutter/ folder'
for step in 'nself build' 'nself start' 'nself db migrate --migration-dir migrations' 'nself db reconcile'; do
  grep -qx "    $step" "$W/overlay.out" || fail "overlay next steps lack: $step"
done
pass 'template overlay wrote the multi-app files and printed its four next steps'
{
  cat <<ENV
ENV=dev
POSTGRES_PASSWORD=$F_PG
HASURA_GRAPHQL_ADMIN_SECRET=$F_HASURA
HASURA_JWT_KEY=$F_JWT
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
  grep '^AUTH_JWT_CUSTOM_CLAIMS=' .env.example
} >>.env.local
chmod 600 .env.local

# ---- build, start, migrate, reconcile --------------------------------------------------------
nself build --force >>"$LOG" 2>&1 || fail 'build'
nself start --timeout 300 --skip-build >>"$LOG" 2>&1 || fail 'start'
PGDB="$(cat .env .env.local 2>/dev/null | sed -n 's/^POSTGRES_DB=//p' | tail -1)"
PGDB="${PGDB:-${NAME//-/_}}"
pg() { docker exec "${NAME}_postgres" psql -X -U postgres -d "$PGDB" -v ON_ERROR_STOP=1 -At -c "$1"; }
nself db migrate --migration-dir migrations >>"$LOG" 2>&1 || fail 'migrate'
nself db migrate --migration-dir migrations >>"$LOG" 2>&1 || fail 'migrate is not re-runnable'
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

# ---- helpers: admin header, HS256 without the key on any command line -------------------------
umask 077
printf 'X-Hasura-Admin-Secret: %s\n' "$F_HASURA" >"$W/admin.hdr"
JWT_KEY="$(nself config get HASURA_JWT_KEY --reveal 2>>"$LOG")" || fail 'config get HASURA_JWT_KEY'
JWT_KEY="${JWT_KEY#HASURA_JWT_KEY=}"
test -n "$JWT_KEY" || fail 'empty JWT key'
b64u() { openssl base64 -A | tr '+/' '-_' | tr -d '='; }
hexof() { od -An -v -tx1 | tr -d ' \n'; }
# shellcheck disable=SC2059 # the line is only \xHH escapes
unhex() { sed 's/../\\x&/g' | { read -r l; printf "$l"; }; }
# hmac_sha256 MESSAGE: HMAC-SHA256 of MESSAGE under $JWT_KEY, raw bytes on stdout. The key only
# ever travels through pipes (openssl's own -hmac option would put it on argv).
hmac_sha256() {
  local hex ipad="" opad="" i=0 b inner
  hex="$(printf '%s' "$JWT_KEY" | hexof)"
  if [ "${#hex}" -gt 128 ]; then hex="$(printf '%s' "$JWT_KEY" | openssl dgst -sha256 -binary | hexof)"; fi
  while [ "${#hex}" -lt 128 ]; do hex="${hex}00"; done
  while [ "$i" -lt 128 ]; do
    b="${hex:$i:2}"
    ipad="$ipad$(printf '%02x' $((0x$b ^ 0x36)))"
    opad="$opad$(printf '%02x' $((0x$b ^ 0x5c)))"
    i=$((i + 2))
  done
  inner="$({ printf '%s' "$ipad" | unhex; printf '%s' "$1"; } | openssl dgst -sha256 -binary | hexof)"
  { printf '%s' "$opad" | unhex; printf '%s' "$inner" | unhex; } | openssl dgst -sha256 -binary
}
# mint FILE USER_ID APP_ID: a user-role token with a source-account claim, as a header file.
mint() {
  local hdr pl now sig
  now="$(date +%s)"
  hdr="$(printf '{"alg":"HS256","typ":"JWT"}' | b64u)"
  pl="$(jq -cn --arg u "$2" --arg a "$3" --argjson n "$now" '{sub:$u,iat:$n,exp:($n+600),
    "https://hasura.io/jwt/claims":{"x-hasura-allowed-roles":["user"],"x-hasura-default-role":"user",
    "x-hasura-user-id":$u,"x-hasura-source-account-id":$a}}' | b64u)"
  sig="$(hmac_sha256 "$hdr.$pl" | b64u)"
  printf 'Authorization: Bearer %s.%s.%s\n' "$hdr" "$pl" "$sig" >"$1"
}
# b64u_dec: decode base64url (padding restored) from stdin.
b64u_dec() {
  local s
  s="$(cat | tr '_-' '/+')"
  while [ $((${#s} % 4)) -ne 0 ]; do s="$s="; done
  printf '%s' "$s" | openssl base64 -d -A
}
# nrows HEADER_FILE TABLE: number of rows of TABLE the token can read; fails unless Hasura returned a list
# (an invalid token or a query error must never look like "zero rows").
nrows() { gql "$1" "{ $2 { id } }" | jq -e ".data.$2|arrays|length"; }
# errcode HEADER_FILE QUERY: the first GraphQL error code ('' when there is none).
errcode() { gql "$1" "$2" | jq -r '.errors[0].extensions.code // ""'; }
# gql HEADER_FILE QUERY: the JSON response; query text only on stdin.
gql() {
  jq -cn --arg q "$2" '{query:$q}' | curl -sS "$HASURA/v1/graphql" -H 'Content-Type: application/json' -H "@$1" --data-binary @-
}
# signup EMAIL APP: password-signup through hasura-auth with the app id in the user metadata.
signup() {
  jq -cn --arg e "$1" --arg p "FAKE-USER-PASSWORD-$1" --arg a "$2" '{email:$e,password:$p,options:{metadata:{source_account_id:$a}}}' |
    curl -fsS "$AUTH/signup/email-password" -H 'Content-Type: application/json' --data-binary @- >"$W/signup-$1.json" || fail "signup $1"
  jq -r '.session.user.id' "$W/signup-$1.json" >"$W/uid-$1"
  jq -r '"Authorization: Bearer " + .session.accessToken' "$W/signup-$1.json" >"$W/tok-$1.hdr"
  rm -f "$W/signup-$1.json"
}
mint "$W/probe.hdr" 00000000-0000-0000-0000-000000000001 app_a
test "$(nrows "$W/probe.hdr" notes)" = 0 || fail 'minted token rejected or leaking'
pass 'HS256 tokens minted with openssl are accepted by Hasura'

# ---- users: three real hasura-auth sign-ups ---------------------------------------------------
signup a-member@example.invalid app_a
signup b-member@example.invalid app_b
signup b-outsider@example.invalid app_b
UA="$(cat "$W/uid-a-member@example.invalid")"
UB="$(cat "$W/uid-b-member@example.invalid")"
claim="$(sed 's/^Authorization: Bearer //' "$W/tok-b-outsider@example.invalid.hdr" | cut -d. -f2 | b64u_dec | jq -r '."https://hasura.io/jwt/claims"["x-hasura-source-account-id"]')"
test "$claim" = app_b || fail "hasura-auth did not put options.metadata.source_account_id in the token (got '$claim')"
pass 'signup metadata becomes the x-hasura-source-account-id claim'
# Membership is server-controlled: only the admin path writes it. The outsider (app_b) has none.
pg "INSERT INTO public.app_members (user_id, source_account_id) VALUES ('$UA','app_a'), ('$UB','app_b')" >>"$LOG" 2>&1 || fail 'membership'
cp "$W/tok-a-member@example.invalid.hdr" "$W/ua.hdr"
cp "$W/tok-b-member@example.invalid.hdr" "$W/ub.hdr"
cp "$W/tok-b-outsider@example.invalid.hdr" "$W/uc.hdr"
mint "$W/forged.hdr" 11111111-1111-4111-8111-111111111111 app_b

# ---- seed with the admin secret: 3 rows in app_a, 2 in app_b, per table ------------------------
# fields TABLE APP LABEL: GraphQL object fields (projects also need a name).
fields() {
  if [ "$1" = projects ]; then printf 'source_account_id:"%s", title:"%s", name:"n-%s"' "$2" "$3" "$3"; else printf 'source_account_id:"%s", title:"%s"' "$2" "$3"; fi
}
seed() { # TABLE APP LABEL
  gql "$W/admin.hdr" "mutation { insert_$1_one(object:{$(fields "$1" "$2" "$3")}) { id } }" |
    jq -e ".data.insert_$1_one.id" >/dev/null || fail "seed $1 $3"
}
count_b() { pg "SELECT count(*) FROM public.$1 WHERE source_account_id='app_b'"; }
for T in notes projects; do
  seed "$T" app_a a1; seed "$T" app_a a2; seed "$T" app_a a3
  seed "$T" app_b b1; seed "$T" app_b b2
  test "$(pg "SELECT count(*) FILTER (WHERE source_account_id='app_a') || '/' || count(*) FILTER (WHERE source_account_id='app_b') FROM public.$T")" = 3/2 || fail "seed counts $T"
done

# ---- the wall, for notes and for projects ------------------------------------------------------
for T in notes projects; do
  r="$(gql "$W/ua.hdr" "{ $T { source_account_id } }")"
  test "$(echo "$r" | jq "[.data.${T}[]|select(.source_account_id==\"app_a\")]|length")" = 3 || fail "$T: app_a member does not read its 3 rows"
  test "$(echo "$r" | jq "[.data.${T}[]|select(.source_account_id==\"app_b\")]|length")" = 0 || fail "$T: app_a member reads app_b rows"
  pass "$T: app_a member reads 3 app_a rows and 0 app_b rows"

  test "$(nrows "$W/forged.hdr" "$T")" = 0 || fail "$T: forged app_b token reads rows"
  pass "$T: forged app_b token (no membership) reads nothing"

  test "$(nrows "$W/uc.hdr" "$T")" = 0 || fail "$T: app_b signup without membership reads app_b rows"
  test "$(errcode "$W/uc.hdr" "mutation { insert_${T}_one(object:{$(fields "$T" app_b outsider | sed 's/source_account_id:"app_b", //')}) { id } }")" = permission-error || fail "$T: app_b signup without membership inserted"
  test "$(count_b "$T")" = 2 || fail "$T: outsider insert changed app_b"
  pass "$T: real app_b signup without app_members reads zero rows and cannot insert (review F10)"

  r="$(gql "$W/ua.hdr" "mutation { insert_${T}_one(object:{$(fields "$T" x mine | sed 's/source_account_id:"x", //')}) { source_account_id } }")"
  test "$(echo "$r" | jq -r ".data.insert_${T}_one.source_account_id")" = app_a || fail "$T: insert did not preset app_a"
  test "$(errcode "$W/ua.hdr" "mutation { insert_${T}_one(object:{$(fields "$T" app_b spoof)}) { id } }")" = validation-failed || fail "$T: client may set source_account_id"
  test "$(count_b "$T")" = 2 || fail "$T: spoofed insert reached app_b"
  pass "$T: insert presets source_account_id=app_a; a client-sent value is refused"

  A1="$(pg "SELECT id FROM public.$T WHERE title='a1'")"
  r="$(gql "$W/ub.hdr" "mutation { update_$T(where:{id:{_eq:\"$A1\"}}, _set:{title:\"hacked\"}) { affected_rows } }")"
  test "$(echo "$r" | jq -r ".data.update_$T.affected_rows")" = 0 || fail "$T: app_b member updated an app_a row"
  r="$(gql "$W/ub.hdr" "mutation { delete_$T(where:{id:{_eq:\"$A1\"}}) { affected_rows } }")"
  test "$(echo "$r" | jq -r ".data.delete_$T.affected_rows")" = 0 || fail "$T: app_b member deleted an app_a row"
  test "$(pg "SELECT title FROM public.$T WHERE id='$A1'")" = a1 || fail "$T: app_a row changed"
  r="$(gql "$W/ub.hdr" "mutation { update_$T(where:{title:{_eq:\"b1\"}}, _set:{title:\"b1-edited\"}) { affected_rows } }")"
  test "$(echo "$r" | jq -r ".data.update_$T.affected_rows")" = 1 || fail "$T: app_b member cannot update its own row"
  test "$(errcode "$W/ub.hdr" "mutation { update_$T(where:{title:{_eq:\"b2\"}}, _set:{source_account_id:\"app_a\"}) { affected_rows } }")" = validation-failed || fail "$T: update may move a row between apps"
  pass "$T: app_b member cannot update or delete an app_a row, can edit its own, cannot move rows"
done

test "$(errcode "$W/ua.hdr" '{ app_members { user_id } }')" = validation-failed || fail 'user role can reach app_members'
pass 'app_members is not reachable by the user role'
echo 'ALL PASS: multi-app isolation'
