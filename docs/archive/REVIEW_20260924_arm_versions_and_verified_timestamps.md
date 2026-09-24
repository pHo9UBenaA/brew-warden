# Arm-only support and the GitHub CLI timestamp boundary

Date: 2026-09-24. This is upstream source research, not authorization to add
Homebrew or gh versions to the running product. The user explicitly declined
x86_64/Rosetta product support and any modification of the host Homebrew.

## Why installed gh 2.62.0 cannot satisfy the current age contract

The public `gh attestation verify --format json` flag existed in gh 2.62.0;
rejecting it is **not** a claim that gh 2.62.0 skips cryptographic signature
verification. Its `go.mod` uses `sigstore-go v0.6.2`. The 2.62.0 verification
path exports `verify.VerificationResult` after `verifier.Verify`, and the GitHub
and public-good verifiers enforce timestamp and/or transparency-log checks.
However, `sigstore-go v0.6.2` serializes a **verified** Tlog timestamp as
`{"type":"Tlog","uri":"TODO",...}`: it does not identify the verified log in
that result. BrewWarden's `oldestVerifiedTimestamp` requires
`type == "Tlog"` and `uri == "https://rekor.sigstore.dev"` for the exact
bottle digest. A process's zero exit, the predicate's time, or an unverified
replacement URI cannot repair this missing attribution. Hence gh 2.62.0 is
incompatible with the *current* bottle-age contract, even if it verifies an
attestation successfully.

The first gh release tag in the inspected sequence whose `go.mod` upgrades
from `sigstore-go v0.6.2` to `v0.7.0` is **gh 2.66.0** (2.65.0 still uses
0.6.2). In 0.7.0 the verified Tlog timestamp takes its URI and time from the
verified result (`vts.URI`, `vts.Time`) instead of the literal `"TODO"`.
**gh 2.70.0** is the first inspected tag whose `attestation verify` help
explicitly describes `verifiedTimestamps` as a verified field; gh 2.69.0's
help does not. This identifies the earliest *source-level* fix for this one
output-field limitation, and the earliest explicit CLI documentation. It does
**not** establish that every release >=2.66.0 preserves all signer, subject,
trust-root, JSON, timestamp and rate-limit guarantees or works with today's
Homebrew bottles. Only **gh 2.101.0** has passed this product's subprocess and
authenticated native VM acceptance and is presently admitted by `PublicGH.Check`.
Do not replace that exact check with `>=2.66` or `>=2.70` without separate
version/cohort review and real signed-bottle acceptance.

Primary source locations:

- [gh v2.62.0 module pin](https://github.com/cli/cli/blob/v2.62.0/go.mod),
  [verification/export path](https://github.com/cli/cli/blob/v2.62.0/pkg/cmd/attestation/verify/verify.go),
  [Sigstore verifier](https://github.com/cli/cli/blob/v2.62.0/pkg/cmd/attestation/verification/sigstore.go);
  [sigstore-go v0.6.2 signed_entity.go](https://github.com/sigstore/sigstore-go/blob/v0.6.2/pkg/verify/signed_entity.go).
- [gh v2.65.0](https://github.com/cli/cli/blob/v2.65.0/go.mod) and
  [v2.66.0](https://github.com/cli/cli/blob/v2.66.0/go.mod) module pins;
  [sigstore-go v0.7.0 signed_entity.go](https://github.com/sigstore/sigstore-go/blob/v0.7.0/pkg/verify/signed_entity.go).
- [gh v2.69.0](https://github.com/cli/cli/blob/v2.69.0/pkg/cmd/attestation/verify/verify.go)
  and [v2.70.0](https://github.com/cli/cli/blob/v2.70.0/pkg/cmd/attestation/verify/verify.go)
  help; [gh v2.101.0](https://github.com/cli/cli/blob/v2.101.0/pkg/cmd/attestation/verify/verify.go)
  inspected, accepted implementation.

## Homebrew on native arm64 macOS

The **only accepted installed Homebrew runtime** is the reviewed arm64 Tahoe
`/opt/homebrew` tree for Homebrew **7.0.4** at revision
`edb70f031e4170c780799633a1226ff73e1077f4`, with the exact executable
and `Library/Homebrew` fingerprint in `internal/adapters/homebrew/runtime.go`.
Printing `Homebrew 7.0.4` alone is insufficient if those bytes changed. A full
authenticated 17-case native VM suite passed for the product at `ca2c480` on
that runtime, not for subsequent revisions.

Homebrew **7.0.6** at `570982948a8a194f0f42f43f4a5bce2d1c9f64cb`
was inspected **only** from the read-only x86_64/Rosetta host checkout. The
`vulns` command/scanner files were unchanged compared with 7.0.4, but installer,
sandbox, keg, formula loading and vulnerability matching/history files changed.
An unchanged public CLI flag or unchanged scanner file does not prove complete
execution binding, network exclusion, after-state or signed-input use. An
arm64 7.0.6 install/upgrade with fresh verified evidence has **not** passed a
disposable VM test and the runtime fingerprint deliberately rejects it.
Therefore neither 7.0.6 nor an open-ended `7.x` version range is supported on
arm64 today. There is no evidence here that 7.0.6 is intrinsically insecure;
it is an unaccepted implementation. Widening support requires reviewing its
arm64 binary/runtime tree and upstream changes, updating an explicit supported
cohort, and repeating the complete native bound-execution suite. No host
Homebrew installation or user credentials were modified or used in this review.
