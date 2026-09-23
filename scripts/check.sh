#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
. ./scripts/env.sh
. ./scripts/tool-versions.env
if [ "$#" -ne 1 ]; then
  printf 'Usage: scripts/check.sh all|build|race|cover|fuzz|lint|vuln\n' >&2
  exit 1
fi
require_tool() {
  binary="$PWD/.cache/tools/$1"
  if [ ! -x "$binary" ]; then
    printf 'Missing %s; run ./scripts/setup-tools.sh explicitly.\n' "$1" >&2
    exit 1
  fi
  metadata=$(go version -m "$binary")
  if ! printf '%s\n' "$metadata" | awk -v module="$2" -v version="$3" \
    '$1 == "mod" && $2 == module && $3 == version { found=1 } END { exit !found }'; then
    printf 'Wrong %s version; run ./scripts/setup-tools.sh.\n' "$1" >&2
    exit 1
  fi
}
run_fuzz() {
  package=$1
  target=$2
  listed=$(go test "$package" -list "^$target$") || {
    printf 'Cannot enumerate fuzz target %s in %s.\n' "$target" "$package" >&2
    exit 1
  }
  if ! printf '%s\n' "$listed" | grep -Fxq "$target"; then
    printf 'Required fuzz target %s is missing from %s.\n' "$target" "$package" >&2
    exit 1
  fi
  go test "$package" -run='^$' -fuzz="^$target$" \
    -fuzztime="${FUZZTIME:-10s}" -parallel=2 -timeout=5m
}
case "$1" in
  all)
    if [ "$(go env GOVERSION)" != "go$(cat .go-version)" ]; then
      printf 'Full verification requires Go %s from .go-version.\n' "$(cat .go-version)" >&2
      exit 1
    fi
    require_tool staticcheck honnef.co/go/tools "$STATICCHECK_VERSION"
    require_tool govulncheck golang.org/x/vuln "$GOVULNCHECK_VERSION"
    ./scripts/verify.sh
    for step in race cover fuzz lint vuln; do
      ./scripts/check.sh "$step"
    done
    ;;
  build)
    mkdir -p bin
    go build -trimpath -o bin/repo-check ./tools/repo-check
    go build -trimpath -o bin/bwd ./cmd/bwd
    go build -trimpath -o bin/brewwarden ./cmd/brewwarden
    ;;
  race)
    CGO_ENABLED=1 go test -race -shuffle=on -count=1 -timeout=5m ./...
    ;;
  cover)
    coverage_packages=$(go list ./... | paste -sd , -)
    go test -coverpkg="$coverage_packages" -covermode=atomic -coverprofile=.cache/coverage.out -shuffle=on -count=1 -timeout=5m ./...
    go tool cover -func=.cache/coverage.out
    ;;
  fuzz)
    run_fuzz ./tools/repo-check FuzzCommitMessage
    run_fuzz ./internal/adapters/localstate FuzzConfig
    run_fuzz ./internal/adapters/homebrew FuzzInFlightRecord
    run_fuzz ./internal/adapters/homebrew FuzzPublicAdvisoryStatus
    run_fuzz ./internal/adapters/attestation FuzzVerifiedSubject
    run_fuzz ./internal/adapters/homebrew FuzzNativeMetadata
    run_fuzz ./tests FuzzPolicyRequiresCompleteEvidence
    ;;
  lint)
    require_tool staticcheck honnef.co/go/tools "$STATICCHECK_VERSION"
    "$binary" ./...
    ;;
  vuln)
    require_tool govulncheck golang.org/x/vuln "$GOVULNCHECK_VERSION"
    # Module downloads stay disabled; only advisory DB access is needed here.
    "$binary" -test ./...
    ./scripts/check.sh build
    "$binary" -mode=binary ./bin/repo-check
    "$binary" -mode=binary ./bin/bwd
    "$binary" -mode=binary ./bin/brewwarden
    ;;
  *) printf 'Unknown check: %s\n' "$1" >&2; exit 1 ;;
esac
