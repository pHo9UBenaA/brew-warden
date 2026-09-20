# Homebrew integration probes

Status: frozen-input installations of a dependency-free official bottle and
a two-package dependency closure are demonstrated in empty isolated prefixes. General execution binding remains
unverified. No Homebrew version is supported for product execution.
The probe is development tooling; the CLI never calls it. Capability ownership
will move to the Homebrew adapter when that adapter has real behavior.

## Reproduce

```sh
sh scripts/probe-homebrew.sh /absolute/path/to/Homebrew-source
# Optional: verify a separately acquired official signed formula API document.
sh scripts/probe-homebrew.sh /absolute/path/to/Homebrew-source /path/to/formula.jws.json
```

Requires macOS with working `/usr/bin/sandbox-exec`, Git, tar, cp, shasum, and an
existing portable Ruby matching the version pinned in the script. It archives
the exact upstream Git commit, never runs the source checkout's `brew`, and
copies the existing Ruby runtime. It neither downloads nor bootstraps tools.
Ruby is a development runtime dependency, not a product dependency; the copied
runtime is not independently authenticated by this probe. No credentials are
passed. A revision pin identifies inspected source, not publisher authentication.

Every run retains its own `/private/tmp/brewwarden-probe.*` directory with source,
fixtures, stdout, stderr, statuses, and environment evidence. The sandbox denies
network access and writes outside that directory (except `/dev/null`). An empty
child environment receives only the explicit settings in the script. Home,
cache, logs, temporary files and user configuration are isolated. System-wide
`/etc/homebrew/brew.env` can still be read; conflicting settings can fail a probe
but cannot remove the outer sandbox's write/network restrictions. Failure to
apply the sandbox stops execution; there is no unsandboxed fallback. Review
retained stderr even when a command exits zero.

Synthetic tap fixtures are trusted only inside the isolated user configuration.
They have no bottles, use nonresolving source URLs and abort if installation is
attempted. They are not production recipes or a proposed installation mechanism.
The default probes use native previews and a negative `--force-bottle` test
against a source-only fixture. The optional official-bottle probe below performs
a real installation only in the newly created isolated prefix.
The nonstandard prefix cannot
establish normal-prefix bottle relocation, official core coverage, or platform
compatibility. The script does not impose a total runtime deadline; interrupt a
stalled run and retain its incomplete evidence as unavailable, never successful.

## Observed runtime results

Tested 2026-09-19 on macOS 26.6.2 (25G83), arm64 host, using Homebrew source
`edb70f031e4170c780799633a1226ff73e1077f4` (local Git tag `7.0.4`) and existing
portable Ruby 4.0.7. The source archive deliberately contains no `.git`; its
`brew --version` reports `Homebrew >=4.3.0 (shallow or no git repository)`.
Use the source revision record, not that generic string, to identify the test.

| Probe | Observation | Consequence |
| --- | --- | --- |
| `hb.verify.subject-coverage` | `verify --json --deps` on the source-only fixture exits 0, prints `[]`, and warns that the `tahoe` bottle is unavailable | Require an exact expected-subject inventory; process success is insufficient |
| `hb.plan.reresolution` | `install --dry-run --formula` first lists `probe-leaf`; after adding a dependency to the same root recipe, a second invocation lists both `probe-leaf` and `probe-extra`; both exit 0 | Previews resolve current metadata; they do not consume an immutable prior plan |
| `hb.artifact.checksum-cache` | Native `Downloadable::VerificationCache` verifies original and unchanged bytes, rejects replacement bytes with the same size and restored mtime, and rejects a missing checksum | Native cache revalidation is reusable; this does not prevent replacement after a check or prove installation consumption |
| `hb.execution.force-root-bottle` | Installing the source-only root with `--force-bottle --formula` exits 1 with `has no bottle` | Root rejection is demonstrated; dependency fallback remains separate |
| `hb.metadata.jws` | Upstream `verify_and_parse_jws` with the bundled Homebrew public key accepts the official document and rejects altered payload bytes and missing signatures | Raw metadata authentication is demonstrated; the resolver's use of those exact bytes remains unproven |

