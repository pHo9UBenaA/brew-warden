#!/bin/sh
# Developer-only, isolated probe. Never run against the maintainer's prefix.
set -eu
if [ "$#" -lt 1 ] || [ "$#" -gt 6 ] || [ "$#" -eq 3 ] || [ "$(uname -s)" != Darwin ]; then
  printf 'Usage (macOS): scripts/probe-homebrew.sh /absolute/Homebrew/source [signed-formula.json [input-directory /absolute/gh [hello|jq [--vm-prefix]]]]\n' >&2
  exit 1
fi
case "$1" in /*) ;; *) exit 1 ;; esac
source_repo=$1
scenario=${5:-hello}
case "$scenario" in
  hello) recipe_names="hello texinfo"; bottles="hello--2.12.3.arm64_tahoe.bottle.1.tar.gz" ;;
  jq) recipe_names="jq oniguruma autoconf automake libtool m4"; bottles="jq--1.8.2.arm64_tahoe.bottle.1.tar.gz oniguruma--6.9.10.arm64_tahoe.bottle.tar.gz" ;;
  *) exit 1 ;;
esac
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
revision=edb70f031e4170c780799633a1226ff73e1077f4
ruby_version=4.0.7
probe_root=$(mktemp -d /private/tmp/brewwarden-probe.XXXXXXXX)
probe_prefix="$probe_root/prefix"
if [ "$#" -eq 6 ]; then
  # A fixed standard prefix is permitted only inside an Apple virtual machine.
  # Refuse existing state; preparation belongs to the disposable VM driver.
  test "$6" = --vm-prefix
  case "$(/usr/sbin/sysctl -n hw.model)" in VirtualMac*) ;; *)
    printf 'Standard-prefix probes require an Apple virtual machine.\n' >&2
    exit 1 ;;
  esac
  probe_prefix=/opt/homebrew
  test ! -e "$probe_prefix" && test ! -L "$probe_prefix"
fi
printf '%s\n' "$probe_prefix" > "$probe_root/install-prefix"
printf 'Probe evidence directory: %s\n' "$probe_root"
printf '%s\n' "$revision" > "$probe_root/source-revision"
sw_vers > "$probe_root/os-version"
uname -m > "$probe_root/architecture"
# Retain every run, including failures, for inspection. No host brew is invoked.
mkdir -p "$probe_prefix" "$probe_root/home" "$probe_root/cache" "$probe_root/tmp" "$probe_root/logs"
git -C "$source_repo" archive "$revision" > "$probe_root/source.tar"
tar -xf "$probe_root/source.tar" -C "$probe_prefix"
mkdir -p "$probe_prefix/Library/Homebrew/vendor/portable-ruby"
if [ "$#" -ge 4 ]; then
  # The real-bottle probe needs the matching native Ruby, not an Intel host copy.
  cp "$3/portable-ruby.tar.gz" "$probe_root/portable-ruby.tar.gz"
  ruby_sha=$(shasum -a 256 "$probe_root/portable-ruby.tar.gz" | cut -d ' ' -f 1)
  test "$ruby_sha" = e0088dff5614b39387300136ec7a5f95bf1e07589547245c919524fc9e8b4197
  tar -xf "$probe_root/portable-ruby.tar.gz" -C "$probe_prefix/Library/Homebrew/vendor"
else
  cp -R "$source_repo/Library/Homebrew/vendor/portable-ruby/$ruby_version" \
    "$probe_prefix/Library/Homebrew/vendor/portable-ruby/$ruby_version"
fi
ln -s "$ruby_version" "$probe_prefix/Library/Homebrew/vendor/portable-ruby/current"
shasum -a 256 "$probe_root/source.tar" \
  "$probe_prefix/Library/Homebrew/vendor/portable-ruby/$ruby_version/bin/ruby" \
  > "$probe_root/inputs.sha256"
cat > "$probe_root/sandbox.sb" <<EOF
(version 1)
(allow default)
(deny network*)
(deny file-write*)
(allow file-write* (subpath "$probe_root") (subpath "$probe_prefix") (literal "/dev/null"))
EOF
. "$script_dir/probe-homebrew-command.sh"
run_brew prefix --prefix
test "$(cat "$probe_root/prefix.stdout")" = "$probe_prefix"
run_brew version --version
tap_dir="$probe_prefix/Library/Taps/brewwarden/homebrew-probe"
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
cp "$script_dir/probe-homebrew-advisories.rb" "$probe_root/advisory-probe.rb"
run_brew advisory-contract ruby "$probe_root/advisory-probe.rb" "$probe_root"
if run_brew force-no-bottle install --force-bottle --formula brewwarden/probe/probe-leaf; then
  printf 'Expected source-only root formula to be refused.\n' >&2
  exit 1
fi
grep -q 'has no bottle' "$probe_root/force-no-bottle.stderr"
if [ "$#" -ge 2 ]; then
  cp "$2" "$probe_root/formula.jws.json"
  cp "$script_dir/probe-homebrew-metadata.rb" "$probe_root/metadata-probe.rb"
  run_brew signed-metadata ruby "$probe_root/metadata-probe.rb" "$probe_root/formula.jws.json"
  printf 'Verified official metadata; rejected changed payload and missing signature.\n'
fi

if [ "$#" -gt 2 ]; then
  case "$4" in /*) ;; *) exit 1 ;; esac
  mkdir -p "$probe_root/inputs" "$probe_root/gh-home"
  for recipe in $recipe_names; do
    cp "$3/$recipe.rb" "$probe_root/inputs/"
  done
  for bottle_name in $bottles; do
    cp "$3/$bottle_name" "$probe_root/inputs/"
    name=${bottle_name%%--*}
    if [ "$scenario" = hello ]; then
      cp "$3/bundle.jsonl" "$probe_root/inputs/$name-bundle.jsonl"
    else
      cp "$3/$name-bundle.jsonl" "$probe_root/inputs/"
    fi
  done
  cp "$4" "$probe_root/inputs/gh"
  /usr/bin/sandbox-exec -f "$probe_root/sandbox.sb" /usr/bin/env -i \
    HOME="$probe_root/gh-home" PATH=/usr/bin:/bin \
    "$probe_root/inputs/gh" --version > "$probe_root/gh-version"
  grep -q '^gh version 2.62.0 ' "$probe_root/gh-version"
  shasum -a 256 "$probe_root/inputs/gh" >> "$probe_root/inputs.sha256"
  # Bundle verification still fetches Sigstore TUF roots. Only this verifier
  # phase permits network, without credentials and with isolated writable state.
  sed '/(deny network\*)/d' "$probe_root/sandbox.sb" > "$probe_root/verifier.sb"
  for bottle_name in $bottles; do
    name=${bottle_name%%--*}
    workflow=publish-commit-bottles
    if [ "$name" = oniguruma ]; then workflow=dispatch-build-bottle; fi
    /usr/bin/sandbox-exec -f "$probe_root/verifier.sb" /usr/bin/env -i \
      HOME="$probe_root/gh-home" PATH=/usr/bin:/bin GH_CONFIG_DIR="$probe_root/gh-home" \
      "$probe_root/inputs/gh" attestation verify "$probe_root/inputs/$bottle_name" \
      --bundle "$probe_root/inputs/$name-bundle.jsonl" --repo Homebrew/homebrew-core \
      --cert-identity "https://github.com/Homebrew/homebrew-core/.github/workflows/$workflow.yml@refs/heads/main" \
      --cert-oidc-issuer https://token.actions.githubusercontent.com --deny-self-hosted-runners \
      --format json > "$probe_root/inputs/$name-verified-attestation.json" 2> "$probe_root/$name-attestation.stderr"
    if /usr/bin/sandbox-exec -f "$probe_root/verifier.sb" /usr/bin/env -i \
      HOME="$probe_root/gh-home" PATH=/usr/bin:/bin GH_CONFIG_DIR="$probe_root/gh-home" \
      "$probe_root/inputs/gh" attestation verify "$probe_root/inputs/$bottle_name" \
      --bundle "$probe_root/inputs/$name-bundle.jsonl" --repo Homebrew/homebrew-core \
      --cert-identity 'https://github.com/Homebrew/homebrew-core/.github/workflows/not-the-publisher.yml@refs/heads/main' \
      --cert-oidc-issuer https://token.actions.githubusercontent.com --deny-self-hosted-runners \
      --format json > "$probe_root/$name-wrong-identity.stdout" 2> "$probe_root/$name-wrong-identity.stderr"; then
      printf 'Unexpected signer identity accepted.\n' >&2
      exit 1
    fi
    grep -q 'Error: verifying with issuer' "$probe_root/$name-wrong-identity.stderr"
  done
  if [ -f "$3/advisories.json" ]; then
    cp "$3/advisories.json" "$probe_root/advisories.json"
    run_brew advisory-sample ruby "$probe_root/advisory-probe.rb" "$probe_root" "$probe_root/advisories.json"
  fi
  core_dir="$probe_prefix/Library/Taps/homebrew/homebrew-core"
  for recipe in $recipe_names; do
    letter=$(printf '%s' "$recipe" | cut -c 1)
    case "$recipe" in lib*) letter=lib ;; esac
    mkdir -p "$core_dir/Formula/$letter"
    cp "$probe_root/inputs/$recipe.rb" "$core_dir/Formula/$letter/$recipe.rb"
  done
  cp "$script_dir/probe-homebrew-install.rb" "$probe_root/install-probe.rb"
  # Immutable inputs for the confined preflight and execution processes.
  cat >> "$probe_root/sandbox.sb" <<EOF
(deny file-write* (subpath "$probe_root/inputs")
  (subpath "$probe_prefix/Library")
  (literal "$probe_root/formula.jws.json")
  (literal "$probe_root/install-probe.rb"))
EOF
  if /usr/bin/sandbox-exec -f "$probe_root/sandbox.sb" /bin/sh -c 'echo changed >> "$1"' sh \
    "$probe_root/inputs/$bottle_name" 2> "$probe_root/immutable-input.stderr"; then
    printf 'Input protection failed.\n' >&2
    exit 1
  fi
  # Fetch using the authenticated, unmodified core recipe; this creates native
  # OCI metadata/cache paths. This phase is not allowed to install packages.
  run_brew authenticate-inputs ruby "$probe_root/install-probe.rb" "$probe_root" inputs "$scenario"
  brew_sandbox="$probe_root/verifier.sb"
  for bottle_name in $bottles; do
    name=${bottle_name%%--*}
    run_brew "fetch-$name" fetch --force-bottle --formula "homebrew/core/$name"
  done
  unset brew_sandbox
  run_brew inspect-cache ruby "$probe_root/install-probe.rb" "$probe_root" inspect "$scenario"
  cached_bottle=$(cat "$probe_root/cached-bottle-path")
  cp "$cached_bottle" "$probe_root/unchanged-bottle"
  /usr/bin/sandbox-exec -f "$probe_root/sandbox.sb" /bin/sh -c 'printf changed >> "$1"' sh "$cached_bottle"
  if run_brew altered-cache ruby "$probe_root/install-probe.rb" "$probe_root" before "$scenario"; then
    printf 'Altered cached bottle accepted.\n' >&2
    exit 1
  fi
  grep -q 'cache digest mismatch' "$probe_root/altered-cache.stderr"
  /usr/bin/sandbox-exec -f "$probe_root/sandbox.sb" /bin/cp "$probe_root/unchanged-bottle" "$cached_bottle"
  run_brew install-before ruby "$probe_root/install-probe.rb" "$probe_root" before "$scenario"
  if /usr/bin/sandbox-exec -f "$probe_root/sandbox.sb" /bin/sh -c 'echo changed >> "$1"' sh \
    "$(cat "$probe_root/cached-bottle-path")" 2> "$probe_root/immutable-cache.stderr"; then
    printf 'Cache protection failed.\n' >&2
    exit 1
  fi
  run_brew install-frozen install --formula --force-bottle "homebrew/core/$scenario"
  run_brew install-after ruby "$probe_root/install-probe.rb" "$probe_root" after "$scenario"
  if [ "$scenario" = hello ]; then
    bottle_name=hello--2.12.3.arm64_tahoe.bottle.1.tar.gz
    tar -xOf "$probe_root/inputs/$bottle_name" hello/2.12.3/bin/hello > "$probe_root/expected-hello"
    cmp "$probe_root/expected-hello" "$probe_prefix/Cellar/hello/2.12.3/bin/hello"
    /usr/bin/sandbox-exec -f "$probe_root/sandbox.sb" /usr/bin/env -i \
      "$probe_prefix/bin/hello" --greeting=brewwarden > "$probe_root/hello.stdout"
    test "$(cat "$probe_root/hello.stdout")" = brewwarden
  else
    /usr/bin/sandbox-exec -f "$probe_root/sandbox.sb" /usr/bin/env -i \
      "$probe_prefix/bin/jq" -n '"brewwarden" | test("^brew")' > "$probe_root/jq.stdout"
    test "$(cat "$probe_root/jq.stdout")" = true
  fi
  printf 'Verified and installed the official %s closure in the isolated prefix.\n' "$scenario"
fi
