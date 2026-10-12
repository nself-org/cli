#!/usr/bin/env bash
# STUB of scripts/admin-merge.sh (test.sh only). Records whether it was handed a
# terminal and what it read from it, then fast-forwards the PR's branch into main.
tty=no; if [ -t 0 ]; then tty=yes; fi
ans=""; read -r ans || ans=""
repo=${2#nself-org/}; n=$4
printf 'repo=%s pr=%s tty=%s ans=%s\n' "$repo" "$n" "$tty" "$ans" >> "$ADMIN_LOG"
[ "$tty" = yes ] && [ "$ans" = y ] || exit 1
bare="$FIX/$repo.git"; head=$(cat "$ST/pr/$repo/$n.head")
sha=$(git --git-dir="$bare" rev-parse "refs/heads/$head")
git --git-dir="$bare" merge-base --is-ancestor refs/heads/main "$sha" || exit 1
git --git-dir="$bare" update-ref refs/heads/main "$sha"
echo MERGED > "$ST/pr/$repo/$n.state"; echo "$sha" > "$ST/pr/$repo/$n.sha"
