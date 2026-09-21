# Distribution

The archive contains `bwd`, its `brewwarden` alias, `runtime/manifest.json`, the
attestation helper, checksums and licenses. It reuses an existing supported
Homebrew installation; Homebrew and Ruby are not redistributed. Users do not need
Go or a separate gh installation. Keep the extracted directory together and add
it to PATH. No privileged installation or automatic update is performed.

## Trust and verification

The source repository is `https://github.com/pHo9UBenaA/brew-warden`. Local builds
are unsigned development distributions. A checksum verifies transport integrity
or repeatability, not publisher identity. Trust the reviewed source and dependency
inputs, verify the archive digest before extraction, and check `SHA256SUMS` in the
extracted directory. A future public release needs publisher-authenticated
provenance for the exact source and archive through a trusted release channel.
Local builds do not imply publication, signing or Apple notarization.

## Reproducible build

Use the Go version in `.go-version`. Explicitly build the maintained attestation
helper with `scripts/build-verifier.sh darwin/arm64`, retaining its module sources.
Run `go run ./tools/runtime-pack` with the reviewed Homebrew source, portable Ruby
archive, helper and a new output directory; run without arguments for usage.
The tool validates pinned inputs and generates the inventory documented in the
[Homebrew contract](../internal/adapters/homebrew/README.md). Homebrew files are
used to generate that inventory, then excluded from the resulting distribution.

Commit verified changes, then run:

```sh
GOMODCACHE=/absolute/verified-module-cache \
  ./scripts/build-product.sh darwin/arm64 \
  /absolute/runtime /absolute/verifier-source
```

The offline build extracts committed source twice and forces compilation. The
binaries and complete archives must match byte for byte. Archive metadata excludes
host paths, users and timestamps. Actual helper build information selects the
linked module license files and notices. The build retains toolchain and module
inventories; it does not install, publish or modify Homebrew.

Run the resulting archive in a disposable macOS VM, including doctor, installation,
upgrade, policy exceptions, failure/reconciliation and history. Changes to runtime
pins or binding behavior require renewed acceptance. The application Go module
uses the standard library; the maintained helper has its own reviewed Go module
graph. Reusing Homebrew does not make its Ruby runtime a BrewWarden implementation
or remove it from compatibility and security maintenance considerations.
