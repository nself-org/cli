#!/bin/sh
# Recording docker stub for the agent E2E sim hosts (P7-SHIP-02), modelled on
# the stubs of internal/controlplane/sim. The sim hosts are sshd containers
# without a Docker engine, so:
# - `compose ... up -d --dry-run` lists every service of the compose file named
#   by -f as "Recreate", and `up -d [--no-deps] [svc...]` records "<service>
#   <image>" lines in /tmp/agent-e2e/running and bumps a start counter;
# - `compose images`, `compose ps`, `inspect -f` answer from that record;
# - `image inspect` reports the image's real registry digest from
#   /tmp/agent-e2e/digests ("<ref> <repo>@sha256:<digest>" lines the harness
#   writes from the operator's pulled images) and the host's platform, so
#   promote's digest compare sees what a real host would report.
S=/tmp/agent-e2e
mkdir -p "$S"
echo "$*" >> "$S/docker.log"
F=docker-compose.yml
prev=''
for a; do [ "$prev" = "-f" ] && F=$a; prev=$a; done
services() { awk '/^services:/{s=1;next} s&&/^[^ ]/{s=0} s&&/^  [A-Za-z0-9_.-]+:/{svc=$1;sub(":","",svc)} s&&svc!=""&&/^    image:/{gsub(/["\047]/,"",$2);print svc" "$2}' "$F" 2>/dev/null | sort -u; }
idof() { printf 'sha256:%s' "$(printf '%s' "$1" | sha256sum | cut -c1-64)"; }
platform() { case "$(uname -m)" in aarch64|arm64) echo linux/arm64 ;; *) echo linux/amd64 ;; esac; }
up() { # svc...: record those services (all when none) as running
  services > "$S/compose.now"
  [ $# -gt 0 ] || set -- $(awk '{print $1}' "$S/compose.now")
  for svc; do
    line=$(awk -v s="$svc" '$1==s' "$S/compose.now")
    [ -n "$line" ] || continue
    { grep -v "^$svc " "$S/running" 2>/dev/null; echo "$line"; } | sort > "$S/running.new"
    mv "$S/running.new" "$S/running"
    n=$(cat "$S/up-$svc" 2>/dev/null || echo 0); echo $((n+1)) > "$S/up-$svc"
  done
}
case "$*" in
  *"config --services"*) services | awk '{print $1}' ;;
  *"config --format json"*) printf '{"services":{}}' ;;
  *"up -d --dry-run"*) services | while read -r svc ref; do printf ' Container app-%s-1 Recreate\n' "$svc"; done ;;
  *" up "*|*" up")
    known=$(services | awk '{print $1}')
    list=''
    for a; do for k in $known; do [ "$a" = "$k" ] && list="$list $a"; done; done
    # shellcheck disable=SC2086
    up $list ;;
  *" images --format json"*)
    printf '['; sep=''
    while read -r svc ref; do
      [ -n "$ref" ] || continue
      base=${ref%@*}; repo=${base%:*}; tag=${base##*:}
      [ "$repo" = "$base" ] && tag=latest
      printf '%s{"Service":"%s","ContainerName":"app-%s-1","Repository":"%s","Tag":"%s","ID":"%s"}' "$sep" "$svc" "$svc" "$repo" "$tag" "$(idof "$ref")"; sep=','
    done < "$S/running" 2>/dev/null
    printf ']' ;;
  *"ps -a --format json"*)
    for last; do :; done
    printf '[{"ID":"id-%s","Name":"app-%s-1","Service":"%s","State":"running","Health":"healthy"}]' "$last" "$last" "$last" ;;
  *"ps --all --format json"*)
    while read -r svc ref; do
      [ -n "$ref" ] && printf '{"Name":"app-%s-1","Service":"%s","State":"running","Health":"healthy"}\n' "$svc" "$svc"
    done < "$S/running" 2>/dev/null ;;
  "inspect -f"*)
    shift 3
    for i; do n=$(cat "$S/up-${i#id-}" 2>/dev/null || echo 0); printf '2026-10-30T00:00:%02dZ\n' "$n"; done ;;
  *"image inspect --format"*)
    for last; do :; done
    while read -r svc ref; do
      [ "$(idof "$ref")" = "$last" ] || continue
      case "$ref" in
        *@sha256:*) base=${ref%@*}; rd="${base%:*}@${ref#*@}" ;;
        *) rd=$(awk -v r="$ref" '$1==r{print $2; exit}' "$S/digests" 2>/dev/null) ;;
      esac
      printf '["%s"]|%s' "$rd" "$(platform)"
      exit 0
    done < "$S/running" 2>/dev/null
    exit 1 ;;
  "network inspect"*) exit 1 ;;
esac
exit 0
