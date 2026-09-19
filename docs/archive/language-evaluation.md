# Language and dependency decision

Historical assessment; current implementation rules are in
[Dependencies and implementation](../dependencies.md).

Status at evaluation: Go is used for the development harness and is the leading proposal for
the product. Product feasibility probes and language benchmarks are not complete.
Research date: 2026-09-19.

## Requirements and recommendation

Require static types, memory-safe handling of untrusted input, HTTP/TLS/JSON,
filesystem and process support, straightforward binary distribution, and a
maintainable toolchain. Go fits the workload with a broad standard library and
few application-level dependencies. Users should not need Go installed to run a
released product binary.

This does not establish that Go is universally safer. Its runtime and standard
library are trusted code that can contain vulnerabilities. Statically linked
fixes require rebuilding and redistributing the application. OS services and
certificate stores also remain dependencies.

| Candidate | Properties for this project | Decision |
| --- | --- | --- |
| Go | Static types; GC and bounds checks in ordinary code; standard HTTP/TLS, JSON, hashing, arguments, and process APIs | Leading candidate |
| Rust | Ownership, enums, and exhaustive matching help represent valid states; HTTP/TLS/JSON normally add crates or external components | Strong alternative if type-level guarantees outweigh the dependency/maintenance tradeoff |
| MoonBit | Static types, enums, native output; official async library supplies I/O and processes, but its inspected README calls it experimental and describes TLS through OpenSSL | Feasible, but no demonstrated dependency advantage for this workload |
| Nushell | Strong and gradual typing, with parse-time and runtime checks; requires the Nu environment | Useful optional operations tooling, not the preferred product implementation |
| Nix | Dynamically typed build/configuration DSL; useful for controlled builds but not a statically typed CLI implementation | Optional build definition later, not the product language |

No dependency-count, binary-size, or performance comparison was measured. These
are workload-specific judgments, not language popularity rankings.

## Count the complete dependency surface

Count linked direct/transitive libraries, external commands and their closures,
OS/native/runtime components, and compiler/test/CI/signing tools. Moving code
behind a subprocess does not remove its trust or update obligations.

| Component | Initial policy |
| --- | --- |
| Third-party Go modules | Target zero, with justified exceptions when safer than custom code |
| CLI and configuration | Standard library, JSON; no CLI framework or YAML/TOML parser initially |
| Storage | Small file-based records; no ORM, SQLite dependency, or custom database |
| Native application code | No application cgo, unsafe, plugins, or dynamic loading; try CGO_ENABLED=0 and inspect each target's actual links |
| Homebrew | Required managed system; count its runtime rather than claiming dependency-free operation |
| Bottle attestation | One supported gh verifier initially; hold when absent, never silently bootstrap it |
| Upstream OpenPGP | Optional gpgv or equivalent for enabled packages, with dedicated allowed keys |
| Cask signatures | Native macOS verification facilities, with format-specific adapter tests |
| Development tools | Go test/vet/fuzz and separately pinned govulncheck; not product runtime prerequisites |

All-in-one means one user workflow, policy, and history. It does not require
implementing every signature scheme in one executable. Eliminating external
verifiers would require a fresh comparison with maintained verification libraries.

## Go-specific safeguards

Go does not provide Rust-style exhaustive enum checking. Make zero values
unassessed, reject unknown states, use typed records and constructors, and avoid
one ambiguous `safe` boolean. Keep `map[string]any` out of policy decisions.

Select the JSON API for the adopted stable Go version and test its exact
semantics. Reject duplicate keys, ambiguous field casing, invalid UTF-8, trailing
values, and unknown fields in configuration/plans. Permit irrelevant API additions
while strictly validating decision-bearing fields. The official JSON docs explain
v1/v2 behavior differences; do not write a new parser to avoid a dependency.

Use os/exec with a fixed executable and argument array. It does not implicitly
invoke a shell, but path selection, option injection, environment, resource
limits, and output handling still require explicit controls.

## Cryptography and release discipline

Use standard SHA-256. Do not implement OpenPGP, Sigstore, or JWS trust verification
from primitive signature operations merely to claim zero modules. Verification
includes identity, trust-root updates, certificates, revocation, timestamps,
transparency evidence, and subject binding.

Build from fixed source and a supported patched Go version; disable implicit
toolchain/module downloads during verification. Record toolchain and dependency
versions. A go.sum entry is not proof of authorship or release authenticity.
Pin CI actions to full commits. Test reproducibility per target and distinguish
unsigned outputs from code-signed/notarized distribution artifacts.

Do not initially implement automatic self-update. Provide digests plus signatures
or attestations with a verifiable trust origin. Initial installation necessarily
trusts an external bootstrap path. Do not replace a verifier in the same attempt
that relies on it. Nix and Nushell are optional developer preferences, not required
runtime or development dependencies.

The repository harness's Go 1.24 language floor accommodates the currently
available local toolchain. It is not an approved production compiler version.
Release selection must use a supported patch release and record validation.

## Feasibility probes before product implementation

- Build macOS arm64 and x86_64 artifacts, run without an installed Go toolchain,
  and enumerate native/runtime dependencies.
- Exercise HTTPS, strict JSON, hashing, durable records, and bounded subprocess
  use without extra modules.
- Establish gh output, authentication, settings, cache behavior, identity and
  subject checks against actual artifacts.
- Demonstrate binding a Homebrew execution to the verified dependency plan.
- Test conflicting writes, interruption, and hostile input without complex tooling.

Only compare a second implementation if a concrete Go limitation justifies it.
Avoid multiple implementation languages in the product.

## Primary sources

- [Go security](https://go.dev/doc/security/), [standard library](https://pkg.go.dev/std), [toolchains](https://go.dev/doc/toolchain).
- [HTTP](https://pkg.go.dev/net/http), [TLS](https://pkg.go.dev/crypto/tls), [os/exec](https://pkg.go.dev/os/exec), [JSON](https://pkg.go.dev/encoding/json).
- [Rust standard library](https://doc.rust-lang.org/std/), [ownership](https://doc.rust-lang.org/book/ch04-01-what-is-ownership.html).
- [MoonBit fundamentals](https://docs.moonbitlang.com/en/latest/language/fundamentals.html), [FFI](https://docs.moonbitlang.com/en/latest/language/ffi.html), [official async library](https://github.com/moonbitlang/async).
- [Nushell types](https://www.nushell.sh/lang-guide/chapters/types/00_types_overview.html), [Nix language](https://nix.dev/tutorials/nix-language).

Latest and nightly documentation can change. Pin implementation contracts to the
versions actually selected and tested.
