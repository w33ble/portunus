#!/usr/bin/env bash
# Writes THIRD_PARTY_NOTICES.md (or $1) covering the Go modules linked into the macOS and Linux
# builds and the frontend's production npm dependencies.
set -euo pipefail

cd "$(dirname "$0")/../.."
out="${1:-THIRD_PARTY_NOTICES.md}"
tmp="$(mktemp -d)"
# go-licenses copies license files read-only from the module cache.
trap 'chmod -R u+w "$tmp"; rm -rf "$tmp"' EXIT

# Union of both platforms' dependency graphs; purego avoids needing GTK/WebKit headers here.
for goos in linux darwin; do
  GOOS=$goos GOFLAGS=-tags=purego go-licenses save ./... \
    --save_path "$tmp/go-$goos" --ignore github.com/w33ble/portunus 2>/dev/null
done

# The Go standard library and runtime are compiled into the binary too.
mkdir -p "$tmp/go-stdlib/go (standard library)"
cp "$(go env GOROOT)/LICENSE" "$tmp/go-stdlib/go (standard library)/LICENSE"

pnpm --dir frontend licenses list --prod --json >"$tmp/npm.json"
node build/release/notices.mjs "$tmp/npm.json" "$tmp/go-stdlib" "$tmp/go-linux" "$tmp/go-darwin" >"$out"
echo "wrote $out"
