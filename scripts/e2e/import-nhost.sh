#!/usr/bin/env bash
# Purpose: Linux acceptance for `nself db import nhost` (P7-ADOPT-11).
# Inputs: W shared host/container path; nself, docker, curl, jq, psql on PATH.
# Outputs: non-vacuous PASS assertions. Constraints: fake credentials only; cleanup owns only the
# resources this run created (compose projects named a11nh-*).
set -euo pipefail
umask 022
export NSELF_V15=1
# Build generates self-signed certs through openssl; the docker CLI image lacks openssl and python3.
command -v openssl >/dev/null || apk add -q openssl >/dev/null
command -v python3 >/dev/null || apk add -q python3 >/dev/null
ROOT=$(pwd)
: "${W:=$(mktemp -d)}"
export TMPDIR="$W"
FIXTURE=$ROOT/test/fixtures/import/nhost
rand() { python3 -c 'import secrets;print(secrets.token_hex(24))'; }
export FIXTURE_PASSWORD FIXTURE_ADMIN_SECRET FIXTURE_JWT_KEY FIXTURE_S3_ACCESS FIXTURE_S3_SECRET FIXTURE_S3_BUCKET=nhost
FIXTURE_PASSWORD=$(rand) FIXTURE_ADMIN_SECRET=$(rand) FIXTURE_JWT_KEY=$(rand) FIXTURE_S3_ACCESS=$(rand) FIXTURE_S3_SECRET=$(rand)
export FIXTURE_PORT=${FIXTURE_PORT:-$((22000 + RANDOM % 20000))}
export FIXTURE_HASURA_PORT=$((FIXTURE_PORT + 1)) FIXTURE_AUTH_PORT=$((FIXTURE_PORT + 7)) FIXTURE_STORAGE_PORT=$((FIXTURE_PORT + 8)) FIXTURE_S3_PORT=$((FIXTURE_PORT + 9))
export TARGET_PG_PORT=$((FIXTURE_PORT + 2)) TARGET_HASURA_PORT=$((FIXTURE_PORT + 3)) TARGET_AUTH_PORT=$((FIXTURE_PORT + 4)) TARGET_HTTP_PORT=$((FIXTURE_PORT + 5)) TARGET_HTTPS_PORT=$((FIXTURE_PORT + 6)) TARGET_S3_PORT=$((FIXTURE_PORT + 10))
PROJECT=a11nh-$(date +%s)-$RANDOM
TARGET=$W/target
mkdir -p "$TARGET"
cleanup() {
  docker compose -p "$PROJECT-src" -f "$FIXTURE/compose.yml" down -v >/dev/null 2>&1 || true
  if [[ -f "$TARGET/docker-compose.yml" ]]; then (cd "$TARGET" && docker compose down -v >/dev/null 2>&1) || true; fi
}
trap cleanup EXIT
trap 'echo "FAIL at line $LINENO"; cat "$W/last.json" 2>/dev/null || true' ERR
S3AUTH="$FIXTURE_S3_ACCESS:$FIXTURE_S3_SECRET"
SRC_HASURA="http://127.0.0.1:$FIXTURE_HASURA_PORT"
SRC_AUTH="http://127.0.0.1:$FIXTURE_AUTH_PORT"
SRC_STORAGE="http://127.0.0.1:$FIXTURE_STORAGE_PORT"
SRC_S3="http://127.0.0.1:$FIXTURE_S3_PORT"
srcpsql() { PGPASSWORD="$FIXTURE_PASSWORD" psql -h 127.0.0.1 -p "$FIXTURE_PORT" -U postgres -d postgres -X -v ON_ERROR_STOP=1 -At -c "$1"; }
meta() { curl -fsS -H "X-Hasura-Admin-Secret: $FIXTURE_ADMIN_SECRET" -H 'Content-Type: application/json' --data "$1" "$SRC_HASURA/v1/metadata"; }
waitfor() { # description, command...
  local what=$1; shift
  for _ in $(seq 1 90); do "$@" >/dev/null 2>&1 && return 0; sleep 2; done
  echo "FAIL: timed out waiting for $what"; docker compose -p "$PROJECT-src" -f "$FIXTURE/compose.yml" logs --tail 20 || true; exit 1
}

