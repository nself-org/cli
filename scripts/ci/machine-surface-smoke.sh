#!/usr/bin/env bash
# Purpose: prove the P7-SURF machine surfaces on the real binary (Epic P7-SURF
#          acceptance 1-9, ADR 0017/0021/0022): the help registry, the HTTP API
#          (`nself admin serve`), the MCP server (`nself mcp --transport http`),
#          the OpenAPI/SDK/tool-list goldens, bearer-token and bind policy,
#          `config explain` and `config validate`, the confirmation round trip
#          and the plugin envelopes. The Go tests prove the pieces in-process;
#          this runs the built `nself` and the built plugin binaries end to end.
# Usage:   NSELF_PLUGINS_REF=<full sha of plugins origin/main> \
#            [NSELF_PLUGINS_DIR=<local plugins checkout>] [NSELF_SMOKE_TS=1] \
#            bash scripts/ci/machine-surface-smoke.sh
# Inputs:  NSELF_PLUGINS_REF (required: the plugins commit the plugin leg
#          builds; unset is a FAIL with exit 1). NSELF_PLUGINS_DIR (optional): a
#          local plugins checkout to clone from instead of GitHub (read-only is
#          fine). NSELF_SMOKE_TS=1 (optional): also run the @nself/cli-api
#          contract test against the live server (needs node and pnpm).
#          go, jq, curl and git on PATH (a missing tool is a FAIL, never a skip).
# Outputs: one line per assertion on stdout, "PASS <name>" or "FAIL <name>",
#          nothing else on stdout. Diagnostics for a FAIL go to stderr. Exit 0
#          only when every assertion passed and at least one ran.
# Constraints: bash 3.2 compatible. Everything runs under a temp HOME, a
#          fixture project, a stub `docker`/`brew` and loopback ports; the
#          caller's NSELF_* variables are dropped. Confirmation expiry (10
#          minutes) is covered by the internal/invoke tests, not waited for here.
set -uo pipefail

cd "$(dirname "$0")/../.." || exit 1
ROOT="$(pwd)"
PASSED=0
FAILED=0

pass() { printf 'PASS %s\n' "$1"; PASSED=$((PASSED + 1)); }
fail() {
  printf 'FAIL %s\n' "$1"
  FAILED=$((FAILED + 1))
  if [ -n "${2:-}" ]; then printf '  %s\n' "$2" >&2; fi
}
# check <name> <command...>: PASS when the command exits 0.
check() {
  local name="$1"
  shift
  if "$@" >/dev/null 2>&1; then pass "$name"; else fail "$name" "$(printf 'check failed: %.200s' "$*")"; fi
}

# The plugin leg needs a recorded ref; refuse before doing any work without it.
PLUGINS_REF="${NSELF_PLUGINS_REF:-}"
PLUGINS_DIR="${NSELF_PLUGINS_DIR:-}"
SMOKE_TS="${NSELF_SMOKE_TS:-}"
if [ -z "$PLUGINS_REF" ]; then
  printf 'FAIL plugins-ref-unset\n'
  echo 'NSELF_PLUGINS_REF is required: the full sha of plugins origin/main the plugin leg builds' >&2
  exit 1
fi
for tool in go jq curl git; do
  command -v "$tool" >/dev/null 2>&1 || { printf 'FAIL tool-%s-missing\n' "$tool"; exit 1; }
done

# The caller's environment must not steer the binary or the plugins.
for v in $(env | sed -n 's/^\(NSELF_[A-Za-z0-9_]*\)=.*/\1/p'); do unset "$v"; done
unset HETZNER_NSELF_TOKEN HCLOUD_TOKEN SENTRY_API_KEY 2>/dev/null || true
# git inside a bind-mounted checkout (another owner) must still work; scoped to this script.
export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=safe.directory GIT_CONFIG_VALUE_0='*'

TMP="$(mktemp -d "${TMPDIR:-/tmp}/machine-surface-smoke.XXXXXX")"
PIDS=""
cleanup() {
  local p
  for p in $PIDS; do kill "$p" 2>/dev/null || true; done
  wait 2>/dev/null || true
  rm -rf "$TMP"
}
trap cleanup EXIT

BIN="$TMP/nself"
JV="$TMP/jsonvalidate"
PROJ="$TMP/proj"
STUB="$TMP/stub"
mkdir -p "$PROJ" "$STUB"
printf '#!/bin/sh\nexit 1\n' > "$STUB/docker"
cp "$STUB/docker" "$STUB/brew"
chmod +x "$STUB/docker" "$STUB/brew"

# -buildvcs=false: a bind-mounted worktree (.git file pointing outside the container) must still build.
if ! CGO_ENABLED=0 go build -mod=vendor -buildvcs=false -o "$BIN" ./cmd/nself 2>"$TMP/build.err" ||
   ! CGO_ENABLED=0 go build -mod=vendor -buildvcs=false -o "$JV" ./tools/jsonvalidate 2>>"$TMP/build.err"; then
  cat "$TMP/build.err" >&2
  printf 'FAIL build\n'
  exit 1
fi
pass build

