#!/usr/bin/env bash
# Purpose: prove the P7-LIVE-20 auth image move (nhost/hasura-auth -> nhost/auth) on a
#          real stack and print one PASS or FAIL line per assertion:
#            1. a new v1.5 project without AUTH_ENCRYPTION_KEY renders auth-v14
#               (nhost/auth 0.36.0) in docker-compose.yml and pins it in nself.lock.yaml
#            2. on auth-v14: password sign-up and sign-in, TOTP MFA enrolment and TOTP
#               sign-in, and a Hasura request with the issued JWT all work
#            3. a verified backup (nself backup create, verify, verify --restore-test)
#            4. the switch to the `auth` ref (current nhost/auth) with a fresh
#               AUTH_ENCRYPTION_KEY: compose and container run the new image
#            5. after the switch: password sign-in, a Hasura request with the new JWT,
#               the pre-upgrade refresh token on POST /token (observed, asserted as
#               observed), the TOTP secret is encrypted at rest and TOTP sign-in still
#               works, and a new user signs up and signs in
#            6. rollback, unsupported path: pin back to auth-v14 WITHOUT restoring the
#               backup; the breakage is shown and counts as the PASS
#            7. rollback, supported path: stop auth and hasura, restore the pre-switch
#               backup (after nself db reset --yes when the plain restore fails), keep the
#               auth-v14 pin, start cleanly; user A signs in, B's TOTP sign-in works,
#               the post-switch user C is gone
# Usage:   NSELF_V15=1 bash scripts/ci/auth-smoke.sh [path-to-nself-binary]
# Inputs:  NSELF_V15=1 (anything else prints FAIL and stops); optional binary (built
#          with go into the temp dir when absent); docker with the compose plugin and
#          a reachable engine, curl (7.55+), python3 and go on PATH (go for the image
#          refs, read from tools/imagelist, never hardcoded). The stack publishes
#          127.0.0.1:4000 (auth), :8080 (Hasura), :5432, :80 and :443, so those must be
#          free. TMPDIR must be a path the Docker engine can bind-mount.
# Outputs: one line per assertion, "PASS <name>" or "FAIL <name>" (detail on stderr);
#          the last line is "AUTH-SMOKE: PASS" or "AUTH-SMOKE: FAIL". Exit 1 on any
#          FAIL, when nothing passed, or when a tool is missing. There is no SKIP: an
#          assertion that cannot run is a FAIL. Tokens, passwords and keys are never
#          printed; detail lines are redacted.
# Constraints: bash 3.2 compatible. The project, HOME and every secret file live in one
#          0700 temp dir; the EXIT trap removes the project's containers, volumes and
#          networks (matched by the unique compose project label) and then the dir.
#          Only synthetic users (*@auth-smoke.test) and runtime-generated secrets are
#          used; secrets travel through stdin and 0600 files, never argv. No real
#          project, server or remote is touched. The pin edit in step 4 and 6 stands in
#          for `nself update` (P7-LIVE-10), the official pin mover, which is not merged.
set -uo pipefail

cd "$(dirname "$0")/../.." || exit 1
ROOT="$PWD"
REAL_HOME="$HOME"

