#!/bin/sh
# Invoked only inside the disposable development container.
set -eu
mkdir -p "$HOME"
./scripts/verify.sh
CGO_ENABLED=1 go test -race -shuffle=on -count=1 -timeout=5m ./...
printf 'Linux container baseline and race checks passed.\n'