# ---- environment helpers ----
H=""      # HOME of the current server or command
V15=""    # "1" selects v1.5 mode
# ns <args...>: run the binary in $PWD with the isolated environment.
ns() {
  env HOME="$H" NSELF_PLUGIN_DIR="$H/plugins" NSELF_PLUGIN_CACHE="$H/cache" PATH="$STUB:$PATH" \
    NSELF_NONINTERACTIVE=1 CI=1 NO_COLOR=1 AI_AUTO_INSTALL=false ${V15:+NSELF_V15=1} "$BIN" "$@"
}
new_home() { H="$TMP/home-$1"; mkdir -p "$H/plugins"; }
# reset_proj [env]: the fixture project, rebuilt so every surface sees the same one.
reset_proj() {
  rm -rf "${PROJ:?}"/* "${PROJ:?}"/.[!.]* 2>/dev/null
  printf 'PROJECT_NAME=fixture\nENV=%s\nMINIO_ENABLED=false\nBASE_DOMAIN=fake.example\n' "${1:-dev}" > "$PROJ/.env"
  chmod 600 "$PROJ/.env"
}
snapshot() { printf '%s | %s' "$(ls -A "$PROJ" | sort | tr '\n' ',')" "$(tr '\n' ';' < "$PROJ/.env" 2>/dev/null)"; }
install_fx_plugin() {
  mkdir -p "$H/plugins/fx" "$H/plugins/bin"
  cat > "$H/plugins/fx/plugin.json" <<'EOF'
{"binaryName": "nself-fx", "category": "infrastructure",
 "commands": {"command": "fx", "binary": "nself-fx", "summary": "fixture commands", "side_effect": "read", "json": "envelope",
   "subcommands": [{"name": "status", "summary": "Read the fixture status", "side_effect": "read", "json": "envelope"}]},
 "description": "fixture", "license": "free", "licenseType": "free", "manifest_version": 2, "maturity": "implemented",
 "minNselfVersion": "1.5.0", "name": "fx", "pluginType": "cli", "requires": {"nself": ">=1.5.0"},
 "service": {"kind": "cli"}, "status": "stable", "tier": "free", "version": "1.0.0"}
EOF
  printf '#!/bin/sh\nprintf '"'"'{"schema_version":"1","command":"fx status","data":{"plugin":"fx","argv":"%%s"}}\\n'"'"' "$*"\n' > "$H/plugins/bin/nself-fx"
  chmod +x "$H/plugins/bin/nself-fx"
}

# free_port: a loopback port nothing accepts on right now.
free_port() {
  local p
  while :; do
    p=$((20000 + RANDOM % 30000))
    (exec 3<>"/dev/tcp/127.0.0.1/$p") 2>/dev/null || { echo "$p"; return; }
  done
}
# start_srv <name> <nself args...>: start a long-running command in $PROJ with the current
# $H/$V15, appending --port. Sets SRV_PID and SRV_PORT; logs in $TMP/<name>.{out,err}.
start_srv() {
  local name="$1"
  shift
  for _ in 1 2 3; do
    SRV_PORT="$(free_port)"
    (cd "$PROJ" && exec env HOME="$H" NSELF_PLUGIN_DIR="$H/plugins" NSELF_PLUGIN_CACHE="$H/cache" PATH="$STUB:$PATH" \
      NSELF_NONINTERACTIVE=1 CI=1 NO_COLOR=1 AI_AUTO_INSTALL=false ${V15:+NSELF_V15=1} \
      "$BIN" "$@" --port "$SRV_PORT") >"$TMP/$name.out" 2>"$TMP/$name.err" &
    SRV_PID=$!
    for _ in $(seq 1 100); do
      if grep -q 'listening' "$TMP/$name.out" "$TMP/$name.err" 2>/dev/null; then PIDS="$PIDS $SRV_PID"; return 0; fi
      kill -0 "$SRV_PID" 2>/dev/null || break
      sleep 0.1
    done
    kill "$SRV_PID" 2>/dev/null || true
  done
  return 1
}
# refuses <name> <expected text in stderr> <env KEY=VAL...> -- <args...>: the command must
# exit non-zero by itself within 6 s and say the text; a server that starts is killed and fails.
refuses() {
  local name="$1" text="$2" rc=0 pid
  shift 2
  local extra=()
  while [ "$1" != "--" ]; do extra+=("$1"); shift; done
  shift
  (cd "$PROJ" && exec env HOME="$H" PATH="$STUB:$PATH" NSELF_NONINTERACTIVE=1 CI=1 NO_COLOR=1 ${V15:+NSELF_V15=1} \
    ${extra[@]+"${extra[@]}"} "$BIN" "$@") >"$TMP/ref.out" 2>"$TMP/ref.err" &
  pid=$!
  for _ in $(seq 1 60); do kill -0 "$pid" 2>/dev/null || break; sleep 0.1; done
  if kill -0 "$pid" 2>/dev/null; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null
    fail "$name" "the command started instead of refusing"
    return
  fi
  wait "$pid" 2>/dev/null || rc=$?
  if [ "$rc" -ne 0 ] && grep -qF "$text" "$TMP/ref.err"; then pass "$name"; else fail "$name" "rc=$rc: $(head -c 300 "$TMP/ref.err")"; fi
}

# ---- HTTP and MCP clients (BASE/TOKEN and MCP_URL/MCP_SID belong to the current server) ----
BASE=""; TOKEN=""; MCP_URL=""; MCP_SID=""; HTTP_CODE=""
read_token() { TOKEN="$(cat "$H/.nself/mcp-token" 2>/dev/null)"; }
# http_post <route> <json body> [curl args...]: body in $TMP/h.body, headers in $TMP/h.hdr.
http_post() {
  local route="$1" body="$2"
  shift 2
  HTTP_CODE="$(curl -s -o "$TMP/h.body" -D "$TMP/h.hdr" -w '%{http_code}' -X POST \
    -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' "$@" -d "$body" "$BASE$route")"
}
hdr() { tr -d '\r' < "$TMP/h.hdr" | awk -F': ' -v k="$1" 'tolower($1)==k{print $2}'; }
mcp_rpc() { # <json> [sid]: raw response body in $TMP/m.body (SSE data lines unwrapped)
  curl -s -D "$TMP/m.hdr" -o "$TMP/m.raw" -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
    -H 'Accept: application/json, text/event-stream' ${MCP_SID:+-H "Mcp-Session-Id: $MCP_SID"} -d "$1" "$MCP_URL"
  if grep -q '^data:' "$TMP/m.raw"; then sed -n 's/^data: *//p' "$TMP/m.raw" > "$TMP/m.body"; else cp "$TMP/m.raw" "$TMP/m.body"; fi
}
mcp_open() {
  MCP_SID=""
  mcp_rpc '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"smoke","version":"1"}}}'
  MCP_SID="$(tr -d '\r' < "$TMP/m.hdr" | awk -F': ' 'tolower($1)=="mcp-session-id"{print $2}')"
  mcp_rpc '{"jsonrpc":"2.0","method":"notifications/initialized"}'
}
# mcp_tool <tool> <args json>: the JSON-RPC answer in $TMP/m.body.
mcp_tool() {
  mcp_rpc "$(jq -nc --arg t "$1" --argjson a "$2" '{jsonrpc:"2.0",id:7,method:"tools/call",params:{name:$t,arguments:$a}}')"
}

