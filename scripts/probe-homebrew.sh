#!/bin/sh
# Developer-only, offline probe. Never run against the maintainer's prefix.
set -eu
if [ "$#" -lt 1 ] || [ "$#" -gt 2 ] || [ "$(uname -s)" != Darwin ]; then
  printf 'Usage (macOS): scripts/probe-homebrew.sh /absolute/Homebrew/source [signed-formula.json]\n' >&2
  exit 1
fi
case "$1" in /*) ;; *) exit 1 ;; esac
source_repo=$1
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
revision=edb70f031e4170c780799633a1226ff73e1077f4
ruby_version=4.0.7
probe_root=$(mktemp -d /private/tmp/brewwarden-probe.XXXXXXXX)
printf 'Probe evidence directory: %s\n' "$probe_root"
printf '%s\n' "$revision" > "$probe_root/source-revision"
sw_vers > "$probe_root/os-version"
uname -m > "$probe_root/architecture"
# Retain every run, including failures, for inspection. No host brew is invoked.
mkdir -p "$probe_root/prefix" "$probe_root/home" "$probe_root/cache" "$probe_root/tmp" "$probe_root/logs"
git -C "$source_repo" archive "$revision" > "$probe_root/source.tar"
tar -xf "$probe_root/source.tar" -C "$probe_root/prefix"
mkdir -p "$probe_root/prefix/Library/Homebrew/vendor/portable-ruby"
cp -R "$source_repo/Library/Homebrew/vendor/portable-ruby/$ruby_version" \
  "$probe_root/prefix/Library/Homebrew/vendor/portable-ruby/$ruby_version"
ln -s "$ruby_version" "$probe_root/prefix/Library/Homebrew/vendor/portable-ruby/current"
shasum -a 256 "$probe_root/source.tar" \
  "$probe_root/prefix/Library/Homebrew/vendor/portable-ruby/$ruby_version/bin/ruby" \
  > "$probe_root/inputs.sha256"
cat > "$probe_root/sandbox.sb" <<EOF
(version 1)
(allow default)
(deny network*)
(deny file-write*)
(allow file-write* (subpath "$probe_root") (literal "/dev/null"))
EOF
run_brew() {
  label=$1
  shift
  status=0
  /usr/bin/sandbox-exec -f "$probe_root/sandbox.sb" /usr/bin/env -i \
    HOME="$probe_root/home" PATH=/usr/bin:/bin:/usr/sbin:/sbin \
    TMPDIR="$probe_root/tmp" XDG_CONFIG_HOME="$probe_root/home/config" \
    HOMEBREW_CACHE="$probe_root/cache" HOMEBREW_LOGS="$probe_root/logs" \
    HOMEBREW_TEMP="$probe_root/tmp" HOMEBREW_NO_AUTO_UPDATE=1 \
    HOMEBREW_NO_ANALYTICS=1 HOMEBREW_NO_INSTALL_FROM_API=1 \
    HOMEBREW_NO_ENV_HINTS=1 HOMEBREW_NO_COLOR=1 HOMEBREW_DEVELOPER=1 \
    HOMEBREW_NO_INSTALL_CLEANUP=1 HOMEBREW_NO_BOOTSNAP=1 \
    "$probe_root/prefix/bin/brew" "$@" \
    > "$probe_root/$label.stdout" 2> "$probe_root/$label.stderr" || status=$?
  printf '%s\n' "$status" > "$probe_root/$label.status"
  printf '%s: exit %s\n' "$label" "$status"
  return "$status"
}
run_brew prefix --prefix
test "$(cat "$probe_root/prefix.stdout")" = "$probe_root/prefix"
run_brew version --version
tap_dir="$probe_root/prefix/Library/Taps/brewwarden/homebrew-probe"
mkdir -p "$tap_dir/Formula"
cat > "$tap_dir/Formula/probe-leaf.rb" <<'EOF'
class ProbeLeaf < Formula
  desc "Offline binding probe; never install"
  homepage "https://example.invalid"
  url "https://example.invalid/probe-leaf-1.0.tar.gz"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000"
  def install
    odie "Probe fixtures must never be installed"
  end
end
EOF
run_brew missing-bottle verify --json --deps brewwarden/probe/probe-leaf
test "$(cat "$probe_root/missing-bottle.stdout")" = '[]'
grep -q 'Bottle for tag .* is unavailable' "$probe_root/missing-bottle.stderr"
printf 'Reproduced: exit zero with no verified subjects. This is not execution binding.\n'
cat > "$tap_dir/Formula/probe-root.rb" <<'EOF'
class ProbeRoot < Formula
  desc "Offline binding probe; never install"
  homepage "https://example.invalid"
  url "https://example.invalid/probe-root-1.0.tar.gz"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000"
  depends_on "brewwarden/probe/probe-leaf"
  def install
    odie "Probe fixtures must never be installed"
  end
end
EOF
run_brew trust-fixtures trust brewwarden/probe
run_brew preview-before install --dry-run --formula brewwarden/probe/probe-root
grep -q 'probe-leaf' "$probe_root/preview-before.stdout"
sed 's/ProbeLeaf/ProbeExtra/g; s/probe-leaf/probe-extra/g' \
  "$tap_dir/Formula/probe-leaf.rb" > "$tap_dir/Formula/probe-extra.rb"
sed '/depends_on/a\
  depends_on "brewwarden/probe/probe-extra"
' "$tap_dir/Formula/probe-root.rb" > "$probe_root/changed.rb"
cp "$probe_root/changed.rb" "$tap_dir/Formula/probe-root.rb"
run_brew preview-after install --dry-run --formula brewwarden/probe/probe-root
grep -q 'probe-extra' "$probe_root/preview-after.stdout"
if grep -q 'probe-extra' "$probe_root/preview-before.stdout"; then
  exit 1
fi
printf 'Reproduced: a later preview resolves changed dependencies; no saved plan is consumed.\n'
cp "$script_dir/probe-homebrew-integrity.rb" "$probe_root/integrity-probe.rb"
run_brew checksum-cache ruby "$probe_root/integrity-probe.rb" "$probe_root"
if run_brew force-no-bottle install --force-bottle --formula brewwarden/probe/probe-leaf; then
  printf 'Expected source-only root formula to be refused.\n' >&2
  exit 1
fi
grep -q 'has no bottle' "$probe_root/force-no-bottle.stderr"
if [ "$#" -eq 2 ]; then
  cp "$2" "$probe_root/formula.jws.json"
  cp "$script_dir/probe-homebrew-metadata.rb" "$probe_root/metadata-probe.rb"
  run_brew signed-metadata ruby "$probe_root/metadata-probe.rb" "$probe_root/formula.jws.json"
  printf 'Verified official metadata; rejected changed payload and missing signature.\n'
fi
