#!/usr/bin/env bash
# sdk-tag.sh - map a CLI release version to the Go SDK tag / module version.
#
# The Go SDK module path ends in /v2 (github.com/nself-org/cli/sdk/go/v2), so Go
# only resolves tags of the form `sdk/go/v2.M.P` (directory prefix `sdk/go/`,
# semantic version v2.M.P). CLI `v1.M.P` therefore publishes SDK `v2.M.P`.
#
# Usage:
#   sdk-tag.sh <cli-version>             prints the git tag, e.g. sdk/go/v2.5.0
#   sdk-tag.sh --version <cli-version>   prints the module version, e.g. v2.5.0
#
# <cli-version> is `v1.M.P` (leading v optional), with an optional pre-release
# suffix (`v1.5.0-rc.1`). Any other major, or any malformed input, exits 1 with
# a message on stderr and nothing on stdout. Used by sdk-publish-go.yml and
# sdk-coherence-check.yml so both agree on the mapping.
set -euo pipefail

mode=tag
if [ "${1:-}" = "--version" ]; then
  mode=version
  shift
fi

if [ "$#" -ne 1 ] || [ -z "$1" ]; then
  echo "usage: sdk-tag.sh [--version] <cli-version e.g. v1.5.0>" >&2
  exit 1
fi

raw="$1"
ver="${raw#v}"
num='(0|[1-9][0-9]*)'
re="^1\\.${num}\\.${num}(-[0-9A-Za-z.-]+)?\$"
major="${ver%%.*}"

if ! [[ "$ver" =~ $re ]]; then
  if [[ "$major" =~ ^[0-9]+$ ]] && [ "$major" != "1" ]; then
    echo "sdk-tag: unsupported CLI major '${major}' in '${raw}': only v1.M.P maps to the SDK (sdk/go/v2.M.P)" >&2
  else
    echo "sdk-tag: '${raw}' is not a CLI version of the form v1.M.P" >&2
  fi
  exit 1
fi

sdk_ver="v2.${ver#1.}"
if [ "$mode" = "version" ]; then
  printf '%s\n' "$sdk_ver"
else
  printf 'sdk/go/%s\n' "$sdk_ver"
fi