# 1. The nhost stack: postgres, hasura, minio first; the bucket; then hasura-auth and hasura-storage,
#    which create the auth and storage schemas and track them in Hasura.
docker compose -p "$PROJECT-src" -f "$FIXTURE/compose.yml" up -d postgres minio hasura --wait || { docker compose -p "$PROJECT-src" -f "$FIXTURE/compose.yml" logs --tail 30 hasura postgres; exit 1; }
waitfor 'minio' curl -fsS "$SRC_S3/minio/health/live"
curl -fsS -o /dev/null --aws-sigv4 'aws:amz:us-east-1:s3' --user "$S3AUTH" -X PUT "$SRC_S3/$FIXTURE_S3_BUCKET"
docker compose -p "$PROJECT-src" -f "$FIXTURE/compose.yml" up -d auth storage
waitfor 'hasura-auth' curl -fsS "$SRC_AUTH/healthz"
waitfor 'hasura-storage' curl -fsS "$SRC_STORAGE/v1/version"
waitfor 'storage.files' test "$(srcpsql "SELECT to_regclass('storage.files') IS NOT NULL")" = t

# 2. Three users through the real signup route (bcrypt hashes made by hasura-auth itself).
export PW_ALICE PW_BOB PW_CAROL
PW_ALICE=$(rand) PW_BOB=$(rand) PW_CAROL=$(rand)
signup() { curl -fsS -o /dev/null -H 'Content-Type: application/json' --data "$(jq -cn --arg e "$1" --arg p "$2" '{email:$e,password:$p}')" "$SRC_AUTH/signup/email-password"; }
signup alice@example.invalid "$PW_ALICE"
signup bob@example.invalid "$PW_BOB"
signup carol@example.invalid "$PW_CAROL"
test "$(srcpsql 'SELECT count(*) FROM auth.users')" = 3
test "$(srcpsql "SELECT count(*) FROM auth.users WHERE password_hash LIKE '\$2%'")" = 3

# 3. Four stored objects through hasura-storage (different sizes, one binary, one a single byte: hasura-storage refuses empty files).
mkdir -p "$W/up"
printf 'alpha object\n' > "$W/up/alpha.txt"
head -c 70000 /dev/urandom > "$W/up/binary.bin"
printf 'x' > "$W/up/tiny.dat"
python3 -c 'print("unicode ✓ " * 500)' > "$W/up/unicode.txt"
for f in alpha.txt binary.bin tiny.dat unicode.txt; do
  curl -fsS -H "X-Hasura-Admin-Secret: $FIXTURE_ADMIN_SECRET" -F 'bucket-id=default' -F "file[]=@$W/up/$f" "$SRC_STORAGE/v1/files" > "$W/upload-$f.json"
  jq -e '.processedFiles|length==1' "$W/upload-$f.json" >/dev/null
done
test "$(srcpsql 'SELECT count(*) FROM storage.files')" = 4

# 4. Two app tables with foreign keys into auth.users and storage.files, relationships and user permissions.
srcpsql "CREATE TABLE public.profiles (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES auth.users (id), avatar_file_id uuid REFERENCES storage.files (id), bio text);
CREATE TABLE public.todos (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), profile_id uuid NOT NULL REFERENCES public.profiles (id), owner_id uuid NOT NULL REFERENCES auth.users (id), title text NOT NULL, done boolean NOT NULL DEFAULT false);
INSERT INTO public.profiles (user_id, avatar_file_id, bio) SELECT u.id, (SELECT id FROM storage.files ORDER BY name LIMIT 1), 'bio of ' || u.email FROM auth.users u;
INSERT INTO public.todos (profile_id, owner_id, title) SELECT p.id, p.user_id, 'task ' || n FROM public.profiles p, generate_series(1, 3) n;" >/dev/null
for t in profiles todos; do meta "{\"type\":\"pg_track_table\",\"args\":{\"source\":\"default\",\"table\":{\"schema\":\"public\",\"name\":\"$t\"}}}" >/dev/null; done
meta '{"type":"bulk","args":[
 {"type":"pg_create_object_relationship","args":{"source":"default","table":{"schema":"public","name":"todos"},"name":"profile","using":{"foreign_key_constraint_on":"profile_id"}}},
 {"type":"pg_create_array_relationship","args":{"source":"default","table":{"schema":"public","name":"profiles"},"name":"todos","using":{"foreign_key_constraint_on":{"table":{"schema":"public","name":"todos"},"column":"profile_id"}}}},
 {"type":"pg_create_select_permission","args":{"source":"default","table":{"schema":"public","name":"profiles"},"role":"user","permission":{"columns":["id","user_id","avatar_file_id","bio"],"filter":{"user_id":{"_eq":"X-Hasura-User-Id"}}}}},
 {"type":"pg_create_select_permission","args":{"source":"default","table":{"schema":"public","name":"todos"},"role":"user","permission":{"columns":["id","profile_id","owner_id","title","done"],"filter":{"owner_id":{"_eq":"X-Hasura-User-Id"}}}}},
 {"type":"pg_create_update_permission","args":{"source":"default","table":{"schema":"public","name":"todos"},"role":"user","permission":{"columns":["done"],"filter":{"owner_id":{"_eq":"X-Hasura-User-Id"}},"check":null}}}]}' >/dev/null
