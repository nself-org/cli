#!/usr/bin/env bash
# Purpose: prove v1.5 core SeaweedFS boot, bucket bootstrap, S3 coverage,
#          secret handling and E138's refusal before any project write.
# Usage:   object-storage.sh SUITE_DIR
# Env:     TMPDIR optional scratch parent (default /tmp).
# Needs:   Go, openssl, Docker socket and permission to run privileged dind.
# Outputs: PASS/FAIL lines, including the S3 suite's per-test results.
# Exit:    0 PASS, 1 FAIL, 2 usage, 3 missing precondition.
# Constraints: bash 3.2 compatible. Builds static binaries and copies them to
#          a unique sibling dind container, so host bind paths are unnecessary.
#          Random credentials travel only in a 0600 file, never on argv.
#          Removes the sibling and its anonymous volumes on every exit.
set -euo pipefail

[ "$#" = 1 ] || { echo "usage: object-storage.sh SUITE_DIR" >&2; exit 2; }
SUITE_DIR="$(cd "$1" && pwd)" || { echo "FAIL: S3 coverage suite directory missing"; exit 3; }
[ -f "$SUITE_DIR/main_test.go" ] || { echo "FAIL: S3 coverage suite missing"; exit 3; }
REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
if ! command -v docker >/dev/null 2>&1; then
  if [ "$(id -u)" = 0 ] && command -v apt-get >/dev/null 2>&1; then
    apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq docker.io >/dev/null 2>&1 || true
  fi
fi
for tool in docker go openssl; do
  command -v "$tool" >/dev/null 2>&1 || { echo "FAIL: missing $tool"; exit 3; }
done
docker info >/dev/null 2>&1 || { echo "FAIL: Docker unavailable"; exit 3; }
WORK="$(mktemp -d "${TMPDIR:-/tmp}/object-storage.XXXXXX")"
SIBLING="p60-dind-$(openssl rand -hex 6)"
DIND_IMAGE='docker:28-dind@sha256:2a232a42256f70d78e3cc5d2b5d6b3276710a0de0596c145f627ecfae90282ac'
cleanup() {
  docker rm -f -v "$SIBLING" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT
fail() { echo "FAIL: $1"; exit 1; }

# golang images set GOTOOLCHAIN=local; if go.mod needs a newer go, let go fetch it.
version_lt() { # true when dotted version $1 is older than $2 (missing parts are 0)
  awk -v a="$1" -v b="$2" 'BEGIN {
    na = split(a, x, "."); nb = split(b, y, ".");
    for (i = 1; i <= 3; i++) {
      if (x[i] + 0 < y[i] + 0) exit 0
      if (x[i] + 0 > y[i] + 0) exit 1
    }
    exit 1 }'
}
GO_HAVE="$(GOTOOLCHAIN=local go env GOVERSION | sed 's/^go//')"
for gomod in "$REPO_ROOT/go.mod" "$SUITE_DIR/../../go.mod"; do
  [ -f "$gomod" ] || continue
  GO_NEED="$(awk '$1 == "go" { print $2; exit }' "$gomod")"
  if version_lt "$GO_HAVE" "$GO_NEED"; then
    export GOTOOLCHAIN=auto
    echo "INFO: go $GO_HAVE is older than $gomod (go $GO_NEED); GOTOOLCHAIN=auto fetches the required toolchain"
  fi
done
echo "INFO: toolchain in use: $(cd "$REPO_ROOT" && go version)"
(cd "$REPO_ROOT" && CGO_ENABLED=0 go build -mod=vendor -o "$WORK/nself" ./cmd/nself) || fail "CLI build"
(cd "$SUITE_DIR/../.." && CGO_ENABLED=0 go test -mod=vendor -c -tags s3coverage \
  -o "$WORK/s3cov.test" ./test/s3coverage) || fail "S3 coverage build"
