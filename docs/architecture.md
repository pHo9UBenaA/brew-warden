# Architecture

Use hexagonal architecture with mandatory dependency boundaries: deterministic
decisions at the center, application workflows around them, and explicit adapters
for effects. The import rules below are enforced by the repository checker.
Keep abstractions limited to real boundaries; an interface for every function
is not required. This does not relax dependency direction or separation of effects.

```text
cmd -> composition -> CLI + application + adapters
                       |         |
                       v         v
                  application -> ports -> domain
                       |                    ^
                       +--------------------+
                   adapters -> ports + domain
```

Arrows describe source imports, not runtime call order. Applications invoke
injected output ports; adapters implement those ports. Ports carry semantic
operations and failures, never gh flags or HTTP client objects.

| Directory | Responsibility | Allowed project imports |
| --- | --- | --- |
| `internal/domain` | Identity, evidence, decisions, age and emergency rules | domain |
| `internal/ports` | Effect contracts expressed in domain terms | ports, domain |
| `internal/application` | Planning, revalidation, execution orchestration | application, ports, domain |
| `internal/adapters/<name>` | Homebrew, network, verifier, store implementations | own adapter, ports, domain |
| `internal/cli` | Arguments and presentation | cli, application, ports, domain |
| `internal/composition` | Construct and connect implementations | every internal layer |
| `cmd/<name>` | Process entry point | composition |
| `tools` | Development harness, not product code | tools |
| `tests` | Cross-boundary integration tests, test files only | all internal product layers |

Domain standard-library imports are a small allowlist of computation packages.
Ports additionally permit `context` and `io`; application may use those packages
as well. Core code cannot import time, os, networking, execution, serialization,
logging, or host/runtime packages. Use explicit epoch values for policy time;
a clock port belongs to the application boundary. Adapters do not import use
cases or other adapters. Share a real semantic contract inward, or compose
separate adapters outside, instead of creating an adapter utility bucket.

The product packages include the two entrypoints, composition, diagnostic CLI,
pure domain evaluator, and an application execution workflow. The workflow
revalidates a bound session, evaluates policy, durably consumes an attempt before
launch and records actual exit/state facts. The local-state attempt journal
provides immutable transitions and replay rejection. No Homebrew executor is
registered in composition yet; there is no unchecked pass-through. `ports.ConfigSource` connects the CLI to `localstate`,
the adapter owning local JSON schemas and filesystem access. Domain acceptance tests live under `tests` and provide
explicit time and evidence without I/O in the domain. Create other
layers with their first real behavior; empty interfaces and placeholder
applications would not strengthen the design.

The `githubrelease` adapter owns exact publisher mappings, bounded public HTTP
requests, release-response validation and upstream publication observations.
It returns typed evidence plus the exact raw bytes for eventual durable storage.
The diagnostic CLI does not instantiate it or treat it as execution permission.

The `osv` adapter owns mapped candidate advisory queries, pagination, exact-tag
applicability, coverage controls and retained raw observations. Its transport
and schema validation remain outside policy; unavailable coverage is never clean.

The `attestation` adapter owns the pinned verifier process and exact subject /
signer output contract. Its maintained cryptographic runtime is built separately
and inventoried in [dependency rules](dependencies.md); it is not an exception
allowing third-party imports into the pure policy or application layers.

## Homebrew boundary

Keep Homebrew command/environment construction, output decoding, and version
compatibility inside `internal/adapters/homebrew`. Core policy consumes typed
claims, subjects, and verification outcomes, not Homebrew flags. Implement missing
effects in separate adapters and compose them outside; keep additional pure rules
in the core. Follow [Homebrew integration](homebrew-integration.md) for the required
capability records and contract tests. Do not create placeholder abstractions.

## Enforcement

`go run ./tools/repo-check architecture` parses every repository `.go` file,
including platform-tagged files and tests. It checks classification, import
direction, local import resolution, package cycles, the core standard-library
allowlist, and banned unsafe/cgo/plugin imports. External test packages do not
create artificial production cycles. Unclassified Go files fail closed.

`go list -m all` and `go list -deps -test ./...` in verification reject extra
modules and nonstandard packages outside this module. Nested go.mod/go.work and
vendor trees are rejected so they cannot silently evade the root checks.

The union of platform-specific imports is checked conservatively. Production
cycles across mutually exclusive files can therefore require a structure change.
The Go compiler still checks the selected platform. The gate does not prove
purity of arbitrary function bodies, runtime safety, or dependency provenance.
`go:linkname` and `go:generate` directives are prohibited by the source gate;
ordinary comments and imports are not a sandbox.

A boundary change must update this document, the checker, and meaningful fixture
tests together. Tests must demonstrate valid wiring and forbidden reverse edges
using existing target packages. Core tests use supplied evidence and ports;
adapter tests separately exercise actual I/O contracts. Integration tests belong
under tests when they need multiple concrete layers.
