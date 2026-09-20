# Distribution

The supported bundle is `brewwarden-darwin-arm64.tar.gz`: one self-contained
user-owned directory with `bwd`, its `brewwarden` alias, a pinned runtime,
per-file checksums, third-party licenses and notices. No Go, user-installed gh,
Homebrew-managed helper bootstrap, automatic update or privileged installer is
required. Existing Homebrew must use `/opt/homebrew` on Apple Silicon macOS 26.x.

## Trust origin

The source identity is `https://github.com/pHo9UBenaA/brew-warden`. Local build
outputs are unsigned development distributions, not published releases or
Apple-notarized software. Their SHA-256 sums demonstrate repeatability and
transport integrity only; a checksum delivered with an untrusted archive does
not authenticate its publisher. For a local build, trust the reviewed source
revision and pinned dependency inputs. Verify the outer archive digest before
extracting and the inner `SHA256SUMS` after extracting.

Publishing is a separate action. Before distributing a public release, attach
publisher-authenticated release provenance identifying this repository, the
reviewed source commit and the exact archive digest, through a trusted release
channel. Do not claim a signing identity or notarization that was not actually
performed. There is no self-update path accepting unauthenticated replacements.

## Reproducible offline build

Use Go from `.go-version`. First explicitly build the attestation helper with
`scripts/build-verifier.sh darwin/arm64`, retaining its source/module inventory.
Package the pinned Homebrew source, portable Ruby bottle and helper with
`go run ./tools/runtime-pack` (run without arguments for usage). That tool checks
all dependency pins; the runtime manifest digest must be
`9424050677790a1c88c66ab769c5167d59a874cd6a02074665268084c97e758b`.

Prepare a directory of original native license documents:

- `Ruby-LEGAL` and `Ruby-BSDL`, extracted from the original
  `https://cache.ruby-lang.org/pub/ruby/4.0/ruby-4.0.7.tar.gz` archive with SHA-256
  `911ace20f90d068ca0e4dda6d0e4f0f81e52e52f2dd4f4004c721e253412e82d`.
- `OpenSSL-LICENSE.txt` from
  `https://raw.githubusercontent.com/openssl/openssl/openssl-4.0.2/LICENSE.txt`.
- `libyaml-LICENSE.txt` from
  `https://raw.githubusercontent.com/yaml/libyaml/0.2.5/License`.

The packager pins all four license digests and rejects substitutions. Runtime
copies retain the original Ruby, Homebrew and bundled gem license files.
The helper's actual binary build information selects its linked Go modules;
module versions/checksums and original license/notice files are included, along
with the Go license. The complete module graph is retained as build evidence.

Commit verified changes, then run:

```sh
GOMODCACHE=/absolute/verified-module-cache \
  ./scripts/build-product.sh darwin/arm64 \
  /absolute/runtime /absolute/verifier-source /absolute/native-notices
```

The build uses two independent source extractions and forced compilation. Both
binaries and complete compressed archives must match byte for byte. Archive
metadata is deterministic; host paths, users and timestamps are excluded. The
build runs offline and never publishes, signs, installs or changes Homebrew.
Run the resulting archive in a disposable macOS VM before release, including
install, upgrade with unchanged dependencies, exceptions, interruption/recovery
and refusal cases. Changing any runtime pin requires renewed native acceptance.

## Dependency scope

The product Go module has no third-party imports. This is not a zero-dependency
product: the runtime includes Homebrew, portable Ruby, its bundled gems, static
OpenSSL 4.0.2 and libyaml 0.2.5, and the separately built maintained attestation
helper. System frameworks and native links must be inspected per target. The
helper is deliberately limited to attestation verification; it does not link the
full GitHub CLI OpenPGP command surface. Build tools and their module inventories
are separate from runtime dependencies, and both require vulnerability review.