umask 077
{
  printf 'S3_ACCESS_KEY=%s\n' "$(openssl rand -hex 12)"
  printf 'S3_SECRET_KEY=%s\n' "$(openssl rand -hex 24)"
  printf 'POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 24)"
} >"$WORK/secrets.env"

cat >"$WORK/run.sh" <<'RUNNER'
#!/usr/bin/env bash
set -euo pipefail
umask 022 # known CLI debt D-0664: nself build fails under umask 077 (E452); secret files get chmod 600 below
export DOCKER_HOST=unix:///var/run/docker.sock NSELF_V15=1
export NSELF_PLUGIN_DIR=/work/plugins
mkdir -p "$HOME" "$NSELF_PLUGIN_DIR" /work/project
fail() { echo "FAIL: $1"; exit 1; }
set -a
# shellcheck disable=SC1091 # copied private per-run file
source /work/secrets.env
set +a
cd /work/project
/work/nself init --non-interactive --name p60-store --no-pgvector >/work/init.log 2>&1 || { tail -40 /work/init.log; fail "init"; }
printf '\nOBJECT_STORAGE_ENABLED=true\nOBJECT_STORAGE_BUCKETS=p60-uploads,p60-archives\n' >>.env
printf 'S3_ACCESS_KEY=%s\nS3_SECRET_KEY=%s\n' "$S3_ACCESS_KEY" "$S3_SECRET_KEY" >>.env.secrets
chmod 600 .env.secrets
/work/nself build >/work/build.log 2>&1 || { tail -40 /work/build.log; fail "build"; }
/work/nself start >/work/start.log 2>&1 || { tail -40 /work/start.log; fail "start"; }
compose() { docker compose --env-file .nself/compose.env -f docker-compose.yml "$@"; }
store="$(compose ps -q seaweedfs)"
[ -n "$store" ] || fail "SeaweedFS container missing"
n=0
until [ "$(docker inspect -f '{{.State.Health.Status}}' "$store")" = healthy ]; do
  n=$((n + 1)); [ "$n" -le 120 ] || fail "SeaweedFS health timeout"
  sleep 2
done
echo 'PASS: SeaweedFS healthy (wget /healthz)'
# nself start force-removes init-container-labelled containers (docker.RunPostStartCleanup), so the
# one-shot seaweedfs-init may be gone before it can be inspected. Only that service creates these
# buckets, so seeing both listed proves the bootstrap ran.
n=0
until printf 's3.bucket.list\n' | docker exec -i "$store" weed shell -master=127.0.0.1:9333 >/work/buckets.out 2>&1 \
  && awk '{print $1}' /work/buckets.out >/work/bucket-names \
  && grep -Fqx p60-uploads /work/bucket-names && grep -Fqx p60-archives /work/bucket-names; do
  n=$((n + 1)); [ "$n" -le 120 ] || fail "bucket init timeout"
  sleep 2
done
echo 'PASS: both configured buckets exist'
# Bootstrap is idempotent against the already populated store.
compose run --rm seaweedfs-init >/work/init-again.log 2>&1 || fail "bucket bootstrap rerun"
echo 'PASS: bucket bootstrap rerun'