# ---- JSON comparison (volatile members masked, nothing else) ----
# norm <file> [digits]: canonical form with timestamps, durations, nonces and plan ids masked.
norm() {
  jq -S --argjson digits "${2:-false}" '
    def mask: with_entries(if (.key | IN("timestamp", "duration", "confirm", "plan_id")) and ((.value | type) == "string" or (.value | type) == "number") and .value != "" then .value = "<v>" else . end);
    walk(if type == "object" then mask elif type == "string" and $digits then gsub("[0-9]+"; "#") else . end)' "$1"
}
same_json() { [ "$(norm "$1" "${3:-false}")" = "$(norm "$2" "${3:-false}")" ]; }
one_valid_envelope() { [ "$(jq -s length "$1")" = 1 ] && valid_envelope "$1"; }
valid_envelope() { (cd "$ROOT" && "$JV" -schema envelope.v1.schema.json "$1") >/dev/null 2>"$TMP/jv.err"; }

# ---- acceptance 1: help --json (v1.5), from a directory with no project ----
new_home help; V15=1
EMPTY="$TMP/empty"; mkdir -p "$EMPTY"
(cd "$EMPTY" && ns help --json) >"$TMP/help.out" 2>/dev/null
check "a1 help --json: every runnable document canon command declares json=envelope" \
  test "$(jq -c '[.data.commands[] | select(.runnable and .output=="document" and (.canon=="core" or .canon=="subcommand")) | .json] | unique' "$TMP/help.out")" = '["envelope"]'

# ---- servers: S1 has no plugins (goldens), S2 mounts the fixture plugin (samples) ----
V15=1
new_home s1; H1="$H"; reset_proj
start_srv s1-http admin serve && { S1_HTTP_PID=$SRV_PID; S1_HTTP="http://127.0.0.1:$SRV_PORT"; } || fail "s1 admin serve starts" "$(head -c 300 "$TMP/s1-http.err")"
start_srv s1-mcp mcp --transport http && { S1_MCP="http://127.0.0.1:$SRV_PORT/mcp"; } || fail "s1 mcp starts" "$(head -c 300 "$TMP/s1-mcp.err")"
new_home s2; H2="$H"; install_fx_plugin; reset_proj
start_srv s2-http admin serve && { S2_HTTP="http://127.0.0.1:$SRV_PORT"; } || fail "s2 admin serve starts" "$(head -c 300 "$TMP/s2-http.err")"
start_srv s2-mcp mcp --transport http && { S2_MCP="http://127.0.0.1:$SRV_PORT/mcp"; } || fail "s2 mcp starts" "$(head -c 300 "$TMP/s2-mcp.err")"
select_srv() { # 1|2
  if [ "$1" = 1 ]; then H="$H1"; BASE="$S1_HTTP"; MCP_URL="$S1_MCP"; else H="$H2"; BASE="$S2_HTTP"; MCP_URL="$S2_MCP"; fi
  read_token; V15=1
}