srcpsql "SELECT count(*) FROM auth.users" > "$W/src-users.txt"
export SRC_URL="postgresql://postgres:$FIXTURE_PASSWORD@127.0.0.1:$FIXTURE_PORT/postgres"
export SRC_SECRET=$FIXTURE_ADMIN_SECRET SRC_S3_ACCESS=$FIXTURE_S3_ACCESS SRC_S3_SECRET=$FIXTURE_S3_SECRET

# 5. The target: a started nSelf project with auth and an object store.
cd "$TARGET"
nself init --non-interactive
python3 - "$PROJECT" <<'PY'
import os, pathlib, secrets, sys
p=pathlib.Path('.env')
with p.open('a') as f:
    for k,v in {'PROJECT_NAME':sys.argv[1], 'POSTGRES_DB':'nself', 'POSTGRES_USER':'postgres', 'POSTGRES_VERSION':'16', 'POSTGRES_PORT':os.environ['TARGET_PG_PORT'], 'HASURA_PORT':os.environ['TARGET_HASURA_PORT'], 'AUTH_PORT':os.environ['TARGET_AUTH_PORT'], 'NGINX_HTTP_PORT':os.environ['TARGET_HTTP_PORT'], 'NGINX_HTTPS_PORT':os.environ['TARGET_HTTPS_PORT'], 'PGVECTOR_ENABLED':'false', 'POSTGRES_EXTENSIONS':'', 'POSTGRES_PASSWORD':secrets.token_hex(24), 'HASURA_GRAPHQL_ADMIN_SECRET':secrets.token_hex(24), 'AUTH_ENABLED':'true', 'REDIS_ENABLED':'false', 'MAILPIT_ENABLED':'false',
                 'MINIO_ENABLED':'false', 'OBJECT_STORAGE_ENABLED':'true', 'OBJECT_STORAGE_BUCKETS':'target-default', 'SEAWEEDFS_PORT':os.environ['TARGET_S3_PORT'], 'S3_ACCESS_KEY':secrets.token_hex(12), 'S3_SECRET_KEY':secrets.token_hex(24)}.items():
        f.write(f'\n{k}={v}\n')
p.chmod(0o600)
PY
nself start
TARGET_AUTH="http://127.0.0.1:$TARGET_AUTH_PORT"
TARGET_S3="http://127.0.0.1:$TARGET_S3_PORT"
T_S3A=$(sed -n 's/^S3_ACCESS_KEY=//p' .env | tail -1)
T_S3S=$(sed -n 's/^S3_SECRET_KEY=//p' .env | tail -1)
T_SECRET=$(sed -n 's/^HASURA_GRAPHQL_ADMIN_SECRET=//p' .env | tail -1)
gql() { # role, user id, query
  curl -fsS -H "X-Hasura-Admin-Secret: $T_SECRET" -H "X-Hasura-Role: $1" -H "X-Hasura-User-Id: $2" -H 'Content-Type: application/json' --data "$(jq -cn --arg q "$3" '{query:$q}')" "http://127.0.0.1:$TARGET_HASURA_PORT/v1/graphql" | tee "$W/last.json"
}