DB_IMAGE='postgres:16@sha256:ca0bd484cb98bf4b24eb1010e73fb3fcbd6714d240fbc1a10eea5b7dbecb641d'
printf 'POSTGRES_USER=s3cov\nPOSTGRES_DB=s3cov\nPOSTGRES_PASSWORD=%s\n' "$POSTGRES_PASSWORD" >/work/db.env
chmod 600 /work/db.env
docker run -d --name p60-suite-db --env-file /work/db.env -p 127.0.0.1:15432:5432 "$DB_IMAGE" >/dev/null
export S3COV_ENDPOINT=http://127.0.0.1:8333 S3COV_TARGET=seaweedfs S3COV_REGION=us-east-1
export S3COV_ACCESS_KEY="$S3_ACCESS_KEY" S3COV_SECRET_KEY="$S3_SECRET_KEY"
export S3COV_DB_URL="postgres://s3cov:${POSTGRES_PASSWORD}@127.0.0.1:15432/s3cov?sslmode=disable"
/work/s3cov.test -test.list '^Test(Op|Plugin)_' >/work/suite-list
expected="$(grep -c '^Test' /work/suite-list || true)"
[ "$expected" -gt 0 ] || fail "coverage suite empty"
# Test errors can include signed URLs. Redact credentials without placing them
# on a process command line, even when printing a failed suite's output.
suite_output() {
  awk '{gsub(ENVIRON["S3_ACCESS_KEY"], "[access-key]");
    gsub(ENVIRON["S3_SECRET_KEY"], "[secret-key]");
    gsub(ENVIRON["POSTGRES_PASSWORD"], "[database-password]"); print}' /work/suite.out
}
install -m 600 /dev/null /work/suite.out
/work/s3cov.test -test.v -test.count=1 -test.timeout=25m >/work/suite.out 2>&1 || {
  suite_output; fail "S3 coverage suite";
}
suite_output
passed="$(grep -cE '^--- PASS: Test(Op|Plugin)_' /work/suite.out || true)"
[ "$passed" = "$expected" ] || fail "coverage test count mismatch"
echo 'PASS: S3 coverage suite'

[ -z "$(find . -name s3.json -print)" ] || fail "identity JSON in project"
printf '%s\n%s\n' "$S3_ACCESS_KEY" "$S3_SECRET_KEY" >/work/key-patterns
chmod 600 /work/key-patterns
if grep -Fq -f /work/key-patterns docker-compose.yml; then fail "credential in compose"; fi
# The designated secret stores may hold the keys; nothing else under the project may.
leaks="$(grep -rlF -f /work/key-patterns . --exclude=.env --exclude=.env.secrets --exclude=.env.local --exclude=compose.env 2>/dev/null || true)"
if [ -n "$leaks" ]; then echo "$leaks"; fail "credential outside the secret files"; fi
echo 'PASS: identity absent from project and credentials absent from compose'
sed '/^OBJECT_STORAGE_ENABLED=/d' .env >/work/env-next
printf 'MINIO_ENABLED=true\n' >>/work/env-next
mv /work/env-next .env
unset OBJECT_STORAGE_ENABLED
project_hash() {
  # Include directories, permissions and symlink targets as well as file bytes.
  # Skip .nself/op.lock: the build lock rewrites its own state file on every attempt, even a refused one.
  { find . -path ./.nself/op.lock -prune -o -exec stat -c '%a %F %N' {} \; ;
    find . -path ./.nself/op.lock -prune -o -type f -exec sha256sum {} \; ; } \
    | LC_ALL=C sort | sha256sum
}
before="$(project_hash)"
set +e
/work/nself build >/work/e138.out 2>&1
code=$?
set -e
[ "$code" = 1 ] || fail "E138 exit was $code"
grep -q E138 /work/e138.out && grep -q 'nself add minio' /work/e138.out || fail "E138 remedy missing"
[ "$(project_hash)" = "$before" ] || fail "E138 changed project files"
echo 'PASS: E138 exits 1 with remedy and changes no project files'
echo PASS
RUNNER

docker run -d --privileged --name "$SIBLING" -e DOCKER_TLS_CERTDIR= "$DIND_IMAGE" >/dev/null || fail "dind start"
n=0
until docker exec "$SIBLING" docker info >/dev/null 2>&1; do
  n=$((n + 1)); [ "$n" -le 60 ] || fail "dind daemon timeout"
  sleep 1
done
docker exec "$SIBLING" apk add --no-cache bash curl openssl jq >/dev/null || fail "dind dependencies"
docker exec "$SIBLING" mkdir -p /work
for file in nself s3cov.test secrets.env run.sh; do
  docker cp "$WORK/$file" "$SIBLING:/work/$file"
done
docker exec "$SIBLING" chmod 600 /work/secrets.env
docker exec "$SIBLING" bash /work/run.sh || fail "generated object store flow"
