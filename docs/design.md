# Product design

Status: initial official-bottle implementation, updated 2026-09-20. Distribution
builds connect authenticated candidate collection, full-closure policy evaluation,
native execution binding and durable attempts. Official core bottle eligibility
is determined by verified capabilities on Apple Silicon macOS Tahoe with the
standard prefix. Unsupported required capabilities hold execution. Builds without a trusted bundled runtime stay
read-only. See [distribution](distribution.md) for build and publication boundaries.

## Purpose and scope

Provide one Homebrew security CLI and policy configuration for release-age
controls, artifact and signature verification, vulnerability checks,
dependency-aware install/upgrade plans, emergency updates, and evidence history.
Feature overlap with other OSS is acceptable. The product is not limited to an
audit viewer. Shared policy, consistent explanations, and reliable execution are
valuable even when individual checks already exist elsewhere.

Start with official core bottles on macOS. Design for casks, third-party taps,
upstream signatures, and Linux, enabling each only after its guarantees have
been tested. Unsupported paths must not fall through to an unchecked brew call.

Go is the product language and is used by the harness. Follow the
[dependency and implementation rules](dependencies.md). See
[architecture](architecture.md) for source boundaries and
[threat model](threat-model.md) for trust assumptions.

## Completion criteria

The goal is a general-purpose tool with a simple prefix interface. The original
jq/oniguruma acceptance establishes a binding baseline, not completion of that goal.
Official core bottle support must derive eligibility from verified capabilities
and authenticated metadata, without formula-name allowlists or per-package build
fingerprints. Required unsupported evidence still holds the entire operation.
Acceptance must exercise multiple unrelated source/release conventions, dependency
graphs, installed states and install/upgrade operations in disposable environments.
Keep detailed security records internally; ordinary use should require only the
requested Homebrew command and an actionable explanation if it cannot proceed.
Reduce duplicate execution paths and runtime dependencies where equivalent binding
can be demonstrated. Casks, other taps and source builds remain separate explicit
capabilities rather than unchecked fallbacks.

## User operations

The product is BrewWarden; its repository is `pHo9UBenaA/brew-warden`.
`bwd` and `brewwarden` expose the same interface:

```sh
bwd brew install jq
bwd brew upgrade jq
bwd brew upgrade
bwd --minimum-release-age 168h brew install jq
bwd --age-exception 'jq=Urgent upstream fix' brew upgrade jq
bwd doctor
bwd history
bwd status
bwd reconcile
```

Only age is waivable, separately for each named candidate. Reasons are bounded,
printable UTF-8; waived artifact hashes and reasons are displayed before execution.
Normal output shows formula names and versions. Detailed binding identifiers stay
in the journal and diagnostic history. `status` lists only unresolved attempts.

