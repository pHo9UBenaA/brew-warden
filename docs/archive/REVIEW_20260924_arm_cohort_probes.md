# Native arm64 release-cohort probes

Date: 2026-09-24. This is pre-commit adapter evidence, not a local
product-ready result for the final distribution. No host Homebrew installation
or host GitHub credentials were changed, copied or used.

## Public Homebrew contracts

Disposable Tahoe Apple Silicon Tart guests received official Homebrew release
source from reviewed upstream tags into a guest-only `/opt/homebrew` prefix.
Read-only public commands for 6.0.19 and 7.0.6 fetched signed
`packages.arm64_tahoe.jws.json`, returned current `jq` bottle metadata and
`brew vulns --json` with an explicit empty `skipped_formulae`. Homebrew
6.0.19's first probe had to download its own arm64 portable Ruby 4.0.6 *inside
the disposable guest* before private-inspection testing; a missing required
Ruby under the product's immutable runtime sandbox remains a hold rather than
permission to bootstrap it. A Git-based official checkout is needed for the
reviewed release identity; tar-only old checkouts report `Homebrew >=4.3.0`
instead of a usable release version. The legacy exact 7.0.4 runtime fingerprint
remains accepted for the original tar-only VM fixture.

A compiled worktree adapter test (`TestLiveReviewedHomebrewPublicEvidenceContract`)
exercised the actual Homebrew commands in a private inspection prefix at
6.0.19, 6.0.22 and 7.0.6. Each produced an authenticated metadata cache,
resolved the complete jq/oniguruma closure, fetched bottles with digests
matching authenticated metadata, checked OCI bottle dependency annotations and
received a scanner report without skipped required subjects. The exact
candidate closure and cache are tied to the same private workspace. These
probes did not install or upgrade anything on the host.

## Signed verifier output and execution

The real arm64 gh 2.66.0 binary verified publicly available signed bundles for
the exact `jq 1.8.2` bottle (SHA-256
`ca67c64d0aaf1e5472790ec2cc081ff7972316f27095d8a8aab81b3321247036`)
in a disposable guest. Both verified results identified the Homebrew core
publisher/main-branch workflow, matched the exact arm64 Tahoe subject digest
and supplied a verified `Tlog` URI `https://rekor.sigstore.dev`. The bundles
were retrieved through the *public* GitHub attestations API without a user's
token, then verified by gh using `--bundle`; the normal installed-gh remote
acquisition path was **not** exercised by that probe. gh 2.66.0 prints a
lower-case device-code prompt; the local VM runner now recognizes it. Neither
human-device code for this probe was approved; the guest credential workspace
was removed and that VM was stopped.

To separate Homebrew execution behavior from online gh authentication, two
other disposable guests used a clearly identified **test-only** gh wrapper.
For every selected bottle, the wrapper retrieved its public signed bundle and
delegated cryptographic verification to a real installed arm64 gh 2.101.0 with
`--bundle`; BrewWarden still hashed the selected bottle, required matching
verified signer/subject/timestamps, evaluated the full closure, froze inputs,
revalidated and executed its normal sandboxed public Homebrew commands. This is
real installer/plan-binding evidence, **not** the shipped online-gh command:

- Homebrew 6.0.19: fresh `jq 1.8.2` and `oniguruma 6.9.10` installation,
  matching opt/linked-keg records, and a verified unchanged rerun.
- Homebrew 7.0.6: the same fresh jq dependency installation; separately, a
  provisioned guest-only older `xz 5.8.3` was bound and actually upgraded to
  `xz 5.8.4` through BrewWarden with a verified observed result.

Homebrew 6.0.22 and 7.0.6 also passed packaged-style `doctor` checks using
worktree binaries and installed reviewed gh versions. The prior 7.0.4 full
17-case suite passed on a different committed revision (`ca2c480`). All probe
VMs were stopped; no probe replaces the final clean commit's authenticated
product-ready gate or public publisher signing.
