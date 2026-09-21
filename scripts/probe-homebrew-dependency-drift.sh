#!/bin/sh
# Development-only reproducer: a CLI handoff does not preserve preflight state.
# Success means the limitation was reproduced, not that execution binding passed.
set -eu
[ "$#" -eq 2 ] || { echo 'usage: probe-homebrew-dependency-drift.sh VM_WORKSPACE VERIFIED_CACHE' >&2; exit 2; }
case "$(/usr/sbin/sysctl -n hw.model)" in VirtualMac*) ;; *) echo 'requires disposable macOS VM' >&2; exit 2 ;; esac
root=$1
cache=$2
case "$root" in /private/tmp/bwd-cli-*) ;; *) exit 2 ;; esac
case "${root#/private/tmp/bwd-cli-}" in ''|*[!a-zA-Z0-9_-]*) exit 2 ;; esac
[ "$(cd "$root" && pwd -P)" = "$root" ] || exit 2
case "$cache" in "$root"/*/cache) ;; *) echo 'cache must be inside VM workspace' >&2; exit 2 ;; esac
case "$cache" in *[!a-zA-Z0-9_./-]*) echo 'invalid cache path' >&2; exit 2 ;; esac
[ "$(cd "$cache" && pwd -P)" = "$cache" ] || exit 2
[ -f "$cache/api/internal/packages.arm64_tahoe.jws.json" ]
[ -d /opt/homebrew/Cellar/pcre2/10.48 ]
mkdir -p "$root/drift/home" "$root/drift/tmp" "$root/drift/logs" "$root/drift/ordinary-cache"
cat > "$root/drift/offline.sb" <<PROFILE
(version 1)
(allow default)
(deny network*)
(deny file-write*)
(allow file-write* (subpath "$root") (subpath "/opt/homebrew") (literal "/dev/null"))
(deny file-write* (subpath "/opt/homebrew/Library") (subpath "/opt/homebrew/.git") (literal "/opt/homebrew/bin/brew") (subpath "$cache/downloads") (subpath "$cache/api"))
PROFILE
run() {
  label=$1
  mode=$2
  shift 2
  selected_cache=$cache
  if [ "$mode" = ordinary ]; then
    selected_cache=$root/drift/ordinary-cache
    set -- /opt/homebrew/bin/brew "$@"
  else
    set -- /usr/bin/sandbox-exec -f "$root/drift/offline.sb" /opt/homebrew/bin/brew "$@"
  fi
  status=0
  /usr/bin/env -i HOME="$root/drift/home" PATH=/usr/bin:/bin:/usr/sbin:/sbin TMPDIR="$root/drift/tmp" \
    HOMEBREW_CACHE="$selected_cache" HOMEBREW_TEMP="$root/drift/tmp" HOMEBREW_LOGS="$root/drift/logs" \
    HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_ANALYTICS=1 HOMEBREW_NO_BOOTSNAP=1 \
    HOMEBREW_NO_INSTALL_CLEANUP=1 HOMEBREW_NO_AUTOREMOVE=1 HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1 \
    HOMEBREW_NO_ENV_HINTS=1 HOMEBREW_NO_COLOR=1 "$@" \
    > "$root/drift/$label.stdout" 2> "$root/drift/$label.stderr" || status=$?
  printf '%s\n' "$status" > "$root/drift/$label.status"
  [ "$status" -eq 0 ]
}
run restore-bottle offline reinstall --formula --force-bottle homebrew/core/pcre2
if [ -e /opt/homebrew/Cellar/ripgrep ]; then
  run remove-target offline uninstall --formula homebrew/core/ripgrep
fi
[ ! -e /opt/homebrew/Cellar/ripgrep ]
lib=/opt/homebrew/Cellar/pcre2/10.48/lib/libpcre2-8.0.dylib
/usr/bin/shasum -a 256 "$lib" > "$root/drift/verified-dependency.sha256"
cp /opt/homebrew/Cellar/pcre2/10.48/INSTALL_RECEIPT.json "$root/drift/verified-receipt.json"
# This is an ordinary user operation, not a modified Homebrew or injected script.
run external-rebuild ordinary reinstall --formula --build-from-source --debug-symbols pcre2
/usr/bin/shasum -a 256 "$lib" > "$root/drift/rebuilt-dependency.sha256"
if /usr/bin/cmp -s "$root/drift/verified-dependency.sha256" "$root/drift/rebuilt-dependency.sha256"; then
  echo 'fixture did not produce different dependency bytes' >&2
  exit 1
fi
cp /opt/homebrew/Cellar/pcre2/10.48/INSTALL_RECEIPT.json "$root/drift/rebuilt-receipt.json"
run candidate-install offline install --formula --force-bottle homebrew/core/ripgrep
/usr/bin/shasum -a 256 "$lib" > "$root/drift/consumed-dependency.sha256"
/usr/bin/cmp "$root/drift/rebuilt-dependency.sha256" "$root/drift/consumed-dependency.sha256"
[ -x /opt/homebrew/Cellar/ripgrep/15.2.0/bin/rg ]
/opt/homebrew/Cellar/ripgrep/15.2.0/bin/rg --version > "$root/drift/installed-version.txt"
echo 'Reproduced: offline public install accepted a dependency changed after preflight.'
