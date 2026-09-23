#!/bin/sh
# Local-only Tart acceptance. Never mounts host directories or forwards host credentials.
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
. ./scripts/env.sh

tart=${BREWWARDEN_VM_TART:-"$PWD/.cache/vm-tools/tart-2.37.0/tart.app/Contents/MacOS/tart"}
tart_home="$PWD/.cache/tart"
tart_user_home="$PWD/.cache/vm-tools/home"
guest_root=/private/tmp/bw-acceptance
guest_home="$guest_root/home"
guest_gh="$guest_root/gh/gh"
guest_bwd="$guest_root/product/brewwarden/bwd"
guest_test="$guest_root/acceptance.test"
guest_path="$guest_root/gh:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin"

usage() {
  printf 'Usage: %s prepare BASE NEW_VM REVIEWED_BREW_TREE GH_BINARY PRODUCT_ARCHIVE\n' "$0" >&2
  printf '       %s auth|suite|finish VM\n' "$0" >&2
  printf '       %s run VM doctor|native|general|public|survey|upgrade|crash|command [CASE_ARGS...]\n' "$0" >&2
  printf '       %s fixture VM absent-jq|absent-xz|older-xz|repair-jq\n' "$0" >&2
  exit 2
}
fail() { printf '%s\n' "$*" >&2; exit 1; }
valid_name() {
  case "$1" in
    ''|[!A-Za-z0-9]*|*[!A-Za-z0-9._-]*) fail 'VM name must be a simple local Tart name' ;;
  esac
}
tart_run() {
  env -i HOME="$tart_user_home" TART_HOME="$tart_home" TART_NO_AUTO_PRUNE=1 \
    PATH=/usr/bin:/bin "$tart" "$@"
}
guest() {
  vm=$1; shift
  tart_run exec "$vm" "$@"
}
guest_auth() {
  guest "$1" /usr/bin/env -i HOME="$guest_home" GH_CONFIG_DIR="$guest_home/.config/gh" \
    PATH="$guest_path" "$guest_gh" auth status >/dev/null 2>&1
}
guest_brew() {
  vm=$1; shift
  guest "$vm" /usr/bin/env -i HOME="$guest_home" PATH="$guest_path" \
    HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_ANALYTICS=1 HOMEBREW_NO_AUTOREMOVE=1 \
    HOMEBREW_NO_INSTALL_CLEANUP=1 /opt/homebrew/bin/brew "$@"
}
require_guest() {
  model=$(guest "$1" /usr/sbin/sysctl -n hw.model) || fail 'Guest agent unavailable; use a fresh clone of the base image'
  arch=$(guest "$1" /usr/bin/arch) || fail 'Guest architecture unavailable'
  case "$model:$arch" in VirtualMac*:arm64) ;; *) fail 'Only a disposable Apple Silicon VirtualMac is supported' ;; esac
}
prepare() {
  [ "$#" -eq 5 ] || usage
  base=$1; vm=$2; source=$3; gh=$4; archive=$5
  valid_name "$base"; valid_name "$vm"
  [ "$base" != "$vm" ] || fail 'A clone cannot replace its base VM'
  [ -x "$tart" ] || fail 'Reviewed Tart executable unavailable'
  [ -d "$source/Library/Homebrew" ] && [ -f "$source/bin/brew" ] || fail 'Explicit reviewed Homebrew tree unavailable'
  [ -f "$gh" ] && [ -f "$archive" ] || fail 'Explicit gh binary or distribution archive unavailable'
  source=$(CDPATH= cd -- "$source" && pwd -P)
  gh=$(CDPATH= cd -- "$(dirname -- "$gh")" && pwd -P)/$(basename -- "$gh")
  archive=$(CDPATH= cd -- "$(dirname -- "$archive")" && pwd -P)/$(basename -- "$archive")
  mkdir -p "$tart_user_home" "$tart_home" .cache
  chmod 0700 "$tart_user_home"
  evidence=$(mktemp -d "$PWD/.cache/vm-acceptance.XXXXXXXX")
  printf 'Acceptance workspace: %s\n' "$evidence"
  git rev-parse HEAD > "$evidence/source-revision"
  shasum -a 256 "$gh" "$archive" > "$evidence/inputs.sha256"
  tar -cf "$evidence/reviewed-brew.tar" -C "$source" bin/brew Library/Homebrew
  shasum -a 256 "$evidence/reviewed-brew.tar" >> "$evidence/inputs.sha256"
  tart_run clone "$base" "$vm"
  tart_run set "$vm" --cpu 4 --memory 4096
  nohup env -i HOME="$tart_user_home" TART_HOME="$tart_home" TART_NO_AUTO_PRUNE=1 \
    PATH=/usr/bin:/bin "$tart" run --no-graphics --no-audio --no-clipboard "$vm" \
    > "$evidence/boot.log" 2>&1 < /dev/null &
  ready=0
  count=0
  while [ "$count" -lt 60 ]; do
    if guest "$vm" /usr/bin/id > /dev/null 2>&1; then ready=1; break; fi
    sleep 3
    count=$((count + 1))
  done
  [ "$ready" -eq 1 ] || fail 'Guest agent did not start; clone retained for diagnosis'
  require_guest "$vm"
  guest "$vm" /bin/sh -c 'umask 077; test ! -e /opt/brewwarden-original-homebrew && test -f /opt/homebrew/bin/brew && mkdir -p /private/tmp/bw-acceptance/gh /private/tmp/bw-acceptance/home /private/tmp/bw-acceptance/product' || fail 'Guest prefix or private workspace is not fresh'
  tart_run exec -i "$vm" /bin/sh -c 'umask 077; cat > /private/tmp/bw-acceptance/gh/gh && chmod 0700 /private/tmp/bw-acceptance/gh/gh' < "$gh"
  tart_run exec -i "$vm" /usr/bin/tar -xzf - -C "$guest_root/product" < "$archive"
  revision=$(git rev-parse HEAD)
  version=$(guest "$vm" /usr/bin/env -i HOME="$guest_home" PATH="$guest_path" "$guest_bwd" --version) || fail 'Shipped binary unavailable'
  [ "$version" = "BrewWarden 0.1.0+$revision" ] || fail 'Archive and local test source are different revisions'
  guest "$vm" /usr/bin/sudo -n /bin/mv /opt/homebrew /opt/brewwarden-original-homebrew
  guest "$vm" /usr/bin/sudo -n /bin/mkdir /opt/homebrew
  guest "$vm" /usr/bin/sudo -n /usr/sbin/chown admin:admin /opt/homebrew
  guest "$vm" /bin/mkdir -p /opt/homebrew/Cellar /opt/homebrew/var/homebrew /opt/homebrew/etc /opt/homebrew/share /opt/homebrew/Library
  tart_run exec -i "$vm" /usr/bin/tar -xf - -C /opt/homebrew < "$evidence/reviewed-brew.tar"
  guest "$vm" /usr/bin/env -i HOME="$guest_home" PATH="$guest_path" "$guest_bwd" doctor \
    > "$evidence/doctor.log" 2>&1 || { /bin/cat "$evidence/doctor.log"; fail 'Installed Homebrew tree did not match reviewed runtime'; }
  /bin/cat "$evidence/doctor.log"
  printf 'Prepare complete. Run: %s auth %s\n' "$0" "$vm"
  printf 'Do not reboot this modified clone: its guest agent still refers to the original prefix.\n'
}
auth() {
  [ "$#" -eq 1 ] || usage
  vm=$1; valid_name "$vm"; require_guest "$vm"
  if guest_auth "$vm"; then printf 'Guest gh is already authenticated.\n'; return; fi
  guest "$vm" /bin/sh -c 'umask 077; if test -f /private/tmp/bw-acceptance/home/device.pid && /bin/kill -0 "$(/bin/cat /private/tmp/bw-acceptance/home/device.pid)" 2>/dev/null; then exit 0; fi; HOME=/private/tmp/bw-acceptance/home PATH=/private/tmp/bw-acceptance/gh:/usr/bin:/bin BROWSER=/usr/bin/true /usr/bin/nohup /private/tmp/bw-acceptance/gh/gh auth login --hostname github.com --git-protocol https --web > /private/tmp/bw-acceptance/home/device.log 2>&1 < /dev/null & echo $! > /private/tmp/bw-acceptance/home/device.pid'
  count=0
  while [ "$count" -lt 20 ]; do
    lines=$(guest "$vm" /bin/sh -c '/usr/bin/grep -E "One-time code|Open this URL" /private/tmp/bw-acceptance/home/device.log 2>/dev/null' || true)
    if [ -n "$lines" ]; then
      printf '%s\n' "$lines"
      printf 'Approve in your browser; do not paste a token here. Standard gh device login requests repo, read:org and gist scopes.\n'
      return
    fi
    sleep 2
    count=$((count + 1))
  done
  fail 'Device authorization code unavailable; inspect guest device.log without copying credentials'
}
run_case() {
  [ "$#" -ge 2 ] || usage
  vm=$1; mode=$2; shift 2
  valid_name "$vm"; require_guest "$vm"
  if [ "$mode" = doctor ]; then
    [ "$#" -eq 0 ] || usage
    guest "$vm" /usr/bin/env -i HOME="$guest_home" PATH="$guest_path" "$guest_bwd" doctor
    return
  fi
  guest_auth "$vm" || fail 'Authenticate gh inside the disposable guest before running acceptance'
  if [ "$mode" = command ]; then
    [ "$#" -gt 0 ] || usage
    guest "$vm" /usr/bin/env -i HOME="$guest_home" GH_CONFIG_DIR="$guest_home/.config/gh" \
      PATH="$guest_path" "$guest_bwd" "$@"
    return
  fi
  [ "$(go env GOVERSION)" = "go$(cat .go-version)" ] || fail 'Pinned Go toolchain required for local VM tests'
  mkdir -p .cache
  output=$(mktemp "$PWD/.cache/vm-acceptance-test.XXXXXXXX")
  GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go test -c -tags=vmacceptance -o "$output" ./tests
  tart_run exec -i "$vm" /bin/sh -c 'umask 077; cat > /private/tmp/bw-acceptance/acceptance.test && chmod 0700 /private/tmp/bw-acceptance/acceptance.test' < "$output"
  rm -f "$output"
  case "$mode" in
    native)
      [ "$#" -le 1 ] || usage
      guest "$vm" /usr/bin/env -i HOME="$guest_home" GH_CONFIG_DIR="$guest_home/.config/gh" PATH="$guest_path" TMPDIR=/private/tmp \
        BREWWARDEN_VM_RUNTIME="$guest_root/product" BREWWARDEN_VM_FAULT="${1:-}" "$guest_test" -test.run '^TestLiveNativeExecution$' -test.v -test.timeout=20m ;;
    general)
      [ "$#" -eq 1 ] || usage
      guest "$vm" /usr/bin/env -i HOME="$guest_home" GH_CONFIG_DIR="$guest_home/.config/gh" PATH="$guest_path" TMPDIR=/private/tmp \
        BREWWARDEN_VM_RUNTIME="$guest_root/product" BREWWARDEN_VM_GENERAL_TARGETS="$1" "$guest_test" -test.run '^TestLiveGeneralBottleExecution$' -test.v -test.timeout=20m ;;
    public)
      [ "$#" -ge 1 ] && [ "$#" -le 2 ] || usage
      guest "$vm" /usr/bin/env -i HOME="$guest_home" GH_CONFIG_DIR="$guest_home/.config/gh" PATH="$guest_path" TMPDIR=/private/tmp \
        BREWWARDEN_VM_PUBLIC_RUNTIME="$guest_root/product" BREWWARDEN_VM_PUBLIC_TARGETS="$1" BREWWARDEN_VM_PUBLIC_FAULT="${2:-}" \
        "$guest_test" -test.run '^TestLivePublicCommandExecution$' -test.v -test.timeout=20m ;;
    survey)
      [ "$#" -eq 1 ] || usage
      guest "$vm" /usr/bin/env -i HOME="$guest_home" GH_CONFIG_DIR="$guest_home/.config/gh" PATH="$guest_path" TMPDIR=/private/tmp \
        BREWWARDEN_VM_PUBLIC_RUNTIME="$guest_root/product" BREWWARDEN_VM_SURVEY_TARGETS="$1" "$guest_test" -test.run '^TestLivePublicCoverageSurvey$' -test.v -test.timeout=40m ;;
    upgrade)
      [ "$#" -eq 1 ] || usage
      guest "$vm" /usr/bin/env -i HOME="$guest_home" GH_CONFIG_DIR="$guest_home/.config/gh" PATH="$guest_path" TMPDIR=/private/tmp \
        BREWWARDEN_VM_RUNTIME="$guest_root/product" BREWWARDEN_VM_UPGRADE_TARGET="$1" "$guest_test" -test.run '^TestLiveExplicitUpgradeChangesSelectedVersion$' -test.v -test.timeout=20m ;;
    crash)
      [ "$#" -eq 0 ] || usage
      guest "$vm" /usr/bin/env -i HOME="$guest_home" GH_CONFIG_DIR="$guest_home/.config/gh" PATH="$guest_path" TMPDIR=/private/tmp \
        BREWWARDEN_VM_DISTRIBUTION_BINARY="$guest_bwd" BREWWARDEN_VM_GH_CONFIG_DIR="$guest_home/.config/gh" \
        "$guest_test" -test.run '^TestLiveDistributionParentCrash$' -test.v -test.timeout=20m ;;
    *) usage ;;
  esac
}
remove_if_installed() {
  vm=$1; name=$2
  if guest "$vm" /bin/test ! -e "/opt/homebrew/Cellar/$name"; then return; fi
  installed=$(guest_brew "$vm" list --formula --versions "$name") || fail "Cannot inspect guest installation of $name"
  [ -n "$installed" ] || fail "Cannot confirm installed guest formula $name"
  guest_brew "$vm" uninstall --formula --force "$name"
}
fixture() {
  [ "$#" -eq 2 ] || usage
  vm=$1; kind=$2; valid_name "$vm"; require_guest "$vm"
  printf 'Modifying only the disposable guest Homebrew prefix for fixture: %s\n' "$kind"
  case "$kind" in
    absent-jq) remove_if_installed "$vm" jq; remove_if_installed "$vm" oniguruma ;;
    absent-xz) remove_if_installed "$vm" zstd; remove_if_installed "$vm" xz ;;
    repair-jq) guest_brew "$vm" link --formula jq ;;
    older-xz)
      guest "$vm" /bin/sh -c 'test ! -e /opt/homebrew/Cellar/xz && test -f /opt/brewwarden-original-homebrew/Cellar/xz/5.8.3/INSTALL_RECEIPT.json' || fail 'Older xz fixture unavailable; first make xz absent'
      guest "$vm" /bin/sh -c '/bin/mkdir -p /opt/homebrew/Cellar/xz && /bin/cp -pR /opt/brewwarden-original-homebrew/Cellar/xz/5.8.3 /opt/homebrew/Cellar/xz/5.8.3'
      guest_brew "$vm" link --formula xz ;;
    *) usage ;;
  esac
}
suite() {
  [ "$#" -eq 1 ] || usage
  vm=$1; valid_name "$vm"; require_guest "$vm"
  guest_auth "$vm" || fail 'Authorize guest gh before starting the local-only suite'
  run_case "$vm" doctor
  run_case "$vm" command brew install jq
  for fault in age age-exception changed-input exception-changed-input; do
    run_case "$vm" native "$fault"
  done
  for fault in changed-input changed-metadata installed-state missing-cache cancel; do
    run_case "$vm" public jq "$fault"
  done
  run_case "$vm" general 'jq xz'
  run_case "$vm" public 'jq xz'
  fixture "$vm" absent-xz
  run_case "$vm" public xz interrupt
  fixture "$vm" absent-xz
  run_case "$vm" crash
  fixture "$vm" absent-xz
  fixture "$vm" older-xz
  run_case "$vm" upgrade xz
  fixture "$vm" absent-xz
  fixture "$vm" older-xz
  run_case "$vm" command brew upgrade
  fixture "$vm" absent-jq
  run_case "$vm" native link-conflict
  fixture "$vm" repair-jq
  run_case "$vm" command brew install jq
  run_case "$vm" survey 'hello zstd wget'
  run_case "$vm" command --age-exception 'lz4=Disposable VM acceptance after verified bottle age hold' brew install zstd jq
  printf 'Local VM acceptance suite passed. Recheck guest evidence, then run finish.\n'
}
finish() {
  [ "$#" -eq 1 ] || usage
  vm=$1; valid_name "$vm"; require_guest "$vm"
  # GH CLI may fall back to a plaintext guest config. Logout first, then
  # discard only this VM's private credentials; never touch host auth state.
  guest "$vm" /usr/bin/env -i HOME="$guest_home" GH_CONFIG_DIR="$guest_home/.config/gh" PATH="$guest_path" \
    "$guest_gh" auth logout --hostname github.com >/dev/null 2>&1 || true
  guest "$vm" /bin/sh -c 'rm -rf /private/tmp/bw-acceptance/home/.config/gh /private/tmp/bw-acceptance/home/.local/state/gh /private/tmp/bw-acceptance/home/device.log /private/tmp/bw-acceptance/home/device.pid'
  tart_run stop "$vm"
  printf 'Guest credentials removed and VM stopped. If device login was approved, also revoke its GitHub CLI OAuth grant in your account settings.\n'
}
[ "$#" -ge 1 ] || usage
command=$1; shift
case "$command" in
  prepare) prepare "$@" ;;
  auth) auth "$@" ;;
  suite) suite "$@" ;;
  run) run_case "$@" ;;
  fixture) fixture "$@" ;;
  finish) finish "$@" ;;
  *) usage ;;
esac
