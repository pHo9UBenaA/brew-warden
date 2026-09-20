# Candidate advisory observations

Capability: `osv.git-candidate.v2`. The collector supplies vulnerability evidence
for authenticated, explicitly unmodified revision-zero sources distributed as
canonical GitHub release assets. The authenticated source URL supplies the
repository and tag, independently of the formula name or version convention.
Composition connects it to the complete candidate collector and execution workflow. Unknown mappings, patches, revisions
and incomplete responses hold; they never fall back to a clean result.

## Delegation and supplement

Homebrew 7.0.4, source `edb70f031e4170c780799633a1226ff73e1077f4`, has
`Library/Homebrew/vulns/osv.rb` query pagination and a scanner which can choose an
installed SBOM. The required subject here is the authenticated candidate source,
not whichever version happens to be installed. The native Homebrew advisory feed
also has limited reviewed coverage; absence is not evidence of a supported clean
lookup. This adapter supplements candidate identity, bounded transport, complete
pagination, exact-tag applicability and retained raw observations. It delegates
advisory discovery to [OSV's public query API](https://google.github.io/osv.dev/post-v1-query/)
and uses the [OSV schema](https://ossf.github.io/osv-schema/) without external modules.

Exact publisher source URLs and source/recipe checksums are prerequisites from
the authenticated Homebrew recipe. A caller must positively establish unmodified
source; a missing patch response is not sufficient. Queries name the exact GIT
repository and upstream tag. OSV may use fuzzy version matching, so every returned
non-withdrawn record must explicitly enumerate the exact affected tag under the
matching repository. An unresolved match holds. No Git range ordering, patch
backport or commit reachability is inferred. Real conversion records can contain
both a last-known affected commit and a later fix; explicit affected tags still
establish positive findings. Ranges are structurally checked, not used to prove
absence of a vulnerability. Unsupported schema major versions hold.

A second, versionless query discovers the repository's advisory references.
After completing pagination, the collector checks at most eight records in
lexical ID order for a structurally valid, non-withdrawn Git range for the exact
repository with at least one explicit affected tag. No built-in formula, tag or
advisory-ID registry is required. Missing coverage, exhausted discovery bounds
or failed requests hold; an empty candidate response alone never passes. This
positive control detects absent project coverage. It does not prove database completeness or that every newly
published vulnerability has been ingested. A complete supported candidate lookup
with no findings means only `NoKnownApplicableFindings`, never that code is safe.
A known finding remains affected even if another record is unavailable; its raw
observation is explicitly incomplete. Incomplete absence never becomes clean.

## Boundaries and evidence

Requests use HTTPS with OS trust roots, no credentials, no redirects, no fallback
cache and no subprocess. The API and its aggregation are trusted for advisory
content; observations are not signed vulnerability declarations. Batch references
and full records must have identical IDs and modification timestamps. Withdrawals
cannot be future-dated or newer than the record modification. Unknown/corrupt
fields used for decisions, duplicate keys, ambiguous casing and invalid JSON fail.

Each response is bounded to 2 MiB, the retained observation to 16 MiB, each query
to 128 candidate references or 1,024 coverage references and eight pages. Pagination retains query-slot identity and rejects
repeated tokens. Requests have 15-second deadlines under a 60-second total budget.
The one-hour evidence validity window bounds reuse, not database ingestion delay.
The observation retains requests, responses, full records, candidate checksums
and completeness, and its exact bytes are hashed for durable storage by the caller.

Tests cover pagination, incomplete/ambiguous records, source mapping, patched
revisions, withdrawals, HTTP failures, limits and fuzzed parser inputs. Explicit
live checks use `BREWWARDEN_LIVE_OSV=1 go test -v ./internal/adapters/osv -run
'^TestLiveOSVCandidates$' -count=1`. On 2026-09-20, jq 1.8.2, oniguruma 6.9.10 and c-ares 1.34.8
returned complete supported lookups with no known applicable findings and
verified positive project controls. This result is time-dependent and does not
establish execution permission by itself. The c-ares provider fixture asserts the
source-inspection precondition; it does not establish native recipe eligibility
or installation acceptance.
