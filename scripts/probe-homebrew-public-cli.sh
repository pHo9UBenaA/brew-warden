#!/bin/sh
# Development-only acceptance for a public-command execution candidate.
# This does not enable product mutations or prove every binding requirement.
set -eu
[ "$#" -ge 1 ] && [ "$#" -le 2 ] || { echo 'usage: probe-homebrew-public-cli.sh WORKSPACE [install|upgrade]' >&2; exit 2; }
mode=${2:-install}
case "$mode" in install|upgrade) ;; *) echo 'unsupported probe operation' >&2; exit 2 ;; esac
case "$(/usr/sbin/sysctl -n hw.model)" in VirtualMac*) ;; *) echo 'requires disposable macOS VM' >&2; exit 2 ;; esac
root=$1
case "$root" in /private/tmp/bwd-cli-*) ;; *) echo 'requires dedicated VM workspace' >&2; exit 2 ;; esac
suffix=${root#/private/tmp/bwd-cli-}
case "$suffix" in ''|*[!a-zA-Z0-9_-]*) echo 'invalid workspace name' >&2; exit 2 ;; esac
[ "$(cd "$root" && pwd -P)" = "$root" ] || { echo 'workspace must not be a symlink' >&2; exit 2; }
[ -d "$root/cache" ] && [ -x /opt/homebrew/bin/brew ]
if [ "$mode" = install ]; then
  [ ! -e /opt/homebrew/Cellar/jq ] && [ ! -e /opt/homebrew/Cellar/oniguruma ] || { echo 'requires absent probe candidates' >&2; exit 2; }
else
  [ -d /opt/homebrew/Cellar/jq/1.8.1 ] && [ ! -e /opt/homebrew/Cellar/jq/1.8.2 ] && [ -d /opt/homebrew/Cellar/oniguruma/6.9.10 ] || { echo 'requires old jq and current oniguruma fixtures' >&2; exit 2; }
fi
mkdir -p "$root/home" "$root/tmp" "$root/logs" "$root/results"
cat > "$root/public-cli.sb" <<PROFILE
(version 1)
(allow default)
(deny network*)
(deny file-write*)
(allow file-write* (subpath "$root") (subpath "/opt/homebrew") (literal "/dev/null"))
(deny file-write* (subpath "/opt/homebrew/Library") (subpath "/opt/homebrew/.git") (literal "/opt/homebrew/bin/brew") (subpath "$root/inputs") (subpath "$root/cache/downloads") (literal "$root/cache/api/internal/packages.arm64_tahoe.jws.json"))
PROFILE
run() {
  label=$1
  shift
  status=0
  /usr/bin/env -i HOME="$root/home" PATH=/usr/bin:/bin:/usr/sbin:/sbin TMPDIR="$root/tmp" \
    HOMEBREW_CACHE="$root/cache" HOMEBREW_TEMP="$root/tmp" HOMEBREW_LOGS="$root/logs" \
    HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_ANALYTICS=1 HOMEBREW_NO_BOOTSNAP=1 HOMEBREW_CURL_RETRIES=0 \
    HOMEBREW_NO_INSTALL_CLEANUP=1 HOMEBREW_NO_AUTOREMOVE=1 HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1 \
    HOMEBREW_NO_ENV_HINTS=1 HOMEBREW_NO_COLOR=1 \
    /usr/bin/sandbox-exec -f "$root/public-cli.sb" /opt/homebrew/bin/brew "$@" \
    > "$root/results/$label.stdout" 2> "$root/results/$label.stderr" || status=$?
  printf '%s\n' "$status" > "$root/results/$label.status"
  printf '%s: exit %s\n' "$label" "$status"
  return "$status"
}
no_installed_payload() {
  # Homebrew may create an empty rack before a download error. Record that
  # side effect; it must never be reported as an installed or verified keg.
  /usr/bin/find /opt/homebrew/Cellar -print > "$root/results/$1-cellar.txt"
  entries=$(/usr/bin/find /opt/homebrew/Cellar -mindepth 2 -print) || return 1
  [ -z "$entries" ]
}
run version --version
run candidate-info info --json=v2 --formula homebrew/core/jq homebrew/core/oniguruma
if [ "$mode" = upgrade ]; then
  /usr/bin/find /opt/homebrew/Cellar/oniguruma -type f -exec /usr/bin/shasum -a 256 '{}' + > "$root/results/dependency-before-upgrade.sha256"
  run upgrade upgrade --formula --force-bottle homebrew/core/jq
  [ "$(/opt/homebrew/bin/jq --version)" = jq-1.8.2 ]
  /usr/bin/find /opt/homebrew/Cellar/oniguruma -type f -exec /usr/bin/shasum -a 256 '{}' + > "$root/results/dependency-after-upgrade.sha256"
  /usr/bin/cmp "$root/results/dependency-before-upgrade.sha256" "$root/results/dependency-after-upgrade.sha256"
  echo 'Public CLI upgrade preserved dependency files; broader binding remains unverified.'
  exit 0
fi
run cache-paths --cache --formula --bottle-tag=arm64_tahoe homebrew/core/jq homebrew/core/oniguruma
jq_cache=$(sed -n '1p' "$root/results/cache-paths.stdout")
dep_cache=$(sed -n '2p' "$root/results/cache-paths.stdout")
for file in "$jq_cache" "$dep_cache"; do
  case "$file" in "$root"/cache/downloads/*) ;; *) echo 'unexpected cache path' >&2; exit 2 ;; esac
  [ -f "$file" ] && [ ! -L "$file" ]
done
cp "$jq_cache" "$root/jq-original.tar.gz"
printf 'corrupt bottle\n' > "$jq_cache"
if run corrupt-bottle install --formula --force-bottle homebrew/core/jq; then echo 'corrupt bottle accepted' >&2; exit 1; fi
no_installed_payload corrupt-bottle
cp "$root/jq-original.tar.gz" "$jq_cache"
mv "$dep_cache" "$root/dependency-original.tar.gz"
if run missing-dependency install --formula --force-bottle homebrew/core/jq; then echo 'missing dependency accepted' >&2; exit 1; fi
no_installed_payload missing-dependency
mv "$root/dependency-original.tar.gz" "$dep_cache"
run install install --formula --force-bottle homebrew/core/jq
run installed list --formula --versions
/opt/homebrew/bin/jq --version > "$root/results/jq-version.txt"
[ -d /opt/homebrew/Cellar/jq/1.8.2 ] && [ -d /opt/homebrew/Cellar/oniguruma/6.9.10 ]
/usr/bin/find /opt/homebrew/Cellar -type f -exec /usr/bin/shasum -a 256 '{}' + > "$root/results/before-rerun.sha256"
run unchanged install --formula --force-bottle homebrew/core/jq
/usr/bin/find /opt/homebrew/Cellar -type f -exec /usr/bin/shasum -a 256 '{}' + > "$root/results/after-rerun.sha256"
/usr/bin/cmp "$root/results/before-rerun.sha256" "$root/results/after-rerun.sha256"
echo 'Public CLI fresh install and unchanged rerun passed; broader binding remains unverified.'
