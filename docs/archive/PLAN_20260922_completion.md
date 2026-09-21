# Complete a maintainable Homebrew security wrapper

Date: 2026-09-22. Status: completed for the agreed local distribution scope.
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
- [x] Complete the public-command execution probe and record exact gaps.
- [x] Resolve the independent-concurrency condition with the user.
- [x] Implement and remove obsolete mechanisms.
- [x] Complete representative coverage and final distribution acceptance.

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

## Accepted concurrency scope and next execution increment

The user approved a no-concurrent-independent-mutation usage condition. They
explicitly prefer avoiding growth to interfere with operations deliberately run
outside BrewWarden. This does not authorize overlapping the wrapper's verification
and installation phases: all required verification must finish first. Do not add
monitoring, interception or private locking solely for independent brew commands.

Proceed with public CLI execution under that condition. Prove fixed metadata and
verified cache binding, installed-dependency selection, source-fallback refusal,
multiple targets, upgrade and interruption. Recheck the wrapper's own inputs;
retain durable partial/unknown outcomes. Then replace the retained native session
and remove its Ruby control protocol, guards and obsolete packaging requirements.
Do not declare all current code indispensable or enable an unverified fallback.

An intermediate distribution from commit `4610330` built twice with identical
binaries and archives. Archive SHA-256:
`5af834a47be72e9a87d7821503a1ec31bbab52966d2cd92e67ed13ab2944c32e`.
The extracted artifact passed its checksum inventory, `bwd doctor` and actual
`bwd brew install fzf` in the disposable VM. It retains the native execution
implementation and is not final public-CLI migration acceptance.

### Public execution capability acceptance

`Collection.PreparePublic` is an integration-test capability, not a product mode.
`Engine.Prepare` still selects the retained native implementation. The candidate
uses normal public `brew info`, `install` and `upgrade` commands, fixed signed API
metadata and verified bottle cache, disabled installation-time network, updates,
cleanup, autoremove and opportunistic dependent maintenance. New dependencies
use the documented `--as-dependency` option. Verification finishes before the
topologically ordered install/upgrade commands begin.

The only additional operation lock is a private BrewWarden lock. It does not lock
out ordinary brew commands. A fixed shell startup gate records the child process
group before releasing it to execute; arguments are not interpolated into shell
source. Process-group cancellation, consumed attempts, input revalidation and
policy expiry checks are retained. Recovery after a parent/process interruption
still needs acceptance before product routing changes.

The disposable `brewwarden-cli-completion-02` VM passed:

- xz 5.8.4 fresh installation, unchanged rerun and replay rejection (112.61 s).
- jq 1.8.1 to 1.8.2 upgrade, unchanged rerun and replay rejection (132.46 s).
- Changed verified input refusal with unchanged installed payload (63.61 s).
- Missing cached bottle refusal with unchanged installed payload (52.98 s).
- Cancelled execution refusal with unchanged installed payload (54.56 s).
- jq/xz multi-target fresh installation with the oniguruma dependency, unrelated
  installed-package preservation, unchanged rerun and replay rejection (134.50 s).

The latter three tests also reject reusing the failed attempt. Logs are named
`public-session-*.log` in the VM's retained results directory. These pre-launch
failures do not substitute for testing interruption during an actual installer.
The first multi-target setup retained an inactive jq 1.8.1 keg after uninstalling
1.8.2. Planning correctly held the unlinked state. The fresh-install fixture was
corrected by explicitly removing that old test version in the disposable VM.
The public plan uses schema 2 so the native recovery adapter cannot accidentally
apply its different process/lock assumptions to that attempt. Recovery refuses
that schema before preparing or launching a native reconciliation process.

Public installed information and receipt flags are observations of user-owned
state, not proof of the digest of an existing installed payload. In particular,
the earlier ordinary source rebuild retained a `poured_from_bottle` flag even
though library bytes changed. Do not claim those flags bind an existing payload
to the newly verified bottle, or silently treat these capability tests as proof
of all old native payload guarantees. Resolve the final contract and remove the
superseded mechanism together; do not ship both as competing user modes.

Baseline verification, race, coverage, all configured fuzz targets and lint
passed. The combined check reached vulnerability scanning but encountered a
sandbox DNS failure; rerunning that check with network access passed source/test
and all three built-binary scans. No host Homebrew installation was modified.
The retained public-capability results archive has SHA-256
`875f9779fa4db59cb09e00c6d93079fc3d26e404273a29363becd1f04ce0deab`.


## Public-only implementation and smaller distribution

