#!/usr/bin/env bash
# Purpose: remove LTO-only sections from job-local wasmtime archives.
# Inputs: archive paths. Outputs: before/after SHA-256; nonzero on failure.
# Constraints: machine code stays intact; never commit the stripped archives.
set -euo pipefail

if command -v objcopy >/dev/null 2>&1; then
  OBJCOPY=objcopy
elif command -v llvm-objcopy >/dev/null 2>&1; then
  OBJCOPY=llvm-objcopy
else
  echo "strip-lto: objcopy or llvm-objcopy is required" >&2
  exit 1
fi

if [ "$#" -eq 0 ]; then
  echo "usage: strip-lto-bitcode.sh ARCHIVE..." >&2
  exit 1
fi

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1"
  else
    shasum -a 256 "$1"
  fi
}

has_bitcode() { LC_ALL=C grep -aEq '\.llvmbc|\.llvmcmd' "$1"; }

for archive in "$@"; do
  if [ ! -f "$archive" ]; then
    echo "strip-lto: archive does not exist: $archive" >&2
    exit 1
  fi
  echo "before: $(sha256 "$archive")"
  if has_bitcode "$archive"; then
    "$OBJCOPY" --remove-section=.llvmbc --remove-section=.llvmcmd "$archive"
  fi
  if has_bitcode "$archive"; then
    echo "strip-lto: bitcode section remains in $archive" >&2
    exit 1
  fi
  echo "after:  $(sha256 "$archive")"
done
