#!/usr/bin/env bash
set -euo pipefail

# Compare the current renderer with origin/main in an isolated Linux workspace.
fixtures=${1:?fixture directory required}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

if [[ -f /src/.golden-base.tar ]]; then
  mkdir -p "$tmp/base"
  tar -xf /src/.golden-base.tar -C "$tmp/base"
else
  git clone -q --depth 1 --branch main https://github.com/nself-org/cli.git "$tmp/base"
fi
(cd "$tmp/base" && GOTOOLCHAIN=auto CGO_ENABLED=0 go build -mod=vendor -o "$tmp/base-nself" ./cmd/nself)
GOTOOLCHAIN=auto CGO_ENABLED=0 go build -mod=vendor -o "$tmp/head-nself" ./cmd/nself

mkdir -p "$tmp/bin"
printf '#!/bin/sh\nexit 1\n' > "$tmp/bin/docker"
chmod +x "$tmp/bin/docker"

for name in minimal full ssl; do
  for rev in base head; do
    project="$tmp/$rev-$name"
    mkdir -p "$project" "$tmp/home-$rev-$name" "$tmp/plugins-$rev-$name"
    cp -R "$fixtures/$name/." "$project/"
    cp "$project/.env.example" "$project/.env"
    if [[ $name == ssl ]]; then
      certs="$project/ssl/certificates/example-test"
      mkdir -p "$certs"
      openssl req -x509 -newkey rsa:2048 -nodes -keyout "$certs/privkey.pem" -out "$certs/fullchain.pem" -days 1 -subj /CN=example.test >/dev/null 2>&1
      cp "$certs/fullchain.pem" "$certs/chain.pem"
    fi
    (cd "$project" && HOME="$tmp/home-$rev-$name" NSELF_PLUGIN_DIR="$tmp/plugins-$rev-$name" PATH="$tmp/bin:$PATH" "$tmp/$rev-nself" build --force > "$tmp/$rev-$name.log" 2>&1) || {
      cat "$tmp/$rev-$name.log"; exit 1;
    }
  done
  diff -ru "$tmp/base-$name/nginx" "$tmp/head-$name/nginx" || exit 1
  printf '%s: nginx tree byte-identical\n' "$name"
done
