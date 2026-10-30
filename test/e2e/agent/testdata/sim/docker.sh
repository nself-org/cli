#!/bin/sh
# Recording docker stub for the agent E2E sim hosts (P7-SHIP-02). It mirrors
# the stubs of internal/controlplane/sim: the sim hosts are sshd containers
# without a Docker engine, so `compose up` records the image references of the
# shipped docker-compose.yml in /tmp/agent-e2e/running and the read commands
# answer from that record. `image inspect` reports the image's real registry
# digest from /tmp/agent-e2e/digests ("<ref> <repo>@sha256:<digest>" lines the
# harness writes from the operator's pulled images) and the host's platform,
# so promote's digest compare sees the same identities a real host would.
S=/tmp/agent-e2e
mkdir -p "$S"
echo "$*" >> "$S/docker.log"
images() { awk '/^    image:/{print $2}' docker-compose.yml 2>/dev/null | tr -d "\"'" | sort -u; }
idof() { printf 'sha256:%s' "$(printf '%s' "$1" | sha256sum | cut -c1-64)"; }
platform() { case "$(uname -m)" in aarch64|arm64) echo linux/arm64 ;; *) echo linux/amd64 ;; esac; }
case "$*" in
  *"config --services"*) awk '/^services:/{s=1;next} s&&/^  [A-Za-z0-9_.-]+:/{sub(":","",$1);print $1} s&&/^[^ ]/{s=0}' docker-compose.yml ;;
  *"config --format json"*) printf '{"services":{}}' ;;
  *"up -d --dry-run"*) : ;;
  *" up "*|*" up") images > "$S/running" ;;
  *" images --format json"*)
    printf '['; sep=''
    while IFS= read -r ref; do
      [ -n "$ref" ] || continue
      base=${ref%@*}; repo=${base%:*}; tag=${base##*:}
      [ "$repo" = "$base" ] && tag=latest
      printf '%s{"ContainerName":"app-%s-1","Repository":"%s","Tag":"%s","ID":"%s"}' "$sep" "$(idof "$ref" | cut -c8-15)" "$repo" "$tag" "$(idof "$ref")"; sep=','
    done < "$S/running" 2>/dev/null
    printf ']' ;;
  *"ps -a --format json"*|*"ps --all --format json"*) printf '[]' ;;
  *"image inspect --format"*)
    for last; do :; done
    while IFS= read -r ref; do
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
