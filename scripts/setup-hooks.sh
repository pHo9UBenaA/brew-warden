#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
git rev-parse --git-dir >/dev/null
existing=$(git config --get core.hooksPath || true)
if [ -n "$existing" ] && [ "$existing" != .githooks ]; then
  printf 'Refusing to replace core.hooksPath=%s. Review and configure it explicitly.\n' "$existing" >&2
  exit 1
fi
git config --local core.hooksPath .githooks
printf 'Enabled repository-local Git hooks.\n'
