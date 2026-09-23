# Bottle provenance and age boundary

Capability: `homebrew.core.provenance.v2` and `homebrew.core.age.v2`.

The collector uses an already-installed, absolute GitHub CLI 2.101.0 path;
other versions hold until their output contracts have been reviewed and tested.
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
checks, failed commands, changed bytes and cancellation. This migration has not
undergone native Apple Silicon distribution acceptance. The current distribution
no longer bundles a verifier; the Homebrew inventory and recovery migration
remain pending.
