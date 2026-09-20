# GitHub release publication boundary

Capability: `github.release.source-publication.v2`. This adapter supplies only
upstream publication evidence. It does not verify Homebrew metadata or bottles,
query vulnerabilities, establish execution binding, or permit installation. The
distribution CLI consumes this claim as one part of complete-plan verification.

## Delegation and scope

The inspected Homebrew API has no demonstrated general publisher release date.
Its Git/bottle/attestation timestamps describe other events. This adapter fills
a narrow gap using the public GitHub REST release-by-tag endpoint and release
asset identity and digest fields, supplementing older assets by hashing the
actual publisher download. Native build timestamps are not used as publication. See GitHub's [release](https://docs.github.com/en/rest/releases/releases#get-a-release-by-tag-name)
and [asset](https://docs.github.com/en/rest/releases/assets#get-a-release-asset)
contracts. The request explicitly selects API version `2022-11-28`, which was
verified against the live service on 2026-09-20.

Candidate identity, source URL and source checksum must come from authenticated
Homebrew metadata and its authenticated recipe. The canonical source URL identifies the owner, repository, tag and asset; no
formula-name registry or assumed version-to-tag convention is used. Only HTTPS
`github.com/OWNER/REPO/releases/download/TAG/ASSET` paths with nonempty ASCII
letters, digits, dots, underscores, hyphens or plus signs in each component are
supported. Dot traversal, encoded components, credentials, ports, queries,
fragments, API redirects and repository moves fail. A source download may follow exactly one HTTPS redirect to the
GitHub release-asset CDN and its release-asset path class. Other hosts, ports,
credentials, protocols, paths and additional redirects are refused. No fuzzy repository search, HTML parsing, credentials, gh installation,
or process execution occurs. Transport and JSON parsing use the Go standard
library; no module or runtime command dependency is introduced.

The response must identify a published non-prerelease with the expected tag and
exactly one matching uploaded source asset. That asset's SHA-256 must equal the
authenticated recipe's source checksum. Its creation/update dates must precede
or equal `published_at`; a new or changed asset attached to an old release does
not inherit that old date. If the API omits a digest, the collector downloads the
exact mapped asset without executing or extracting it, compares its size and
SHA-256 with the authenticated recipe, and refetches release metadata. The release
ID, tag, publication time and complete decision-bearing asset identity must remain
unchanged. A conflicting nonempty digest is never repaired by downloading bytes.
Missing dates, states, booleans or identity fields never imply success. Duplicate keys, ambiguous casing, invalid types,
invalid UTF-8, trailing values, oversized responses and excessive nesting fail.
Unrelated new API fields remain compatible.

## Trust and evidence

The claim relies on the publisher identified by the authenticated recipe, GitHub's release/asset
metadata, HTTPS and the OS trust roots. It is not a cryptographic signature over
the publication date or proof against a compromised publisher/GitHub. The exact
source checksum links the dated source asset to the authenticated recipe; separate
Homebrew metadata and provenance claims must bind the selected bottle bytes.

The event is `UpstreamPublication`, never bottle creation, Homebrew adoption or
bottle rebuild publication. A verified same-version rebuild can reference the
same upstream source event, but this collector says nothing about the rebuild's
age or eligibility. It cannot rescue failed bottle integrity/provenance checks.

The collector returns the typed claim and exact observation bytes, hashing the
latter for durable storage. Digest-bearing responses retain the raw API document;
download verification returns an envelope with both raw API documents, exact source
URL, measured size and measured SHA-256. The public source archive itself is not
retained by this collector. Explicit observation time, a 15-second request timeout,
a 45-second total deadline, a 1 MiB API response bound, a 32 MiB source bound and
a one-hour observation validity window are enforced. Source reads stop at the
reported asset size plus one byte, so oversized bodies fail without unbounded reads. That window bounds reuse of mutable release metadata;
it is not a guarantee that the upstream release can never change. The caller must
persist the bytes and revalidate the entire plan before any execution. A saved
claim is not authorization. No fallback cache or silent retry is implemented.

## Verification

Unit/transport-contract tests cover exact requests, digest/URL mismatches,
unpublished/future/replaced assets, malformed and ambiguous JSON, redirects,
HTTP errors, cancellation and unsupported URL forms. A transport test covers a formula whose name, version,
repository, tag and asset follow different conventions. The public-response parser
also has an active fuzz target. Transport substitution in unit tests does not
claim live network coverage.

Run the explicit real-service integration separately:

```sh
BREWWARDEN_LIVE_GITHUB=1 go test -v ./internal/adapters/githubrelease -run '^TestLiveGitHubPublication$' -count=1
```

Observed: `jq 1.8.2` source digest
`71b8d6e8f5fe81f6c6d0d110e3892251f6ce76ed095abd315e26e6e1193af3af`
matched its public asset and `published_at` was `2026-06-20T14:11:27Z`.
The `oniguruma 6.9.10` asset has no publisher-reported digest. The extended
collector downloads 979,159 bytes, verifies source SHA-256
`2a5cfc5ae259e4e97f86b68dfffc152cdaffe94e2060b770cb827238d769fc05`,
and confirms unchanged asset ID 217074598, with publication
`2025-01-01T01:48:04Z`. The original timestamp-only parser still refuses missing
digests; only successful byte verification plus metadata revalidation supplies
the additional evidence. Mismatched bytes, short/long downloads, changed assets,
changed releases, source errors and failed rechecks are refusal cases. These observations do not establish clean advisory results or product
execution readiness for either candidate.
