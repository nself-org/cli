#!/usr/bin/env bash
# Purpose: controls for scripts/ci/sdk-remote-mutation.sh. A synthetic tree
#          with a fake scripts/mutation.sh checks header parsing, the
#          file-set check and exit propagation; a last case dry-runs the real
#          repo so the committed header must name the real package files.
# Inputs:  none.
# Outputs: "ok <case>" per passing case; exit 0 when all pass, exit 1 on the
#          first mismatch.
# Constraints: bash 3.2 compatible; the synthetic cases are offline
#          (stdlib-only package, no requirements); needs the go toolchain.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REAL_ROOT="$(cd "${HERE}/../.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

HDR="# Equivalent mutants for: bash scripts/mutation.sh sdk/go/remote --only"
TAIL="--allow remote/testdata/mutation-allow.txt"

# fixture DIR ONLY: a tree whose remote package compiles a.go and b.go on
# linux (other.go is !unix, a_test.go is a test), with ONLY in the header.
fixture() {
  local m="$1" only="$2" r="$1/sdk/go/remote"
  mkdir -p "${m}/scripts/ci" "${r}/testdata"
  cp "${HERE}/sdk-remote-mutation.sh" "${m}/scripts/ci/"
  cat > "${m}/scripts/mutation.sh" <<'FAKE'
#!/usr/bin/env bash
# Fake gate: record argv and GOFLAGS, exit as told.
printf 'GOFLAGS=%s ARGS=%s\n' "${GOFLAGS:-}" "$*" > "$(dirname "$0")/../fake-called.txt"
exit "${FAKE_EXIT:-0}"
FAKE
  printf 'module example.com/sdk\n\ngo 1.21\n' > "${m}/sdk/go/go.mod"
  printf 'package remote\n\nfunc A() {}\n' > "${r}/a.go"
  printf 'package remote\n\nfunc B() {}\n' > "${r}/b.go"
  printf '//go:build !unix\n\npackage remote\n\nfunc Other() {}\n' > "${r}/other.go"
  printf 'package remote\n\nimport "testing"\n\nfunc TestA(t *testing.T) { A() }\n' > "${r}/a_test.go"
  printf '%s %s %s\nARITHMETIC_BASE at a.go:3:1 | synthetic\n' "${HDR}" "${only}" "${TAIL}" > "${r}/testdata/mutation-allow.txt"
}

# expect CASE WANT_EXIT DIR NEEDLE [ARGS...]: run the script, compare exit and output.
expect() {
  local name="$1" want="$2" dir="$3" needle="$4" got=0
  shift 4
  (cd "${dir}" && GOWORK=off bash scripts/ci/sdk-remote-mutation.sh "$@") \
    > "${WORK}/${name}.out" 2>&1 || got=$?
  if [ "${got}" -ne "${want}" ]; then
    echo "FAIL ${name}: exit ${got}, want ${want}" >&2
    cat "${WORK}/${name}.out" >&2
    exit 1
  fi
  if [ -n "${needle}" ] && ! grep -qF -- "${needle}" "${WORK}/${name}.out"; then
    echo "FAIL ${name}: output lacks ${needle}" >&2
    cat "${WORK}/${name}.out" >&2
    exit 1
  fi
  echo "ok ${name}"
}

d="${WORK}/dry"; fixture "${d}" "a.go,b.go"
expect dry-run 0 "${d}" "GOFLAGS=-mod=mod bash scripts/mutation.sh sdk/go/remote --only a.go,b.go ${TAIL}" --dry-run
[ ! -e "${d}/fake-called.txt" ] || { echo "FAIL dry-run: the gate ran" >&2; exit 1; }

d="${WORK}/run"; fixture "${d}" "b.go,a.go"
expect run-passes 0 "${d}" ""
grep -qxF "GOFLAGS=-mod=mod ARGS=sdk/go/remote --only b.go,a.go ${TAIL}" "${d}/fake-called.txt" \
  || { echo "FAIL run-passes: gate argv: $(cat "${d}/fake-called.txt")" >&2; exit 1; }
echo "ok run-argv"

d="${WORK}/lived"; fixture "${d}" "a.go,b.go"
FAKE_EXIT=1 expect lived-mutant-fails 1 "${d}" ""

d="${WORK}/newfile"; fixture "${d}" "a.go,b.go"
printf 'package remote\n\nfunc C() {}\n' > "${d}/sdk/go/remote/c.go"
expect new-file-not-in-set 1 "${d}" "< c.go"

d="${WORK}/stale"; fixture "${d}" "a.go,b.go,gone.go"
expect stale-entry-in-set 1 "${d}" "> gone.go"

d="${WORK}/header"; fixture "${d}" "a.go,b.go"
printf '# some other comment\n' > "${d}/sdk/go/remote/testdata/mutation-allow.txt"
expect header-missing 1 "${d}" "is not the gate command"

d="${WORK}/empty"; fixture "${d}" ""
expect empty-set 1 "${d}" "empty or holds a space"

d="${WORK}/noallow"; fixture "${d}" "a.go,b.go"
rm "${d}/sdk/go/remote/testdata/mutation-allow.txt"
expect allowlist-missing 1 "${d}" "is missing"

d="${WORK}/usage"; fixture "${d}" "a.go,b.go"
expect bad-flag 2 "${d}" "usage:" --bogus

# The committed header must cover the real package (needs the module cache).
expect real-repo-dry-run 0 "${REAL_ROOT}" "scripts/mutation.sh sdk/go/remote --only" --dry-run

echo "sdk-remote-mutation_test: all cases passed"
