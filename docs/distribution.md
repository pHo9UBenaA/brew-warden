# Distribution

The archive contains `bwd`, its `brewwarden` alias, checksums, documentation
and licenses. No Homebrew source, portable Ruby, runtime inventory or attestation
verifier executable is bundled. Users need an existing Homebrew installation
and authenticated installed gh within the [current supported scope](support.md).
The product archive targets
**darwin/arm64 only**; an unsupported installation cannot execute a protected
mutation. BrewWarden does not install or upgrade either command.
Add the extracted directory to PATH; no privileged installation or automatic
update is performed. Do not relocate an x86_64 Homebrew tree to `/opt/homebrew`
or bypass the runtime fingerprint to make an unsupported installation appear
compatible.

## Trust and verification

The source repository is `https://github.com/pHo9UBenaA/brew-warden`. Local builds
are unsigned development distributions. A checksum verifies transport integrity
or repeatability, not publisher identity. Trust reviewed source and dependencies,
verify the archive digest before extraction, and check `SHA256SUMS` in the
extracted directory. A public release needs publisher-authenticated provenance
for the exact source and archive through a trusted release channel. Local builds
do not imply publication, signing or Apple notarization.

## Reproducible build

Use the Go version in `.go-version`. Commit verified changes, then run:

```sh
./scripts/build-product.sh darwin/arm64
```

The offline build extracts committed source twice, sets a build marker bound to
the source revision and forces compilation. The binary and full archive must
match byte for byte across builds. Archive metadata excludes host paths, users
and timestamps. The build retains toolchain and application module inventories;
it does not install, publish or modify Homebrew. The marker is not publisher
authentication or an execution permit: the supported installed Homebrew tree,
verified candidate evidence and complete plan are checked on each operation.

Run the archive with the [local-only Tart acceptance runner](../scripts/macos-vm.md)
in a disposable Apple Silicon macOS VM with supported installed Homebrew and gh,
including doctor, installation, upgrade, exceptions, interruption, failure and
fresh retry. The VM cases are explicitly build-tagged out of baseline CI.
For reproducible checks and their limits, see [verification](verification.md).
Changes to the installed-runtime identity or execution binding require renewed
native acceptance. The application Go module uses the standard library; Homebrew and gh remain external compatibility and security
dependencies. Local acceptance is not publisher authentication.
