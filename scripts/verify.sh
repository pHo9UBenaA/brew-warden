#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
. ./scripts/env.sh
# Missing product directories are intentional; check each existing tree below.
for tree in tools internal cmd tests; do
  if [ -d "$tree" ]; then
    formatted=$(gofmt -l "$tree")
    if [ -n "$formatted" ]; then
      printf 'Run gofmt on:\n%s\n' "$formatted" >&2
      exit 1
    fi
  fi
done
go run ./tools/repo-check hygiene
go run ./tools/repo-check architecture
modules=$(go list -m all)
if [ "$modules" != "github.com/pHo9UBenaA/brew-warden" ]; then
  printf 'Unexpected module dependencies:\n%s\n' "$modules" >&2
  exit 1
fi
unexpected=$(go list -deps -test -f '{{if and (not .Standard) (not .Module.Main)}}{{.ImportPath}}{{end}}' ./...)
if [ -n "$unexpected" ]; then
  printf 'Unexpected package dependencies:\n%s\n' "$unexpected" >&2
  exit 1
fi
# Validate metadata without rewriting the worktree or relying on Git tracking.
go mod tidy -diff
go mod verify
for script in scripts/*.sh .githooks/*; do
  sh -n "$script"
done
go vet ./...
go test -shuffle=on -count=1 -timeout=5m ./...
printf 'Offline verification passed.\n'