The public path replaced the native product route. Removed the seven product
Ruby scripts, the native control protocol, formula locks, installer monkey
patches and installed-payload reconstruction. Removed obsolete developer Ruby
probes; their results and original code remain in Git history. Candidate metadata
and fetch now use the signed API without downloading/evaluating build recipes.
A Go data check reconciles the selected OCI bottle digest/ref and complete
runtime dependency names. Existing user-owned installed state is observed rather
than represented as newly verified payload bytes. The wrapper does not modify
unrelated packages to manufacture provenance for externally installed files.

Recovery uses the private operation lock and recorded owned process groups,
then inventories selected racks and opt links without following links. Tests
cover partial receipts, changed payloads and live process-group refusal. The
public VM path passed an in-flight cancellation/recovery case (59.23 s), age
refusal (58.45 s), age exception (62.57 s), exception plus tampered input refusal
(59.19 s), and durable stopped-session reconciliation (55.37 s).

The distribution no longer includes Homebrew or portable Ruby. Its schema-2
inventory identifies the reviewed existing Homebrew files copied into a private
empty inspection prefix; the verifier remains bundled. New inventory SHA-256:
`d50f6a3f967fae22b9a84cf705d59b29e15b8ee2311c984f050ce85b2f8c1ce7`.
This removes the second shipped Homebrew/Ruby runtime and their redistribution
notice inputs. Reproducible final building and archive-based VM acceptance must
still complete before marking the plan finished.


### Completion follow-through: recovery and generic coverage

Homebrew 7.0.4 `system_command.rb` and `utils/popen.rb` start some children in
separate process groups. The final public execution schema is therefore 3: each
command starts in its own POSIX session and completion/recovery checks the entire
session, not just the original process group. A test kills a session leader while
a child in another group remains alive; that child must still prevent recovery.
The corrected live interruption fixture cancels at actual download-phase entry
rather than after a possibly completed pour. It passed with stopped-session
reconciliation in 54.02 s. An initial rerun correctly found no install to interrupt
because xz was already installed; the guest-only fixture was reset before retesting.

The representative survey exposed an unnecessary relocatable-only restriction:
openssl@3 and git use bottles whose cellar is `/opt/homebrew/Cellar`. Upstream
`BottleSpecification#compatible_locations?` explicitly accepts the identical
cellar. Since public execution uses that exact prefix, accept it alongside the
relocatable markers and reject other fixed prefixes. Architecture-independent
`all` bottles also participate in exact digest and closure verification.

Validated immutable attestation responses and bottle registration facts are cached
privately by exact identity. Signatures and current advisory status are still
verified on every collection. Anonymous GitHub API quota exhaustion was observed
and is a provider limitation, not negative vulnerability evidence. Cache corruption
or changed recipe/artifact identity cannot reuse the old registration fact.
Baseline, race, coverage, all fuzz targets and lint passed. The vulnerability step
encountered sandbox DNS failure; the source/test and three binary scans passed
when rerun with network access. Remaining work is final generic execution and
reproducible distribution acceptance, not another architecture decision.


### Representative compatibility corrections

The shipped public CLI from `2cd1ca7` passed actual parent-process death after a
durable start (152.87 s). Its installer continued independently; `status` reported
unfinished, and `reconcile` waited for the owned operation to stop and recorded
facts without inventing success or replaying installation. This case invokes the
extracted distribution executable, not only the Go adapter. The initial harness
path omitted the archive's `brewwarden/` directory and was corrected before testing.

The expanded survey found three generic adapter restrictions, not missing trust:

- Architecture-independent `all` bottles omit architecture in their OCI tab and
  use attestations of the byte-identical original platform filenames. The pinned
  Homebrew attestation source explicitly describes that merge. Require the exact
  arm64_tahoe name, version, revision, rebuild and authenticated digest, or a direct
  `all` attestation; do not accept an arbitrary filename prefix.
- Git's older bottle lists gettext 1.0, whereas current gettext adds json-c.
  Check every recorded bottle dependency against the fully verified selected API
  closure and require all direct dependencies. Each selected dependency's own
  bottle is checked separately. Do not reject a newly expanded transitive closure
  merely because an older parent bottle does not repeat the new grandchild.
- OpenSSL includes regular default files in `.bottle/etc`. Public Homebrew restores
  those verified files with its existing modified-config handling. Accept regular
  files/directories under `.bottle/etc` and `.bottle/var`; reject other restoration
  roots and shared-prefix links. No separate installer or Ruby call is introduced.

The Linux container found that the standard-library Getsid wrapper exists on
macOS but not Linux. A small platform-specific syscall adapter fixes that build;
Linux baseline, race and full-filesystem failure acceptance now pass. No dependency
was added. Repeated full checks and final distribution acceptance follow these
corrections before the completion gate is closed.


### Final review correction and diagnostics

Installer descendants must not rewrite the sandbox profile used by a later
command or the recorded process identities used for crash recovery. Protect the
profile and the bounded 128 possible process-record paths inside the existing
sandbox. The VM acceptance test attempts both writes using the actual profile
and requires permission denial before executing a normal verified installation.
This adds no new lock service, Ruby API or external-operation interception.

