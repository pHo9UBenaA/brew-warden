# Product design

Status: simplified public-command implementation; authenticated native
install/upgrade and interruption acceptance remains outstanding. Distribution
builds connect authenticated candidate collection, full-closure policy evaluation,
public-command execution binding and minimal owned-process state. Official core bottle eligibility
is determined by verified capabilities on Apple Silicon macOS Tahoe with the
standard prefix. Unsupported required capabilities hold execution. Development builds without
the distribution build marker stay read-only. See [distribution](distribution.md) for build and publication boundaries.

## Purpose and scope

Provide one Homebrew security CLI and policy configuration for release-age
controls, artifact and signature verification, vulnerability checks,
dependency-aware install/upgrade plans, and emergency updates.
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
Keep bounded current-operation diagnostics; ordinary use should require only the
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
```

Only age is waivable, separately for each named candidate. Reasons are bounded,
printable UTF-8; waived artifact hashes and reasons are displayed before execution.
Normal output shows formula names and versions. Bound process identity is held
privately only while an owned Homebrew session may still be running.

Follow the prefix interaction documented by
[Socket Firewall Free](https://docs.socket.dev/docs/socket-firewall-free): users
add a wrapper to their package-manager command. BrewWarden's security policy and
execution binding remain its own contracts, including cached artifacts.

Invoking a supported mutation authorizes that operation and its required
dependency changes. Internally resolve a plan, verify evidence, revalidate the
exact execution target, execute, and report the observed result. If checks pass, proceed
without a BrewWarden confirmation prompt, including in noninteractive use.
Holds, denials, and unavailable required evidence stop execution with a reason.
Homebrew's own prompts and privilege requirements remain separate. Resolve all
requested targets and their complete dependency closure before any installation
begins. If any target or required dependency is held or denied, stop the entire
operation; do not silently skip targets. This does not make execution atomic:
report partial or unknown outcomes if a failure occurs after installation starts.

Plans are private, short-lived inputs for one command, not a public `plan`/`apply` workflow. Do not initially
add BrewWarden `--yes`, `--dry-run`, or universal `--force` options. The read-only `doctor` command checks environment support without authorizing
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
  executed; send wrapper diagnostics to stderr. Propagate cancellation and report
  partial/unknown outcomes. Wrapper failures exit nonzero without launching the
  protected mutation; do not fall back to unwrapped execution.

Installing the binary on PATH is sufficient to invoke the prefix form; it does
not intercept plain `brew`. Any future shell alias or PATH shim is optional,
reversible, and subject to separate integration tests. Do not replace Homebrew's
executable or silently edit shell startup files. Direct Homebrew paths, a different
PATH, and other clients remain outside the wrapper's coverage.

Keep verification and installation as separate phases: finish required checks for
all targets and dependencies before starting any installation. Do not run another
Homebrew mutation against the same prefix during a BrewWarden operation. This is
a usage condition, not a claim that BrewWarden blocks external commands. Prefer
public CLI execution without machinery added solely to prevent independent brew
operations. Checks binding the wrapper's own artifacts and dependency plan remain
required. Changes to execution mechanisms require replacement acceptance.

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

Measure the minimum age of the exact Homebrew bottle selected for installation,
not the upstream software version. A bottle rebuilt yesterday is one day old even
if the author's version was published a month ago. A different bottle digest
must establish its own age evidence, including same-version rebuilds.

Use the earliest verified GitHub CLI attestation timestamp for the exact bottle
digest, across every verified result and timestamp returned for that digest.
Missing, future, malformed or saturated attestation results hold the whole
operation, even with an age exception. A later attestation for identical bytes
does not restart the age; a new digest must establish its own age. Do not use
recipe history, source-release dates, or download times as age evidence.

Expose `--minimum-release-age <duration>` before `brew`; `168h` means seven days
and remains the default. An explicit argument overrides optional configuration.
A first invocation does not start a new waiting period for an old bottle.
Unrelated recipe edits must not reset the clock when verified attestations
establish the same bottle digest. Age exceptions remain explicit and artifact-bound;
they never waive integrity, provenance, vulnerabilities or execution binding.
Legacy upstream/distribution publication observations cannot satisfy this policy.

## Vulnerability evidence

Use public `brew vulns --json` for OSV and official Homebrew advisory data for
Homebrew-specific applicability and patch fixes. Scan explicit complete candidate
closures in an empty inspection prefix so an installed SBOM cannot select an old
version. Do not repeat the same OSV lookup in a separate collector.
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
| Verified attestation too young | May waive the specific age rule with a recorded reason |
| Missing or invalid attestation timestamp | Hold; cannot waive |
| Hash mismatch or invalid signature | Deny; cannot waive |
| Required verification unavailable, missing, or unsupported | Hold; cannot waive |
| Unapproved tap or signing identity | Separate trust-policy change and replan required |
| Unknown or additional dependency | Include and verify it before execution |
| New finding violating vulnerability policy | Deny; emergency mode cannot override |
| Plan, artifact, policy, or environment changed | Replan |

An age exception binds to the plan ID, full package identity, digest, dependency
graph, policy digest, reason, expiry, and one execution attempt. Identify and
justify dependency age exceptions individually. Do not update unrelated packages
as part of an emergency. After failure, inspect the current installation and retry with fresh discovery,
verification and a new plan rather than replaying an exception or marking a
package permanently trusted.

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
| Owned process | One operation identity, process/session identity, short-lived workspace; no saved result or authorization |

Zero and unrecognized values must never mean allow. Keep stable reason codes
alongside explanations. Deserialized `authorized: true` is not authorization.

The initial domain evaluator accepts explicit time, complete reachable dependency
graphs, exact artifact subjects and attributed evidence. An eligibility `Allow`
is not an execution permit. Callers must derive binding digests from the actual
plan, policy, graph and environment and acquire an exclusive operation lock. Before Homebrew
launches, durably record the owned child process session; a lost wrapper cannot
authorize another mutation while its child may remain active. Distribution builds connect this evaluator through the application
execution workflow and the public-command session.

State flow: draft -> evidence collected -> allow/hold/deny -> revalidated ->
executing -> succeeded/partial/failed/unknown. An emergency reevaluates only the
age condition; it cannot jump from deny directly to executing.

## Binding verification to execution

The official-bottle path freezes signed metadata, verified bottle downloads and
OCI dependency data. The complete runtime closure is checked before mutation.
Public Homebrew commands install dependencies before targets, with auto-update,
cleanup, autoremove and opportunistic dependent maintenance disabled. No network
or changes to verified inputs are allowed during execution. Source fallback or
an extra download cannot silently substitute an unverified artifact.

Selected existing kegs are observed through public metadata, receipts and active
links. They are trusted user-owned state: checking a newly downloaded bottle does
not cryptographically verify an already installed payload. BrewWarden binds its
own new installations to verified bottle bytes and records the existing state
used by the plan. It does not reconstruct or replace unchanged packages merely
to claim that it installed them. Unrelated packages are not scheduled for
maintenance. The no-concurrent-independent-mutation condition above applies.

A preview, `--force-bottle`, disabled updates or a wrapper lock alone is not proof
of binding. The owning [adapter contract](../internal/adapters/homebrew/README.md)
records the fixed input, cache and actual execution checks. No private Ruby API
or generated executable recipe is used by BrewWarden. A new unsupported path
must remain unavailable until its relevant binding is demonstrated.

An upgrade can partially succeed. Report both exit status and actual state.
Post-install matching does not prevent prior code execution. Do not automatically
roll back: applications and their data may not support downgrades. Interrupted commands have unknown outcomes. A fresh retry rediscovers and
revalidates the entire closure; no saved plan is replayed or marked successful.

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

Observations and frozen plans are private, short-lived files for the pending
operation. Their exact stored bytes are hashed and revalidated before execution;
a hash does not authenticate a local administrator. Normal session closure removes
the collection. An owned Homebrew child that survives its wrapper retains the
workspace until that entire session stops. Incomplete collections cannot execute.

### Owned-process interruption state

A nonblocking operation lock covers plan preparation and every mutation. Before
a launched child passes the startup gate, persist its plan/attempt identity,
collection name and process/session ID in a strict, private `inflight.json` record
with file and directory synchronization. Its pending rename state is treated as
potentially active, not ignored. If recording fails, kill the gated child; never
start Homebrew without the record. Malformed, unsafe or unreadable state holds new
mutations. Parent death does not stop a surviving child or authorize another
mutation. Check the whole owned session, including child process groups, before
removing a stopped record and its workspace under the operation lock. Inventory
all process owners: a Homebrew descendant can change user ID without leaving its
session. An unavailable process inventory holds mutation.

Do not save an execution outcome, interpret a stale record as success, restore an
age exception, replay a plan, or roll back. Report current failure or interruption
as partial/unknown when appropriate. A retry after the child stops starts fresh
discovery, attestation, advisory scanning, policy evaluation and binding. The
`history`, `status` and `reconcile` commands are unsupported; `doctor` remains a
read-only environment check. Legacy attempt-journal directories are rejected
with an actionable hold rather than silently migrated.

## Implementation and acceptance

1. Probe Homebrew plan binding, authenticated publication metadata, existing
   verification capabilities and their gaps, delegated helpers, and advisory coverage.
2. Implement typed evidence, strict configuration, and pure decisions;
   model normal and emergency decisions together.
3. Integrate official bottle planning and checks; enable normal and emergency
   execution only for demonstrated paths.
4. Add cask, third-party tap, and upstream-signature support by explicit capability.
5. Improve presentation/performance and design optional automation separately.

The public-command path is exercised through candidate, installation, upgrade,
unchanged-dependency, exception and interruption tests in a disposable macOS VM.
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