The script asserts these observations against real upstream commands. Neither is
a mock verifier or a simulated Homebrew implementation. Preview output repeats
some actions and is human-readable; it is deliberately not a product plan parser.
The successful run emitted nonstandard-prefix warnings and denied attempts by
clang to write an xcrun cache outside the sandbox. Preserve that limitation:
this is not a clean installation test. The original probes did not verify real bottle attestations or vulnerability
responses; the additional probes below address parts of those boundaries. The checksum test uses real local
fixture bytes and an explicit expected digest, not a downloaded official bottle.

The optional metadata probe calls the private upstream verifier only as a test;
it is not yet a supported product adapter. The tested official document came from
`https://formulae.brew.sh/api/formula.jws.json`, with SHA-256
`611a287a2b6a0c98218fe51cebf62e96810a0cd66002ee1396fcb8aa2e87cc71`.
It contained 8,608 formulae. The authenticated `wget` sample was version 1.25.0,
revision 0, bottle rebuild 2, including platform-specific digests. Its top-level
date/time-related fields were `outdated`, `deprecation_date`, and `disable_date`;
none establishes release publication time. This observation is scoped to this
sample, not proof that no usable publisher timestamp source exists.

## Frozen official-bottle installation

The optional macOS arm64/Tahoe probe is deliberately restricted to `hello 2.12.3`,
revision 0, rebuild 1, in an empty prefix. It is not a product execution helper.
Run with an explicitly supplied existing `gh` 2.62.0 binary:

```sh
sh scripts/probe-homebrew.sh /absolute/path/to/Homebrew-source /path/to/formula.jws.json /path/to/probe-inputs /absolute/path/to/gh
```

The input directory must contain these separately acquired public inputs. The
script authenticates recipes before evaluating them; it verifies the attestation
again, rather than accepting a caller-supplied verification result.

| File | Source / identity |
| --- | --- |
| `hello--2.12.3.arm64_tahoe.bottle.1.tar.gz` | Signed formula metadata's `arm64_tahoe` URL; SHA-256 `ae6237e3001bd354783f469d754cee875ee9828910461b85a5803f5990213dde` |
| `hello.rb` | `Homebrew/homebrew-core` at `6e45565b20d8278093c17d0e5464b814030be473`, `Formula/h/hello.rb`; SHA-256 `5b7509fca45647cc02a4cf9bb7937809f32e8ccb014f255d45dcede17e38bcbf` |
| `texinfo.rb` | Same commit, `Formula/t/texinfo.rb`; SHA-256 `261567b0fb6e021c1658be4a127ac3ea4699b609c172bcd20fa2acc5f29c96db` |
| `portable-ruby.tar.gz` | Public GHCR `homebrew/core/portable-ruby` blob `e0088dff5614b39387300136ec7a5f95bf1e07589547245c919524fc9e8b4197`, pinned by the inspected source's `vendor/portable-ruby-arm64-darwin`; Ruby 4.0.7 |
| `bundle.jsonl` | Public GitHub REST `repos/Homebrew/homebrew-core/attestations/sha256:<bottle-digest>` response; each `attestations[].bundle` serialized as a JSON line |
| `advisories.json` (optional) | `https://formulae.brew.sh/api/advisories.json`; bounded to 64 MiB for this development probe |

Public GHCR downloads accepted the anonymous `Authorization: Bearer QQ==` value;
no personal credential was used. Public GitHub attestation retrieval also required
no credentials. `gh` bundle verification nevertheless fetched Sigstore TUF roots;
without network it failed even with the previously populated cache. Network is
allowed only for verification and native `brew fetch`, with writes confined to
the new probe directory. Installation and its validation run offline.

The verifier constrains the repository, OIDC issuer
`https://token.actions.githubusercontent.com`, exact workflow
`https://github.com/Homebrew/homebrew-core/.github/workflows/publish-commit-bottles.yml@refs/heads/main`,
and GitHub-hosted runners. The result must include the exact bottle name/digest
and matching certificate fields. Changing the expected workflow makes verification
fail. This is an explicit supplemental verification because Homebrew's local and
cache paths can skip native attestation checks. The installed verifier is copied
and its hash recorded; it is a development dependency, not a supported product
verifier or independently authenticated tool distribution.

