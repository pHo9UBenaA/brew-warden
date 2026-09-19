# Implementation checkpoint: 2026-09-20

Work and verification began on 2026-09-19 and finished after midnight JST.

This is a historical verification record, not the current policy contract.
Product readiness was not achieved at this checkpoint. No Homebrew product
execution, push, tag, publication, or host Homebrew modification was performed.

## Implemented increments

- `3b1027d`: typed artifact/evidence eligibility, complete reachable graphs,
  explicit time, normal and bounded emergency age evaluation. Eligibility is
  not execution authorization; bindings and prior-attempt state still require
  trusted producers and a durable execution journal.
- `b6269bf`: strict bounded configuration, optional user defaults, explicit
  configuration paths and minimum-age overrides; verification cannot be disabled.
- `2a8fa27`: immutable refusal records, exact-byte content IDs, private files,
  writer locking, sync/rename durability, strict revalidation and read-only history.
  This is refusal history, not completed installation history or exception storage.
- `1e000e4`: isolated upstream signature, checksum-cache and missing-root-bottle
  probes, in addition to earlier subject-coverage and dependency re-resolution tests.
- `4e796fd`: fixed-source repeat development builds for both macOS architectures.

## Verified boundaries

Using Go 1.27.1, offline verification, race tests, coverage, both active fuzz
targets, Staticcheck, and govulncheck completed. The combined check initially
stopped at advisory DNS access in the restricted sandbox; rerunning the advisory
step with network access succeeded. Source/tests and the checker, bwd, and
brewwarden binaries reported no known vulnerabilities. This is not proof of
software safety or Homebrew candidate coverage. Cross-package coverage was 78.5%;
separately built subprocess entrypoints are exercised but not instrumented.

Real filesystem tests cover repeated requests without overwriting, corrupt
records, recomputed digests around invalid schemas, unsafe permissions, symlinks,
concurrent writer refusal and incomplete pre-rename records. CLI entrypoint tests
use a tripwire brew executable and isolated user directories.

The upstream source revision and exact probe commands live in the probe script.
The successful extended run demonstrated official JWS verification with the
upstream public key, rejection of changed signed payloads and missing signatures,
checksum-cache rejection after same-size/mtime replacement, and root-only
`--force-bottle` rejection. It did not verify a real bottle attestation or install
an official package. No authenticated publication timestamp or complete candidate
advisory feed was established by these tests.

## Development build evidence

Source: `4e796fd35fbc6fa2e1081017c2fb30a345ddad11`. Two separate Git archive
extractions and forced recompilation passes matched byte for byte, using the
pinned compiler, disabled application cgo, trimpath and explicit development
version. The artifacts are not publisher-signed/notarized releases. The inspected
arm64 artifact has the Go linker's ad-hoc signature and no TeamIdentifier.
Both architectures link the OS-provided libSystem and libresolv libraries.
arm64 and amd64 version commands ran successfully on the test Mac.

| Target | Binary | SHA-256 |
| --- | --- | --- |
| darwin/arm64 | bwd | `46a5580fdc0775434e4ff04a56bb9fa0aaf54efa699d88e5ca1f7c5a666f8855` |
| darwin/arm64 | brewwarden | `cb4c40db3e66efc7284684cba4942103273d09164c0f7b3bbeb016dc6fa62444` |
| darwin/amd64 | bwd | `1ffd39f0bfd5bf7e41794d047c9fccd062842efc1b665dc38968318e1e51221a` |
| darwin/amd64 | brewwarden | `94c02c531cfe2c60dac41e2406ccfcf39fa9f3d78ed377b2420335af9fdb4926` |

## Remaining release gates

- Demonstrate a supported execution mechanism consuming exact authenticated
  metadata, artifact bytes and the entire dependency/change closure; resolve
  source fallback, affected dependents, concurrent changes, helpers and cache paths.
- Obtain an isolated normal-prefix macOS test environment. The available Docker
  daemon was not running; only temporary-prefix macOS sandbox probes were run.
  Environment availability does not itself solve execution binding.
- Establish publisher/distribution publication evidence for each selected
  artifact, including same-version rebuilds; define actual source freshness.
- Establish complete, revision-aware candidate advisory mapping, withdrawals,
  patch handling, unavailable/stale sources and positive evidence of fixes.
- Wire live evidence, planning and revalidation; add durable plans, observations,
  exception consumption and attempt reconciliation. Refusal history cannot stand
  in for these protocols.
- Test real execution failures, full disks and abrupt process interruption;
  establish supported version/platform scope and release identity/module path,
  license, publisher signatures and provenance before distributing a release.

Normal and emergency execution remain unavailable. The independent code above
must not be used to label the product production-ready before these gates pass.
