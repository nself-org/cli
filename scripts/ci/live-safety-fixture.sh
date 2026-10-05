#!/usr/bin/env bash
# Purpose: prove the P7-LIVE-03 live-safety guarantees on the real binary:
#          `nself build --plan` writes nothing and starts nothing, a prod-class
#          change needs --yes (E403, exit 4, in v1.5 mode), --plan-id refuses a
#          stale plan (E450), and a plan after an apply is empty. Prod and dev
#          fixture projects are built in temp dirs; no real project, host or
#          remote is touched.
# Usage:   bash scripts/ci/live-safety-fixture.sh <path-to-nself-binary>
# Inputs:  the binary; python3 on PATH. Local Docker is used only to list
#          containers (and to create, never start, one labelled scratch container
#          when an image is cached); without Docker the container checks are
#          skipped with a note.
# Outputs: one line per assertion ("ok <name>"); the last line is
#          "LIVE-SAFETY-FIXTURE: PASS" on success. Any failure prints
#          "FAIL <name>" to stderr and exits 1.
# Constraints: bash 3.2 compatible. Colima/Docker is never stopped. The scratch
#          container is removed on exit.
set -euo pipefail

BIN="${1:?usage: live-safety-fixture.sh <nself-binary>}"
case "$BIN" in /*) ;; *) BIN="$PWD/$BIN" ;; esac
[ -x "$BIN" ] || { echo "FAIL: $BIN is not executable" >&2; exit 1; }
command -v python3 >/dev/null || { echo "FAIL: python3 is required" >&2; exit 1; }
cd "$(dirname "$0")/../.."
FX="$PWD/internal/reconcile/testdata/fixtures"
REAL_HOME="$HOME"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/live-safety.XXXXXX")"
SCRATCH=""

cleanup() {
  if [ -n "$SCRATCH" ]; then docker rm -f "$SCRATCH" >/dev/null 2>&1 || true; fi
  rm -rf "$WORK"
}
trap cleanup EXIT

ok() { printf 'ok %s\n' "$1"; }
fail() { printf 'FAIL %s\n' "$1" >&2; exit 1; }

# mkproj <name> <fixture>: a project with HOME and the plugin dir beside it.
mkproj() {
  local d="$WORK/$1" f="$FX/$2"
  mkdir -p "$d/project/.nself" "$d/home" "$d/plugins"
  cat "$f/project/dotenv" "$FX/secrets.env" > "$d/project/.env"
  chmod 600 "$d/project/.env"
  if [ -d "$f/plugins" ]; then cp -R "$f/plugins/." "$d/plugins/"; fi
  if [ -f "$f/certs.txt" ]; then
    for c in $(cat "$f/certs.txt"); do
      mkdir -p "$d/project/ssl/certificates/$c"
      echo CERT > "$d/project/ssl/certificates/$c/fullchain.pem"
      echo KEY > "$d/project/ssl/certificates/$c/privkey.pem"
    done
  fi
}

# tree <name>: a hash of every file (content) and directory under the root,
# except the command guard's lock file.
tree() {
  (cd "$WORK/$1" && {
    find . -type d | LC_ALL=C sort
    find . -type f ! -name op.lock | LC_ALL=C sort | while IFS= read -r p; do
      printf '%s %s\n' "$(python3 -c 'import hashlib,sys;print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$p")" "$p"
    done
  } | python3 -c 'import hashlib,sys;print(hashlib.sha256(sys.stdin.buffer.read()).hexdigest())')
}

# run_nself <name> <v15:0|1> <args...>: run in the project; stdout to $WORK/out,
# stderr to $WORK/err; exit status in $RC.
RC=0
EXTRA_ENV=""
run_nself() {
  local name="$1" v15="$2"
  shift 2
  local envs="NSELF_CMD_LOG_ENABLED=false $EXTRA_ENV"
  if [ "$v15" = 1 ]; then envs="$envs NSELF_V15=1"; fi
  set +e
  (cd "$WORK/$name/project" && env -u NSELF_V15 $envs HOME="$WORK/$name/home" NSELF_PLUGIN_DIR="$WORK/$name/plugins" \
    DOCKER_CONFIG="${DOCKER_CONFIG:-$REAL_HOME/.docker}" AI_AUTO_INSTALL=false "$BIN" "$@" > "$WORK/out" 2> "$WORK/err" < /dev/null)
  RC=$?
  set -e
}

# jget <python-expr over d>: evaluate against the JSON on $WORK/out (data field).
jget() {
  python3 -c 'import json,sys;d=json.load(open(sys.argv[1]));d=d.get("data",d);print(eval(sys.argv[2]))' "$WORK/out" "$1"
}

containers() { docker ps -a --format '{{.ID}} {{.Names}} {{.State}}' 2>/dev/null | LC_ALL=C sort; }
HAVE_DOCKER=0
if docker info >/dev/null 2>&1; then HAVE_DOCKER=1; fi

# ---- prod fixture -----------------------------------------------------------
mkproj prod prod-plugins
if [ "$HAVE_DOCKER" = 1 ]; then
  IMG="$(docker image ls --format '{{.Repository}}:{{.Tag}}' | grep -v '<none>' | head -1 || true)"
  if [ -n "$IMG" ]; then
    SCRATCH="l03-fx-$$"
    docker create --name "$SCRATCH" --label com.docker.compose.project=fx --label com.docker.compose.service=hasura \
      --label com.docker.compose.config-hash=stale "$IMG" >/dev/null
  fi
fi
CBEFORE=""
if [ "$HAVE_DOCKER" = 1 ]; then CBEFORE="$(containers)"; fi
T0="$(tree prod)"

run_nself prod 1 build --plan --json
[ "$RC" = 0 ] || fail "plan exit $RC: $(cat "$WORK/err")"
[ "$(tree prod)" = "$T0" ] || fail "build --plan changed the project tree, HOME or plugin dir"
ok "plan writes nothing (tree hash unchanged)"
if [ "$HAVE_DOCKER" = 1 ]; then
  [ "$(containers)" = "$CBEFORE" ] || fail "build --plan changed the container list"
  ok "plan starts, stops and creates no container"
fi
[ "$(jget 'd["env_class"]')" = prod ] || fail "plan env_class is not prod"
[ "$(jget 'len(d["artifacts"])>5')" = True ] || fail "plan lists no artifacts"
[ "$(jget 'd["requires_confirmation"]')" = True ] || fail "plan does not require confirmation"
ok "prod plan lists artifacts and requires confirmation"
if [ -n "$SCRATCH" ]; then
  if [ "$(jget 'd["containers"]["known"]')" = True ]; then
    [ "$(jget '[i["action"] for i in d["containers"]["items"] if i["service"]=="hasura"]')" = "['recreate']" ] \
      || fail "the stale-labelled hasura container is not planned as recreate"
    ok "container impact: stale config hash plans recreate"
  else
    echo "note: docker compose could not hash the plan here; container impact reported unknown"
  fi
fi
SHOWN="$(jget 'd["plan_id"]')"

run_nself prod 1 build --json
[ "$RC" = 4 ] || fail "non-interactive prod build without --yes exited $RC, want 4"
[ "$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["error"]["code"])' "$WORK/out")" = E403 ] || fail "refusal is not E403"
[ "$(tree prod)" = "$T0" ] || fail "the refused build wrote"
ok "v1.5 prod build without --yes: E403, exit 4, nothing written"

run_nself prod 0 build
[ "$RC" = 0 ] || fail "v1.4 prod build exited $RC"
grep -q "v1.5 will require --yes" "$WORK/err" || fail "v1.4 build printed no notice"
ok "v1.4 prod build proceeds with a notice"

# back to the fresh state for the --yes path
rm -rf "$WORK/prod"
mkproj prod prod-plugins
T0="$(tree prod)"
run_nself prod 1 build --plan --json
SHOWN="$(jget 'd["plan_id"]')"
echo "MONITORING_ENABLED=true" >> "$WORK/prod/project/.env"
T1="$(tree prod)"
run_nself prod 1 build --yes --plan-id "$SHOWN" --json
[ "$RC" = 1 ] || fail "stale --plan-id exited $RC, want 1"
[ "$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["error"]["code"])' "$WORK/out")" = E450 ] || fail "stale plan is not E450"
[ "$(tree prod)" = "$T1" ] || fail "the E450 build wrote"
ok "stale --plan-id: E450, exit 1, nothing written"

# The stale-labelled scratch container would (correctly) keep the plan non-empty.
if [ -n "$SCRATCH" ]; then docker rm -f "$SCRATCH" >/dev/null 2>&1 || true; SCRATCH=""; fi
run_nself prod 1 build --plan --json
FRESH="$(jget 'd["plan_id"]')"
run_nself prod 1 build --yes --plan-id "$FRESH"
[ "$RC" = 0 ] || fail "build --yes --plan-id (fresh) exited $RC: $(cat "$WORK/err")"
[ -f "$WORK/prod/project/docker-compose.yml" ] || fail "--yes did not build"
run_nself prod 1 build --plan --json
[ "$(jget 'd["empty"]')" = True ] || fail "plan after apply is not empty"
ok "--yes --plan-id applies; the plan afterwards is empty"

# A same-shape content change (one env value swapped) must also refuse the old id.
run_nself prod 1 build --plan --json
SAME="$(jget 'd["plan_id"]')"
sed -i.bak 's/^MINIO_ROOT_USER=minio-user-Tq8Zk/MINIO_ROOT_USER=minio-user-Tq8Zj/' "$WORK/prod/project/.env" && rm -f "$WORK/prod/project/.env.bak"
T2="$(tree prod)"
run_nself prod 1 build --yes --plan-id "$SAME" --json
[ "$RC" = 1 ] || fail "same-shape change: stale --plan-id exited $RC, want 1"
[ "$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["error"]["code"])' "$WORK/out")" = E450 ] || fail "same-shape change is not E450"
[ "$(tree prod)" = "$T2" ] || fail "the same-shape E450 build wrote"
ok "same-shape content change: old --plan-id refused (E450)"

# ---- review round: one render, planned bytes, no env leakage ----------------------
errcode() { python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["error"]["code"])' "$WORK/out"; }

# ENV set in a cascade file other than .env resolves the same in plan and apply.
mkproj envx dev-minimal
printf 'ENV=prod\n' > "$WORK/envx/project/.env.local"
printf 'BASE_DOMAIN=prodonly.example.org\nSSL_MODE=none\n' > "$WORK/envx/project/.env.prod"
run_nself envx 1 build --yes
[ "$RC" = 0 ] || fail "cascade-ENV build exited $RC"
run_nself envx 1 build --plan --json
[ "$(jget 'd["empty"]')" = True ] || fail "plan after a cascade-ENV apply is not empty (the plan run leaked its env into the write)"
ok "ENV from a cascade file: plan equals apply"

# NSELF_V15=0 in project env files must not switch the v1.5 refusal off.
mkproj gate prod-ssl
printf 'NSELF_V15=0\n' >> "$WORK/gate/project/.env"
printf 'NSELF_V15=false\n' > "$WORK/gate/project/.env.local"
G0="$(tree gate)"
run_nself gate 1 build --json
[ "$RC" = 4 ] && [ "$(errcode)" = E403 ] || fail "NSELF_V15=0 in env files bypassed the v1.5 gate (rc=$RC)"
[ "$(tree gate)" = "$G0" ] || fail "the refused build wrote"
ok "NSELF_V15=0 in env files cannot switch the gate off"

# A first build generates secrets: --plan-id is refused (E451), --yes applies.
mkproj fresh prod-ssl
python3 - "$WORK/fresh/project/.env" <<'PY'
import sys
p = sys.argv[1]
keep = [l for l in open(p).read().split("\n") if not l.startswith(("PLUGIN_INTERNAL_SECRET", "NOTIFY_INTERNAL_SECRET", "CRON_INTERNAL_SECRET", "HASURA_GRAPHQL_JWT_SECRET"))]
open(p, "w").write("\n".join(keep))
PY
run_nself fresh 1 build --plan --json
[ "$(jget '".env.secrets" in [a["path"] for a in d["artifacts"]]')" = True ] || fail ".env.secrets is not a planned artifact"
FID="$(jget 'd["plan_id"]')"
F0="$(tree fresh)"
run_nself fresh 1 build --yes --plan-id "$FID" --json
[ "$RC" = 1 ] && [ "$(errcode)" = E451 ] || fail "generated secrets with --plan-id: rc=$RC code=$(errcode)"
[ "$(tree fresh)" = "$F0" ] || fail "the E451 build wrote"
run_nself fresh 1 build --yes
[ "$RC" = 0 ] && [ -f "$WORK/fresh/project/.env.secrets" ] || fail "the first build did not persist secrets"
ok "first build: planned .env.secrets, --plan-id refused (E451), --yes applies"

# A plugin compose fragment edited after the plan is E450.
mkproj plug dev-plugin
run_nself plug 1 build --plan --json
[ "$(jget '"@plugins/nself-alpha/docker-compose.plugin.yml" in [a["path"] for a in d["artifacts"]]')" = True ] || fail "the rewritten plugin fragment is not a plan artifact"
PID="$(jget 'd["plan_id"]')"
sed -i.bak 's/"3901:3901"/"0.0.0.0:22:3901"/' "$WORK/plug/plugins/nself-alpha/docker-compose.plugin.yml" && rm -f "$WORK/plug/plugins/nself-alpha/docker-compose.plugin.yml.bak"
P0="$(tree plug)"
run_nself plug 1 build --plan-id "$PID" --json
[ "$RC" = 1 ] && [ "$(errcode)" = E450 ] || fail "fragment edited after the plan: rc=$RC code=$(errcode)"
[ "$(tree plug)" = "$P0" ] || fail "the E450 build wrote"
ok "plugin fragment edited after the plan: E450, nothing written"

# A 0644 .env is a mode change in the plan and is applied.
mkproj mode dev-minimal
chmod 644 "$WORK/mode/project/.env"
run_nself mode 1 build --plan --json
[ "$(jget '[a["diff_lines"] for a in d["artifacts"] if a["path"]==".env"]')" = "[0]" ] || fail "a 0644 .env is not a mode change in the plan"
run_nself mode 1 build
[ "$(python3 -c 'import os,sys;print(oct(os.stat(sys.argv[1]).st_mode & 0o777))' "$WORK/mode/project/.env")" = 0o600 ] || fail ".env was not chmodded to 0600"
ok "permission-only change is planned and applied"

# An expired plugin is removed inside the held apply and its env does not leak.
mkproj lc dev-minimal
printf 'ENV=prod\n' > "$WORK/lc/project/.env.local"
printf 'BASE_DOMAIN=prodonly.example.org\nSSL_MODE=none\n' > "$WORK/lc/project/.env.prod"
mkdir -p "$WORK/lc/plugins/oldplug" "$WORK/lc/home/.config/nself"
printf '{"name":"oldplug","port":3920,"language":"go"}' > "$WORK/lc/plugins/oldplug/plugin.json"
printf '{"version":1,"records":{"oldplug":{"name":"oldplug","state":"dormant","license_expiry":"2020-01-01T00:00:00Z","dormant_since":"2020-02-01T00:00:00Z","grace_period":1000000000}}}' > "$WORK/lc/home/.config/nself/plugin-lifecycle.json"
run_nself lc 1 build --plan --json
[ "$(jget '"plugin-remove" in [e["kind"] for e in d["effects"]]')" = True ] || fail "the plan does not list the expired-plugin removal"
[ -d "$WORK/lc/plugins/oldplug" ] || fail "--plan removed the plugin"
run_nself lc 1 build --yes
[ "$RC" = 0 ] && [ ! -d "$WORK/lc/plugins/oldplug" ] || fail "the expired plugin was not removed (rc=$RC)"
run_nself lc 1 build --plan --json
[ "$(jget 'd["empty"]')" = True ] || fail "plan after a removal apply is not empty (the lifecycle step leaked its env into the write)"
ok "expired plugin removed inside the held apply; plan equals apply"

# A change outside .env (the freshness cache cannot see it) is still written.
mkproj sec prod-ssl
run_nself sec 1 build --yes
[ "$RC" = 0 ] || fail "setup build exited $RC"
touch -t 202001010000 "$WORK/sec/project/.env"
printf 'POSTGRES_PASSWORD=Zq7Wx3Tn9Rk5Hb2Vc8Lm4Pd6Sa1FgJu0Ye8\n' > "$WORK/sec/project/.env.secrets"
run_nself sec 1 build --plan --json
[ "$(jget 'd["empty"]')" = False ] || fail "the .env.secrets rotation is not planned"
SID="$(jget 'd["plan_id"]')"
run_nself sec 1 build --yes --plan-id "$SID"
[ "$RC" = 0 ] || fail "applying the .env.secrets rotation exited $RC"
grep -q 'Zq7Wx3Tn9Rk5Hb2Vc8Lm4Pd6Sa1FgJu0Ye8' "$WORK/sec/project/.nself/compose.env" || fail "the rotated password was reported applied but not written (freshness cache skipped the build)"
run_nself sec 1 build --plan --json
[ "$(jget 'd["empty"]')" = True ] || fail "plan after the rotation apply is not empty"
ok ".env.secrets-only change is written despite a fresh cache"

# A plan that installs or removes plugins refuses --plan-id (E453).
mkproj lc2 dev-minimal
mkdir -p "$WORK/lc2/plugins/oldplug" "$WORK/lc2/home/.config/nself"
printf '{"name":"oldplug","port":3920,"language":"go"}' > "$WORK/lc2/plugins/oldplug/plugin.json"
printf '{"version":1,"records":{"oldplug":{"name":"oldplug","state":"dormant","license_expiry":"2020-01-01T00:00:00Z","dormant_since":"2020-02-01T00:00:00Z","grace_period":1000000000}}}' > "$WORK/lc2/home/.config/nself/plugin-lifecycle.json"
run_nself lc2 1 build --plan --json
L2="$(jget 'd["plan_id"]')"
run_nself lc2 1 build --yes --plan-id "$L2" --json
[ "$RC" = 1 ] && [ "$(errcode)" = E453 ] || fail "plugin removal with --plan-id: rc=$RC code=$(errcode)"
[ -d "$WORK/lc2/plugins/oldplug" ] || fail "E453 ran the removal"
ok "plan with plugin changes: --plan-id refused (E453), nothing ran"

# Docker unreachable: the plan is never empty.
EXTRA_ENV="DOCKER_HOST=unix:///nonexistent.sock"
run_nself envx 1 build --plan --json
EXTRA_ENV=""
[ "$(jget 'd["empty"]')" = False ] && [ "$(jget 'd["containers"]["known"]')" = False ] || fail "unknown Docker state read as empty"
ok "unknown container state is never empty"

# ---- dev fixture --------------------------------------------------------------
mkproj dev dev-minimal
run_nself dev 1 build --plan --json
[ "$(jget 'd["requires_confirmation"]')" = False ] || fail "a dev plan requires confirmation"
run_nself dev 1 build
[ "$RC" = 0 ] || fail "dev build in v1.5 mode exited $RC without --yes"
run_nself dev 1 build --plan --json
[ "$(jget 'd["empty"]')" = True ] || fail "dev plan after build is not empty"
ok "dev build needs no confirmation; the plan afterwards is empty"

echo "LIVE-SAFETY-FIXTURE: PASS"