After authenticating both recipe snapshots, native `fetch --force-bottle` stages
the bottle and OCI manifest. The native resolver, authenticated API and embedded
recipe must agree on the exact version and empty dependency closure. The manifest's
runtime dependency list must also be empty. OCI annotations are not authenticated
by the bottle checksum; they must never silently expand the accepted closure.
The install sandbox denies writes to input artifacts, recipes, Homebrew's Library,
and existing cache files. Explicit attempts to append to the artifact and cached
bottle fail. Installation uses the frozen official recipe by `homebrew/core/hello`,
with no network or source artifacts available.

Observed on macOS 26.6.2 arm64 with the pinned source: install exits 0, the Cellar
contains only `hello/2.12.3`, the receipt records `poured_from_bottle: true` and no
runtime dependencies, and the installed embedded recipe matches the archive.
The installed `bin/hello` is byte-identical to the verified bottle's binary
(SHA-256 `2d8f0045734079d1ea64254cc27762f0698f6ee42bbf45c1047487d05015048d`)
and runs successfully. Input constraints precede installation; post-install
matching supplements them rather than substituting for prevention.

Two real failures informed this path: the host's copied Ruby was x86_64 and was
rejected for the selected arm64 bottle; loading the local bottle alone discarded
the current recipe's relocation stanza and failed with the long isolated prefix.
The named official-recipe path preserves that metadata. Linking additionally
reads the `texinfo` recipe to locate `install-info`; it does not install texinfo
in this empty prefix. Its authenticated snapshot is included explicitly.
Nonstandard-prefix and denied clang xcrun-cache warnings remain in stderr.

This does not establish arbitrary dependency graphs, affected dependents,
existing installed helpers, upgrades, normal-prefix behavior, concurrent external
Homebrew operations, or crash/cancellation reconciliation. Product execution stays
disabled. The earlier claim that no successful installation was demonstrated is
superseded only for the narrowly defined probe above.

## Dependency-bearing installation

A second bounded scenario installs `jq 1.8.2` (revision 0, rebuild 1) and its
complete runtime dependency `oniguruma 6.9.10` (revision/rebuild 0) together:

```sh
sh scripts/probe-homebrew.sh /absolute/path/to/Homebrew-source /path/to/formula.jws.json /path/to/jq-inputs /absolute/path/to/gh jq
```

Use the same Ruby archive as above. Supply `jq.rb`, `oniguruma.rb`, `autoconf.rb`,
`automake.rb`, `libtool.rb`, and `m4.rb` from the authenticated API's pinned source
commit and paths. Each snapshot's exact bytes are authenticated before native
recipe loading. The build-only recipes are needed for native dependency analysis;
no build-tool bottles or source archives are staged or installed. `fetch --deps`
expands build dependencies as well, so the probe fetches each explicitly accepted
runtime bottle individually and independently checks the full runtime graph.

| Artifact file | SHA-256 |
| --- | --- |
| `jq--1.8.2.arm64_tahoe.bottle.1.tar.gz` | `ca67c64d0aaf1e5472790ec2cc081ff7972316f27095d8a8aab81b3321247036` |
| `oniguruma--6.9.10.arm64_tahoe.bottle.tar.gz` | `eb6bda3b333f497b5d294388f39fd0902a5c79a52ae16858eff711d2d104cc4d` |