# ---- acceptance 3: OpenAPI and SDK ----
select_srv 1
CGO_ENABLED=0 go run -mod=vendor ./tools/openapigen -check >"$TMP/oa.out" 2>&1; check "a3 openapigen -check: .github/openapi.json regenerates with no diff" test $? -eq 0
CGO_ENABLED=0 go run -mod=vendor ./tools/sdkgen -lang ts -check >"$TMP/sdk.out" 2>&1; check "a3 sdkgen -lang ts -check: @nself/cli-api regenerates with no diff" test $? -eq 0
curl -s -H "Authorization: Bearer $TOKEN" "$BASE/openapi.json" > "$TMP/openapi.served"
check "a3 served /openapi.json equals .github/openapi.json byte for byte" cmp "$TMP/openapi.served" .github/openapi.json
check "a3 openapi.json is OpenAPI 3.1" test "$(jq -r .openapi .github/openapi.json | cut -c1-3)" = 3.1
CGO_ENABLED=0 go test -mod=vendor -count=1 ./internal/repoqa -run 'TestOpenAPIValidatesOAS31|TestOpenAPIIsCurrent' >"$TMP/oas.out" 2>&1
check "a3 openapi.json validates against the OAS 3.1 schema (repoqa)" test $? -eq 0
if [ -n "$SMOKE_TS" ]; then
  command -v pnpm >/dev/null 2>&1 || { printf 'FAIL tool-pnpm-missing\n'; exit 1; }
  (cd sdk/ts-cli-api && pnpm install --frozen-lockfile >"$TMP/ts-install.out" 2>&1 && NSELF_BIN="$BIN" pnpm test >"$TMP/ts-test.out" 2>&1)
  check "a3 @nself/cli-api contract test passes against a live nself admin serve" test $? -eq 0
  [ -f "$TMP/ts-test.out" ] && grep -qE 'Tests:.*passed' "$TMP/ts-test.out" && ! grep -qE 'Tests:.*skipped' "$TMP/ts-test.out" || fail "a3 @nself/cli-api contract test ran (not skipped)" "$(tail -5 "$TMP/ts-test.out" 2>/dev/null)"
fi

