# Product design

Status: proposed design, updated 2026-09-19. The development harness exists;
product behavior and installation enforcement do not yet exist.

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

Go is the proposed product language and is used by the harness. Follow the
[dependency and implementation rules](docs/dependencies.md). See
[architecture](docs/architecture.md) for source boundaries and
[threat model](docs/threat-model.md) for trust assumptions.

## User operations

The name `brew-security` is provisional. Compatibility with `brew security` and
Homebrew's environment filtering requires an integration experiment.

| Operation | Behavior |
| --- | --- |
| `status` | Show installed state, holds, urgent candidates, and unknown evidence |
| `plan install <name>` | Resolve, verify, and save an installation plan |
| `plan upgrade [name...]` | Resolve and verify an update, including changed dependencies |
| `apply <plan-id>` | Revalidate and execute the exact supported plan |
| `emergency plan <name> --advisory <id>` | Propose a bounded age exception tied to verified fix evidence |
| `emergency plan <name> --reason <text>` | Record a user-requested age exception separately from verified advisories |
| `explain <plan-id>` / `history [name]` | Explain evidence, changes, exceptions, and outcomes |
| `doctor` | Diagnose supported Homebrew/verifier versions, configuration, and storage |

Future convenience commands may combine planning and application. Interactive
and noninteractive paths must share the policy engine. `--yes` can skip a prompt,
never verification or a hold. There is no universal `--force` bypass.

No monitoring schedule or automatic update is enabled by default. The initial
emergency workflow suggests an update and waits for the user's execution choice.

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
- A missing or unsupported required verifier holds the operation. Do not install
  the verifier automatically while checking the package.
- Preserve artifact changes as facts. A legitimate rebuild can change a digest
  without changing the upstream version; this is not automatically malware.

## Age policy

Keep upstream publication time, Homebrew adoption time, bottle creation time,
and local first-observation time separate. Record each source and its trust
conditions. A Git author or committer timestamp is not an independently verified
publication timestamp.

The provisional default is seven days measured from first observation of the
exact digest (`observed`). Evaluate onboarding friction before fixing this
choice. Also support a `published` policy with explicit acceptable timestamp
sources; `both` requires both conditions. Unknown dates never become ancient
releases implicitly.

Repeated observations do not reset the first observation. New digest means new
artifact. Preserve revision, rebuild, OS, and architecture; separately evaluate
recipe and dependency changes that can affect installation. An unrelated prose
change need not reset artifact age.

Limitations: old releases are new observations on first use; infrequent runs
create delays; unobserved replacement-and-restoration events may be missed;
local clock/state tampering is outside the trust boundary. Detect clock rollback
and hold, but do not claim to prevent all clock manipulation. Waiting alone does
not detect an already malicious release. A public observation service would add
operational trust and is not part of the initial product.

## Vulnerability evidence

Reuse maintained sources or Homebrew capabilities through adapters. First verify
candidate-version queries, machine-readable output, and Homebrew patch handling.
Do not deny a package based only on a fuzzy name match.

Represent `affected`, `not_affected`, and `unknown`, with separate source
availability, package identity, version applicability, and freshness. Zero search
results do not mean safe. Do not apply an upstream advisory blindly to a patched
Homebrew revision.

Normal eligibility and urgency are separate. Show fixed, remaining, and newly
introduced findings. Enforce the configured vulnerability policy, including
new problems, without silently treating an emergency as permission to ignore it.
Incomplete source responses must not make a candidate look fixed.

## Emergency updates

Emergency updates are a first-class workflow: propose, explain the exact target
and dependencies, then apply at the user's choice. Unattended emergency execution
requires a later explicit opt-in design.

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

Use the [hexagonal structure](docs/architecture.md): domain policy, core-owned
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

State flow: draft -> evidence collected -> allow/hold/deny -> revalidated ->
executing -> succeeded/partial/failed/unknown. An emergency reevaluates only the
age condition; it cannot jump from deny directly to executing.

## Binding verification to execution

This is the largest unresolved feasibility issue. Disabling Homebrew auto-update
and checking just before execution do not alone eliminate concurrent changes or
re-resolution. A lock held by this CLI does not lock every other brew process.

For supported brew versions, demonstrate an execution path that consumes the
verified metadata, artifact digests, and complete dependency plan. Include cache
handling, source-build fallback rejection, concurrent changes, and attestation
exceptions. Passing a local bottle alone does not prove dependencies are fixed.
`--force-bottle` is not assumed to universally prohibit source builds.

Possible approaches include a constrained immutable-metadata execution path or
an upstream-supported pre-execution integration. Do not generate executable Ruby
recipes as an unexamined shortcut. If binding cannot be demonstrated, ship
planning and inspection while explicitly rejecting apply. This is a release gate,
not a retreat from the all-in-one product goal.

An upgrade can partially succeed. Record both exit status and actual state.
Post-install matching does not prevent prior code execution. Do not automatically
roll back: applications and their data may not support downgrades. Recovery is a
separate plan. Interrupted executing attempts become unknown until reconciled.

## Configuration and storage

Start with one user-owned JSON configuration, not executable configuration or
implicit current-directory policy. Future project policy must be explicit and
must not weaken user policy through merging.

```json
{
  "schemaVersion": 1,
  "age": { "mode": "observed", "minimumHours": 168 },
  "trust": { "allowedTaps": ["homebrew/core"] },
  "verification": { "requireChecksum": true, "requireBottleAttestation": true },
  "emergency": { "mode": "suggest", "waivableRules": ["age"] }
}
```

Reject unknown configuration fields, ambiguous duplicate keys, invalid values,
and implicit type coercion. A policy change invalidates prior decisions.

Store separate immutable JSON records for observations, plans, and attempts in
private directories. Use a single writer, temporary files, synchronization,
same-filesystem rename, and required directory synchronization. Prevent path and
symlink escape; never use raw external names as storage paths. Indexes must be
rebuildable. Do not invent a database or JSON canonicalization protocol.

An ID can hash the exact stored bytes. Revalidate their schema and contents when
reading. A hash is not a signature or protection against the local administrator.
If an execution record cannot be persisted, do not begin a mutation. Read-only
reconciliation diagnostics should remain available.

## Implementation and acceptance

1. Probe Go distribution, verifiers, Homebrew plan binding, and advisory coverage.
2. Implement typed evidence, strict configuration, pure decisions, and history;
   model normal and emergency decisions together.
3. Integrate official bottle planning and checks; enable apply and emergency apply
   only for demonstrated execution paths.
4. Add cask, third-party tap, and upstream-signature support by explicit capability.
5. Improve presentation/performance and design optional automation separately.

Acceptance must cover invalid signatures under emergency mode, expired/replayed
exceptions, changed policy/digests, incomplete advisory responses, same-version
rebottles, platform-specific artifacts, changed dependencies, cache substitution,
concurrent metadata changes, source fallback, malformed/oversized input, timeouts,
unsafe paths, full disks, and interrupted processes. Test real boundaries in an
isolated environment; do not upgrade the maintainer's normal Homebrew prefix.

Dependency inventory, supported brew/verifier versions, and reproducibility
results belong to release evidence. The current harness does not establish
product enforcement, verifier coverage, or release readiness.
