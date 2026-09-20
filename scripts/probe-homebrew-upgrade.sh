#!/bin/sh
# Developer-only continuation of a successful standard-prefix jq VM probe.
set -eu
test "$#" -eq 2
case "$(/usr/sbin/sysctl -n hw.model)" in VirtualMac*) ;; *)
  printf 'Upgrade probes require an Apple virtual machine.\n' >&2
  exit 1 ;;
esac
case "$1" in /private/tmp/brewwarden-probe.*) ;; *) exit 1 ;; esac
case "$1" in *[!A-Za-z0-9_./-]*) exit 1 ;; esac
probe_root=$1
test "$(CDPATH= cd -P -- "$probe_root" && pwd)" = "$probe_root"
probe_prefix=/opt/homebrew
test "$(cat "$probe_root/install-prefix")" = "$probe_prefix"
test "$(cat "$probe_root/install-after.status")" = 0
test ! -e "$probe_root/upgrade-started"
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
. "$script_dir/probe-homebrew-command.sh"
cp "$script_dir/probe-homebrew-install.rb" "$probe_root/upgrade-acceptance.rb"
mkdir "$probe_root/upgrade-inputs"
old_bottle=jq--1.8.1.arm64_tahoe.bottle.tar.gz
cp "$2/$old_bottle" "$probe_root/upgrade-inputs/"
cp "$2/jq-old-bundle.jsonl" "$probe_root/upgrade-inputs/"
test "$(shasum -a 256 "$probe_root/upgrade-inputs/$old_bottle" | cut -d ' ' -f 1)" = 90b0fe4ad51959380f16fe8d84c5be8ab525478c32f1f7034c72d99de2442c9b
/usr/bin/sandbox-exec -f "$probe_root/verifier.sb" /usr/bin/env -i \
  HOME="$probe_root/gh-home" PATH=/usr/bin:/bin GH_CONFIG_DIR="$probe_root/gh-home" \
  "$probe_root/inputs/gh" attestation verify "$probe_root/upgrade-inputs/$old_bottle" \
  --bundle "$probe_root/upgrade-inputs/jq-old-bundle.jsonl" --repo Homebrew/homebrew-core \
  --cert-identity 'https://github.com/Homebrew/homebrew-core/.github/workflows/dispatch-build-bottle.yml@refs/heads/main' \
  --cert-oidc-issuer https://token.actions.githubusercontent.com --deny-self-hosted-runners \
  --format json > "$probe_root/old-jq-verified.json" 2> "$probe_root/old-jq-verified.stderr"
cat >> "$probe_root/sandbox.sb" <<EOF
(deny file-write* (subpath "$probe_root/upgrade-inputs")
  (literal "$probe_root/upgrade-acceptance.rb"))
EOF
# Revalidate the completed empty-prefix scenario before changing this fixture.
run_brew upgrade-fixture-check ruby "$probe_root/upgrade-acceptance.rb" "$probe_root" after jq
touch "$probe_root/upgrade-started"
# This fixture preparation is not a product downgrade or an approved old version.
# The authenticated old bottle establishes an actual native installed state.
run_brew remove-fixture-jq uninstall --formula jq
run_brew install-old-jq install --formula --force-bottle "$probe_root/upgrade-inputs/$old_bottle"
run_brew upgrade-before ruby "$probe_root/upgrade-acceptance.rb" "$probe_root" upgrade-before jq
run_brew upgrade-frozen ruby "$probe_root/bound-probe.rb" "$probe_root" upgrade
run_brew upgrade-after ruby "$probe_root/upgrade-acceptance.rb" "$probe_root" upgrade-after jq
/usr/bin/sandbox-exec -f "$probe_root/sandbox.sb" /usr/bin/env -i \
  "$probe_prefix/bin/jq" -n '"brewwarden" | test("^brew")' > "$probe_root/upgrade-smoke.stdout"
test "$(cat "$probe_root/upgrade-smoke.stdout")" = true
printf 'Verified native jq upgrade with the existing oniguruma dependency.\n'
