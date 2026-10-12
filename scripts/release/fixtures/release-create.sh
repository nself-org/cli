#!/usr/bin/env bash
# STUB of scripts/release-create.sh (test.sh only): logs argv, creates the tag.
printf '%s\n' "$*" >> "$RC_LOG"
tag=$1; sha=""; prev=""
for a in "$@"; do [ "$prev" = --target ] && sha=$a; prev=$a; done
git --git-dir="$FIX/cli.git" update-ref "refs/tags/$tag" "$sha"
