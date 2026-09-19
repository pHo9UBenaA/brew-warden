#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
. ./scripts/env.sh
. ./scripts/tool-versions.env
if [ "$(go env GOVERSION)" != "go$(cat .go-version)" ]; then
  printf 'Use Go %s from .go-version before preparing tools.\n' "$(cat .go-version)" >&2
  exit 1
fi
# Explicit network bootstrap, separate from verification and product modules.
export GOPROXY=https://proxy.golang.org GOSUMDB=sum.golang.org
export GOPRIVATE= GONOPROXY= GONOSUMDB= GOFLAGS=
export GOBIN="$PWD/.cache/tools" GOMODCACHE="$PWD/.cache/tool-modules"
mkdir -p "$GOBIN" "$GOMODCACHE"
go install "golang.org/x/vuln/cmd/govulncheck@$GOVULNCHECK_VERSION"
go install "honnef.co/go/tools/cmd/staticcheck@$STATICCHECK_VERSION"
printf 'Pinned development tools installed in .cache/tools.\n'
