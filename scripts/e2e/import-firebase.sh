#!/usr/bin/env bash
# P7-ADOPT-14 Linux e2e: `nself db import firebase` against the Firebase emulator suite.
# Seeds 2 collections (one with a sub-collection), 12 documents, 3 auth users and 2 storage
# objects through the emulator REST APIs, imports them into a started nSelf project and checks the
# report, the data, the reset list and the refusal to import twice. Runs as the verification line of
# the Ticket: docker:27-cli, host network, docker socket, the nself Linux binary on PATH.
set -euo pipefail
umask 022
export NSELF_V15=1
ROOT=$(pwd)
: "${W:=$(mktemp -d)}"
export TMPDIR="$W"
# nself build generates the local TLS certificate with openssl when mkcert is absent.
command -v openssl >/dev/null 2>&1 || apk add -q openssl >/dev/null 2>&1 || true
rand() { od -An -N"$1" -tx1 /dev/urandom | tr -d ' \n'; }
BASE=${FB_BASE_PORT:-$((22000 + RANDOM % 20000))}
export FB_FIRESTORE_PORT=$BASE FB_AUTH_PORT=$((BASE + 1)) FB_STORAGE_PORT=$((BASE + 2))
export SEAWEEDFS_PORT=$((BASE + 8))
export TARGET_PG_PORT=$((BASE + 3)) TARGET_HASURA_PORT=$((BASE + 4)) TARGET_AUTH_PORT=$((BASE + 5)) TARGET_HTTP_PORT=$((BASE + 6)) TARGET_HTTPS_PORT=$((BASE + 7))
FIXTURE=$ROOT/test/fixtures/import/firebase/compose.yml
PROJECT=a14-$(date +%s)-$RANDOM
TARGET=$W/target
mkdir -p "$TARGET"
cleanup() {
  docker compose -p "$PROJECT-src" -f "$FIXTURE" down -v >/dev/null 2>&1 || true
  if [[ -f "$TARGET/docker-compose.yml" ]]; then (cd "$TARGET" && docker compose down -v >/dev/null 2>&1) || true; fi
}
trap cleanup EXIT
docker compose -p "$PROJECT-src" -f "$FIXTURE" up -d --wait

FS="http://127.0.0.1:$FB_FIRESTORE_PORT/v1/projects/demo-nself/databases/(default)/documents"
ST="http://127.0.0.1:$FB_STORAGE_PORT"
AU="http://127.0.0.1:$FB_AUTH_PORT"
BUCKET=demo-nself.appspot.com
H='Authorization: Bearer owner'
doc() { # doc <collection path> <id> <fields json>
  curl -fsS -H "$H" -H 'Content-Type: application/json' -X POST "$FS/$1?documentId=$2" -d "{\"fields\":$3}" >/dev/null
}
s() { printf '{"stringValue":"%s"}' "$1"; }
i() { printf '{"integerValue":"%s"}' "$1"; }
b() { printf '{"booleanValue":%s}' "$1"; }
doc users u1 "{\"name\":$(s Ann),\"age\":$(i 30),\"active\":$(b true),\"joined\":{\"timestampValue\":\"2024-05-01T12:00:00.123456Z\"},\"tags\":{\"arrayValue\":{\"values\":[$(s a),$(s b)]}},\"profile\":{\"mapValue\":{\"fields\":{\"city\":$(s Oslo)}}}}"
doc users u2 "{\"name\":$(s Bob),\"age\":$(s unknown),\"active\":$(b false)}"
doc users u3 "{\"name\":$(s Cy),\"age\":$(i 41),\"active\":$(b true)}"
for n in 1 2 3; do doc users/u1/orders "o$n" "{\"total\":$(i $((n * 10)))}"; done
for n in 1 2; do doc users/u2/orders "o$n" "{\"total\":$(i $((n * 5)))}"; done
doc notes n1 "{\"title\":$(s one)}"
doc notes n2 "{\"title\":$(s '')}"
doc notes n3 "{\"title\":$(s three),\"views\":$(i 9007199254740993)}"
doc notes n4 "{}"
curl -fsS -H "$H" -H 'Content-Type: image/png' --data-binary 'PNG-ANN' "$ST/upload/storage/v1/b/$BUCKET/o?uploadType=media&name=avatars%2Fann.png" >/dev/null
curl -fsS -H "$H" -H 'Content-Type: text/plain' --data-binary 'hello' "$ST/upload/storage/v1/b/$BUCKET/o?uploadType=media&name=docs%2Freadme.txt" >/dev/null
for u in ann bob cy; do
  curl -fsS -H 'Content-Type: application/json' -X POST "$AU/identitytoolkit.googleapis.com/v1/accounts:signUp?key=fake" -d "{\"email\":\"$u@example.test\",\"password\":\"pw-$u-secret-1\",\"displayName\":\"$u\"}" >/dev/null
done
# The same call `firebase auth:export` makes (accounts:batchGet); the file has the `users` array.
curl -fsS -H "$H" "$AU/identitytoolkit.googleapis.com/v1/projects/demo-nself/accounts:batchGet?maxResults=1000" | jq '{users: .users}' > "$W/auth-export.json"
test "$(jq '.users|length' "$W/auth-export.json")" = 3