# 6. Refusals before any write: S3 flags missing while the source holds files, a wrong admin secret.
if nself db import nhost --db-url-env SRC_URL --hasura-url "$SRC_HASURA" --admin-secret-env SRC_SECRET --plan --json > "$W/noflags.json" 2> "$W/noflags.err"; then echo 'FAIL: files without S3 flags accepted'; exit 1; fi
grep -q E517 "$W/noflags.json" "$W/noflags.err"
SAVED=$SRC_SECRET; export SRC_SECRET=wrong-secret-for-the-refusal-check
if nself db import nhost --db-url-env SRC_URL --hasura-url "$SRC_HASURA" --admin-secret-env SRC_SECRET --storage-endpoint "$SRC_S3" --s3-access-key-env SRC_S3_ACCESS --s3-secret-env SRC_S3_SECRET --bucket "$FIXTURE_S3_BUCKET" --plan --json > "$W/badsecret.json" 2> "$W/badsecret.err"; then echo 'FAIL: wrong admin secret accepted'; exit 1; fi
grep -q E517 "$W/badsecret.json" "$W/badsecret.err"
export SRC_SECRET=$SAVED
test ! -d .nself/import
IMPORT=(nself db import nhost --db-url-env SRC_URL --hasura-url "$SRC_HASURA" --admin-secret-env SRC_SECRET --storage-endpoint "$SRC_S3" --s3-access-key-env SRC_S3_ACCESS --s3-secret-env SRC_S3_SECRET --bucket "$FIXTURE_S3_BUCKET")
# A user with a second factor is refused up front: the factor is not carried, so the import would
# silently let that user sign in with the password alone.
srcpsql "UPDATE auth.users SET active_mfa_type = 'totp' WHERE email = 'carol@example.invalid'" > /dev/null
if "${IMPORT[@]}" --plan --json > "$W/mfa.json" 2> "$W/mfa.err"; then echo 'FAIL: a source user with MFA was accepted'; exit 1; fi
grep -q E517 "$W/mfa.json" "$W/mfa.err"
grep -q 'MFA' "$W/mfa.json" "$W/mfa.err"
srcpsql "UPDATE auth.users SET active_mfa_type = NULL WHERE email = 'carol@example.invalid'" > /dev/null
"${IMPORT[@]}" --plan --json > "$W/plan.json"
jq -e '.data.result=="pass" and (.data.steps|length)>0 and .data.detection.kind=="nhost" and .data.detection.source_users==3' "$W/plan.json"
test ! -d .nself/import

# D-0723: an object already in the target store under a bundle key is refused before any write, and
# the target's own object is left as it was.
srcpsql 'SELECT id FROM storage.files ORDER BY id' > "$W/file-ids.txt"
test "$(wc -l < "$W/file-ids.txt")" -eq 4
FIRST=$(head -1 "$W/file-ids.txt")
curl -fsS -o /dev/null --aws-sigv4 'aws:amz:us-east-1:s3' --user "$T_S3A:$T_S3S" -X PUT "$TARGET_S3/$FIXTURE_S3_BUCKET"
printf 'target-owned object' | curl -fsS -o /dev/null --aws-sigv4 'aws:amz:us-east-1:s3' --user "$T_S3A:$T_S3S" -X PUT --data-binary @- "$TARGET_S3/$FIXTURE_S3_BUCKET/$FIRST"
if "${IMPORT[@]}" --json > "$W/seeded.json" 2> "$W/seeded.err"; then echo 'FAIL: import over an existing target object accepted'; exit 1; fi
grep -q E519 "$W/seeded.json" "$W/seeded.err"
grep -q 'already has an object' "$W/seeded.json" "$W/seeded.err"
jq -e '.wrote==false' "$(ls -dt .nself/import/*/report.json | head -1)"
test "$(PGPASSWORD="$(sed -n 's/^POSTGRES_PASSWORD=//p' .env | tail -1)" psql -h 127.0.0.1 -p "$TARGET_PG_PORT" -U postgres -d nself -X -At -c 'SELECT count(*) FROM auth.users')" = 0
test "$(curl -fsS --aws-sigv4 'aws:amz:us-east-1:s3' --user "$T_S3A:$T_S3S" "$TARGET_S3/$FIXTURE_S3_BUCKET/$FIRST")" = 'target-owned object'
curl -fsS -o /dev/null --aws-sigv4 'aws:amz:us-east-1:s3' --user "$T_S3A:$T_S3S" -X DELETE "$TARGET_S3/$FIXTURE_S3_BUCKET/$FIRST"
rm -rf .nself/import

