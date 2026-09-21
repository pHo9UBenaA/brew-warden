# Complete a maintainable Homebrew security wrapper

Date: 2026-09-22. Status: investigation and implementation in progress.
Baseline: d79416f. Supersedes the remaining work in PLAN_20260922.md; the product
contract remains docs/design.md. This file records decisions and acceptance.

## Outcome and boundaries

Deliver the simple `bwd brew install/upgrade` interface using supported Homebrew
interfaces wherever possible. Do not call the current implementation complete
merely because three packages install successfully. Completion requires evidence
for maintenance, useful package coverage, safe execution, diagnostics, recovery,
and a reproducible distributable artifact. Do not publish or modify host Homebrew.
Initial platform remains official core bottles on Apple Silicon macOS Tahoe;
other platforms and package kinds are not silently accepted.

## Correction established from upstream documentation

Homebrew's [external command documentation](https://docs.brew.sh/External-Commands)
explicitly provides no compatibility guarantee for internals. The
[FormulaInstaller documentation](https://docs.brew.sh/rubydoc/FormulaInstaller.html)
marks the current installer/lock methods private and warns about changes without
notice. A documented external-command entrypoint does not make every method it
can reach a supported API. Pinned source and successful tests establish observed
behavior only; they do not transfer upstream compatibility or quality guarantees.

The user challenged the proposed retention of private APIs. That question is not
approval to retain them. The plan therefore does not assume that the existing
native session, Ruby analysis, payload reconstruction or runtime bundle is an
acceptable final architecture. Do not ask the user to accept them before the
public-command alternatives have been investigated concretely.

## Investigation and implementation order

1. Inventory the actual guarantees and upstream contracts. Separate public command
   behavior, documented configuration, private cache layouts, private Ruby calls,
   OS confinement, and supplemental policy. Inspect installed pinned source and
   current official documentation. Record what each test does and does not prove.
2. Probe ordinary public installation using authenticated fixed metadata, verified
   bottle cache, disabled updates/dependent maintenance and no installation-time
   network, in a fresh disposable standard-prefix VM. Use no private Ruby bridge
   in this candidate path. Test fresh install, upgrade and unchanged dependencies;
   then changed inputs, missing bottles, additional dependencies, concurrent brew
   operations and partial failure. Keep this path out of product routing until
   its artifact/closure binding is demonstrated.
3. Use the results to select the smallest maintainable execution design. Test
   whether existing installed-state and per-action requirements are original
   security requirements or implementation-specific restrictions. Any change to
   required guarantees needs a concrete explanation and user decision; neither
   successful output nor a post-install comparison is preventive verification.
4. Survey representative unrelated core formulae and their complete runtime
   closures. Classify failures by actual unsupported evidence versus adapter
   restrictions. Improve generic maintained-source mappings where justified;
   never add package-name exceptions or turn unavailable evidence into success.
   Report the supported scope concretely rather than promise all of Homebrew.
5. Implement the selected path behind the existing core/application interfaces.
   Remove replaced scripts and packaging paths after all callers disappear.
   Keep the user interface small; distinguish actionable unsupported evidence,
   provider failures, integrity refusal and interrupted installation.
6. Run final acceptance from the distribution, not just individual adapters.
   Verify fresh install, upgrade, multiple targets, unchanged dependencies,
   tampering, unsupported inputs, age-only exceptions, concurrent access,
   interruption/reconciliation and durable history in an isolated macOS VM.
   Run baseline, race, fuzz, lint and vulnerability checks appropriate to changes.
   Rebuild twice from committed source and compare archives. Record exact source,
   artifact hashes, VM environment, pass/failure evidence and remaining limits.
7. Synchronize README and current design/contracts with actual final behavior.
   Commit verified increments using Conventional Commits. Public release signing,
   notarization and publication require their own available identity and explicit
   publication authorization; local completion must not imply these happened.

## Completion gate

- The chosen mechanism is justified by documented contracts plus realistic tests,
  with no unsupported private-API quality claim.
- Every enabled mutation consumes the verified artifacts and complete permitted
  closure; unverified alternatives stop before the protected mutation.
- Required evidence has representative coverage and explicit unsupported cases.
- Typical use needs no manual plan, extra confirmation or assembled toolchain.
- The final artifact passes isolated native acceptance and reproducible building.
- No unresolved acceptance failure is relabeled as completion or hidden by docs.

## Progress and decisions

- [x] Re-read repository contracts and the previous migration record.
- [x] Confirm upstream's explicit lack of internal API compatibility guarantees.
- [ ] Complete the public-command execution probe and record exact gaps.
- [ ] Resolve any material guarantee/scope decision with the user.
- [ ] Implement and remove obsolete mechanisms.
- [ ] Complete representative coverage and final distribution acceptance.

## Findings from this investigation

### Public command execution

A fresh disposable `brewwarden-cli-completion-01` VirtualMac runs macOS 26.6.2
arm64. Its original Homebrew 6.0.22 prefix was preserved inside the guest; the
reviewed 7.0.4 source/runtime was placed at the normal prefix for this comparison.
No host prefix or shared host directory was used. The probe itself invokes only
public commands, without a Ruby bridge or private method calls.

`scripts/probe-homebrew-public-cli.sh` passed fresh jq/oniguruma installation and
an unchanged rerun using fixed signed API data, the verified bottle cache,
no installation-time network, and documented controls disabling auto-update,
cleanup, autoremove and opportunistic dependent maintenance. It rejected altered
bottle bytes and a missing dependency bottle. The missing-bottle path created an
empty rack but no installed keg/payload; the probe records that side effect rather
than claim a zero-write failure. A first harness attempt used `brew list` without
`--formula` and failed on an absent Caskroom after successful installation; the
probe now asks for formula inventory explicitly.

These results disprove a blanket claim that private installer calls are needed
for ordinary offline bottle installation. They do not yet prove continuous
installed-state binding under competing Homebrew operations, complete prohibition
of every source fallback, or final product acceptance. Keep the candidate path
out of product routing until those gaps are resolved.

### Practical evidence coverage

A diagnostic inventory of 21 representative names in the locally cached formula
snapshot found only six source URLs matching the existing canonical GitHub release
asset provider: openssl@3, jq, cmake, xz, c-ares and oniguruma. This is only a URL
shape survey, not six successful policy evaluations; dependencies and other checks
can narrow support further. The other names were git, wget, curl, python@3.14,
node, ripgrep, fd, fzf, bat, yq, ninja, zstd, sqlite, pkgconf and libuv.

The current advisory collector additionally requires a historical advisory for
the repository, beyond a correctly mapped successful candidate lookup. This
restriction must not be confused with a guarantee of database completeness.
Homebrew's public `vulns --json` exists, but its JSON omits per-subject clean-query
records, and its scanner can select an installed SBOM rather than the candidate.
It is therefore not yet a drop-in proof of the existing complete candidate claim.

## Accepted advisory direction (2026-09-22)

The user approved this combination after reviewing the actual upstream behavior:

- Delegate OSV scanning to the public `brew vulns` command.
- Supplement it with the official Homebrew Advisory Database, using its published
  JSON feed and Homebrew package/version/revision identities.
- Do not repeat the same OSV query in BrewWarden. If a supported future public
  command covers both sources, remove the separate feed lookup after equivalent
  acceptance tests pass.

This is a planned replacement, not a claim that the current product already uses
these sources. Keep current behavior documented as current until the replacement
is verified. The old positive historical-advisory requirement is not part of the
new contract: a completed, supported lookup with no applicable finding is a valid
negative observation. Source failures, skipped subjects and unresolved version
applicability remain unknown. Missing records alone are not transport failure,
proof of safety, or a reason to invent a historical-record requirement.

### Source findings and attribution

The inspected Homebrew 7.0.4 scanner queries OSV by repository/tag and accounts for
formula patch annotations; it does not consume the Homebrew Advisory Database.
Inspect the exact release selected for implementation again before relying on it.
The current upstream [scanner source](https://github.com/Homebrew/brew/blob/main/Library/Homebrew/vulns/scanner.rb)
corroborates the distinction. This is source inspection, not completed behavioral
acceptance of our replacement.

The official [database contribution guide](https://github.com/Homebrew/advisory-database/blob/main/CONTRIBUTING.md)
describes generated patch records, reviewed matches from external sources, and
manual contributions. The public feed is
https://formulae.brew.sh/api/advisories.json. Some records derive from OSV, so these
are complementary scopes, not independent corroboration. Its README and current
contribution guide describe different stages of development; do not assume the
older README's patch-only description establishes current coverage.
[Matching documentation](https://docs.brew.sh/Advisory-Matching) explicitly records
omitted ambiguous candidates. Do not promise exhaustive database coverage.

## Ordered implementation work packages

### 1. Prove public advisory command behavior before replacing the collector

Owner: Homebrew adapter; domain policy remains independent of command syntax.
Use a disposable environment and the exact candidate metadata selected for the
installation. Start by testing `brew vulns --json --deps` with explicit formula
arguments; finalize supported flags only after checking the selected release.
Do not apply severity or fix-availability filters that suppress known findings.

The decisive fixture is an installed old version and a different planned version:
prove which one the command actually scans. Compare a vulnerable-to-fixed upgrade
and a clean-to-vulnerable candidate. The installed SBOM must not substitute the
old subject for the candidate. Prefer an isolated candidate inspection context
using public commands; do not invent a private Ruby entrypoint to force selection.
If public interfaces cannot select and account for candidates, document the exact
reproducer and remaining options before changing the agreed architecture.

Also test unrelated source URL conventions, multiple roots, shared dependencies,
a clean response, an applicable advisory, patch resolution, skipped formulae,
malformed/truncated output, network failure and cancellation. Establish how all
requested subjects are accounted for even when JSON lists only findings and a
checked count. A zero exit status alone is insufficient. Compare the requested
closure to checked/skipped accounting; no per-package allowlists or guessed
clean subjects.

Deliverable: a versioned capability record with exact commands, machine-readable
fixtures, live isolated evidence and explicit gaps. This gate precedes product
routing or deletion of the existing collector.

### 2. Add the Homebrew advisory feed as the sole supplemental lookup

Owner: a focused external-data adapter behind the existing evidence boundary.
Use official formula identity and full Homebrew package version, including
revision; never compare these versions as plain strings or assume SemVer.
Evaluate whether a public Homebrew output already exposes equivalent candidate
status before adding a comparator. Document the smallest justified supplement.

Define and test bounded JSON/schema parsing, pagination if present, version-range
semantics, withdrawn records, source freshness, redirects, timeouts and cache
revalidation. Inspect the feed's actual authentication and update metadata; do
not call TLS-fetched data signed. Freshness must describe what can be established,
not invent a publication time from the HTTP fetch time.

Test introduced/fixed boundaries, revisions, multiple disjoint ranges, open-ended
ranges, aliases, patch-fixed records, unknown comparison rules and an absent
formula entry. Absence means no matching record in this feed; it must neither
turn a skipped OSV check into success nor force an unrelated historical witness.
Reuse a maintained comparator/library when needed, with dependency justification.

### 3. Combine evidence and retire duplicate OSV machinery

Preserve attribution to both inputs. For each candidate and required dependency:

| Evidence | Result |
| --- | --- |
| Known applicable, non-withdrawn finding | Deny |
| OSV upstream finding with demonstrated applicable Homebrew fix | Evaluate as fixed and retain both records |
| Contradictory evidence with unresolved applicability | Hold; never silently prefer the cleaner source |
| Required lookup failed, subject skipped, stale data or unknown version mapping | Hold |
| Both supported lookups completed with no applicable finding | No known applicable findings |

Deduplicate CVE/GHSA aliases for presentation without discarding provenance.
Absence from one source never cancels a finding in the other. Patch evidence must
match the planned Homebrew version/revision and advisory, not just its name.
Age exceptions cannot waive vulnerability findings or incomplete checks.

After the replacement passes adapter and application acceptance, remove the direct
OSV candidate-query path and historical coverage lookup, update wiring and related
tests, and retain only code needed by the selected providers. Update design,
capability index, adapter contracts and README together when behavior changes.

### 4. Resolve Homebrew-side age semantics separately

The user asked to use Homebrew release information; they did not approve switching
to an upstream author's release date or an attestation timestamp. Investigate a
public Homebrew distribution event bound to the selected artifact. Keep source
mtime, OCI index creation, bottle build, attestation log inclusion and publication
separate. None is silently substituted for another.

The investigation found that index timestamps may predate a platform bottle and
reproducible archive/SBOM timestamps may derive from source mtime. Verified
transparency-log timestamps date attestations; they do not prove distribution
publication. If no suitable public distribution timestamp can be established,
present the concrete alternatives and ask for a semantic decision. Do not stop
independent advisory and execution work while that decision is pending.

### 5. Complete public execution binding and remove obsolete mechanisms

Continue the existing public CLI probe with concurrent ordinary brew operations,
changed metadata/dependency plans, missing or replaced caches, source fallback,
interruption and partial installation. Verify prevention before mutation; a final
file comparison cannot replace that proof. Keep newly proposed mutation paths
unavailable until the full gate passes.

Fresh install, unchanged rerun and jq 1.8.1 to 1.8.2 upgrade with oniguruma 6.9.10
file checksums unchanged have passed in the disposable VM. The upgrade fixture
used developer mode only to install its old local bottle; the tested candidate
upgrade did not. These results do not prove all installed metadata, symlinks or
concurrent behavior. Collected evidence archive SHA-256:
`89847e2f487da029b8e27a81711cc745f2b1ddd78858674de8c6fc1ff96cc943`.

Replace native-session/private-API machinery only after equivalent required
binding is demonstrated. Remove obsolete runtime bundles, Ruby bridges and
packaging complexity when their callers disappear; do not retain them by default.

### 6. Product completion and commit boundaries

Use separate Conventional Commits for the capability probes, advisory adapter,
policy/wiring migration, execution migration, and final documentation/distribution
work as each becomes verified. Run affected tests and `./scripts/verify.sh` for
changes; complete `./scripts/check.sh all` and isolated distribution acceptance
before claiming product readiness. Report unavailable checks explicitly.

Final acceptance covers multiple unrelated formulae and full closures; new and
already-installed states; install, explicit upgrade and upgrade-all; clean,
affected, patched, skipped and unavailable advisory outcomes; invalid integrity
and provenance; age exceptions; concurrency; interruption and reconciliation.
Measure coverage failures by cause. A successful jq example is not completion.
Keep the user-facing prefix interface unchanged and explanations actionable.

Completion requires the final distributable to pass these gates and current docs
to describe its actual behavior. Host Homebrew remains unmodified; push,
publication, signing identity acquisition and notarization are not authorized by
this planning decision.

## Public advisory probe and adapter increment

The copied, manifest-verified Homebrew 7.0.4 runtime passed
`TestLivePublicVulnsCandidateSelection` on Apple Silicon macOS with live OSV
queries. No host Homebrew prefix was modified. This is an isolated metadata
fixture test, not a real installed-old-version VM acceptance.

- `brew vulns --json --deps homebrew/core/jq` returned exit zero while listing
  m4, autoconf, automake and libtool as skipped. They are build dependencies
  outside the selected bottle runtime closure. Therefore use explicit full
  candidate lists, without the scanner's independent `--deps` expansion.
- Explicit jq 1.8.2 and oniguruma 6.9.10 in an empty inspection prefix returned
  empty findings and skipped arrays. JSON contains neither a checked count nor
  a list of clean subjects; its interpretation relies on the inspected command,
  explicit arguments and isolated authenticated candidate context.
- A synthetic jq 1.6 keg with an upstream-source SBOM changed the public result
  to jq 1.6 with 20 findings and exit one. Removing only that fixture restored
  the candidate result. The candidate adapter rejects nonempty Cellar/opt state
  before launching a command, rather than deleting installed data.
- Denying network access produced exit one and no JSON, not a clean report.

`vulns.go` adds a not-yet-routed replacement capability. It reconciles public
`info` metadata with the selected identities/digests and runtime dependencies,
then invokes the public scanner with explicit candidates. Its sandbox prevents
writes outside the inspection workspace and changes to metadata, Homebrew
Library, Cellar and opt during inspection. No private Ruby entrypoint or direct
OSV HTTP query is used by this capability. Parsing rejects omitted/ambiguous
fields, skipped subjects, wrong versions, unexpected formulae, and inconsistent
exit status. Raw findings retain open/patched attribution and advisory aliases.

The official Homebrew feed fetched during this increment was 44,637,704 bytes,
with 600 formula keys and 13,017 records. Its SHA-256 was
`c1332dde0989ee7cbe5a953c181cd7396dc63324039b5b416f367c76d313d0e7`.
All observed ranges used `ECOSYSTEM`; jq, oniguruma and openssl@3 had no entries
in that snapshot. Those absences must not become a historical-coverage gate.
The feed is too large for the existing small-response limits. The supplemental
adapter must fetch it once per operation, use an explicit bounded parser and
retain one shared observation rather than download/copy it for every candidate.

Remaining before routing: supplemental feed version/range evaluation and patch
reconciliation; representative live subjects beyond jq/oniguruma; vulnerable
candidate with an installed clean version; candidate omission/transport schema
fault tests at the actual command boundary; and complete distribution acceptance.
The old collector remains wired until the replacement passes these gates.

A further simplification candidate was confirmed through public formula JSON:
`lrzsz` 0.12.20 revision 1 reports CVE-2018-10195 as patched; `node` 26.9.0
revision 0 reports eight bump-fixed records; jq has no `vulnerabilities` field.
The inspected API generator obtains these statuses from the Homebrew advisory
database using Homebrew's own version comparison. Before adding a comparator,
evaluate this supported HTTP output as the supplementary evidence provider.
Match its exact candidate version/revision and artifact metadata; reconcile absent
fields against a successfully fetched feed inventory. The generator also omits
this field when database acquisition fails, so absence alone cannot mean clean.
Public HTTP freshness and cross-snapshot consistency remain acceptance items.

## Accepted bottle-age policy and provider migration

The user confirmed that a bottle rebuilt yesterday is dated yesterday even when
its upstream version is older. Use Homebrew's recorded introduction of the exact
bottle digest. Do not retain author release dates as the default or silently
substitute an attestation-log timestamp.

Implemented the two-source advisory collector using public `brew vulns` and
Homebrew's public advisory feed/formula API. Public API status delegates Homebrew
version comparison and patch attribution; a completed feed distinguishes a
missing record from an omitted status for an indexed formula. The complete feed
is retained once as gzip with its decoded SHA-256, avoiding per-candidate copies.
Direct OSV queries and their historical-record coverage gate are removed.

Implemented digest-bound registration history anchored to the authenticated tap
commit and current recipe hash. Follow consecutive history with the same bottle
SHA-256; stop at the previous different bottle. Bounded history that does not
reach introduction yields a conservative recorded upper bound, marked separately
from an exact transition. Future/nonmonotonic clocks or mismatched history are
unavailable evidence. Recipe response bytes and commit metadata are retained.
Policy accepts the new bottle-registration event, not legacy upstream dates.

The old GitHub release provider, direct OSV adapter, SourceCandidate port and
Ripper source-procedure recognition were removed after their replacement probes
passed. Native execution restrictions and retained installer sessions are still
present pending the separate public execution binding gate.

Live two-source collection passed fzf 0.74.4, jq 1.8.2, oniguruma 6.9.10,
pcre2 10.48 and ripgrep 15.2.0. The jq arm64_tahoe digest was dated to its
2026-08-25T09:47:35Z registration, not the upstream version release or a later
unrelated platform bottle update. Offline tests distinguish unrelated recipe
edits from a rebuild, omitted API status from no feed records, and matching
patch fixes from unrelated/absent fixes.

## Native acceptance after provider migration

Commit `8bc31b8` replaces the providers and removes their obsolete implementations.
Baseline, race, coverage, fuzz, lint and source/binary vulnerability checks passed.
Full live candidate collection passed jq/oniguruma with all five claims.

A fresh `brewwarden-cli-completion-02` VM reproduced a genuine overly strict
dependency check: ripgrep 15.2.0's bottle records build dependency pcre2 10.47_1,
while current authenticated metadata selects pcre2 10.48. The old equality check
held the operation before mutation. Homebrew treats those historical versions as
minimum requirements; the current dependency is separately selected and verified.
The correction checks the complete transitive name closure and minimum versions,
using Homebrew's version ordering. It does not authorize unknown dependencies or
skip current artifact/installed-payload verification. A native regression test
rejects missing/extra/duplicate dependencies, newer requirements and missing
revision data.

After the correction, actual product service acceptance installed fzf 0.74.4,
pcre2 10.48 and ripgrep 15.2.0, preserved unrelated installed packages, and passed
a fully revalidated unchanged rerun (148.36 seconds). This exercises the retained
native execution path, not the proposed public-command replacement. The separate
public CLI fresh-install/corruption/missing-cache probe also passed in this VM.

## Public CLI installed-dependency drift reproducer

`scripts/probe-homebrew-dependency-drift.sh` reproduced a concrete limitation
without modifying Homebrew code or calling private methods. After establishing
a pcre2 10.48 bottle dependency, an ordinary external command ran
`brew reinstall --formula --build-from-source --debug-symbols pcre2`.
The subsequent `brew install --formula --force-bottle homebrew/core/ripgrep`
used fixed API/cache inputs with network access denied, returned zero, installed
the selected ripgrep bottle, and retained the changed pcre2 library.

The verified bottle's `libpcre2-8.0.dylib` SHA-256 was
`6b698ffc744550e149829d94045912448b8ac4b692cb267dce9fc2f3ed60659d`;
the external rebuild and the library consumed after installation both had
`b8a76923fb5d3722f51b0610afaa92a04ff134b342554a479f94ec1b93f0aec1`.
This demonstrates a preflight-to-launch gap, not an exploit or a compromised
Homebrew. The reproducer's zero exit means the limitation was reproduced; it is
not a passing execution-binding acceptance.

An earlier source build without debug symbols produced identical library bytes
and correctly failed the reproducer's fixture check. An oniguruma source-build
attempt failed in autoreconf after installing build dependencies, so it was not
used as evidence of dependency substitution. All operations were guest-only.

The user has been asked whether concurrent independent Homebrew mutations may
be excluded from the supported usage contract in favor of the public CLI design.
This is a pending decision, not permission to weaken the current guarantee.
Keep the new public mutation path unavailable until the decision and remaining
binding acceptance are resolved. A retained parent lock also cannot simply be
handed to the ordinary CLI, as the earlier real-command lock test demonstrated.
These findings do not establish that every possible public design is impossible.
The retained VM evidence archive SHA-256 is
`0c0f5446ca57684635d3e87293f9fb082a9014fe772919265a5669dbea0f9bfa`.