cd "$TARGET"
nself init --non-interactive
{
  printf '\nPROJECT_NAME=%s\nPOSTGRES_DB=nself\nPOSTGRES_USER=postgres\nPOSTGRES_VERSION=16\nPOSTGRES_PORT=%s\nHASURA_PORT=%s\nAUTH_PORT=%s\n' "$PROJECT" "$TARGET_PG_PORT" "$TARGET_HASURA_PORT" "$TARGET_AUTH_PORT"
  printf 'NGINX_HTTP_PORT=%s\nNGINX_HTTPS_PORT=%s\nPGVECTOR_ENABLED=false\nPOSTGRES_EXTENSIONS=\nAUTH_ENABLED=true\nREDIS_ENABLED=false\nMAILPIT_ENABLED=false\n' "$TARGET_HTTP_PORT" "$TARGET_HTTPS_PORT"
  printf 'POSTGRES_PASSWORD=%s\nHASURA_GRAPHQL_ADMIN_SECRET=%s\n' "$(rand 24)" "$(rand 24)"
  # Objects need an active store: core SeaweedFS on a loopback port (the importer reads SEAWEEDFS_PORT).
  printf 'MINIO_ENABLED=false\nOBJECT_STORAGE_ENABLED=true\nSEAWEEDFS_PORT=%s\nS3_ACCESS_KEY=%s\nS3_SECRET_KEY=%s\n' "$SEAWEEDFS_PORT" "$(rand 12)" "$(rand 24)"
} >> .env
chmod 600 .env
nself start

export FIRESTORE_EMULATOR_HOST=127.0.0.1:$FB_FIRESTORE_PORT STORAGE_EMULATOR_HOST=127.0.0.1:$FB_STORAGE_PORT
ARGS=(--project demo-nself --auth-export "$W/auth-export.json" --bucket "$BUCKET")
nself db import firebase "${ARGS[@]}" --plan --json > "$W/plan.json"
jq -e '.data.result=="pass" and (.data.steps|length)>0 and .data.detection.kind=="firebase" and .data.detection.source_users==3' "$W/plan.json"
test ! -d .nself/import
nself db import firebase "${ARGS[@]}" --keep-stage --json > "$W/import.json"
jq -e '.data.result=="pass" and (.data.mismatches|length)==0 and (.data.tables|length)==3 and ([.data.tables[].equal]|all)' "$W/import.json"
jq -e '.data.users.source==3 and .data.users.target==3 and .data.users.equal==true and (.data.users.reset_required|length)==3' "$W/import.json"
jq -e '(.data.users.reset_required|all(.id!="" and (.email|endswith("@example.test"))))' "$W/import.json"
jq -e '.data.storage.source_objects==2 and .data.storage.target_objects==2 and .data.storage.equal==true' "$W/import.json"
# The documented exemption for data is none: only mapping notes (kind metadata) may appear.
jq -e '[.data.exemptions[]|select(.kind!="metadata")]|length==0' "$W/import.json"
jq -e '[.data.exemptions[].id]|index("firebase.users.age")!=null' "$W/import.json"

q() { docker compose exec -T postgres psql -U postgres -d nself -Atc "$1"; }
test "$(q 'SELECT count(*) FROM firebase.users')" = 3
test "$(q 'SELECT count(*) FROM firebase.notes')" = 4
test "$(q 'SELECT count(*) FROM firebase.users__orders')" = 5
test "$(q "SELECT data->'profile'->>'city' FROM firebase.users WHERE id='u1'")" = Oslo
test "$(q "SELECT data_type FROM information_schema.columns WHERE table_schema='firebase' AND table_name='users' AND column_name='name'")" = text
test "$(q "SELECT count(*) FROM information_schema.columns WHERE table_schema='firebase' AND table_name='users' AND column_name='age'")" = 0
test "$(q "SELECT data->>'age' FROM firebase.users WHERE id='u2'")" = unknown
test "$(q "SELECT views FROM firebase.notes WHERE id='n3'")" = 9007199254740993
test "$(q "SELECT title FROM firebase.notes WHERE id='n2'")" = ''
test "$(q "SELECT count(*) FROM firebase.notes WHERE title IS NULL")" = 1
test "$(q 'SELECT count(*) FROM auth.users')" = 3
test "$(q 'SELECT count(*) FROM auth.users WHERE password_hash IS NULL')" = 3
test "$(q "SELECT count(*) FROM auth.users WHERE metadata->'firebase'->>'uid' IS NOT NULL")" = 3
# No password material (the emulator's hashes embed the clear text) reaches the bundle or the reports.
if grep -rIl -e 'fakeHash' -e 'pw-ann-secret' -e 'pw-bob-secret' .nself/import "$W/plan.json" "$W/import.json" >/dev/null 2>&1; then echo 'FAIL: password material in the bundle or reports'; exit 1; fi
test "$(stat -c %a "$(ls -d .nself/import/*/stage | head -1)")" = 700

# A second import is refused (E519) and changes nothing.
q 'SELECT count(*), sum(hashtextextended(t::text,0)::numeric) FROM firebase.users t' > "$W/before.txt"
if nself db import firebase "${ARGS[@]}" --json > "$W/rerun.json" 2> "$W/rerun.err"; then echo 'FAIL: rerun did not refuse'; exit 1; fi
grep -q E519 "$W/rerun.json" "$W/rerun.err"
q 'SELECT count(*), sum(hashtextextended(t::text,0)::numeric) FROM firebase.users t' > "$W/after.txt"
cmp "$W/before.txt" "$W/after.txt"

# An unreachable Firestore fails before any write.
if FIRESTORE_EMULATOR_HOST=127.0.0.1:1 nself db import firebase "${ARGS[@]}" --plan --json > "$W/unreachable.json" 2> "$W/unreachable.err"; then echo 'FAIL: unreachable source accepted'; exit 1; fi
grep -q E517 "$W/unreachable.json" "$W/unreachable.err"
# Missing flags are named before the importer runs.
if nself db import firebase --auth-export "$W/auth-export.json" --plan > "$W/noproject.out" 2>&1; then echo 'FAIL: missing --project accepted'; exit 1; fi
grep -q -e '--project' "$W/noproject.out"
echo 'PASS Linux Firebase import: 12 documents, 3 reset-required users, 2 objects, rerun refusal, redaction'
