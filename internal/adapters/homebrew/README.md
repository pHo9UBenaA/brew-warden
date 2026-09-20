# Native Homebrew boundary

Current capabilities: authenticated candidate collection and private runtime
materialization. Distribution builds wire its bound execution session through the application. The separate developer VM
probe has demonstrated same-process formula locks across real install/upgrade.

## Runtime and source identity

The supported native implementation is Homebrew 7.0.4 at commit
`edb70f031e4170c780799633a1226ff73e1077f4`, portable Ruby 4.0.7, macOS Tahoe on
Apple Silicon, and the pinned [attestation helper](../attestation/README.md).
`tools/runtime-pack` archives that exact Homebrew source, checks the Ruby and
verifier archive/file digests, and inventories all regular files and symlinks.
It refuses unsafe archive paths, hard links, symlink parents and duplicate files.
This is an explicit build tool, never an implicit product download or host brew
installation. Runtime licensing and distribution packaging remain release work.

The distribution selects the runtime-manifest SHA-256. Materialization validates
its exact schema, version, path inventory and file hashes, rejects extra inputs,
and copies only verified bytes into a private workspace. Symlinks must resolve
inside that workspace. There is no search for an unpinned helper on PATH. The
runtime has 5,096 inventoried files/links with the current packaging inputs;
its manifest SHA-256 is
`9424050677790a1c88c66ab769c5167d59a874cd6a02074665268084c97e758b`.

All native argument arrays, environment variables and sandbox construction belong
here. Native child architecture is explicitly arm64: a Rosetta parent must not
silently select Intel bottle metadata. Candidate collection uses a private prefix,
cache, HOME, config, logs and temporary directory, with host writes denied.
Even reading bottle manifests creates native download locks; these remain private.
No user credentials, proxy variables or Homebrew settings are inherited.

## Authenticated candidate inspection

`metadata.rb` delegates JWS signature verification to the pinned Homebrew API and
its bundled trust root before selecting any recipe. It resolves both runtime and
build recipe dependencies, rejects duplicate formula identities and bounds the
inventory. The Go bridge rejects malformed schemas, missing fields, unsupported
platforms, mismatched paths/URLs, incomplete graphs and cycles. Metadata is bounded
to 80 MiB and machine-readable bridge responses to 8 MiB.

Recipes are downloaded at the authenticated tap commit and must match the signed
recipe checksum before evaluation. Candidate bottle bytes and signatures are
separate requirements. `candidate.rb` then compares the authenticated current
recipe, verified embedded bottle recipe and OCI runtime-dependency metadata.
It rejects source/graph differences, requirements, options, post-install actions,
resources, optional/test dependencies, migrations and conflicts. The selected
native bottle digest, tag and rebuild must match the authenticated candidate.
Cache files must resolve inside the private workspace and match the actual digest.

An empty patch list alone is not an unmodified-source claim. For the mapped jq
and oniguruma sources, `source.rb` compares the current and embedded install
methods with explicitly reviewed build methods using Ruby's maintained Ripper
parser. Only token source coordinates are removed; instructions, arguments and
conditions remain part of the digest. Additional instance-method overrides or
unrecognized build logic make source assessment unknown. A future version can
reuse the reviewed build procedure, but a changed patch/build method requires
review. This is a narrow recognition rule, not a general analyzer of arbitrary
Ruby or protection against compromised Homebrew (outside the threat model).

## Tested boundaries

The explicit `BREWWARDEN_LIVE_RUNTIME` test uses a built runtime and cached signed
metadata/bottles in a temporary workspace. It has verified the complete jq and
oniguruma runtime closure plus four build recipe dependencies, with no network or
host writes. Separate VM probes establish native mutations; parser and mocked
port tests do not establish installation binding. Runtime substitution, extra
files, unsafe manifests, archive traversal, ambiguous JSON and changed dependency
closures have negative tests.

`bootstrap.rb` reuses Homebrew's initialization and reexecutes Ruby with a selected
standard prefix while retaining private source paths. A disposable VM has verified
that private source can upgrade the standard prefix under retained native locks.
The product must additionally bind persisted evidence, the complete action plan,
state and attempt lifecycle before exposing that path as a CLI command.

## Existing keg payload binding

`payload.rb` reconstructs a verified bottle in a private directory using native
checksum verification, extraction, placeholder relocation and Mach-O fixing. It
compares every actual file, directory mode and symlink against that reconstruction,
including additional or missing entries. Inspection uses the exact versioned
Cellar directory: Homebrew's `Formula#prefix` may instead return an `opt` symlink,
which filesystem traversal would not follow. Candidate activation must resolve
to the expected versioned directory.

Homebrew generates the receipt and changes SBOM creation metadata at installation.
The receipt requires separate candidate identity/dependency checks; it is never
used as proof of payload bytes. SBOM reconstruction delegates to native
`SBOM.update_pour_metadata` with the observed, bounded installation timestamp and
creator, plus the selected bottle's supplement. All remaining SBOM bytes must
match. An observed creator is not provenance evidence. Absolute symlinks requiring
build-prefix relocation are unsupported. Reconstruction redirects a pinned Keg
instance's payload path while retaining its native identity and relocation code;
this private API requires renewed acceptance on Homebrew upgrades.

