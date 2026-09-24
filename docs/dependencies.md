# Dependencies and implementation

Go is the product and harness language. Use a supported patched Go toolchain;
the module's Go 1.24 directive is a compatibility floor, not a release pin.
The public module identity is `github.com/pHo9UBenaA/brew-warden`.

## Dependency policy

- Prefer the standard library. Additional modules require a documented rationale
  and an updated dependency gate. Count transitive libraries, external commands,
  native/runtime components, and build/CI tools as part of the trust surface.
- Start with standard-library CLI handling, JSON configuration, and minimal owned-process state.
  No application cgo, unsafe, plugins, dynamic loading, or executable configuration.
- Prefer demonstrated Homebrew capabilities before adding verification libraries
  or executables. Follow [Homebrew integration](homebrew-integration.md) for
  guarantee boundaries, evidence, compatibility, and delegated dependencies.
- Do not categorically require embedded verification or prohibit Homebrew's own
  use of gh. Existing or Homebrew-managed helpers are candidates, not assumed
  bundled components. Account for installation, updates, credentials, bootstrap
  exceptions, and unplanned mutations before enabling such a path. Do not make
  users manually assemble a separate toolchain or silently add unrelated helpers.
- Implement missing cryptographic guarantees with maintained libraries when
  needed; do not hand-roll OpenPGP, Sigstore, or JWS protocols. Audit selected
  modules and update dependency gates. Unsupported required verification holds
  the operation, regardless of which component was expected to provide it.
- Development tools are separate from product dependencies. Pin additional tools
  explicitly; tests must not implicitly download tools or modules.

## Input and process boundaries

Use typed evidence and constructors. Zero/unknown states are unassessed, never
allowed; do not use `map[string]any` for policy decisions.

Test the selected JSON API's behavior: configuration and plans must reject
unknown fields, duplicate keys, ambiguous field casing, invalid UTF-8, and trailing
values. External APIs may add irrelevant fields, but decision-bearing fields
require strict validation. Use maintained parsers.

Launch fixed executables with argument arrays, never shell-built commands.
Constrain executable paths, options, environment, resource limits, and output.

## Installed attestation command

The [provenance adapter](../internal/adapters/attestation/README.md) delegates
cryptography to installed GitHub CLI 2.66.0 through 2.101.0, with mandatory
exact-digest signer and verified-Rekor-time output checks. It is not bundled,
installed, downloaded or updated by BrewWarden. A missing or incompatible
command holds operations requiring its evidence; its version and trust roots
remain explicit product dependencies even though the application module uses
only the standard library.

## Release requirements

Build fixed source with a supported pinned toolchain; record tool and dependency
versions and inspect target-specific native links. Verify reproducibility per
platform, distinguishing unsigned outputs from signed/notarized artifacts.
Pin CI actions to full commits. A `go.sum` entry is not author authentication.

Distribute verifiable digests and signatures/attestations with an explicit trust
origin. Do not implement automatic self-update initially or replace a verifier
within an attempt that relies on it. Released binaries must not require Go on
the user's machine.