Acquire the corresponding GitHub attestation bundles as above, using filenames
`jq-bundle.jsonl` and `oniguruma-bundle.jsonl`. The jq signer is the publish-commit
workflow already described. The oniguruma signer is explicitly constrained to
`https://github.com/Homebrew/homebrew-core/.github/workflows/dispatch-build-bottle.yml@refs/heads/main`.
This additional identity was checked against the
[pinned official workflow](https://github.com/Homebrew/homebrew-core/blob/5710e3f85ca59a6dfe9619404049704992ea1142/.github/workflows/dispatch-build-bottle.yml):
its upload job attests the bottle archives before publishing them to GHCR. An
unexpected identity remains a failure; there is no repository-wide wildcard or
automatic trust of the subject supplied by an input certificate. These identities
are probe scope, not a product trust policy.

The API, current recipes, embedded recipes, and OCI metadata must agree on exact
runtime edges, candidate versions and revisions. Recipes requiring post-install
steps, optional/test dependencies or requirements are rejected by this bounded
probe. Build-only declarations are checked separately. After successful integrity
inspection, the probe deliberately changes the selected cached bottle (oniguruma
in this scenario) and requires `cache digest mismatch` before installation. It
restores the verified bytes, revalidates everything, freezes inputs, and then
executes the real native install. Attempts to change frozen input/cache bytes
are separately required to fail.

Observed on the same macOS arm64 environment: only `jq/1.8.2` and
`oniguruma/6.9.10` are installed, both receipts record bottle pours, and receipt
runtime edges and embedded recipe bytes match the verified closure. The jq
regular-expression smoke test runs successfully against its installed dependency.
These `:any` bottles require relocation: unlike the `hello` `:any_skip_relocation`
binary, their installed binaries are not claimed byte-identical to the archive.
The exact consumed bottle archives are authenticated, rehashed, and frozen before
native relocation. General relocation correctness, existing-prefix upgrades,
affected dependents and external concurrency remain separate acceptance work.

## Advisory and publication boundaries

`probe-homebrew-advisories.rb` exercises the pinned native `AdvisoryDatabase`,
`Vulnerability`, and `CachedFeed` implementations with controlled records and real
filesystem/cache refresh failure, not a replacement evaluator. It demonstrates:

- A Homebrew revision boundary `2.12.3_1` distinguishes an affected base version
  from a revision carrying a patch; an uncovered package returns `nil`.
- The lower-level status method still reports a withdrawn record as open, and
  maps an incomplete applicability range to open. A product adapter must retain
  withdrawal and unknown-applicability semantics instead of blindly forwarding it.
- Failed refresh returns an existing two-day-old cache despite `max_age: 1`;
  a future cache mtime also passes the freshness comparison. Returned data alone
  is not evidence that the requested freshness limit was met.

The public feed sampled on 2026-09-20 was 44,002,063 bytes, with 12,768 records
across 597 formula keys, schema version `1.7.3`. It had no `hello` entry, so this
sample does not establish a clean vulnerability result for the installed probe.
The sample's `meta` has no publication/freshness timestamp. An 8 MiB acquisition
limit correctly rejected this feed; the explicitly bounded development probe uses
64 MiB. No product input limit or source freshness policy was relaxed.

The verified attestation includes a transparency-log timestamp. It is not used
as publication time. An unauthenticated request to the GitHub REST organization
package-version endpoint for `core/hello` returned 401. No usable artifact-bound
publication timestamp has yet been demonstrated, and no credentials were borrowed
to bypass that response. These remain independent blockers to eligibility even
for this successfully installed development fixture.

## Inspected capabilities and gaps

All source links below pin the same inspected revision. These are source findings;
only the tests above are runtime results. None enables product execution. The design and
integration contract remain authoritative for policy.

| Capability | Native guarantee candidate and source | Remaining work before delegation |
| --- | --- | --- |
| `hb.artifact.checksum` | [Downloadable](https://github.com/Homebrew/brew/blob/edb70f031e4170c780799633a1226ff73e1077f4/Library/Homebrew/downloadable.rb) checks fetched bytes by default | Authenticate the expected digest; test changed cached bytes and bind the consumed bottle and dependency closure |
| `hb.metadata.jws` | [API](https://github.com/Homebrew/brew/blob/edb70f031e4170c780799633a1226ff73e1077f4/Library/Homebrew/api.rb) selects `homebrew-1`, requires PS512 with unencoded payload, and verifies against its bundled RSA key; signed payload caches also verify signatures | Demonstrate which metadata path the actual resolver consumes, invalid signatures, cache replacement, API overrides, and concurrent changes; unsigned info JSON is not equivalent evidence |
| `hb.bottle.attestation` | [Verifier](https://github.com/Homebrew/brew/blob/edb70f031e4170c780799633a1226ff73e1077f4/Library/Homebrew/attestation.rb) delegates digest/signature verification to `gh attestation verify`, constrains repository and matches subjects | Core verification does not constrain one workflow; older bottles can use a separate backfill repository with a cutoff; evaluate identities explicitly, including `all` tags |
| `hb.helpers.bootstrap` | The same verifier calls `Utils::Executable.ensure!("gh", latest: true)` with attestation disabled during bootstrap and requires GitHub credentials | Establish helper version, dependency closure, credentials and changes before allowing this path; do not invoke it as a supposedly harmless check |
| `hb.execution.attestation` | [Installer](https://github.com/Homebrew/brew/blob/edb70f031e4170c780799633a1226ff73e1077f4/Library/Homebrew/formula_installer.rb) schedules attestation checks for selected fresh downloads | Cached downloads, local bottle paths, and `gh` itself have skip paths. [Utility](https://github.com/Homebrew/brew/blob/edb70f031e4170c780799633a1226ff73e1077f4/Library/Homebrew/utils/attestation.rb) warns on unsupported taps. Enabling verification is not per-subject evidence |
| `hb.age.publication` | Formula identity and bottle digest/rebuild are candidates; attestation timestamps provide a different event | No authenticated publisher/distribution publication field bound to each selected artifact has been established. Source inspection of formula/API/bottle classes found no usable general release date. Do not interpret a build, Git, feed update or local timestamp as release publication |
| `hb.advisories.coverage` | [`vulns`](https://github.com/Homebrew/brew/blob/edb70f031e4170c780799633a1226ff73e1077f4/Library/Homebrew/cmd/vulns.rb) has `--deps`, `--json`, skipped reporting and patch handling | [Scanner](https://github.com/Homebrew/brew/blob/edb70f031e4170c780799633a1226ff73e1077f4/Library/Homebrew/vulns/scanner.rb) can select an installed version/SBOM instead of the requested upgrade candidate. JSON findings omit clean-subject evidence. Test exact candidate/revision mapping, completeness, withdrawals and freshness |
| `hb.advisories.revisions` | [Advisory database](https://github.com/Homebrew/brew/blob/edb70f031e4170c780799633a1226ff73e1077f4/Library/Homebrew/vulns/advisory_database.rb) evaluates Homebrew version ranges and patch/fixed states, returning nil when uncovered | Its [feed cache](https://github.com/Homebrew/brew/blob/edb70f031e4170c780799633a1226ff73e1077f4/Library/Homebrew/vulns/cached_feed.rb) allows stale fallback after refresh failure. This is a separate source path from assuming `vulns --json` covers every Homebrew revision. Establish freshness and coverage before use |
| `hb.execution.binding` | [Install](https://github.com/Homebrew/brew/blob/edb70f031e4170c780799633a1226ff73e1077f4/Library/Homebrew/install.rb) previews computed dependencies; installer rejects missing root bottles with `--force-bottle` | Dependencies construct installers with `force_bottle: false`; inspect affected dependents and source fallback. No supported mechanism consuming a complete verified immutable plan has been demonstrated |

## Next acceptance work

1. In a disposable VM with the normal Homebrew prefix, capture the complete
   official-bottle closure and investigate a native execution hook or immutable
   metadata path. A local bottle argument alone is insufficient.
2. Exercise cached byte substitution, same-version rebottles, new dependencies,
   affected dependents, metadata changes between phases, source fallback and
   attestation exceptions before any consumption. Record actual inputs before
   code execution, not just post-install versions.
3. Verify JWS failure/cache paths and attestation subject/identity coverage with
   controlled fixtures and preprovisioned helpers. Account for every helper and
   credential; do not silently bootstrap dependencies.
4. Select authenticated publication sources and revision-aware advisory sources;
   test missing, future/conflicting dates, incomplete responses and stale feeds.

The local Docker daemon was unavailable during this investigation. No VM or
normal-prefix installation was run. These outstanding tests keep execution
unavailable; they do not prevent independent policy/configuration work.

## Standard-prefix macOS VM acceptance

The optional sixth argument `--vm-prefix` selects `/opt/homebrew` only when
`sysctl hw.model` identifies an Apple virtual machine and that prefix does not
exist (including dangling symlinks). Physical-host invocations fail before prefix
creation. The default remains a fresh temporary prefix. This guard prevents an
accidental physical-host invocation; it is not remote attestation of a guest.
See [the disposable VM procedure](macos-vm.md) for the tested environment.

The `jq` scenario passes in an empty standard prefix on macOS 26.6.2 (25G83),
arm64, VirtualMac2,1. Authentication, wrong-signer refusal, changed-cache refusal,
frozen-input write refusal, offline installation, exact installed closure/receipts,
and the dependency-loading smoke test all pass. The installed closure contains
only `jq 1.8.2` and `oniguruma 6.9.10`. The sandbox denies an incidental xcrun
cache write outside the test directories; native installation still exits zero.

This proves the bounded empty-prefix scenario at the native platform prefix.
The bounded upgrade below additionally covers an unchanged existing dependency.
Affected dependents, external Homebrew concurrency and interrupted execution
remain unverified. No product execution is enabled.

## Upgrade with an unchanged existing dependency

After a successful standard-prefix `jq` VM probe, run this developer-only
continuation inside the same disposable guest:

```sh
sh scripts/probe-homebrew-upgrade.sh /private/tmp/brewwarden-probe.RUN /path/to/old-jq-inputs
```

The input directory must contain `jq--1.8.1.arm64_tahoe.bottle.tar.gz` and
`jq-old-bundle.jsonl`. The old bottle SHA-256 is
`90b0fe4ad51959380f16fe8d84c5be8ab525478c32f1f7034c72d99de2442c9b`.
It is available from the public `homebrew/core/jq` OCI index tag `1.8.1`;
request `application/vnd.oci.image.index.v1+json` explicitly. The old fixture's
provenance must verify against the same pinned dispatch-build-bottle identity
used for oniguruma. This establishes a real historical bottle for fixture setup,
not an approved downgrade, an old signed-API snapshot, or vulnerability clearance.

The script refuses physical hosts, unexpected paths, unsuccessful initial probes,
and repeated preparation attempts. It revalidates the initial installed closure,
removes only the fixture jq with native autoremove disabled, and installs the
verified old local bottle. No dependency-removal suppression is exposed as a
product option. The common command boundary owns all Homebrew environment options.

Before upgrade it verifies the current candidate's metadata, provenance, archive
bytes and graph again; checks that the installed versions are exactly jq 1.8.1
and oniguruma 6.9.10; and records every existing dependency file's SHA-256, mode
and symlink target. It then runs native `upgrade --formula --force-bottle` with
network denied. The final jq receipt, installed recipe and runtime dependencies
must match the verified 1.8.2 candidate. The old 1.8.1 keg remains because cleanup
is disabled. The dependency snapshot must remain identical, and a regex smoke
test must load the installed oniguruma successfully.

Observed on macOS 26.6.2 (25G83), arm64, in a fresh Tart clone: all checks pass.
Native output reports exactly one requested upgrade, jq 1.8.1 to 1.8.2. The first
attempt revealed two actual upstream behaviors: uninstall autoremoves unused
dependencies unless disabled, and fetching an already cached OCI manifest still
recreates its convenience symlink. The common probe now freezes every regular
cache input (including actual symlink targets), while permitting native recreation
of these output aliases. Exact artifact bytes remain immutable; attempts to alter
the cached bottle still fail. This change was retested from a fresh VM, including
the original cache-substitution and immutable-byte negative controls.

This is one real upgrade path with one unchanged dependency. It does not prove
closure discovery for affected dependents, same-version rebuild upgrades,
concurrent external mutations, or crash recovery. Product execution remains off.

## Retained native locks

The jq probe now runs verification and the native install/upgrade command in one
Ruby process. `probe-homebrew-bound.rb` acquires the upstream formula locks in
name order and registers them with `FormulaInstaller.locked`; native installers
therefore reuse the session lock lifetime. No lock is released between candidate
verification, installation and final state inspection. A separate real Homebrew
Ruby process fails to acquire the target lock with `OperationInProgressError`.

Verified in disposable VM acceptance-03 on 2026-09-20: both empty standard-prefix
installation and jq 1.8.1 to 1.8.2 upgrade pass under retained locks, while the
oniguruma file/mode/link snapshot remains unchanged. Evidence is retained under
probe `brewwarden-probe.3mwiEaQB`. This is a pinned private-API contract for the
previously identified Homebrew source, not a claim about arbitrary versions,
unlocked package managers, affected dependents or the product CLI. Frozen inputs
and offline installation remain required; holding formula locks alone does not
freeze metadata or authenticate an artifact.