PASSED=0
FAILED=0
pass() { printf 'PASS %s\n' "$1"; PASSED=$((PASSED + 1)); }
# redact: strip anything that looks like a JWT or a long hex secret from detail lines.
redact() { sed -E 's/eyJ[A-Za-z0-9_.-]+/<jwt>/g; s/[A-Fa-f0-9]{32,}/<hex>/g'; }
fail() {
  printf 'FAIL %s\n' "$1"
  FAILED=$((FAILED + 1))
  if [ -n "${2:-}" ]; then printf '  %s\n' "$2" | redact >&2; fi
}
finish() {
  printf '%s assertions: %s passed, %s failed\n' "$((PASSED + FAILED))" "$PASSED" "$FAILED" >&2
  if [ "$FAILED" -eq 0 ] && [ "$PASSED" -gt 0 ]; then
    echo "AUTH-SMOKE: PASS"
    exit 0
  fi
  echo "AUTH-SMOKE: FAIL"
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

if [ "${NSELF_V15:-}" != "1" ]; then
  fatal "NSELF_V15 is not 1 (run: NSELF_V15=1 bash scripts/ci/auth-smoke.sh)"
fi

for t in docker curl python3 go; do
  command -v "$t" >/dev/null 2>&1 || fatal "tool-$t-missing"
done
docker info >/dev/null 2>&1 || fatal "docker-engine-unreachable"
docker compose version >/dev/null 2>&1 || fatal "docker-compose-plugin-missing"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/auth-smoke.XXXXXX")" || fatal "mktemp-failed"
PROJECT="authsmoke$(python3 -c 'import secrets;print(secrets.token_hex(3))')"
PDIR="$WORK/project"
# shellcheck disable=SC2329 # invoked by the EXIT trap
cleanup() {
  local ids
  ids="$(docker ps -aq --filter "label=com.docker.compose.project=$PROJECT" 2>/dev/null)"
  ids="$ids $(docker ps -aq --filter "name=^${PROJECT}_" 2>/dev/null)"
  # shellcheck disable=SC2086
  if [ -n "${ids// /}" ]; then docker rm -f -v $ids >/dev/null 2>&1; fi
  ids="$(docker volume ls -q --filter "label=com.docker.compose.project=$PROJECT" 2>/dev/null)"
  # shellcheck disable=SC2086
  if [ -n "$ids" ]; then docker volume rm -f $ids >/dev/null 2>&1; fi
  ids="$(docker network ls -q --filter "label=com.docker.compose.project=$PROJECT" 2>/dev/null)"
  # shellcheck disable=SC2086
  if [ -n "$ids" ]; then docker network rm $ids >/dev/null 2>&1; fi
  rm -rf "$WORK"
}
trap cleanup EXIT
mkdir -p "$PDIR" "$WORK/home"
# Files that can hold tokens are created 0600 here and only ever truncated afterwards.
# (nself itself runs under the default umask: its build plan assumes 0644 files.)
(umask 077; : > "$WORK/resp"; : > "$WORK/out"; : > "$WORK/err"; : > "$WORK/curl.err"; : > "$WORK/b.totp"; : > "$WORK/body")

# ---- the binary and the image refs ---------------------------------------------------
BIN="${1:-}"
if [ -z "$BIN" ]; then
  mkdir -p "$WORK/bin"
  BIN="$WORK/bin/nself"
  if CGO_ENABLED=0 go build -mod=vendor -buildvcs=false -o "$BIN" ./cmd/nself 2>"$WORK/build.err"; then
    pass "build the nself binary"
  else
    redact < "$WORK/build.err" >&2
    fatal "build the nself binary"
  fi
fi
case "$BIN" in /*) ;; *) BIN="$ROOT/$BIN" ;; esac
[ -x "$BIN" ] || fatal "binary $BIN is not executable"

if CGO_ENABLED=0 go run -mod=vendor ./tools/imagelist > "$WORK/images.tsv" 2>"$WORK/imagelist.err"; then
  REF_V14="$(awk -F'\t' '$1=="auth-v14"{print $2}' "$WORK/images.tsv")"
  REF_NEW="$(awk -F'\t' '$1=="auth"{print $2}' "$WORK/images.tsv")"
else
  redact < "$WORK/imagelist.err" >&2
  fatal "read the image refs with tools/imagelist"
fi
if [ -n "$REF_V14" ] && [ -n "$REF_NEW" ] && [ "$REF_V14" != "$REF_NEW" ]; then
  pass "image lock has distinct auth-v14 and auth refs"
else
  fatal "image lock has distinct auth-v14 and auth refs"
fi
info "host=$(uname -sm) engine=$(docker version --format '{{.Server.Os}}/{{.Server.Arch}}' 2>/dev/null) binary=$(uname -sm)"
info "auth-v14=${REF_V14%%@*} auth=${REF_NEW%%@*}"

# ---- helpers -------------------------------------------------------------------------
AUTH_URL="http://127.0.0.1:4000"
HASURA_URL="http://127.0.0.1:8080"
RC=0
# nsf <args...>: run nself in the project; stdout $WORK/out, stderr $WORK/err, status $RC.
nsf() {
  local dh=""
  RC=0
  if [ -n "${DOCKER_HOST:-}" ]; then dh="DOCKER_HOST=$DOCKER_HOST"; fi
  # The caller's environment never steers nself at another stack: env -i plus a fixed
  # allow-list (a db reset aimed at someone's own project would be destructive).
  # shellcheck disable=SC2086
  (umask 022; cd "$PDIR" && env -i PATH="$PATH" HOME="$WORK/home" TMPDIR="${TMPDIR:-/tmp}" \
    DOCKER_CONFIG="${DOCKER_CONFIG:-$REAL_HOME/.docker}" $dh NSELF_V15=1 NSELF_CMD_LOG_ENABLED=false \
    AI_AUTO_INSTALL=false LANG="${LANG:-C}" "$BIN" "$@" > "$WORK/out" 2> "$WORK/err" < /dev/null) || RC=$?
}
nsf_tail() { { tail -n 6 "$WORK/err"; tail -n 4 "$WORK/out"; } | redact | tr '\n' '|'; }

HTTP=000
# jbody <printf format> <args...>: write the next POST body to a 0600 file (not a pipe:
# a pipeline would run call in a subshell and lose HTTP).
# shellcheck disable=SC2059 # the caller supplies the format
jbody() { printf "$@" > "$WORK/body"; }
# call <GET|POST> <url> [auth-header-file] [extra-header]: the POST body is $WORK/body;
# sets HTTP and writes the response to $WORK/resp.
call() {
  local m="$1" u="$2" a="${3:-}" x="${4:-}"
  : > "$WORK/resp"
  set -- -sS -m 30 -o "$WORK/resp" -w '%{http_code}' -X "$m"
  if [ -n "$a" ]; then set -- "$@" -H "@$a"; fi
  if [ -n "$x" ]; then set -- "$@" -H "$x"; fi
  if [ "$m" = GET ]; then
    HTTP="$(curl "$@" "$u" 2>"$WORK/curl.err" < /dev/null)" || HTTP=000
  else
    HTTP="$(curl "$@" -H 'Content-Type: application/json' --data-binary "@$WORK/body" "$u" 2>"$WORK/curl.err")" || HTTP=000
  fi
}
# jv <python expr over d>: evaluate against the last response; empty on any error.
jv() {
  python3 -c 'import json,sys
try:
    d = json.load(open(sys.argv[1]))
    v = eval(sys.argv[2])
    print("" if v is None else v)
except Exception:
    print("")' "$WORK/resp" "$1"
}
# why: a short, token-free description of the last response.
why() {
  local m
  m="$(jv 'd.get("error") or d.get("message") or d.get("errors")[0]["message"]' | cut -c1-120)"
  printf 'http=%s %s' "$HTTP" "$m"
}
hdr() { (umask 077; printf 'Authorization: Bearer %s\n' "$2" > "$WORK/auth.$1"); }

PW="Aa1-$(python3 -c 'import secrets;print(secrets.token_hex(10))')"
EMAIL_A="a@auth-smoke.test"
EMAIL_B="b@auth-smoke.test"
EMAIL_C="c@auth-smoke.test"
TOK=""
RT=""
# signup / signin <email>: set TOK (access) and RT (refresh) on success.
sign_in_out() {
  TOK="$(jv 'd["session"]["accessToken"]')"
  RT="$(jv 'd["session"]["refreshToken"]')"
}
signup() { jbody '{"email":"%s","password":"%s"}' "$1" "$PW"; call POST "$AUTH_URL/signup/email-password"; sign_in_out; }
signin() { jbody '{"email":"%s","password":"%s"}' "$1" "$PW"; call POST "$AUTH_URL/signin/email-password"; sign_in_out; }
have_session() { [ "$HTTP" = 200 ] && [ -n "$TOK" ] && [ -n "$RT" ]; }

# totp <secret-file> [step-offset]: RFC 6238 code (SHA1, 30 s, 6 digits).
totp() {
  python3 -c 'import sys,hmac,hashlib,struct,time,base64
s = open(sys.argv[1]).read().strip().upper()
s += "=" * (-len(s) % 8)
c = int(time.time()) // 30 + int(sys.argv[2])
h = hmac.new(base64.b32decode(s), struct.pack(">Q", c), hashlib.sha1).digest()
o = h[-1] & 15
print("%06d" % ((struct.unpack(">I", h[o:o + 4])[0] & 0x7FFFFFFF) % 1000000))' "$1" "${2:-0}"
}
next_window() { python3 -c 'import time;time.sleep(30 - time.time() % 30 + 1)'; }

# totp_signin <email> <secret-file>: password sign-in must answer with an MFA ticket,
# then /signin/mfa/totp must issue a session. A second attempt in the next 30 s
# window rules out a rejected replay of the code used just before. Sets MFA_WHY.
MFA_WHY=""
MFA_STAGE=""
totp_signin() {
  local email="$1" sf="$2" ticket code try
  try=0
  while [ "$try" -lt 2 ]; do
    signin "$email"
    ticket="$(jv 'd["mfa"]["ticket"]')"
    if [ "$HTTP" != 200 ] || [ -z "$ticket" ]; then
      MFA_STAGE=password
      MFA_WHY="password sign-in gave no MFA ticket ($(why))"
      return 1
    fi
    code="$(totp "$sf")"
    jbody '{"ticket":"%s","otp":"%s"}' "$ticket" "$code"; call POST "$AUTH_URL/signin/mfa/totp"
    sign_in_out
    if have_session; then MFA_WHY="ok"; return 0; fi
    MFA_STAGE=totp
    MFA_WHY="TOTP step refused ($(why))"
    try=$((try + 1))
    if [ "$try" -lt 2 ]; then next_window; fi
  done
  return 1
}

# hasura <auth-header-file>: a GraphQL request as role user; sets HASURA_OK=1 when
# Hasura answers 200 with data and without errors.
HASURA_OK=0
hasura() {
  jbody '{"query":"{ __typename }"}'; call POST "$HASURA_URL/v1/graphql" "$1" 'X-Hasura-Role: user'
  HASURA_OK=0
  if [ "$HTTP" = 200 ] && [ "$(jv '"errors" not in d and d["data"]["__typename"] == "query_root"')" = True ]; then HASURA_OK=1; fi
}

# dbq <sql>: run one statement in the project's postgres (SQL over stdin, never argv).
dbq() {
  printf '%s\n' "$1" | docker exec -i "${PROJECT}_postgres" psql -U postgres -d "${2:-$PROJECT}" -At -v ON_ERROR_STOP=1 -f - 2>/dev/null
}
db_secret_is_plain() {
  # prints t when auth.users.totp_secret for B equals the plaintext in $WORK/b.totp
  dbq "$(printf "select totp_secret = '%s' from auth.users where email = '%s';" "$(cat "$WORK/b.totp")" "$EMAIL_B")"
}

# compose_auth_image / lock_auth_pin / running_auth_image
compose_auth_image() {
  awk '/^    auth:$/{f=1;next} f && /^        image:/{print $2;exit} /^    [a-z]/{f=0}' "$PDIR/docker-compose.yml"
}
lock_auth_pin() { sed -n 's/^  auth: //p' "$PDIR/nself.lock.yaml"; }
running_auth_image() { docker inspect -f '{{.Config.Image}}' "${PROJECT}_auth" 2>/dev/null; }
# set_pin <ref>: stand-in for `nself update` (P7-LIVE-10, not merged): rewrite the auth
# pin line of nself.lock.yaml.
set_pin() {
  sed "s|^  auth: .*|  auth: $1|" "$PDIR/nself.lock.yaml" > "$PDIR/nself.lock.yaml.new" \
    && mv "$PDIR/nself.lock.yaml.new" "$PDIR/nself.lock.yaml"
}
# wait_stack: wait up to 240 s for auth and Hasura to answer /healthz.
wait_stack() {
  local i=0
  while [ "$i" -lt 80 ]; do
    if [ "$(curl -s -m 5 -o /dev/null -w '%{http_code}' "$AUTH_URL/healthz" 2>/dev/null)" = 200 ] \
      && [ "$(curl -s -m 5 -o /dev/null -w '%{http_code}' "$HASURA_URL/healthz" 2>/dev/null)" = 200 ]; then
      return 0
    fi
    i=$((i + 1))
    sleep 3
  done
  return 1
}
# build_start <label>: nself build then nself start, then wait for health.
build_start() {
  nsf build
  if [ "$RC" -ne 0 ]; then fail "$1: nself build exits 0" "rc=$RC $(nsf_tail)"; return 1; fi
  pass "$1: nself build exits 0"
  nsf start
  if [ "$RC" -ne 0 ]; then fail "$1: nself start exits 0" "rc=$RC $(nsf_tail)"; return 1; fi
  pass "$1: nself start exits 0"
  if wait_stack; then pass "$1: auth and Hasura healthy"; else fail "$1: auth and Hasura healthy" "$(docker ps -a --format '{{.Names}} {{.Status}}' | grep "^$PROJECT" | tr '\n' '|')"; return 1; fi
  return 0
}

# ---- step 1: new project on auth-v14 -----------------------------------------------
step "1 new project, auth-v14"
nsf init --non-interactive --name "$PROJECT" --domain local.nself.org --no-pgvector
[ "$RC" -eq 0 ] || fatal "nself init --non-interactive exits 0" "rc=$RC $(nsf_tail)"
pass "nself init --non-interactive exits 0"
if [ "$(grep -c "^PROJECT_NAME=$PROJECT\$" "$PDIR/.env")" -eq 1 ]; then
  pass "project .env names PROJECT_NAME=$PROJECT"
else
  fatal "project .env names PROJECT_NAME=$PROJECT" "refusing to continue: the project is not the one this run created"
fi
printf 'AUTH_MFA_ENABLED=true\n' >> "$PDIR/.env"
check "project .env has no AUTH_ENCRYPTION_KEY" test "$(grep -c '^AUTH_ENCRYPTION_KEY' "$PDIR/.env")" -eq 0
nsf build
[ "$RC" -eq 0 ] || fatal "nself build exits 0" "rc=$RC $(nsf_tail)"
pass "nself build exits 0"
if [ "$(compose_auth_image)" = "$REF_V14" ]; then pass "compose auth image is the auth-v14 ref"; else fail "compose auth image is the auth-v14 ref" "got $(compose_auth_image)"; fi
if [ "$(lock_auth_pin)" = "$REF_V14" ]; then pass "nself.lock.yaml pins auth to the auth-v14 ref"; else fail "nself.lock.yaml pins auth to the auth-v14 ref" "got $(lock_auth_pin)"; fi
check "compose forwards AUTH_MFA_ENABLED=true" grep -q 'AUTH_MFA_ENABLED: "true"' "$PDIR/docker-compose.yml"
check "compose carries no AUTH_ENCRYPTION_KEY" test "$(grep -c AUTH_ENCRYPTION_KEY "$PDIR/docker-compose.yml")" -eq 0
nsf start
[ "$RC" -eq 0 ] || fatal "nself start exits 0" "rc=$RC $(nsf_tail)"
pass "nself start exits 0"
wait_stack || fatal "auth and Hasura healthy on auth-v14"
pass "auth and Hasura healthy on auth-v14"
if [ "$(running_auth_image)" = "$REF_V14" ]; then pass "running auth container uses the auth-v14 ref"; else fail "running auth container uses the auth-v14 ref" "got $(running_auth_image)"; fi

# ---- step 2: seed data on auth-v14 -----------------------------------------------------
step "2 seed users on auth-v14"
signup "$EMAIL_A"
if have_session; then pass "v14: user A signs up"; else fail "v14: user A signs up" "$(why)"; fi
signin "$EMAIL_A"
if have_session; then pass "v14: user A signs in with the password"; else fail "v14: user A signs in with the password" "$(why)"; fi
A_RT_OLD="$RT"
hdr a "$TOK"
hasura "$WORK/auth.a"
if [ "$HASURA_OK" = 1 ]; then pass "v14: Hasura accepts A's JWT (role user)"; else fail "v14: Hasura accepts A's JWT (role user)" "$(why)"; fi
hdr bad "invalid.token.value"
hasura "$WORK/auth.bad"
if [ "$HASURA_OK" = 0 ] && [ "$(jv 'd["errors"][0]["extensions"]["code"]')" = invalid-jwt ]; then
  pass "v14: Hasura rejects a bad JWT (control)"
else
  fail "v14: Hasura rejects a bad JWT (control)" "$(why)"
fi

signup "$EMAIL_B"
if have_session; then pass "v14: user B signs up"; else fail "v14: user B signs up" "$(why)"; fi
hdr b "$TOK"
call GET "$AUTH_URL/mfa/totp/generate" "$WORK/auth.b"
jv 'd["totpSecret"]' > "$WORK/b.totp"
if [ "$HTTP" = 200 ] && [ -s "$WORK/b.totp" ]; then pass "v14: B gets a TOTP secret"; else fail "v14: B gets a TOTP secret" "$(why)"; fi
jbody '{"code":"%s","activeMfaType":"totp"}' "$(totp "$WORK/b.totp")"; call POST "$AUTH_URL/user/mfa" "$WORK/auth.b"
if [ "$HTTP" = 200 ]; then pass "v14: B enables TOTP MFA"; else fail "v14: B enables TOTP MFA" "$(why)"; fi
if [ "$(db_secret_is_plain)" = t ]; then pass "v14: B's TOTP secret is stored as plaintext (baseline)"; else fail "v14: B's TOTP secret is stored as plaintext (baseline)" "psql answered '$(db_secret_is_plain)'"; fi
if totp_signin "$EMAIL_B" "$WORK/b.totp"; then pass "v14: B signs in with password + TOTP"; else fail "v14: B signs in with password + TOTP" "$MFA_WHY"; fi

# ---- step 3: verified backup -----------------------------------------------------------
step "3 verified backup"
nsf backup create
if [ "$RC" -eq 0 ]; then pass "nself backup create exits 0"; else fail "nself backup create exits 0" "rc=$RC $(nsf_tail)"; fi
nsf backup list --format json
BID="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["data"][0]["id"])' "$WORK/out" 2>/dev/null)"
if [ -n "$BID" ] && [ -f "$PDIR/backups/$BID.dump" ]; then
  pass "backup recorded: id $BID, path backups/$BID.dump"
else
  fatal "backup recorded (id and file)" "id='$BID' $(find "$PDIR/backups" -type f 2>&1 | head -3 | tr '\n' ' ')"
fi
nsf backup verify "$BID"
if [ "$RC" -eq 0 ]; then pass "nself backup verify exits 0"; else fail "nself backup verify exits 0" "rc=$RC $(nsf_tail)"; fi
nsf backup verify "$BID" --restore-test
if [ "$RC" -eq 0 ] && grep -q 'restore-test' "$WORK/out"; then pass "nself backup verify --restore-test exits 0"; else fail "nself backup verify --restore-test exits 0" "rc=$RC $(nsf_tail)"; fi

# ---- step 4: switch to the auth ref -------------------------------------------------
step "4 switch to the auth ref"
AUTH_KEY="$(python3 -c 'import secrets;print(secrets.token_hex(32))')"
printf 'AUTH_ENCRYPTION_KEY=%s\n' "$AUTH_KEY" >> "$PDIR/.env"
unset AUTH_KEY
set_pin "$REF_NEW"
# nself update is not merged (P7-LIVE-10): the pin edit above is its stand-in.
nsf build
if [ "$RC" -eq 0 ]; then pass "switch: nself build exits 0"; else fail "switch: nself build exits 0" "rc=$RC $(nsf_tail)"; fi
if [ "$(compose_auth_image)" = "$REF_NEW" ]; then pass "switch: compose auth image is the auth ref"; else fail "switch: compose auth image is the auth ref" "got $(compose_auth_image)"; fi
if [ "$(lock_auth_pin)" = "$REF_NEW" ]; then pass "switch: nself.lock.yaml pins the auth ref"; else fail "switch: nself.lock.yaml pins the auth ref" "got $(lock_auth_pin)"; fi
nsf start
if [ "$RC" -eq 0 ]; then pass "switch: nself start exits 0"; else fail "switch: nself start exits 0" "rc=$RC $(nsf_tail)"; fi
if wait_stack; then pass "switch: auth and Hasura healthy"; else fail "switch: auth and Hasura healthy" "$(docker logs --tail 5 "${PROJECT}_auth" 2>&1 | redact | tr '\n' '|')"; fi
if [ "$(running_auth_image)" = "$REF_NEW" ]; then pass "switch: running auth container uses the auth ref"; else fail "switch: running auth container uses the auth ref" "got $(running_auth_image)"; fi

# ---- step 5: after the switch ----------------------------------------------------------
step "5 after the switch"
signin "$EMAIL_A"
if have_session; then pass "auth: user A signs in with the password"; else fail "auth: user A signs in with the password" "$(why)"; fi
hdr a2 "$TOK"
hasura "$WORK/auth.a2"
if [ "$HASURA_OK" = 1 ]; then pass "auth: Hasura accepts the new JWT"; else fail "auth: Hasura accepts the new JWT" "$(why)"; fi

# A's pre-upgrade refresh token: observed behaviour is asserted. nhost/auth >= 0.38.0
# rotates refresh tokens on /token: the old token is accepted once and replaced.
jbody '{"refreshToken":"%s"}' "$A_RT_OLD"; call POST "$AUTH_URL/token"
RT_NEW="$(jv 'd["refreshToken"]')"
if [ "$HTTP" = 200 ] && [ -n "$RT_NEW" ] && [ "$RT_NEW" != "$A_RT_OLD" ]; then
  pass "auth: pre-upgrade refresh token on /token is accepted once and a NEW refresh token is returned"
  jbody '{"refreshToken":"%s"}' "$A_RT_OLD"; call POST "$AUTH_URL/token"
  if [ "$HTTP" = 401 ] && [ "$(jv 'd.get("error")')" = invalid-refresh-token ]; then
    pass "auth: the old refresh token is rejected after rotation (http=401 invalid-refresh-token)"
  else
    fail "auth: the old refresh token is rejected after rotation" "expected 401 invalid-refresh-token, observed $(why)"
  fi
  jbody '{"refreshToken":"%s"}' "$RT_NEW"; call POST "$AUTH_URL/token"
  if [ "$HTTP" = 200 ]; then pass "auth: the rotated refresh token works"; else fail "auth: the rotated refresh token works" "$(why)"; fi
else
  fail "auth: pre-upgrade refresh token on /token is accepted once and a NEW refresh token is returned" "observed: $(why), new token returned: $([ -n "$RT_NEW" ] && echo yes || echo no), same as old: $([ "$RT_NEW" = "$A_RT_OLD" ] && echo yes || echo no)"
fi

if [ "$(db_secret_is_plain)" = f ]; then pass "auth: B's TOTP secret is encrypted at rest after the first start"; else fail "auth: B's TOTP secret is encrypted at rest after the first start" "psql answered '$(db_secret_is_plain)'"; fi
if totp_signin "$EMAIL_B" "$WORK/b.totp"; then pass "auth: B's TOTP sign-in still works (secret migration)"; else fail "auth: B's TOTP sign-in still works (secret migration)" "$MFA_WHY"; fi
signup "$EMAIL_C"
if have_session; then pass "auth: new user C signs up"; else fail "auth: new user C signs up" "$(why)"; fi
signin "$EMAIL_C"
if have_session; then pass "auth: new user C signs in"; else fail "auth: new user C signs in" "$(why)"; fi

# ---- step 6: rollback, unsupported (pin back, no restore) -----------------------------
step "6 pin back to auth-v14 without restoring"
set_pin "$REF_V14"
# nself update is not merged (P7-LIVE-10): the pin edit above is its stand-in.
# PASS only when everything but the TOTP step is healthy and the TOTP step is refused
# with the observed answer (HTTP 401, error invalid-otp). Anything else is a FAIL.
nsf build
if [ "$RC" -ne 0 ]; then
  fail "pin-back-without-restore-is-unsupported" "nself build failed, rc=$RC $(nsf_tail)"
else
  nsf start
  if [ "$RC" -ne 0 ]; then
    fail "pin-back-without-restore-is-unsupported" "nself start failed, rc=$RC $(nsf_tail)"
  elif ! wait_stack; then
    fail "pin-back-without-restore-is-unsupported" "the stack is unhealthy on auth-v14"
  elif [ "$(running_auth_image)" != "$REF_V14" ]; then
    fail "pin-back-without-restore-is-unsupported" "running auth image is '$(running_auth_image)', not the auth-v14 ref"
  else
    signin "$EMAIL_A"
    if ! have_session; then
      fail "pin-back-without-restore-is-unsupported" "A's password sign-in fails on auth-v14 ($(why))"
    elif totp_signin "$EMAIL_B" "$WORK/b.totp"; then
      fail "pin-back-without-restore-is-unsupported" "no breakage observed: B's TOTP sign-in worked on auth-v14 after the migration"
    elif [ "$MFA_STAGE" = totp ] && [ "$HTTP" = 401 ] && [ "$(jv 'd.get("error")')" = invalid-otp ]; then
      pass "pin-back-without-restore-is-unsupported (A signs in; B's TOTP step is refused on auth-v14: http=401 invalid-otp)"
    else
      fail "pin-back-without-restore-is-unsupported" "expected the TOTP step to answer 401 invalid-otp; observed stage=$MFA_STAGE $MFA_WHY"
    fi
  fi
fi

# ---- step 7: rollback, supported (restore the pre-switch backup) ---------------------
step "7 restore the pre-switch backup, keep auth-v14"
if [ "$(lock_auth_pin)" = "$REF_V14" ]; then pass "restore: lock still pins the auth-v14 ref"; else fail "restore: lock still pins the auth-v14 ref" "got $(lock_auth_pin)"; fi
# Stop only auth and Hasura (postgres must stay up for the restore).
nsf stop auth hasura
if [ "$RC" -eq 0 ]; then pass "restore: nself stop auth hasura exits 0"; else fail "restore: nself stop auth hasura exits 0" "rc=$RC $(nsf_tail)"; fi
nsf backup restore "$BID" --only pg --yes
if [ "$RC" -eq 0 ]; then
  pass "restore: nself backup restore --only pg --yes exits 0 on the migrated database"
else
  # Observed limit: pg_restore --clean cannot drop auth.users while objects that the
  # newer auth release added still depend on it. The plain failure is asserted as
  # observed (E207 or the "cannot drop" reason), not hidden. Only then is the database
  # reset with the CLI (nself db reset --yes: drop, create, re-initialise schemas and
  # extensions; it runs no migrations or seeds) and the restore repeated.
  RESTORE_E="$(grep -o 'E207' "$WORK/err" | head -1)"
  RESTORE_WHY="$(grep -o 'cannot drop [a-z ]*[a-z._]* because other objects depend on it' "$WORK/err" | head -1)"
  if [ -z "$RESTORE_E" ] && [ -z "$RESTORE_WHY" ]; then
    fatal "plain restore failed for the expected reason (E207 / cannot drop)" "rc=$RC observed: $(nsf_tail)"
  fi
  # The reset target must be this run's own postgres.
  if [ "$(docker inspect -f '{{index .Config.Labels "com.docker.compose.project"}}' "${PROJECT}_postgres" 2>/dev/null)" != "$PROJECT" ]; then
    fatal "db reset target ${PROJECT}_postgres carries this run's compose project label" "refusing to run nself db reset"
  fi
  pass "db reset target ${PROJECT}_postgres carries this run's compose project label"
  nsf db reset --yes
  if [ "$RC" -eq 0 ]; then pass "restore: nself db reset --yes exits 0"; else fail "restore: nself db reset --yes exits 0" "rc=$RC $(nsf_tail)"; fi
  nsf backup restore "$BID" --only pg --yes
  if [ "$RC" -eq 0 ]; then
    pass "restore-over-migrated-db-needs-a-db-reset (plain restore failed ${RESTORE_E:-without E207}: ${RESTORE_WHY:-no dependency detail}; after nself db reset --yes the restore exits 0)"
  else
    fail "restore after nself db reset --yes exits 0" "rc=$RC $(nsf_tail)"
  fi
fi
build_start "restore" || true
if [ "$(compose_auth_image)" = "$REF_V14" ] && [ "$(running_auth_image)" = "$REF_V14" ]; then
  pass "restore: compose and container run the auth-v14 ref"
else
  fail "restore: compose and container run the auth-v14 ref" "compose=$(compose_auth_image) running=$(running_auth_image)"
fi
signin "$EMAIL_A"
if have_session; then pass "restore: user A signs in"; else fail "restore: user A signs in" "$(why)"; fi
hdr a3 "$TOK"
hasura "$WORK/auth.a3"
if [ "$HASURA_OK" = 1 ]; then pass "restore: Hasura accepts A's JWT"; else fail "restore: Hasura accepts A's JWT" "$(why)"; fi
if [ "$(db_secret_is_plain)" = t ]; then pass "restore: B's TOTP secret is plaintext again"; else fail "restore: B's TOTP secret is plaintext again" "psql answered '$(db_secret_is_plain)'"; fi
if totp_signin "$EMAIL_B" "$WORK/b.totp"; then pass "restore: B's TOTP sign-in works again"; else fail "restore: B's TOTP sign-in works again" "$MFA_WHY"; fi
signin "$EMAIL_C"
if [ "$HTTP" != 200 ] && [ -z "$TOK" ]; then pass "restore: user C cannot sign in ($(why))"; else fail "restore: user C cannot sign in" "http=$HTTP"; fi
if [ "$(dbq "select count(*) from auth.users where email = '$EMAIL_C';")" = 0 ]; then pass "restore: user C does not exist in auth.users"; else fail "restore: user C does not exist in auth.users"; fi

finish