Follow the prefix interaction documented by
[Socket Firewall Free](https://docs.socket.dev/docs/socket-firewall-free): users
add a wrapper to their package-manager command. BrewWarden's security policy and
execution binding remain its own contracts, including cached artifacts.

Invoking a supported mutation authorizes that operation and its required
dependency changes. Internally resolve a plan, verify evidence, revalidate the
exact execution target, execute, and record the result. If checks pass, proceed
without a BrewWarden confirmation prompt, including in noninteractive use.
Holds, denials, and unavailable required evidence stop execution with a reason.
Homebrew's own prompts and privilege requirements remain separate. Resolve all
requested targets and their complete dependency closure before any installation
begins. If any target or required dependency is held or denied, stop the entire
operation; do not silently skip targets. This does not make execution atomic:
record partial results if a failure occurs after installation starts.

Plans are internal records, not a public `plan`/`apply` workflow. Do not initially
add BrewWarden `--yes`, `--dry-run`, or universal `--force` options. Diagnostic
commands (`status`, `history`, `doctor`) may expose evidence without authorizing
mutations. Emergency age exceptions require explicit, bounded user intent;
ordinary install/upgrade commands never grant them automatically. Their syntax
must be designed with the exception implementation, not as a generic bypass.

## Argument handling and routing

- Wrapper options such as `--help` and `--version` precede `brew`. Tokens after
  `brew` belong to Homebrew; do not consume a child's `--help` or reinterpret its
  `--dry-run` as a BrewWarden option. Support child flags only after their meaning
  and execution binding have been verified for that command and brew version.
- Initially accept the literal `brew` target, not arbitrary executables or shell
  strings. Resolve Homebrew to an explicit executable path and avoid recursion.
  Preserve argument boundaries; never concatenate a shell command.
- Classify commands, aliases, flags, and nested operations. Unsupported mutations,
  external commands, and ambiguous arguments fail explicitly. Only documented
  read-only operations may bypass the mutation flow. An unrestricted brew call
  after a preflight check is not verified-plan execution.
- Preserve child standard input/output, terminal behavior, and exit status when
  executed; send wrapper diagnostics to stderr. Propagate cancellation and record
  partial/unknown outcomes. Wrapper failures exit nonzero without launching the
  protected mutation; do not fall back to unwrapped execution.

Installing the binary on PATH is sufficient to invoke the prefix form; it does
not intercept plain `brew`. Any future shell alias or PATH shim is optional,
reversible, and subject to separate integration tests. Do not replace Homebrew's
executable or silently edit shell startup files. Direct Homebrew paths, a different
PATH, and other clients remain outside the wrapper's coverage.

## Artifact and signature evidence

- Hash actual downloaded bytes and compare them with authenticated metadata.
  Receiving unsigned JSON is not proof that Homebrew's signed metadata was
  verified. The adapter must establish and document this boundary.
- Bind bottle attestations to the expected repository/signing identity and
  artifact digest. TLS authentication is a different property.
- Use upstream OpenPGP only where the signature, signed object, and allowed key
  fingerprint are explicitly mapped. Never fetch a key from an untrusted
  artifact host and immediately treat it as trusted. A source signature does not
  authenticate the Homebrew-built bottle by itself.
- Model cask checksums, macOS signatures, and notarization separately. A locally
  calculated digest does not turn `no_check` into an upstream checksum match.
- Reuse Homebrew's demonstrated checks and supplement their gaps according to
  [Homebrew integration](homebrew-integration.md). Homebrew-managed helper use
  is eligible for evaluation; its dependencies, credentials, bootstrap exceptions,
  and side effects remain explicit. An enabled option alone is not verification
  evidence. Unsupported required verification still holds the operation.
- Preserve artifact changes as facts. A legitimate rebuild can change a digest
  without changing the upstream version; this is not automatically malware.

## Age policy

Keep upstream publication time, Homebrew adoption time, bottle creation time,
and local first-observation time separate. Record each source and its trust
conditions. A Git author or committer timestamp is not an independently verified
publication timestamp.

Fetch metadata dynamically and measure age from a supported publisher's release
or distribution publication time bound to the selected version/artifact. Do not
start a fresh waiting period when the user first installs or invokes BrewWarden.
For example, a release published ten days ago satisfies a seven-day threshold on
first use if the publication evidence for the selected candidate is sufficient.

Expose `--minimum-release-age <duration>` before `brew`; `168h` expresses seven
days. An explicit argument overrides the optional configuration value. Keep seven
days as the provisional default until representative metadata has been evaluated.
The minimum age setting does not waive any integrity, provenance, or vulnerability
check. Local observations are audit/cache data, not the default age clock.

The adapter must distinguish upstream publication from Homebrew adoption and
bottle publication/rebuilds, and document which event the threshold covers. Do
not inherit an old upstream date for an unverified replacement artifact. Select
and test the authoritative timestamp sources during the first integration probe;
a usable authenticated publication timestamp is not assumed to exist for every
package. Missing, conflicting, or future-dated evidence holds the age check rather
than fabricating a date or falling back to local first-observation waiting.

## Vulnerability evidence

Reuse maintained sources or Homebrew capabilities through adapters. First verify
candidate-version queries, machine-readable output, and Homebrew patch handling.
Do not deny a package based only on a fuzzy name match.

Represent `affected`, `not_affected`, and `unknown`, with separate source
availability, package identity, version applicability, and freshness. Zero search
results do not mean safe. Do not apply an upstream advisory blindly to a patched
Homebrew revision.

Use built-in refusal rules rather than requiring users to configure a security
policy matrix. Deny any known, applicable, non-withdrawn vulnerability affecting
a selected candidate or its required dependency closure, regardless of severity
or fix availability. Do not classify a withdrawn advisory or a demonstrably fixed
Homebrew revision as affected solely from an upstream version string.

A successful supported lookup with no applicable advisory means "no known
applicable findings", not proof of safety. An unavailable/stale required source,
unsupported package/version mapping, or unresolved applicability holds the
operation. Define concrete source freshness limits with the source adapter;
users should not need to select CVSS thresholds or resolve package mappings.
Normal eligibility and urgency are separate. Incomplete source responses must
not make a candidate look fixed; emergency age exceptions never waive these rules.

## Emergency updates

Emergency updates are a first-class workflow: explain the exact target and
dependencies, then execute only when the user explicitly requests the bounded
age exception. A recommendation alone never authorizes an exception or execution.
No background monitoring or automatic emergency updates are enabled by default.

Automatic recommendation requires:

1. The installed artifact's version and revision are known.
2. Evidence establishes that the advisory applies to that installed state.
3. Evidence establishes that the candidate fixes the problem. Fewer findings
   alone are insufficient.
4. Required integrity, provenance, and trust checks pass.
5. Every necessary dependency change is included in the verified plan.

Severity and exploitation information can prioritize candidates but cannot
prove that a version fixes a problem. Display urgent-but-unconfirmed information
without automatically granting an exception. A user's manual reason remains a
user assertion and does not become a verified advisory.

| Condition | Emergency behavior |
| --- | --- |
| Too young or age evidence unknown | May waive the specific age rule with a recorded reason |
| Hash mismatch or invalid signature | Deny; cannot waive |
| Required verification unavailable, missing, or unsupported | Hold; cannot waive |
| Unapproved tap or signing identity | Separate trust-policy change and replan required |
| Unknown or additional dependency | Include and verify it before execution |
| New finding violating vulnerability policy | Deny; emergency mode cannot override |
| Plan, artifact, policy, or environment changed | Replan |

An age exception binds to the plan ID, full package identity, digest, dependency
graph, policy digest, reason, expiry, and one execution attempt. Identify and
justify dependency age exceptions individually. Do not update unrelated packages
as part of an emergency. After failure, reconcile state and create a new plan
rather than replaying an exception or marking the package permanently trusted.

## Architecture and data

Use the [hexagonal structure](architecture.md): domain policy, core-owned
ports, application workflows, concrete adapters, CLI, and outer composition.
The policy engine accepts typed evidence, policy, and explicit time, and performs
no I/O. Do not interpret external prose as execution instructions.

| Concept | Required information |
| --- | --- |
| PackageID | Kind, tap, full name; distinguish formulae and casks |
| ArtifactID | Version, revision, rebuild, OS/CPU, digest |
| Evidence | Kind, subject, source, observation time, freshness, verifier, status, raw-data digest |
| VerificationStatus | unassessed, verified, failed, unavailable, unsupported |
| Decision | allow, hold, deny; all reasons and explicitly waivable rules |
| Plan | Tool/brew/verifier versions, targets, closure, artifacts, environment, policy, evidence, expiry |
| AgeException | Plan- and artifact-specific age waiver |
| Attempt | Before state, start record, exit status, after state, partial/unknown outcomes |

Zero and unrecognized values must never mean allow. Keep stable reason codes
alongside explanations. Deserialized `authorized: true` is not authorization.

The initial domain evaluator accepts explicit time, complete reachable dependency
graphs, exact artifact subjects and attributed evidence. An eligibility `Allow`
is not an execution permit. Callers must derive binding digests from the actual
plan, policy, graph and environment and obtain prior-attempt state from a durable
journal. Distribution builds connect this evaluator through the application
execution workflow and the native bound session.

State flow: draft -> evidence collected -> allow/hold/deny -> revalidated ->
executing -> succeeded/partial/failed/unknown. An emergency reevaluates only the
age condition; it cannot jump from deny directly to executing.

## Binding verification to execution

The supported official-bottle path freezes authenticated inputs, retains native
Homebrew formula locks through installation and state verification, and rejects
source fallback or unplanned installers. See the owning
[adapter contract](../internal/adapters/homebrew/README.md) for tested guarantees.
Disabling auto-update or holding only a wrapper lock would not establish this
binding. New execution paths must independently satisfy the same release gate.

For supported brew versions, demonstrate an execution path that consumes the
verified metadata, artifact digests, and complete dependency plan. Include cache
handling, source-build fallback rejection, concurrent changes, and attestation
exceptions. Passing a local bottle alone does not prove dependencies are fixed.
`--force-bottle` is not assumed to universally prohibit source builds.

The implementation uses pinned native APIs and authenticated original recipes.
Do not generate executable Ruby recipes as an unexamined shortcut. If binding
cannot be demonstrated for a new path, ship
inspection while explicitly rejecting mutation commands. This is a release gate,
not a retreat from the all-in-one product goal.

An upgrade can partially succeed. Record both exit status and actual state.
Post-install matching does not prevent prior code execution. Do not automatically
roll back: applications and their data may not support downgrades. Recovery is a
separate plan. Interrupted executing attempts become unknown until reconciled.

## Configuration and storage

Provide built-in defaults and CLI options; no configuration file is required
for ordinary use. An optional user-owned JSON configuration can persist settings.
Do not use executable configuration or implicit current-directory policy. Future project policy must be explicit and
must not weaken user policy through merging.

```json
{
  "schemaVersion": 1,
  "age": { "minimumHours": 168 },
  "trust": { "allowedTaps": ["homebrew/core"] },
  "verification": { "requireChecksum": true, "requireBottleAttestation": true },
  "emergency": { "mode": "suggest", "waivableRules": ["age"] }
}
```

Reject unknown configuration fields, ambiguous duplicate keys, invalid values,
and implicit type coercion. A policy change invalidates prior decisions.

The initial local configuration adapter supports this schema with defaults for
omitted sections. `schemaVersion` is required. It rejects nulls, duplicate or
mis-cased keys, invalid UTF-8, trailing values and documents above 64 KiB. Only
`homebrew/core`, mandatory checksum/provenance verification, and `suggest` mode
with age-only waivers are accepted. Configured trust expansion and disabling
required checks remain unsupported. Minimum hours are nonnegative integers;
the CLI duration must be an exact nonnegative number of seconds.

The default path is `brewwarden/config.json` within Go's OS user configuration
directory (`~/Library/Application Support` on macOS). `--config PATH` explicitly
selects another file; no current-directory configuration is discovered. A missing
default file uses defaults; an explicitly requested missing file fails. Final
symlinks, nonregular files, and group/other-writable files are rejected. Parent
directories follow the trusted-user filesystem boundary in the threat model.

Store separate immutable JSON records for observations, plans, and attempts in
private directories. Use a single writer, temporary files, synchronization,
same-filesystem rename, and required directory synchronization. Prevent path and
symlink escape; never use raw external names as storage paths. Indexes must be
rebuildable. Do not invent a database or JSON canonicalization protocol.

An ID can hash the exact stored bytes. Revalidate their schema and contents when
reading. A hash is not a signature or protection against the local administrator.
If an execution record cannot be persisted, do not begin a mutation. Read-only
reconciliation diagnostics should remain available.

The `history` command lists execution attempts and legacy pre-execution refusals.
In builds without a trusted runtime, supported install/upgrade request shapes
produce immutable refusal records under `history` beside the default user
configuration. Distribution builds record execution attempts separately; policy
holds before reservation do not create an attempt. Changing `--config` does not
redirect history. Other unsupported arguments are not logged. Records use random
event IDs, SHA-256 IDs over exact stored bytes, mode 0600 files in a 0700 directory,
a nonblocking single-writer lock, file synchronization, same-directory rename,
and directory synchronization. Reads revalidate schema and content digests.
History remains readable with invalid policy configuration. A history failure is
reported and never enables execution. Uncommitted `.pending-*` refusal files are
ignored; they cannot represent an executing attempt. This refusal store does not consume emergency exceptions or reconcile interrupted
installations; the separate attempt journal owns those transitions. The directory
inventory is bounded to 10,000 entries and rejects an oversized/corrupt inventory
rather than dropping old records. The separate attempt journal has real full-filesystem (isolated Linux tmpfs) and
abrupt-process termination tests. Its contract is documented below.

### Execution attempt journal

The application revalidates an execution session and its original binding, policy,
before-state and expiry. It reevaluates evidence using an explicit clock both
before durable reservation and immediately before launch. A failed start write
never permits execution. A cancellation or expiry after reservation consumes the
attempt as `not_started`, without inventing a child exit status.

The journal stores immutable, hash-linked start and outcome documents in a
separate private directory. A nonblocking OS lock covers validation plus append;
reads take a shared lock. Strict schemas, content digests, chain continuity and
one-time attempt/exception identity are checked on every access. Unknown or
unfinished attempts prevent new execution. Explicit `bwd reconcile` selects the
sole unresolved attempt; an optional attempt ID remains available when selection
is needed. It does not run automatically on
install or upgrade. Reconciliation appends observed state
and permits a fresh plan; it never marks a lost process successful or restores an
exception. Old records cannot be overwritten, and inventory exhaustion fails.

Exit status, actual state and policy eligibility remain separate. A zero exit
with an unexpected state is unknown. A nonzero exit with changed state is partial.
Outcome persistence failure leaves an unresolved attempt requiring reconciliation.
These contracts are wired to the native session in distribution builds. Native
reconciliation reacquires the original candidate locks, retains observed state,
and appends a reconciliation outcome without claiming a lost process succeeded.

## Implementation and acceptance

1. Probe Homebrew plan binding, authenticated publication metadata, existing
   verification capabilities and their gaps, delegated helpers, and advisory coverage.
2. Implement typed evidence, strict configuration, pure decisions, and history;
   model normal and emergency decisions together.
3. Integrate official bottle planning and checks; enable normal and emergency
   execution only for demonstrated paths.
4. Add cask, third-party tap, and upstream-signature support by explicit capability.
5. Improve presentation/performance and design optional automation separately.

The initial supported path has passed native candidate, installation, upgrade,
unchanged-dependency, exception and lock/recovery probes in a disposable macOS VM.
The [integration contract](homebrew-integration.md#preflight-investigation)
continues to apply whenever support expands. Future casks, taps, signatures and
platforms are separate capabilities, never unchecked fallbacks.

Acceptance must cover invalid signatures under emergency mode, expired/replayed
exceptions, changed policy/digests, incomplete advisory responses, same-version
rebottles, platform-specific artifacts, changed dependencies, cache substitution,
concurrent metadata changes, source fallback, malformed/oversized input, timeouts,
unsafe paths, full disks, and interrupted processes. Test real boundaries in an
isolated environment; do not upgrade the maintainer's normal Homebrew prefix.

Dependency inventory, supported brew/verifier versions, and reproducibility
results belong to release evidence. Publication/signing is a separate authorized release action; local reproducibility
and native acceptance are not publisher authentication.
