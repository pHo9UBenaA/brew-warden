#!/bin/sh
# Build unsigned diagnostic-only development artifacts; never publish them.
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
. ./scripts/env.sh
if [ "$#" -ne 1 ]; then
  printf 'Usage: scripts/build-product.sh darwin/arm64|darwin/amd64\n' >&2
  exit 1
fi
case "$1" in darwin/arm64|darwin/amd64) ;; *) exit 1 ;; esac
if [ "$(go env GOVERSION)" != "go$(cat .go-version)" ]; then
  printf 'Product builds require the pinned Go toolchain.\n' >&2
  exit 1
fi
if [ -n "$(git status --porcelain --untracked-files=all)" ]; then
  printf 'Commit the verified source before building reproducibility artifacts.\n' >&2
  exit 1
fi
source_revision=$(git rev-parse HEAD)
product_version="0.0.0-dev+$source_revision"
export GOOS="${1%/*}" GOARCH="${1#*/}"
build_root=$(mktemp -d "$PWD/.cache/product-build.XXXXXXXX")
printf 'Development build evidence: %s\n' "$build_root"
printf '%s\n' "$source_revision" > "$build_root/source-revision"
go version > "$build_root/toolchain"
go list -m all > "$build_root/modules"
git archive "$source_revision" > "$build_root/source.tar"
for pass in first second; do
  mkdir -p "$build_root/$pass/source"
  tar -xf "$build_root/source.tar" -C "$build_root/$pass/source"
  for command in bwd brewwarden; do
    # Force recompilation so a shared object cache cannot stand in for a repeat build.
    (cd "$build_root/$pass/source" && go build -a -trimpath -buildvcs=false \
      -ldflags="-s -w -X brewwarden/internal/cli.Version=$product_version" \
      -o "$build_root/$pass/$command" "./cmd/$command")
  done
done
for command in bwd brewwarden; do
  cmp "$build_root/first/$command" "$build_root/second/$command"
  go version -m "$build_root/first/$command" > "$build_root/$command.build-info"
done
(cd "$build_root/first" && shasum -a 256 bwd brewwarden) > "$build_root/SHA256SUMS"
printf 'Repeated %s builds match. Artifacts remain development-only; execution is disabled.\n' "$1"
