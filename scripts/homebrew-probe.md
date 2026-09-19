# Homebrew integration probes

Status: execution binding remains unverified. No supported product execution
versions, delegated runtime guarantees, or successful installation are claimed.
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
Install invocations are native previews and a negative `--force-bottle` test
against a source-only fixture. The latter must fail before installing anything.
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

The script asserts both observations against real upstream commands. Neither is
a mock verifier or a simulated Homebrew implementation. Preview output repeats
some actions and is human-readable; it is deliberately not a product plan parser.
The successful run emitted nonstandard-prefix warnings and denied attempts by
clang to write an xcrun cache outside the sandbox. Preserve that limitation:
this is not a clean installation test. No real bottle attestation or vulnerability
response was verified in the runtime probe. The checksum test uses real local
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