Retain failed public scanner output and identify canonical skipped formula names
in errors. Retain the underlying registration failure in the unavailable-evidence
record. The final broad survey establishes real scanner limitations rather than
adapter corruption: ca-certificates and its parent OpenSSL cannot currently obtain
complete public-scanner coverage; Git, curl and Python closures also encounter
skipped subjects. zstd's lz4 dependency was registered less than seven days ago.
None of those holds is a successful install or justification to weaken policy.

The final CLI acceptance initially stopped before tree installation when anonymous
GitHub core quota reached zero. Verified immutable caches from earlier runs in the
same VM were copied into its test HOME; product validation still checked exact
artifact/recipe identity, signatures and current advisories. This exercises normal
cache reuse, not a quota bypass or evidence waiver. Original cold-cache survey
results and both accepted and held outcomes are retained.


## Completion acceptance

All implementation and acceptance items above are complete. The accepted scope is
Homebrew 7.0.4 at the normal `/opt/homebrew` prefix on Apple Silicon macOS Tahoe,
using official core bottles. Unsupported or incomplete evidence remains a hold;
completion does not mean every Homebrew formula has scanner coverage. Signing,
notarization and publication were not requested and have not been performed.

The final archive was built from source
`2676e4ad5931e4dbf3605e50c070d8bb5de51738`, using Go 1.27.1 on darwin/amd64 with
explicit darwin/arm64 output. Two clean builds produced identical executables and
complete archives. The archive contains no bundled Homebrew or Ruby runtime;
product execution uses public commands from the verified existing Homebrew.
This final historical documentation commit does not change the accepted code or
require substituting an untested archive.

| Acceptance | Result |
| --- | --- |
| Full host checks | Baseline, architecture, race, coverage, all six fuzz targets, Staticcheck, source/test and three binary vulnerability scans passed. |
| Isolated Linux checks | Baseline, race and actual full-filesystem failure test passed. |
| Final archive CLI | Version, doctor, rejected cask input, jq upgrade, fresh tree/cmake installation, unchanged rerun, age hold, bounded age exception, history and status passed. |
| Existing dependency preservation | oniguruma file hashes were unchanged across jq upgrade; full Cellar hashes were unchanged across the repeated installation and age hold. |
| Parent process death | Final archive crash test passed in 119.52 s; the installer continued independently, recovery waited for owned processes and recorded reconciliation without inventing success or replaying installation. |
| Installer control protection | Real sandbox denied writes to its profile and process records; verified installation and replay rejection passed in 266.37 s. |
| Missing verified cache | Execution stopped without installed file changes and could not replay the consumed attempt; passed in 282.79 s. |
| Interruption and partial failure | Session-aware interruption/recovery passed in 54.02 s; actual link conflict preserved the existing file and recorded partial installation in 75.31 s. |
| Runtime integrity | A guest-only altered runtime manifest was rejected before installation. |

Distribution acceptance ran in an isolated macOS 26.6.2 arm64 VM with no host
mounts. Host Homebrew was not modified. The final executable reported the exact
source revision above, and installed jq, tree and cmake reported their expected
versions. The retained CLI procedure and logs reproduce the tested operations.

Cold-cache representative collection established complete evidence for cmake
4.4.3 and tree 2.3.2, followed by successful installation from the final archive.
zstd was held because lz4 was younger than seven days. ca-certificates, OpenSSL,
Git, curl and Python closures were held for skipped public-scanner subjects;
ca-certificates' bottle attestation itself was cryptographically verified. These
outcomes remain explicit limitations, with no package-specific policy bypass.
Final CLI acceptance reused previously validated immutable evidence caches after
anonymous GitHub quota exhaustion, as recorded above; current advisories and
artifact identity/signatures were still checked.

Local deliverables are `dist/brewwarden-darwin-arm64.tar.gz`,
`dist/SHA256SUMS` and `.cache/completion-evidence.tar.gz`. The evidence package
contains host and container checks, reproducible-build output, toolchain/source
records, VM procedures/logs, both representative surveys and crash acceptance.

| Object | SHA-256 |
| --- | --- |
| Distribution archive | `0d4eb66748e4ccc00ed4521239deec2792e42907af93ad92c872d71a05613685` |
| bwd executable | `42192460a3006edcd2f3f5179eb8cd78b9bf96b950d085ab2319b2220bf797ce` |
| Runtime inventory | `d50f6a3f967fae22b9a84cf705d59b29e15b8ee2311c984f050ce85b2f8c1ce7` |
| Evidence archive | `898be7dd7de4b8345d80274b0a5198a21085b533259f136062e51f0a6f74db0d` |
