#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
. ./scripts/env.sh
. ./scripts/tools.sh
if [ "$#" -ne 1 ]; then
  printf 'Usage: scripts/format.sh check|write\n' >&2
  exit 1
fi
case "$1" in
  check) mode=l ;;
  write) mode=w ;;
  *)
    printf 'Unknown format mode: %s\n' "$1" >&2
    exit 1
    ;;
esac
binary=$(checked_tool_path gofumpt mvdan.cc/gofumpt "$GOFUMPT_VERSION")
shell_formatter=$(checked_tool_path shfmt mvdan.cc/sh/v3 "$SHFMT_VERSION")
# Explicit files include tagged tests and avoid gofumpt's generated/testdata skips.
# Name reviewed extra rules explicitly so tool updates cannot enable new ones.
formatted=$(find tools internal cmd tests -type f -name '*.go' -exec "$binary" \
  -extra=group_params,clothe_returns,balance_calls "-$mode" {} +)
if [ "$1" = check ] && [ -n "$formatted" ]; then
  printf 'Run ./scripts/format.sh write on:\n%s\n' "$formatted" >&2
  exit 1
fi
# Match existing two-space, indented-case and spaced-redirection conventions.
# Force POSIX parsing; do not simplify shell semantics.
if ! "$shell_formatter" -ln posix -i 2 -ci -sr "-$mode" scripts/*.sh .githooks/*; then
  printf 'Shell formatting failed; run ./scripts/format.sh write for formatting drift.\n' >&2
  exit 1
fi
