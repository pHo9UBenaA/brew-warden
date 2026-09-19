# GitHub release publication boundary

Capability: `github.release.source-publication.v1`. This adapter supplies only
upstream publication evidence. It does not verify Homebrew metadata or bottles,
query vulnerabilities, establish execution binding, or permit installation. The
product CLI does not call it yet; mutations remain unavailable.

## Delegation and scope

The inspected Homebrew API has no demonstrated general publisher release date.
Its Git/bottle/attestation timestamps describe other events. This adapter fills
a narrow gap using the public GitHub REST release-by-tag endpoint and release
asset digest fields, rather than interpreting those native timestamps as release
publication. See GitHub's [release](https://docs.github.com/en/rest/releases/releases#get-a-release-by-tag-name)
and [asset](https://docs.github.com/en/rest/releases/assets#get-a-release-asset)
contracts. The request explicitly selects API version `2022-11-28`, which was
verified against the live service on 2026-09-20.

Candidate identity, source URL and source checksum must come from authenticated
Homebrew metadata and its authenticated recipe. Currently only the exact
`jq` → `jqlang/jq` and `oniguruma` → `kkos/oniguruma` source-release mappings are
recognized. Unsupported names, URLs, version formats, redirects and repository
moves fail. No fuzzy repository search, HTML parsing, credentials, gh installation,
or process execution occurs. Transport and JSON parsing use the Go standard
library; no module or runtime command dependency is introduced.

The response must identify a published non-prerelease with the expected tag and
exactly one matching uploaded source asset. That asset's SHA-256 must equal the
authenticated recipe's source checksum. Its creation/update dates must precede
or equal `published_at`; a new or changed asset attached to an old release does
not inherit that old date. Missing digests, dates, states, booleans or identity
fields never imply success. Duplicate keys, ambiguous casing, invalid types,
invalid UTF-8, trailing values, oversized responses and excessive nesting fail.
Unrelated new API fields remain compatible.

## Trust and evidence

The claim relies on the explicitly mapped publisher, GitHub's release/asset
metadata, HTTPS and the OS trust roots. It is not a cryptographic signature over
the publication date or proof against a compromised publisher/GitHub. The exact
source checksum links the dated source asset to the authenticated recipe; separate
Homebrew metadata and provenance claims must bind the selected bottle bytes.

The event is `UpstreamPublication`, never bottle creation, Homebrew adoption or
bottle rebuild publication. A verified same-version rebuild can reference the
same upstream source event, but this collector says nothing about the rebuild's
age or eligibility. It cannot rescue failed bottle integrity/provenance checks.

The collector returns both the typed claim and exact raw response bytes, hashing
the latter for durable observation storage. It uses explicit observation time,
a 15-second request deadline, a 1 MiB decompressed response bound and a one-hour
observation validity window. That window bounds reuse of mutable release metadata;
it is not a guarantee that the upstream release can never change. The caller must
persist the bytes and revalidate the entire plan before any execution. A saved
claim is not authorization. No fallback cache or silent retry is implemented.

## Verification

Unit/transport-contract tests cover exact requests, digest/URL mismatches,
unpublished/future/replaced assets, malformed and ambiguous JSON, redirects,
HTTP errors, cancellation and unsupported mappings. The public-response parser
also has an active fuzz target. Transport substitution in unit tests does not
claim live network coverage.

Run the explicit real-service integration separately:

```sh
BREWWARDEN_LIVE_GITHUB=1 go test -v ./internal/adapters/githubrelease -run '^TestLiveGitHubPublication$' -count=1
```

Observed: `jq 1.8.2` source digest
`71b8d6e8f5fe81f6c6d0d110e3892251f6ce76ed095abd315e26e6e1193af3af`
matched its public asset and `published_at` was `2026-06-20T14:11:27Z`.
The `oniguruma 6.9.10` asset had no publisher digest; the collector refused it
specifically for that missing evidence. An outage is not accepted as this negative
case. These observations do not establish clean advisory results or product
execution readiness for either candidate.
