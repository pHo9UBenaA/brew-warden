#!/bin/sh
# Explicit offline distribution build. No signing, publication or installation.
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
. ./scripts/env.sh
if [ "$#" -ne 2 ] || [ "$1" != darwin/arm64 ]; then
  printf 'Usage: scripts/build-product.sh darwin/arm64 RUNTIME\n' >&2
  exit 1
fi
if [ "$(go env GOVERSION)" != "go$(cat .go-version)" ]; then
  printf 'Product builds require the pinned Go toolchain.\n' >&2
  exit 1
fi
if [ -n "$(git status --porcelain --untracked-files=all)" ]; then
  printf 'Commit verified source before building distribution artifacts.\n' >&2
  exit 1
fi
runtime_root=$(CDPATH= cd -- "$2" && pwd -P)
source_revision=$(git rev-parse HEAD)
product_version="0.1.0+$source_revision"
runtime_digest=$(shasum -a 256 "$runtime_root/manifest.json" | cut -d ' ' -f 1)
build_root=$(mktemp -d "$PWD/.cache/product-build.XXXXXXXX")
printf 'Distribution build evidence: %s\n' "$build_root"
printf '%s\n' "$source_revision" > "$build_root/source-revision"
go version > "$build_root/toolchain"
go list -m all > "$build_root/modules"
git archive "$source_revision" > "$build_root/source.tar"
module=github.com/pHo9UBenaA/brew-warden
go_license="$(go env GOROOT)/LICENSE"
for pass in first second; do
  mkdir -p "$build_root/$pass/source"
  tar -xf "$build_root/source.tar" -C "$build_root/$pass/source"
  (cd "$build_root/$pass/source" && GOOS=darwin GOARCH=arm64 go build -a -trimpath -buildvcs=false \
    -ldflags="-s -w -X $module/internal/cli.Version=$product_version -X $module/internal/composition.RuntimeSHA256=$runtime_digest" \
    -o "$build_root/$pass/bwd" ./cmd/bwd)
  go run ./tools/release-pack "$build_root/$pass/bwd" "$runtime_root" \
    "$go_license" "$source_revision" "$build_root/$pass/brewwarden-darwin-arm64.tar.gz"
done
cmp "$build_root/first/bwd" "$build_root/second/bwd"
cmp "$build_root/first/brewwarden-darwin-arm64.tar.gz" "$build_root/second/brewwarden-darwin-arm64.tar.gz"
go version -m "$build_root/first/bwd" > "$build_root/bwd.build-info"
(cd "$build_root/first" && shasum -a 256 bwd brewwarden-darwin-arm64.tar.gz) > "$build_root/SHA256SUMS"
printf 'Repeated binaries and complete archives match. No publisher signature or notarization was added.\n'
