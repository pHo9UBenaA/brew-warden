# Bottle provenance and age boundary

Capability: `homebrew.core.provenance.v2` and `homebrew.core.age.v2`.

The collector uses an already-installed, absolute GitHub CLI path. The reviewed
version cohort is **2.66.0 through 2.101.0**, inclusive; versions outside the
bounds or with missing required verified output hold. gh 2.62.0 through 2.65.0
use sigstore-go 0.6.2, which serializes a verified Tlog time with
`uri: "TODO"`, not the verified Rekor URI required by our age contract. gh
2.66.0 first uses sigstore-go 0.7.0 with the verified Tlog URI; 2.70.0 first
documents the verified-timestamp JSON contract. The executable is hashed before
and after verification, and the observed version is attributed in both evidence
claims. Real native arm64 gh 2.66.0, 2.70.0, 2.74.0, 2.80.0, 2.97.0 and 2.101.0
binaries verified signed public bundles for the same exact jq bottle. Their
complete captured output is parsed by ordinary Go contract tests, including
negative changed-subject, future-time and unknown-log-URI cases. Offline-bundle
verification does not establish authenticated GitHub API acquisition in every
old client. The normal online path and authenticated final distribution gate
passed with gh 2.101.0 on commit `cf86592`; a later test-only revision does not
inherit that revision-bound readiness result.
It invokes `gh attestation verify BOTTLE --repo Homebrew/homebrew-core
--predicate-type https://slsa.dev/provenance/v1 --format json --limit 100`.
GitHub CLI owns Sigstore verification, public trust roots and the user's normal
credentials. It is not installed, upgraded, bundled or invoked through a shell.

The adapter hashes the actual selected bottle and gh executable before and
after verification. It bounds time, file sizes, stdout, stderr and JSON nesting.
Each verified result must identify the exact bottle name and digest (allowing
the byte-identical arm64_tahoe subject for an `all` bottle), the official core
repository, a supported main-branch Homebrew workflow and GitHub-hosted runner.
It checks every verified Rekor timestamp, rejects absent, future or malformed
times, empty output and a result set reaching the fetch limit. The earliest
valid timestamp across all matching results is the age of those exact bytes.
The raw verified response is retained by digest in the pending operation's
frozen observations for both provenance and age. A new digest cannot inherit a
prior bottle's age; later attestations of unchanged bytes do not reset it.

Upstream JSON help for GitHub CLI v2.101.0 distinguishes verified certificate
and timestamps from workflow-controlled predicate fields. `--repo` and the
pinned predicate type filter verification; BrewWarden validates output coverage
rather than treating a successful process alone as proof of all subjects.
Missing gh, authentication/rate-limit failure, unsupported signer, saturation,
invalid output, changed bytes and timeout all hold before mutation. Only a
verified but too-young timestamp can be waived by the age exception policy.

Subprocess tests check digest binding, multiple attestations and timestamps,
oldest-time selection, `all` bottles, result limits, invalid output, version
checks, failed commands, changed bytes and cancellation. The distribution
bundles neither a verifier nor Homebrew runtime. Representative authenticated
Apple Silicon distribution acceptance passed for the supported scope; see
[verification](../../../docs/verification.md) for exercised cases and limits.
