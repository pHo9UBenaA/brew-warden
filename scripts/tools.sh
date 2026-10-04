# Sourced from the repository root after env.sh. No implicit tool installation.
. ./scripts/tool-versions.env

# Return the validated executable path, rather than mutate caller state.
checked_tool_path() (
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
  printf '%s\n' "$binary"
)

# Keep ordinary and VM-tagged analysis on the same readability rules.
run_staticcheck() (
  binary=$(checked_tool_path staticcheck honnef.co/go/tools "$STATICCHECK_VERSION")
  "$binary" -checks='inherit,ST1003,ST1016,ST1020,ST1021,ST1022,ST1023' "$@"
)
