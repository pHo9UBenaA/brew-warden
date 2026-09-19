# Dependencies and implementation

Go is used for the harness and remains the proposed product language. Product
feasibility is not yet demonstrated. Use a supported patched Go toolchain;
the module's Go 1.24 directive is a compatibility floor, not a release pin.
The local module path must be replaced with the actual public identity before
publishing imports.

## Dependency policy

- Prefer the standard library. Additional modules require a documented rationale
  and an updated dependency gate. Count transitive libraries, external commands,
  native/runtime components, and build/CI tools as part of the trust surface.
- Start with standard-library CLI handling, JSON configuration, and file records.
  No application cgo, unsafe, plugins, dynamic loading, or executable configuration.
- Use maintained verifiers for OpenPGP, Sigstore, and JWS; do not reimplement their
  trust protocols to avoid dependencies. Missing required verifiers hold the
  operation; never install them implicitly.
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

## Release requirements

Build fixed source with a supported pinned toolchain; record tool and dependency
versions and inspect target-specific native links. Verify reproducibility per
platform, distinguishing unsigned outputs from signed/notarized artifacts.
Pin CI actions to full commits. A `go.sum` entry is not author authentication.

Distribute verifiable digests and signatures/attestations with an explicit trust
origin. Do not implement automatic self-update initially or replace a verifier
within an attempt that relies on it. Released binaries must not require Go on
the user's machine.