The developer-only `scripts/probe-homebrew-payload.rb` requires a disposable
VirtualMac at the standard prefix. In the existing acceptance VM, both jq and
oniguruma match their verified, reconstructed payloads. For each package it
rejects altered executable bytes, added/missing files, changed permissions,
changed SBOM package identity and missing receipts, then verifies fixture
restoration. Native candidate tests also reject declarative `post_install_steps`
and service definitions before installation. Legacy `post_install` detection
alone does not cover these native execution paths.

## Live acquisition

`Collector` now performs the complete acquisition sequence in a fresh private
workspace: authenticate the signed API, fetch checksum-bound recipes at the exact
tap commit, use native bottle fetching, copy and hash actual cached bytes, verify
bundle subjects/signers, inspect native recipes/OCI closure, and collect publication
and OSV claims. Provenance failure stops before embedded recipe evaluation.
Every evidence item is checked against its candidate, claim, observation time,
one-hour maximum validity and exact saved observation bytes.

Metadata HTTP requests allow only explicit public origins, have bounded responses
and deadlines, reject redirects and do not inherit proxy or credential settings.
Attestation API envelopes reject duplicate/mis-cased duplicate fields; extracting a
bundle is not verification. Native downloads remain in the private cache. The
current product acquisition limit is 128 MiB per bottle. Required collection
failures remain unavailable, including vulnerability lookup failures; a publication
outage never becomes an invented date. Incomplete workspaces cannot create sessions.

`TestLiveCandidateCollection` is explicitly enabled with
`BREWWARDEN_LIVE_COLLECTION_RUNTIME`; it uses the concrete publication, OSV and
provenance adapters against public endpoints. It has fetched the current jq and
oniguruma closure and verified all five claims for each candidate without host
Homebrew mutation. It retains its private workspace for examination. Normal tests
remain offline and cover response bounds, forbidden destinations, ambiguous bundle
envelopes, changed cached bytes and substituted saved observations.

## Retained native execution session

The internal `Collection.Prepare` boundary now creates a session with upstream
`FormulaLock` locks covering installed racks and every candidate. It snapshots
candidate keg payloads, installed receipts and activation links. It refuses an
unverified affected installed dependent before execution, unsupported installed
identities, downgrades, pinned candidates and ambiguous/unlinked existing state.
Existing candidate payloads must match native reconstruction; same-version
replacement bottles therefore hold instead of being silently adopted.

The persisted plan includes exact evidence, targets, dependency graph, actions,
policy, runtime/environment, before-state, frozen inputs, expiry and attempt
identity. Explicit age waivers bind to that complete plan and attempt. Collection
inputs are inventoried when collection completes and compared again before
planning. Every raw observation is included in the frozen inventory. The parent
recomputes plan/input identities; the child revalidates input hashes, native
candidates, expiry and installed state immediately before execution.

Private atomic command/event files connect the Go workflow and the retained Ruby
process. The child cannot modify its frozen recipes, bottles, observations, plan
or seal, and installation has no network access. It expires while waiting for
commands. Cancellation terminates the native process group; lost state events or
missing durable result records remain unknown and require reconciliation.

Execution calls the pinned `Homebrew::Install.install_formula` implementation in
dependency order, retaining native dependency and installation checks. It does not
invoke the broad install/upgrade command's opportunistic dependent scans. Required
external dependent changes are held during planning. An installer guard rejects
unplanned identities and source builds. Recipe checks reject service/post-install
hooks, shared link replacement and unsupported dependency kinds. A bounded tar
inventory additionally rejects paths/links outside the versioned keg, special or
privileged files, duplicate entries and `.bottle` shared-prefix restoration.
Native extraction, relocation, linking, receipts and SBOM generation remain
Homebrew's implementation.

`TestLiveNativeExecution` requires an explicit runtime in a disposable VirtualMac.
It has demonstrated complete public evidence acquisition, lock exclusion against
a separate native process, fresh jq/oniguruma installation, jq 1.8.1 -> 1.8.2
upgrade with byte-for-byte unchanged oniguruma, and durable success records through
the real application workflow. A too-young policy holds without any attempt start
or installed-state change. An explicit per-artifact age exception still cannot
execute a substituted bottle. The distribution CLI uses this same workflow, with a pinned bundled runtime.

The application has also completed a real upgrade with per-artifact age waivers
and an otherwise impossible age threshold. A VM fixture with an installed external
dependent confirms that the affected-dependent hold occurs before any package
change. Source-build and unplanned-installer guards are exercised against the
real native installer in an isolated read-only-host test. Before/after state
snapshots are retained as content-addressed JSON; plans and observations are
published only after file synchronization and atomic rename.

Explicit recovery has passed the native active-lock rejection and stopped-session
snapshot tests. It does not mutate kegs or invent success. The normal distribution
CLI has passed doctor, plan display, installation verification and durable history
against the disposable standard-prefix VM.
