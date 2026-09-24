#!/bin/sh
# Local-only, two-phase acceptance gate. No publisher signing or host Homebrew access.
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
. ./scripts/env.sh
umask 077
export GOCACHE="$PWD/.cache/go-build" GOVULNDB=https://vuln.go.dev
# Do not let a caller substitute a different VM driver into the readiness gate.
unset BREWWARDEN_VM_TART

usage() {
  printf 'Usage: %s start BASE NEW_VM REVIEWED_BREW_TREE GH_ARM64\n' "$0" >&2
  printf '       %s complete VM\n' "$0" >&2
  printf '       %s cancel VM\n' "$0" >&2
  exit 2
}
fail() { printf '%s\n' "$*" >&2; exit 1; }
valid_name() {
  case "$1" in
    ''|[!A-Za-z0-9]*|*[!A-Za-z0-9._-]*) fail 'VM name must be a simple local Tart name' ;;
  esac
}
clean_source() {
  [ -z "$(git status --porcelain --untracked-files=all)" ] || fail 'Commit all repository changes before local product-readiness acceptance'
  [ "$(git rev-parse HEAD)" = "$revision" ] || fail 'Source revision changed since readiness checks started'
  [ "$(go env GOVERSION)" = "go$(cat .go-version)" ] || fail 'Pinned Go toolchain required'
}
read_line() {
  IFS= read -r value < "$1" || fail "Incomplete readiness evidence: $1"
  [ -n "$value" ] || fail "Empty readiness evidence: $1"
  printf '%s' "$value"
}
run_logged() {
  label=$1; shift
  printf 'Checking %s...\n' "$label"
  if "$@" > "$root/$label.log" 2>&1; then
    printf 'Passed %s.\n' "$label"
  else
    result=$?
    /usr/bin/tail -n 55 "$root/$label.log" >&2
    printf 'Failed %s (exit %s); evidence: %s\n' "$label" "$result" "$root/$label.log" >&2
    return "$result"
  fi
}
start() {
  [ "$#" -eq 4 ] || usage
  base=$1; vm=$2; source=$3; gh=$4
  valid_name "$base"; valid_name "$vm"
  [ "$base" != "$vm" ] || fail 'Refusing to replace the base VM'
  revision=$(git rev-parse HEAD)
  clean_source
  [ -d "$source/Library/Homebrew" ] && [ -f "$source/bin/brew" ] && [ -f "$gh" ] || fail 'Explicit guest provisioning inputs required'
  [ -x "$PWD/.cache/vm-tools/tart-2.37.0/tart.app/Contents/MacOS/tart" ] || fail 'Inspected local Tart executable unavailable'
  root="$PWD/.cache/product-ready.$vm"
  mkdir -m 700 "$root" || fail 'Use a new VM name; previous readiness evidence must not be overwritten'
  printf '%s\n' "$revision" > "$root/revision"
  printf 'Local acceptance evidence: %s\n' "$root"
  # check.sh all includes verify, normal tests, race, coverage, fuzz, lint and
  # source/binary vulnerability scans. Never let caller-selected FUZZTIME skip fuzzing.
  run_logged check-all /usr/bin/env FUZZTIME=10s ./scripts/check.sh all
  run_logged vm-vet /usr/bin/env GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go vet -tags=vmacceptance ./tests/vm
  run_logged vm-lint "$PWD/.cache/tools/staticcheck" -tags=vmacceptance ./tests/vm
  run_logged vm-vuln "$PWD/.cache/tools/govulncheck" -tags=vmacceptance -test ./tests/vm
  run_logged vm-compile /usr/bin/env GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go test -c -tags=vmacceptance -o "$root/acceptance.test" ./tests/vm
  run_logged distribution ./scripts/build-product.sh darwin/arm64
  build_dir=$(awk -F ': ' '/^Distribution build evidence: / {print $2}' "$root/distribution.log")
  case "$build_dir" in "$PWD"/.cache/product-build.*) ;; *) fail 'Distribution build evidence path unavailable' ;; esac
  archive="$build_dir/first/brewwarden-darwin-arm64.tar.gz"
  [ -f "$archive" ] || fail 'Reproducible product archive unavailable'
  printf '%s\n' "$archive" > "$root/archive"
  /usr/bin/shasum -a 256 "$archive" | /usr/bin/awk '{print $1}' > "$root/archive-sha256"
  clean_source
  trap 'mark_prepare_failure $?' 0
  trap 'exit 130' INT
  trap 'exit 143' TERM
  run_logged vm-prepare ./scripts/macos-vm-acceptance.sh prepare "$base" "$vm" "$source" "$gh" "$archive"
  trap - 0 INT TERM
  printf '%s\n' awaiting-device-approval > "$root/state"
  printf 'Offline checks and repeat build passed; private Apple Silicon VM prepared.\n'
  trap 'finish_on_exit $?' 0
  trap 'exit 130' INT
  trap 'exit 143' TERM
  ./scripts/macos-vm-acceptance.sh auth "$vm"
  trap - 0 INT TERM
  printf 'Approve the device code in your browser, then run: %s complete %s\n' "$0" "$vm"
  printf 'If approval expires, request a fresh code with the guest runner or run: %s cancel %s\n' "$0" "$vm"
}
mark_prepare_failure() {
  result=$1
  trap - 0
  if [ "$result" -ne 0 ]; then
    printf '%s\n' failed > "$root/state"
    printf 'VM preparation failed; NOT product-ready. Review %s and confirm clone cleanup.\n' "$root" >&2
  fi
  exit "$result"
}
finish_on_exit() {
  result=$1
  trap - 0
  if ./scripts/macos-vm-acceptance.sh finish "$vm" > "$root/vm-finish.log" 2>&1; then
    printf 'Guest credentials removed and VM stopped.\n'
  else
    /usr/bin/tail -n 20 "$root/vm-finish.log" >&2
    printf 'Guest cleanup unconfirmed: verify credentials were removed and %s is stopped; NOT product-ready.\n' "$vm" >&2
    result=1
  fi
  if [ "$result" -eq 0 ]; then
    if [ -z "$(git status --porcelain --untracked-files=all)" ] && [ "$(git rev-parse HEAD)" = "$revision" ]; then
      printf 'revision=%s\narchive_sha256=%s\nverified_at_utc=%s\n' \
        "$revision" "$(read_line "$root/archive-sha256")" "$(/bin/date -u '+%Y-%m-%dT%H:%M:%SZ')" > "$root/product-ready.txt"
      printf '%s\n' passed > "$root/state"
      printf 'LOCAL PRODUCT-READY for the tested initial scope: %s\nEvidence: %s\n' "$revision" "$root"
    else
      printf 'Source changed during VM acceptance; NOT product-ready.\n' >&2
      result=1
    fi
  fi
  if [ "$result" -ne 0 ]; then
    printf '%s\n' failed > "$root/state"
    printf 'Local acceptance failed; NOT product-ready. Review %s\n' "$root" >&2
  fi
  exit "$result"
}
cancel() {
  [ "$#" -eq 1 ] || usage
  vm=$1; valid_name "$vm"
  root="$PWD/.cache/product-ready.$vm"
  [ -d "$root" ] && [ ! -L "$root" ] && [ "$(read_line "$root/state")" = awaiting-device-approval ] || fail 'No pending readiness run for this VM; cannot cancel an unrelated clone'
  printf 'Cancelling pending VM readiness; NOT product-ready.\n'
  trap 'finish_on_exit 1' 0
  trap 'exit 130' INT
  trap 'exit 143' TERM
  exit 1
}
complete() {
  [ "$#" -eq 1 ] || usage
  vm=$1; valid_name "$vm"
  root="$PWD/.cache/product-ready.$vm"
  [ -d "$root" ] && [ ! -L "$root" ] && [ "$(read_line "$root/state")" = awaiting-device-approval ] || fail 'No pending readiness run for this VM; start with a fresh clone'
  # Do not stop a still-pending device login: human approval is required.
  ./scripts/macos-vm-acceptance.sh auth-status "$vm" >/dev/null || fail 'Approve the guest device code first; no readiness checks were skipped'
  trap 'finish_on_exit $?' 0
  trap 'exit 130' INT
  trap 'exit 143' TERM
  revision=$(read_line "$root/revision")
  clean_source
  archive=$(read_line "$root/archive")
  case "$archive" in "$PWD"/.cache/product-build.*/first/brewwarden-darwin-arm64.tar.gz) ;; *) fail 'Unsupported archive evidence path' ;; esac
  [ -f "$archive" ] || fail 'Verified archive disappeared'
  [ "$(/usr/bin/shasum -a 256 "$archive" | /usr/bin/awk '{print $1}')" = "$(read_line "$root/archive-sha256")" ] || fail 'Verified archive changed'
  run_logged vm-suite ./scripts/macos-vm-acceptance.sh suite "$vm"
  /usr/bin/grep -Fxq 'Local VM acceptance suite passed. Recheck guest evidence, then run finish.' "$root/vm-suite.log" || fail 'Native acceptance did not report completion'
  clean_source
}
[ "$#" -ge 1 ] || usage
operation=$1; shift
case "$operation" in
  start) start "$@" ;;
  complete) complete "$@" ;;
  cancel) cancel "$@" ;;
  *) usage ;;
esac