# ---- acceptance 4: MCP tool list, aliases, capabilities ----
select_srv 1; mcp_open
mcp_rpc '{"jsonrpc":"2.0","id":3,"method":"tools/list"}'
jq -S '{tools: .result.tools}' "$TMP/m.body" > "$TMP/tools.live"
jq -S . internal/mcpgen/testdata/tools.golden.json > "$TMP/tools.golden"
check "a4 live tools/list equals internal/mcpgen/testdata/tools.golden.json" cmp "$TMP/tools.live" "$TMP/tools.golden"
check "a4 no tool name appears twice" test "$(jq '[.tools[].name] | length' "$TMP/tools.live")" = "$(jq '[.tools[].name] | unique | length' "$TMP/tools.live")"
bad=""
for f in $(grep -rl 'mcp\.NewTool(' cmd internal --include='*.go' | grep -v '_test\.go$'); do
  case "$f" in internal/mcpgen/*|cmd/commands/mcp.go|cmd/commands/mcp_sentry.go) ;; *) bad="$bad $f" ;; esac
done
check "a4 mcp.NewTool( occurs only in internal/mcpgen and the v1.4 legacy registration" test -z "$bad"
CGO_ENABLED=0 go test -mod=vendor -count=1 ./internal/mcpgen/ >"$TMP/mcpgen.out" 2>&1
check "a4 mcpgen tests: golden, merged tools and legacy aliases" test $? -eq 0
reset_proj; V15=1; ok=0; n=0
while IFS='|' read -r q want; do
  n=$((n + 1)); (cd "$PROJ" && ns help capabilities "$q" --json) >"$TMP/cap.out" 2>/dev/null
  jq -e --arg w "$want" '[.data.results[:3][] | select(.kind=="command") | .path] | index($w) != null' "$TMP/cap.out" >/dev/null 2>&1 && ok=$((ok + 1))
done <<'EOF'
check service health|status
diagnose problems|doctor
rotate a secret|config secrets rotate
run database migrations|db migrate up
restore a backup|backup restore
deploy to production|deploy
explain a config key|config explain
EOF
check "a4 help capabilities finds the 7 recorded command queries in the top 3" test "$ok" -eq 7 -a "$n" -eq 7
CGO_ENABLED=0 go test -mod=vendor -count=1 ./cmd/commands -run 'TestHelpCapabilities$' >"$TMP/cap.go.out" 2>&1
check "a4 help capabilities answers all 8 recorded queries (incl. the k8s plugin one)" test $? -eq 0

# ---- acceptance 2: 10 sample commands, CLI = HTTP = MCP ----
select_srv 2; mcp_open
NSAMPLES=0
# sample <name> <route> <tool> <request json> <mode> <digits> <cli args...>
#   mode: plain | fail (error envelope expected) | confirm (nonce round trip) | plan (staging plan form)
sample() {
  local name="$1" route="$2" tool="$3" req="$4" mode="$5" digits="$6" rc cli_snap
  shift 6
  NSAMPLES=$((NSAMPLES + 1))
  local envname=dev; [ "$mode" = plan ] && envname=staging
  reset_proj "$envname"
  (cd "$PROJ" && ns "$@" --json) >"$TMP/cli.out" 2>/dev/null; rc=$?
  cli_snap="$(snapshot)"
  local has_data=false; jq -e 'has("data")' "$TMP/cli.out" >/dev/null 2>&1 && has_data=true
  if [ "$mode" = plan ]; then
    reset_proj "$envname"; http_post "$route" '{}'
    check "a2 [$name] HTTP answers 428 with the plan the CLI --plan prints" \
      test "$HTTP_CODE" = 428 -a "$(jq -r .data.kind "$TMP/h.body")" = plan
    jq '.data' "$TMP/cli.out" > "$TMP/cli.data"; jq '.data.plan' "$TMP/h.body" > "$TMP/h.plan"
    check "a2 [$name] HTTP confirmation.plan equals the CLI --plan data" same_json "$TMP/cli.data" "$TMP/h.plan"
    reset_proj "$envname"; mcp_tool "$tool" '{}'
    jq '.result.structuredContent.plan' "$TMP/m.body" > "$TMP/m.plan"
    check "a2 [$name] MCP confirmation.plan equals the CLI --plan data" same_json "$TMP/cli.data" "$TMP/m.plan"
    return
  fi
  local hreq="$req" mreq="$req"
  if [ "$mode" = confirm ]; then
    reset_proj; http_post "$route" "$req"
    check "a2 [$name] HTTP without confirm answers 428 and runs nothing" test "$HTTP_CODE" = 428 -a "$(snapshot)" = "$(reset_proj; snapshot)"
    hreq="$(jq -c --arg c "$(jq -r .data.confirm "$TMP/h.body")" '. + {confirm: $c}' <<<"$req")"
    reset_proj; mcp_tool "$tool" "$req"
    check "a2 [$name] MCP without confirm answers confirmation_required" \
      test "$(jq -r '.result.structuredContent.confirmation_required' "$TMP/m.body")" = true
    mreq="$(jq -c --arg c "$(jq -r '.result.structuredContent.confirm' "$TMP/m.body")" '. + {confirm: $c}' <<<"$req")"
  fi
  reset_proj; http_post "$route" "$hreq"
  if cmp -s "$TMP/h.body" "$TMP/cli.out"; then pass "a2 [$name] HTTP body equals CLI --json byte for byte"
  elif same_json "$TMP/h.body" "$TMP/cli.out" "$digits"; then pass "a2 [$name] HTTP body equals CLI --json (volatile members masked)"
  else fail "a2 [$name] HTTP body equals CLI --json" "$(diff <(norm "$TMP/cli.out" "$digits") <(norm "$TMP/h.body" "$digits") | head -8)"; fi
  check "a2 [$name] HTTP project state equals the CLI run" test "$(snapshot)" = "$cli_snap"
  check "a2 [$name] HTTP body is a v1 envelope" valid_envelope "$TMP/h.body"
  if $has_data; then check "a2 [$name] HTTP status 200 for a data envelope" test "$HTTP_CODE" = 200
  else check "a2 [$name] HTTP status >= 400 for an error envelope" test "$HTTP_CODE" -ge 400; fi
  reset_proj; mcp_tool "$tool" "$mreq"
  check "a2 [$name] MCP text equals CLI --json" same_json <(jq -r '.result.content[0].text' "$TMP/m.body") "$TMP/cli.out" "$digits"
  check "a2 [$name] MCP project state equals the CLI run" test "$(snapshot)" = "$cli_snap"
  if $has_data; then
    jq '.result.structuredContent' "$TMP/m.body" > "$TMP/m.sc"; jq '.data' "$TMP/cli.out" > "$TMP/cli.data"
    check "a2 [$name] MCP structuredContent equals the CLI data" same_json "$TMP/m.sc" "$TMP/cli.data" "$digits"
  else
    check "a2 [$name] MCP isError for an error envelope" test "$(jq -r '.result.isError' "$TMP/m.body")" = true
  fi
}
sample "status" /v1/commands/status nself_status '{}' plain false status
sample "config get" /v1/commands/config/get nself_config_get '{"args":["PROJECT_NAME"]}' plain false config get PROJECT_NAME
sample "config list" /v1/commands/config/list nself_config_list '{}' plain false config list
sample "help capabilities" /v1/commands/help/capabilities nself_help_capabilities '{"args":["backup"]}' plain false help capabilities backup
sample "doctor" /v1/commands/doctor nself_doctor '{}' plain true doctor
sample "build --plan" /v1/commands/build nself_build '{}' plan false build --plan
sample "config set (confirm)" /v1/commands/config/set nself_config_set '{"args":["PROJECT_DESCRIPTION","surface identity"]}' confirm false config set PROJECT_DESCRIPTION "surface identity" --yes=true
sample "reset (confirm)" /v1/commands/reset nself_reset '{}' confirm false reset --yes=true
sample "fx status (mounted plugin)" /v1/commands/fx/status nself_fx_status '{}' plain false fx status
sample "logs (fails without containers)" /v1/commands/logs nself_logs '{}' fail false logs
check "a2 the sample is 10 commands (>=3 read, >=2 write, >=2 confirmation, 1 mounted plugin)" test "$NSAMPLES" -eq 10

# ---- acceptance 8: confirmation round trip (HTTP then MCP) ----
select_srv 2; mcp_open
SECRET="smoke-arg-value-7f3a91"
conf_req() { jq -nc --arg v "$1" '{args: ["PROJECT_DESCRIPTION", $v]}'; }
nonce() { jq -r '.data.confirm' "$TMP/h.body"; }
reset_proj; before="$(snapshot)"
http_post /v1/commands/config/set "$(conf_req "$SECRET")"
check "a8 HTTP: a destructive command without confirm returns 428 and writes nothing" test "$HTTP_CODE" = 428 -a "$(snapshot)" = "$before"
N="$(nonce)"
http_post /v1/commands/config/set "$(conf_req "other-$SECRET" | jq -c --arg c "$N" '. + {confirm: $c}')"
check "a8 HTTP: a confirm issued for a different request is refused (E424) and writes nothing" \
  test "$HTTP_CODE" = 409 -a "$(jq -r .error.code "$TMP/h.body")" = E424 -a "$(snapshot)" = "$before"
http_post /v1/commands/config/set "$(conf_req "$SECRET" | jq -c '. + {confirm: "0000000000000000000000000000000000000000000000000000000000000000"}')"
check "a8 HTTP: a locally computed confirm is refused (E424) and writes nothing" \
  test "$HTTP_CODE" = 409 -a "$(jq -r .error.code "$TMP/h.body")" = E424 -a "$(snapshot)" = "$before"
http_post /v1/commands/config/set "$(conf_req "$SECRET")"; N="$(nonce)"
http_post /v1/commands/config/set "$(conf_req "$SECRET" | jq -c --arg c "$N" '. + {confirm: $c}')"
check "a8 HTTP: the issued confirm runs the command once" test "$HTTP_CODE" = 200 -a "$(grep -c "^PROJECT_DESCRIPTION=$SECRET$" "$PROJ/.env")" = 1
reset_proj; before="$(snapshot)"
http_post /v1/commands/config/set "$(conf_req "$SECRET" | jq -c --arg c "$N" '. + {confirm: $c}')"
check "a8 HTTP: a reused confirm is refused (E424) and writes nothing" \
  test "$HTTP_CODE" = 409 -a "$(jq -r .error.code "$TMP/h.body")" = E424 -a "$(snapshot)" = "$before"
mcp_tool nself_config_set "$(conf_req "$SECRET")"
M="$(jq -r '.result.structuredContent.confirm' "$TMP/m.body")"
check "a8 MCP: a destructive command without confirm returns a confirmation and writes nothing" \
  test "$(jq -r '.result.structuredContent.confirmation_required' "$TMP/m.body")" = true -a "$(snapshot)" = "$before"
mcp_tool nself_config_set "$(conf_req "other-$SECRET" | jq -c --arg c "$M" '. + {confirm: $c}')"
check "a8 MCP: a confirm issued for a different request is refused (E424)" \
  test "$(jq -r '.result.isError' "$TMP/m.body")" = true -a "$(jq -r '.result.structuredContent.error.code // .result.content[0].text' "$TMP/m.body" | grep -c E424)" -ge 1
mcp_tool nself_config_set "$(conf_req "$SECRET")"; M="$(jq -r '.result.structuredContent.confirm' "$TMP/m.body")"
mcp_tool nself_config_set "$(conf_req "$SECRET" | jq -c --arg c "$M" '. + {confirm: $c}')"
check "a8 MCP: the issued confirm runs the command once" \
  test "$(jq -r '.result.isError' "$TMP/m.body")" != true -a "$(grep -c "^PROJECT_DESCRIPTION=$SECRET$" "$PROJ/.env")" = 1
reset_proj
mcp_tool nself_config_set "$(conf_req "$SECRET" | jq -c --arg c "$M" '. + {confirm: $c}')"
check "a8 MCP: a reused confirm is refused (E424)" \
  test "$(jq -r '.result.isError' "$TMP/m.body")" = true -a "$(jq -r '.result.content[0].text' "$TMP/m.body" | grep -c E424)" -ge 1
AUDIT="$H2/.nself/logs/machine-audit.jsonl"
check "a8 the audit log records both transports and the refusals" \
  test "$(jq -sr '[.[].transport] | unique | join(",")' "$AUDIT")" = "http,mcp-http" -a "$(grep -c E424 "$AUDIT")" -ge 3
check "a8 the audit log holds no secret arg value" test "$(grep -c "$SECRET" "$AUDIT")" = 0

# ---- acceptance 5: token and bind policy ----
curl_code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
for mode in 14 15; do
  if [ "$mode" = 15 ]; then V15=1; else V15=""; fi
  new_home "a5-$mode"; reset_proj
  if start_srv "a5-http-$mode" admin serve; then
    A_BASE="http://127.0.0.1:$SRV_PORT"; A_TOK="$(cat "$H/.nself/mcp-token" 2>/dev/null)"
    check "a5 admin serve (v1.$((mode == 15 ? 5 : 4))): 401 without a token on /openapi.json" test "$(curl_code "$A_BASE/openapi.json")" = 401
    check "a5 admin serve (v1.$((mode == 15 ? 5 : 4))): 401 without a token on a command" test "$(curl_code -X POST -H 'Content-Type: application/json' -d '{}' "$A_BASE/v1/commands/status")" = 401
    check "a5 admin serve (v1.$((mode == 15 ? 5 : 4))): 200 with the token" test "$(curl_code -H "Authorization: Bearer $A_TOK" "$A_BASE/openapi.json")" = 200
    kill "$SRV_PID" 2>/dev/null; wait "$SRV_PID" 2>/dev/null
    check "a5 no token value reaches the server streams (v1.$((mode == 15 ? 5 : 4)))" test -n "$A_TOK" -a "$(cat "$TMP/a5-http-$mode.out" "$TMP/a5-http-$mode.err" | grep -cF "$A_TOK")" = 0
  else fail "a5 admin serve (v1.$((mode == 15 ? 5 : 4))) starts" "$(head -c 300 "$TMP/a5-http-$mode.err")"; fi
  if [ "$mode" = 15 ]; then
    if start_srv "a5-mcp-$mode" mcp --transport http; then
      check "a5 mcp http (v1.5): 401 without a token" test "$(curl_code -X POST -d '{}' "http://127.0.0.1:$SRV_PORT/mcp")" = 401
      kill "$SRV_PID" 2>/dev/null; wait "$SRV_PID" 2>/dev/null
    else fail "a5 mcp http (v1.5) starts" "$(head -c 300 "$TMP/a5-mcp-$mode.err")"; fi
  else
    new_home a5-mcp-14; reset_proj
    if start_srv "a5-mcp-$mode" mcp --transport http; then
      check "a5 mcp http (v1.4, no token): serves" test "$(curl_code -X POST -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"smoke","version":"1"}}}' -H 'Accept: application/json, text/event-stream' "http://127.0.0.1:$SRV_PORT/mcp")" = 200
      check "a5 mcp http (v1.4, no token): exactly one warning" test "$(grep -c '^warning:' "$TMP/a5-mcp-$mode.err")" = 1
      kill "$SRV_PID" 2>/dev/null; wait "$SRV_PID" 2>/dev/null
    else fail "a5 mcp http (v1.4) starts" "$(head -c 300 "$TMP/a5-mcp-$mode.err")"; fi
  fi
done
FAKETOK="smoke-fixture-token-0123456789-abcdef-xyz"
for mode in 14 15; do
  if [ "$mode" = 15 ]; then V15=1; else V15=""; fi
  new_home "a5r-$mode"; reset_proj
  refuses "a5 admin serve --host 0.0.0.0 without --allowed-host is refused (v1.$((mode == 15 ? 5 : 4)))" E426 NSELF_MCP_TOKEN="$FAKETOK" -- admin serve --host 0.0.0.0 --port "$(free_port)"
  check "a5 the refusal prints no token value" test "$(cat "$TMP/ref.out" "$TMP/ref.err" | grep -cF "$FAKETOK")" = 0
  refuses "a5 mcp http --host 0.0.0.0 without --allowed-host is refused (v1.$((mode == 15 ? 5 : 4)))" E426 NSELF_MCP_TOKEN="$FAKETOK" -- mcp --transport http --host 0.0.0.0 --port "$(free_port)"
  check "a5 the refusal prints no token value (mcp)" test "$(cat "$TMP/ref.out" "$TMP/ref.err" | grep -cF "$FAKETOK")" = 0
done
V15=1
tok1="$(cat "$H1/.nself/mcp-token" 2>/dev/null)"; tok2="$(cat "$H2/.nself/mcp-token" 2>/dev/null)"
check "a5 no token value in any server stream or the audit log" \
  test -n "$tok1" -a -n "$tok2" -a "$(cat "$TMP"/s1-*.out "$TMP"/s1-*.err "$TMP"/s2-*.out "$TMP"/s2-*.err "$H1"/.nself/logs/* "$H2"/.nself/logs/* 2>/dev/null | grep -cF -e "$tok1" -e "$tok2")" = 0

# ---- acceptance 6: config explain and the bindings gate ----
new_home a6; V15=1; reset_proj
(cd "$PROJ" && ns config explain BASE_DOMAIN) >"$TMP/explain.out" 2>/dev/null
for want in "Env name:    BASE_DOMAIN" "Flags:" "MCP nself_init" "HTTP POST /v1/commands/init" "nself.yaml:" "wins"; do
  check "a6 config explain BASE_DOMAIN prints: $want" grep -qF -- "$want" "$TMP/explain.out"
done
(cd "$PROJ" && ns config explain BASE_DOMAIN --json) >"$TMP/explain.json" 2>/dev/null
check "a6 config explain --json: env name, bound flag with MCP and HTTP parameters, effective source" \
  jq -e '.data.env_name == "BASE_DOMAIN" and (.data.flags | length) >= 1 and .data.flags[0].mcp_parameter != null and .data.flags[0].http_parameter != null and .data.effective.source != null and (.data | has("nself_yaml"))' "$TMP/explain.json"
CGO_ENABLED=0 go test -mod=vendor -count=1 ./cmd/commands -run 'TestConfigBindingsGate' ./internal/config/bindings/ >"$TMP/bind.out" 2>&1
check "a6 the bindings gate passes and fails on an unbound config-named flag" test $? -eq 0

# ---- acceptance 7: config validate on the four reference-app fixtures ----
for app in nclaw nchat ntask nsentry; do
  for mode in 14 15; do
    if [ "$mode" = 15 ]; then V15=1; else V15=""; fi
    reset_proj; cp "internal/build/testdata/manifest/$app.yaml" "$PROJ/nself.yaml"
    (cd "$PROJ" && ns config validate --json) >"$TMP/val.$app.$mode.json" 2>"$TMP/val.$app.$mode.err"; echo $? > "$TMP/val.$app.$mode.rc"
  done
  check "a7 $app (v1.4): exits 0 with warnings" test "$(cat "$TMP/val.$app.14.rc")" = 0 -a "$(grep -c 'warning:' "$TMP/val.$app.14.err")" -ge 1
done
check "a7 nclaw (v1.5): fails (E436) naming project and plugins.pro[0]" \
  test "$(cat "$TMP/val.nclaw.15.rc")" = 1 -a "$(jq '[.data.manifest_findings[] | select(contains("project:") or contains("plugins.pro[0]"))] | length' "$TMP/val.nclaw.15.json")" -ge 2
check "a7 nclaw (v1.4): warns on project and plugins.pro[0]" \
  grep -q 'plugins\.pro\[0\]' "$TMP/val.nclaw.14.err"
check "a7 nchat (v1.5): names the nested plugins.required" jq -e '[.data.manifest_findings[] | select(contains("plugins.required"))] | length >= 1' "$TMP/val.nchat.15.json"
check "a7 nchat (v1.4): names the nested plugins.required as a warning" grep -q 'plugins\.required' "$TMP/val.nchat.14.err"
for app in ntask nsentry; do
  check "a7 $app (v1.5): unknown keys are errors, count matches the findings" \
    jq -e '.data.valid == false and .data.error_count == (.data.manifest_findings | length) and .data.error_count >= 1' "$TMP/val.$app.15.json"
done

# ---- acceptance 9: plugin leg ----
V15=1; new_home a9; reset_proj
PL="$TMP/plugins"
src="${PLUGINS_DIR:-https://github.com/nself-org/plugins.git}"
# A local checkout is cloned in full (no network); GitHub gets a blobless clone.
clone_plugins() {
  if [ -n "$PLUGINS_DIR" ]; then git clone -q --no-checkout "$PLUGINS_DIR" "$PL"; else git clone -q --no-checkout --filter=blob:none "$src" "$PL"; fi
}
if clone_plugins 2>"$TMP/clone.err" && git -C "$PL" checkout -q --detach "$PLUGINS_REF" 2>>"$TMP/clone.err"; then
  check "a9 plugins checkout is at the recorded ref $PLUGINS_REF" test "$(git -C "$PL" rev-parse HEAD)" = "$PLUGINS_REF"
else
  fail "a9 plugins checkout at $PLUGINS_REF" "$(head -c 300 "$TMP/clone.err")"
fi
if [ -d "$PL/free/infra" ]; then
  for spec in infra:cmd functions:cmd plugin-dev:cmd sentry-cli:cmd/sentry; do
    p="${spec%%:*}"; main="${spec#*:}"; bn="$(jq -r .binaryName "$PL/free/$p/plugin.json")"; pn="$(jq -r .name "$PL/free/$p/plugin.json")"
    mkdir -p "$H/plugins/$pn" "$H/plugins/bin"
    cp "$PL/free/$p/plugin.json" "$H/plugins/$pn/plugin.json"
    if (cd "$PL/free/$p" && CGO_ENABLED=0 go build -mod=vendor -buildvcs=false -o "$H/plugins/bin/$bn" "./$main") 2>"$TMP/pbuild.$p.err"; then
      pass "a9 built plugin $p ($bn)"
    else fail "a9 build plugin $p" "$(head -c 400 "$TMP/pbuild.$p.err")"; fi
  done
  # plugin_case <label> <expected: data|<error code>|any> <nself args...> (run through nself, v1.5)
  plugin_case() {
    local label="$1" want="$2" rc=0
    shift 2
    (cd "$PROJ" && ns "$@" --json) >"$TMP/pl.out" 2>"$TMP/pl.err" || rc=$?
    check "a9 [$label] one v1 envelope that validates against envelope.v1.schema.json" one_valid_envelope "$TMP/pl.out"
    check "a9 [$label] command field and exit code agree with the envelope" \
      jq -e --arg c "$label" --argjson rc "$rc" '.command == $c and ($rc == (.error.exit_code // 0))' "$TMP/pl.out"
    case "$want" in
      any) ;;
      data) check "a9 [$label] answers with data" jq -e 'has("data")' "$TMP/pl.out" ;;
      *) check "a9 [$label] answers with $want" jq -e --arg w "$want" '.error.code == $w' "$TMP/pl.out" ;;
    esac
  }
  plugin_case "infra validate" any infra validate
  plugin_case "infra server list" E742 infra server list
  plugin_case "functions list" data functions list
  plugin_case "plugin-dev init" data plugin-dev init smokeplug --no-interactive --out "$TMP/scaffold"
  plugin_case "sentry monitors list" E782 sentry monitors list --api-url http://127.0.0.1:9
  plugin_case "sentry status" E783 sentry status --url http://127.0.0.1:9
fi

# ---- servers still healthy at the end ----
for p in $S1_HTTP_PID; do check "s1 admin serve survived the run" kill -0 "$p"; done

if [ "$FAILED" -gt 0 ] || [ "$PASSED" -eq 0 ]; then exit 1; fi
exit 0
