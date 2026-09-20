# BrewWarden

Verified Homebrew installation with release-age policy, provenance, vulnerability
coverage checks and an immutable execution history. `bwd` and `brewwarden` are
the same CLI. Commands execute after checks pass without an additional prompt.

## Supported scope

The initial distribution supports official **jq and oniguruma** bottles on
**Apple Silicon macOS Tahoe (26.x)** with Homebrew at **`/opt/homebrew`**.
It includes pinned Homebrew 7.0.4, portable Ruby 4.0.7 and an attestation-only
verifier based on GitHub CLI 2.101.0. Go and a separate `gh` installation are not
needed by users. Unsupported formulae, casks, third-party taps, source builds,
services, post-install hooks and unverified affected dependents stop before
installation. Plain `brew` commands are not intercepted.

Every candidate, including dependencies already installed, needs authenticated
metadata, matching bottle bytes, expected publisher provenance, bound upstream
publication and current mapped advisory coverage. Missing evidence stops the
command. Native Homebrew locks and frozen inputs bind checks to execution;
installed payloads are compared against native reconstruction of verified bottles.
These checks reduce specific supply-chain risks; they cannot establish that
software has no undiscovered vulnerabilities.

## Install and use

Extract a verified distribution archive into a user-owned directory, keeping
`bwd`, `brewwarden` and `runtime/` together. Add that directory to `PATH`, or link
`bwd` and `brewwarden` from an existing directory on `PATH`. Do not copy a binary
away from its runtime. No privileged installer or automatic updates are used.
See [distribution verification and building](docs/distribution.md) for the trust
origin and reproducible local build. This repository does not imply that an
unsigned local build is a published or notarized release.

```sh
bwd doctor
bwd brew install jq
bwd brew upgrade jq
bwd history
bwd status
```

`brew upgrade` without targets checks the entire installed inventory. If any
required candidate or affected dependent is unsupported, it stops. Unknown
Homebrew arguments are rejected rather than passed through.

The default minimum upstream release age is seven days. This dates the verified
upstream source publication, not a bottle rebuild. An explicit normal-policy
override is available:

```sh
bwd --minimum-release-age 336h brew install jq
```

An urgent update can waive only age for each explicitly named artifact, for one
plan and one attempt. Dependencies need their own reasons if their age is waived:

```sh
bwd --age-exception 'jq=Urgent upstream fix' brew upgrade jq
```

The exact bottle digests and exceptions are shown before execution. Signature,
checksum, trust, advisory and dependency checks remain mandatory. Exceptions
expire with the plan (at most ten minutes) and cannot be replayed.

## Configuration and recovery

Optional configuration is `~/Library/Application Support/brewwarden/config.json`:

```json
{"schemaVersion":1,"age":{"minimumHours":168}}
```

Use `--config PATH` before `brew` for a different policy file. Configuration does
not redirect state. Private `attempts/`, `history/` and `collections/` live beside
the default configuration. Collections retain plans, exact observations, bottle
inputs, runtime copies and before/after snapshots for inspection. They can be
large; there is no automatic garbage collection. Preserve them while an attempt
is unresolved. History remains readable if policy configuration is invalid.

If execution is interrupted or its outcome cannot be durably established,
`status` reports an unresolved attempt and new installations stop:

```sh
bwd reconcile ATTEMPT_ID
bwd status
```

Reconciliation reacquires native candidate locks and records observed state. It
cannot run while the original session holds those locks. It neither claims the
lost process succeeded nor undoes installation. A fresh command creates a new
plan; partially installed or inconsistent packages may require manual Homebrew
repair before they can pass verification. Never delete the journal to retry.

## Development

Use the pinned Go version in `.go-version`, Git and a POSIX shell. Full checks
also require a C compiler for race detection.

```sh
./scripts/setup-hooks.sh
./scripts/verify.sh
./scripts/setup-tools.sh  # Explicit download of pinned development tools
./scripts/check.sh all   # Includes advisory database access
```

Plain `go run ./cmd/bwd` remains diagnostic-only: a trusted runtime digest is
embedded only by the distribution build. Native mutation acceptance tests require
a disposable macOS VM and refuse to run on physical hardware. They never update
the maintainer's Homebrew prefix. See [verification](docs/verification.md).

- [Product design](docs/design.md)
- [Architecture](docs/architecture.md)
- [Threat model](docs/threat-model.md)
- [Contributing](CONTRIBUTING.md)

BrewWarden is licensed under [MIT](LICENSE). Bundled dependencies retain their own
licenses; distribution archives include `THIRD_PARTY_NOTICES.txt` and `licenses/`.
