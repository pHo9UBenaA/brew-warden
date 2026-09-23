# Distribution

The archive contains `bwd`, its `brewwarden` alias, a reviewed Homebrew
compatibility manifest, checksums and licenses. Neither Homebrew, portable Ruby
nor an attestation verifier executable is bundled. Users need installed,
supported Homebrew and GitHub CLI (`gh` 2.101.0). BrewWarden does not install
or upgrade either command. Keep the extracted directory together and add it
to PATH; no privileged installation or automatic update is performed.

## Trust and verification

The source repository is `https://github.com/pHo9UBenaA/brew-warden`. Local builds
are unsigned development distributions. A checksum verifies transport integrity
or repeatability, not publisher identity. Trust reviewed source and dependency
inputs, verify the archive digest before extraction, and check `SHA256SUMS` in the
extracted directory. A public release requires publisher-authenticated provenance
for the exact source and archive through a trusted release channel. Local builds
do not imply publication, signing or Apple notarization.

## Reproducible build

Use the Go version in `.go-version`. Run `go run ./tools/runtime-pack` with the
reviewed Homebrew source, portable Ruby archive and a new output directory;
run without arguments for usage. It validates the pinned inputs and generates
the manifest documented in the [Homebrew contract](../internal/adapters/homebrew/README.md).
Homebrew and Ruby bytes are excluded from the output. This compatibility
inventory is still part of the distribution and remains migration work.

Commit verified changes, then run:

```sh
./scripts/build-product.sh darwin/arm64 /absolute/runtime
```

The offline build extracts committed source twice and forces compilation. The
binaries and complete archives must match byte for byte. Archive metadata excludes
host paths, users and timestamps. The build retains toolchain and application
module inventories; it does not install, publish or modify Homebrew.

Run the archive in a disposable Apple Silicon macOS VM, including doctor,
installation, upgrade, policy exceptions, interruption, failure and fresh retry.
Changes to runtime pins or binding behavior require renewed native acceptance.
The application Go module uses the standard library; installed Homebrew and gh
remain external compatibility and security dependencies. The public-gh migration
has not passed this native distribution acceptance yet.
