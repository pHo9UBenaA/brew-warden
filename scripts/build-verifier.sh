#!/bin/sh
# Explicit networked build of the narrowly scoped maintained upstream verifier.
# The application module remains separate; this runtime dependency is inventoried.
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
if [ "$#" -ne 1 ]; then
  printf 'Usage: scripts/build-verifier.sh darwin/arm64|darwin/amd64\n' >&2
  exit 1
fi
case "$1" in darwin/arm64|darwin/amd64) ;; *) exit 1 ;; esac
export GOTOOLCHAIN=local GOWORK=off GOFLAGS=-mod=readonly CGO_ENABLED=0
export GOMODCACHE="$PWD/.cache/attestation-mod" GOCACHE="$PWD/.cache/attestation-go"
export GOPROXY=https://proxy.golang.org GOSUMDB=sum.golang.org
if [ "$(go env GOVERSION)" != "go$(cat .go-version)" ]; then
  printf 'Verifier builds require the pinned Go toolchain.\n' >&2
  exit 1
fi
module=github.com/cli/cli/v2
version=v2.101.0
module_sum='h1:zJ+YxyhomQ9PHDjB5wwv4tCFnjq6vEbfI0pRdOsJ0xo='
output=$(mktemp -d "$PWD/.cache/verifier-build.XXXXXXXX")
printf 'Verifier build evidence: %s\n' "$output"
go mod download -json "$module@$version" > "$output/module.json"
# Match the exact checksum field, not an error, origin or go.mod checksum.
expected=$(printf '\t"Sum": "%s",' "$module_sum")
if ! grep -Fx "$expected" "$output/module.json" > /dev/null; then
  printf 'Unexpected upstream verifier module checksum.\n' >&2
  exit 1
fi
source_dir="$GOMODCACHE/$module@$version"
cp -R "$source_dir" "$output/source"
# Downloaded module files are read-only; only add our entrypoint to a private copy.
chmod u+w "$output/source" "$output/source/cmd"
mkdir "$output/source/cmd/brewwarden-verifier"
cp scripts/verifier-main.go.tmpl "$output/source/cmd/brewwarden-verifier/main.go"
go -C "$output/source" mod download
go -C "$output/source" mod verify > "$output/module-verification"
go -C "$output/source" list -m all > "$output/modules"
export GOOS="${1%/*}" GOARCH="${1#*/}"
for pass in first second; do
  mkdir "$output/$pass"
  go -C "$output/source" build -a -trimpath -buildvcs=false \
    -o "$output/$pass/brewwarden-verifier" ./cmd/brewwarden-verifier
done
cmp "$output/first/brewwarden-verifier" "$output/second/brewwarden-verifier"
go version -m "$output/first/brewwarden-verifier" > "$output/build-info"
shasum -a 256 "$output/first/brewwarden-verifier" > "$output/SHA256SUMS"
printf 'Verifier repeat builds match; run vulnerability and native contract checks before distribution.\n'