# 7. The import: zero mismatches, users with bcrypt, objects with sha256, FK tables, metadata.
"${IMPORT[@]}" --keep-stage --json > "$W/import.json"
jq -e '.data.result=="pass" and (.data.mismatches|length)==0 and ([.data.tables[].equal]|all)' "$W/import.json"
jq -e '[.data.tables[]|select(.schema=="public")|.name]|sort==["profiles","todos"]' "$W/import.json"
jq -e '.data.users.source==3 and .data.users.target==3 and .data.users.equal==true and (.data.users.reset_required|length)==0' "$W/import.json"
jq -e '.data.storage.source_objects==4 and .data.storage.target_objects==4 and .data.storage.equal==true' "$W/import.json"
jq -e '.data.hasura.equal==true and (.data.hasura.diffs|length)==0' "$W/import.json"
BUNDLE=$(ls -d .nself/import/*/stage | head -1)
jq -e '.producer.source=="nhost" and .auth.users==3 and .auth.hash_algorithms.bcrypt==3 and (.storage.objects|length)==4 and (.storage.objects|all(.sha256|length==64))' "$BUNDLE/manifest.json"
jq -e '[.source_counts[]|select(.kind=="users")|.count]==[3] and [.source_counts[]|select(.kind=="objects")|.count]==[4]' "$BUNDLE/manifest.json"
jq -se 'all(.[]; .hash_algorithm=="bcrypt")' "$BUNDLE/auth/users.jsonl"
test "$(jq -r '.storage.objects|map(.bytes)|add' "$BUNDLE/manifest.json")" = "$(( $(wc -c < "$W/up/alpha.txt") + 70000 + $(wc -c < "$W/up/tiny.dat") + $(wc -c < "$W/up/unicode.txt") ))"

# 8. A migrated user signs in against the target hasura-auth with the original password.
signin() { curl -sS -o "$W/signin.json" -w '%{http_code}' -H 'Content-Type: application/json' --data "$(jq -cn --arg e "$1" --arg p "$2" '{email:$e,password:$p}')" "$TARGET_AUTH/signin/email-password"; }
test "$(signin alice@example.invalid "$PW_ALICE")" = 200
jq -e '.session.accessToken|length>20' "$W/signin.json"
test "$(signin bob@example.invalid "$PW_BOB")" = 200
test "$(signin carol@example.invalid "$PW_CAROL")" = 200
test "$(signin alice@example.invalid "not-the-password")" = 401

# 9. Every object is in the target store byte for byte, and the app tables behave as before.
while read -r id; do
  curl -fsS --aws-sigv4 'aws:amz:us-east-1:s3' --user "$S3AUTH" "$SRC_S3/$FIXTURE_S3_BUCKET/$id" -o "$W/src-object"
  curl -fsS --aws-sigv4 'aws:amz:us-east-1:s3' --user "$T_S3A:$T_S3S" "$TARGET_S3/$FIXTURE_S3_BUCKET/$id" -o "$W/target-object"
  cmp "$W/src-object" "$W/target-object"
done < "$W/file-ids.txt"
ALICE=$(srcpsql "SELECT id FROM auth.users WHERE email='alice@example.invalid'")
gql user "$ALICE" '{ profiles { bio todos(order_by: {title: asc}) { title } } }' | jq -e '(.errors|not) and (.data.profiles|length)==1 and (.data.profiles[0].todos|length)==3'
gql user "$ALICE" '{ todos { profile { bio } } }' | jq -e '(.errors|not) and (.data.todos|length)==3'
gql user "$ALICE" '{ todos { id } }' | jq -e '.data.todos|length==3'

# 10. A second import is refused (E519) and changes nothing; no secret reached any output.
if "${IMPORT[@]}" --json > "$W/rerun.json" 2> "$W/rerun.err"; then echo 'FAIL: rerun did not refuse'; exit 1; fi
grep -q E519 "$W/rerun.json" "$W/rerun.err"
python3 - "$W" <<'PY'
import os,pathlib,sys
# Logs, reports and the whole kept bundle except the binary dump: no credential may appear in any of them
# (bcrypt hashes in auth/users.jsonl are expected and are not in this list).
paths=[p for p in pathlib.Path(sys.argv[1]).rglob('*') if p.is_file() and p.parent.name != 'up' and p.name not in ('data.dump','src-object','target-object') and (p.suffix in ('.json','.err','.jsonl','.txt','.sql') or 'stage' in p.parts)]
secrets=[os.environ[k].encode() for k in ('FIXTURE_PASSWORD','FIXTURE_ADMIN_SECRET','FIXTURE_S3_ACCESS','FIXTURE_S3_SECRET','FIXTURE_JWT_KEY','PW_ALICE','PW_BOB','PW_CAROL')]
assert paths
for p in paths:
    data=p.read_bytes()
    for s in secrets: assert s not in data, 'credential appeared in output ' + p.name
print('PASS redaction; files read=',len(paths))
PY
echo 'PASS Linux nhost import: bcrypt sign-in, objects byte for byte, FK tables, counts, metadata, refusals, redaction'
