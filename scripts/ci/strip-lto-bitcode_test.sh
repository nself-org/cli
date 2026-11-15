#!/usr/bin/env bash
# Purpose: prove both sections are removed from every archive member.
# Inputs: optional script path. Outputs: control assertions, nonzero on failure.
# Constraints: Linux binutils or macOS LLVM, a C compiler, disposable scratch.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="${1:-$HERE/strip-lto-bitcode.sh}"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/strip-lto-test.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
NM_ARGS=(-s)
if [ "$(uname -s)" = Darwin ]; then
  OBJCOPY=llvm-objcopy
  OBJDUMP=llvm-objdump
  NM=llvm-nm
  # Compare native symbols, rather than asking nm to read the planted bitcode.
  NM_ARGS=(--no-llvm-bc --print-armap)
  # Match the vendored Darwin archive's unsorted __.SYMDEF index format.
  AR=llvm-ar
  BITCODE=__LLVM,__bitcode
  CMDLINE=__LLVM,__cmdline
  SECTIONS='__bitcode|__cmdline'
elif command -v objcopy >/dev/null 2>&1; then
  OBJCOPY=objcopy
else
  OBJCOPY=llvm-objcopy
fi
OBJDUMP=${OBJDUMP:-objdump}
NM=${NM:-nm}
AR=${AR:-ar}
BITCODE=${BITCODE:-.llvmbc}
CMDLINE=${CMDLINE:-.llvmcmd}
SECTIONS=${SECTIONS:-'\.llvmbc|\.llvmcmd'}
printf 'payload' > "$WORK/section"
for member in one two; do
  printf 'int %s(void) { return 0; }\n' "$member" | "${CC:-cc}" -x c -c -o "$WORK/$member.o" -
done
"$AR" rcs "$WORK/clean.a" "$WORK/one.o" "$WORK/two.o"
cp "$WORK/clean.a" "$WORK/clean-before.a"
for member in one two; do
  "$OBJCOPY" --add-section "$BITCODE=$WORK/section" --add-section "$CMDLINE=$WORK/section" "$WORK/$member.o"
done
"$AR" rcs "$WORK/lto.a" "$WORK/one.o" "$WORK/two.o"
"$AR" t "$WORK/lto.a" > "$WORK/members-before"
"$NM" "${NM_ARGS[@]}" "$WORK/lto.a" > "$WORK/symbols-before"
bash "$SCRIPT" "$WORK/lto.a" "$WORK/clean.a"
"$AR" t "$WORK/lto.a" > "$WORK/members-after"
"$NM" "${NM_ARGS[@]}" "$WORK/lto.a" > "$WORK/symbols-after"
cmp "$WORK/members-before" "$WORK/members-after"
cmp "$WORK/symbols-before" "$WORK/symbols-after"
cmp "$WORK/clean-before.a" "$WORK/clean.a"
for member in one two; do
  "$AR" p "$WORK/lto.a" "$member.o" > "$WORK/stripped.o"
  if "$OBJDUMP" -h "$WORK/stripped.o" | grep -Eq "$SECTIONS"; then
    echo "FAIL: section remains in $member" >&2
    exit 1
  fi
done
if bash "$SCRIPT" "$WORK/missing.a" > "$WORK/missing.log" 2>&1; then
  echo "FAIL: missing archive accepted" >&2
  exit 1
fi
grep -F 'archive does not exist' "$WORK/missing.log"
echo "strip-lto-bitcode_test: all cases passed"
