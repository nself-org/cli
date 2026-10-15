#!/usr/bin/env bash
# Exercise the conversion harness with controlled CLI output and real schemas.
set -euo pipefail
cd "$(dirname "$0")/../.."
tmp="$(mktemp -d "${TMPDIR:-/tmp}/json-conversion-test.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin"
# The harness serializes compilation with BSD lockf; this isolated test also
# runs on Linux, where its single build needs no serialization.
printf '#!/bin/sh\nshift 2\nexec "$@"\n' > "$tmp/bin/lockf"
cat > "$tmp/cli" <<'CLI'
#!/bin/sh
case " $* " in
  *' --json '*)
    [ "${NSELF_V15:-}" = 1 ] || exit 99
    printf '%s' "$TEST_JSON"
    exit "$TEST_RC" ;;
esac
printf 'human failure\n' >&2
exit 1
CLI
chmod +x "$tmp/cli" "$tmp/bin/lockf"
export PATH="$tmp/bin:$PATH" JCC_HEAD_BIN="$tmp/cli" JCC_BASE_BIN="$tmp/cli"
export JCC_ARGV_FILE="$tmp/argv"
valid='{"schema_version":"1","command":"stop","error":{"code":"E002","class":"infra","message":"offline","exit_code":2}}'
pass=0
check() {
  local name="$1" want="$2" rc=0
  export TEST_JSON="$3" TEST_RC="$4"
  printf 'stop #| %s\n' "${5:-error-json=E002}" > "$JCC_ARGV_FILE"
  bash scripts/ci/json-conversion-check.sh lifecycle > "$tmp/result" 2>&1 || rc=$?
  if { [ "$want" = pass ] && [ "$rc" -ne 0 ]; } || { [ "$want" = fail ] && [ "$rc" -eq 0 ]; }; then
    echo "FAIL $name: expected $want, exit $rc"
    cat "$tmp/result"
    exit 1
  fi
  echo "PASS $name"
  pass=$((pass+1))
}
check 'valid error envelope' pass "$valid" 2
check 'successful exit with error' fail "$valid" 0
check 'wrong error code' fail "${valid/E002/E003}" 2
check 'multiple documents' fail "$valid $valid" 2
check 'missing schema version' fail "${valid/\"schema_version\":\"1\",/}" 2
check 'invalid error shape' fail '{"schema_version":"1","command":"stop","error":{"code":"E002"}}' 2
check 'data and error' fail "${valid/\"command\":/\"data\":{},\"command\":}" 2
check 'missing document' fail '' 2
check 'non-JSON stdout' fail 'human failure' 2
check 'error tag overrides no-json' fail '' 2 'no-json=fixture error-json=E002'
echo "PASS summary: $pass harness cases"
