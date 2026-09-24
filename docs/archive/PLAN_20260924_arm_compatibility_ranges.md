# Plan: bounded version ranges for native arm64 Homebrew bottles

Date: 2026-09-24. Status: researched candidate ranges; product changes are NOT
implemented by this plan. It supersedes the per-version/full-VM and x86_64
proposals in `PLAN_20260924_installed_brew_compatibility.md`. The current
execution contract remains `docs/design.md`. Never modify the host Homebrew;
all integration writes stay in disposable guests or private workspaces.

## Answers and evidence as of 2026-09-24

| Dependency | Actually admitted today | Candidate range to establish | Why these boundaries? |
| --- | --- | --- | --- |
| Homebrew, native arm64 macOS Tahoe at `/opt/homebrew` | Exact reviewed 7.0.4 runtime at `edb70f031e4170c780799633a1226ff73e1077f4` (including its portable Ruby and implementation fingerprint) | **6.0.19 through 7.0.6**, inclusive, CONDITIONAL on contract checks below | 6.0.19 is the first inspected release whose `brew vulns --json` reports `skipped_formulae`, required to avoid declaring an unscanned dependency clean. Signed internal packages JWS/index and `brew fetch --bottle-tag` exist by then. 6.0.18's vulnerability JSON is only an array of findings, with no skipped-subject inventory; it does not meet the present coverage contract. 7.0.6 was the latest released tag checked. |
| Installed gh attestation verifier | Exactly 2.101.0, tested with authenticated bottles | **2.66.0 through 2.101.0**, inclusive, CONDITIONAL on signed-result and trust-root checks below | gh 2.65.0 and earlier in the inspected sequence use `sigstore-go v0.6.2`, which emits `uri: "TODO"` for a verified Tlog timestamp. gh 2.66.0 first uses v0.7.0 and emits its verified URI; gh 2.70.0 first documents the JSON `verifiedTimestamps` trust contract. 2.101.0 was the latest released tag checked. |

These are **proposed investigation/implementation envelopes, not already
supported versions** and not a guarantee that every combination works. Existing
7.0.4/2.101.0 with a matching runtime fingerprint is the only accepted tuple.
Lower bounds apply to the *current public-command verification contract*; an
alternative source of authenticated skip/time evidence could warrant a separate
proposal. Upper bounds apply only to releases available at the date above.
Unreleased, future or incompatible builds must hold until reviewed. Do not
infer that a version number alone establishes an execution permit.

Read-only source checks: Homebrew tags 6.0.18/6.0.19/6.0.22 and 7.0.0 through
7.0.6 from the locally available official checkout, plus public tagged gh
`go.mod` and `verify.go` and sigstore-go `signed_entity.go`. Homebrew's
[`vulns` JSON change](https://github.com/Homebrew/brew/commit/f49304fc37bdf57ed1893706cddda969d63d9132)
introduced the skip list; Homebrew's [6.0.19 signed packages
index](https://github.com/Homebrew/brew/blob/6.0.19/Library/Homebrew/api/packages_index.rb)
refers to revalidated signed JWS bytes. gh 2.65.0 and 2.66.0 module pins, and
sigstore-go 0.6.2 vs 0.7.0 verified URI behavior, are recorded in
`REVIEW_20260924_arm_versions_and_verified_timestamps.md`. Public latest-release
API responses identified Homebrew 7.0.6 and gh 2.101.0 on this date. Static
source inspection identified minimum *necessary* fields; it has not proved
that all combinations satisfy the full execution-binding contract.

## Evidence model: not a VM for every version

Pure unit and property tests establish decisions given explicit evidence,
including age waiver, missing/replayed attestations, closure and ambiguity.
Adapter contract tests exercise the real JSON/command/filesystem boundary with
isolated inputs and negative outcomes: wrong signer/subject/digest/log URI,
empty or skipped scanner results, unsigned or changed metadata, changed
bottle/cache, dependency graph drift, unexpected fetch/source fallback, partial
link, and changed Homebrew code between collection and mutation. Such tests
can cover **every relevant upstream version cohort** without installing a
bottle for every patch release. A parser accepting a synthetic success does
not prove that upstream really checked a signed JWS or that installation used
the frozen plan; that is why some native execution evidence still matters.

Read source diffs and upstream tests for the owning Homebrew/gh interfaces at
both range endpoints and every **material change**, not every changed test or
unrelated cask file. Relevant 7.0.x transitions include 7.0.1 -> 7.0.2
(API/info/keg), 7.0.2 -> 7.0.3 (vulnerability sources), and 7.0.4 -> 7.0.5
(installer sandbox, formula loading, linking); 7.0.3 -> 7.0.4 and 7.0.5 ->
7.0.6 had no changes in the selected command/adapter source paths inspected.
The 6.x -> 7.x transition changes additional signed API and installer source.
Relevant gh cohort changes include sigstore-go 0.7.x -> 1.0, 1.1, 1.2 and
1.3, and trust-root/verified-output code. Revisit whether the source and
contract tests really cover each transition; a relevant changed behavior must
fail closed until its guarantee is established.

Use a **small representative native VM suite for materially distinct execution
behaviors** (and always the exact final distribution path), not a full Cartesian
product or a compulsory run for each version. Examples: oldest 6.x API/OSV
cohort with skipped subjects, the reviewed 7.0.4 baseline, and the newest
7.0.6 installer/sandbox cohort; exercise the oldest viable gh verified-result
cohort and latest gh in a disposable guest. Select any further case based on
an identified behavioral difference or failed contract test. The native cases
must verify that a real install/upgrade consumes bound evidence and its complete
closure and that missing evidence and partial results hold. They cannot prove
absence of all vulnerabilities; unit tests cannot alone prove a third-party
installer's side effects. Neither test type is an arbitrary approval quota.

## Implementation sequence

1. **Compatibility survey**: inventory supported `brew info/fetch/--cache/vulns`
   output and signed API/JWS behavior, bottle/OCI tag and cellar, installer
   flags, sandbox writes/network, and opt/linked-keg after-state across the
   6.0.19–7.0.6 cohorts. Inspect gh signer, certificate, trust roots and
   verified timestamps across 2.66.0–2.101.0, including older/nonmatching
   signed subjects. If a required invariant fails, raise the floor or split
   the range at the actual break, with a reproducible input; do not relax age,
   integrity, advisory coverage or plan binding.
2. **Replace the monolithic runtime digest gate** with reviewed-version bounds
   and capability-specific fail-closed checks where the needed behavior can be
   observed, while still comparing the installed Homebrew implementation
   before private inspection and immediately before mutation. Record the
   selected binary, runtime/source identity, brew version, OS/architecture,
   bottle tag and gh binary/version in the one-use plan. A changed implementation
   during a command holds; user- or locally-modified implementations outside a
   reviewed cohort are not silently trusted. Keep the same-user attacker and
   trusted Homebrew assumptions explicit. Do not use `PATH` discovery as trust
   proof or authorize `/usr/local`/Rosetta while widening versions.
3. **Prove each adapter contract** with upstream-derived outputs and private
   copied runtimes, including negative tests for source changes, skipped
   subjects, missing signed JWS, log URI `TODO`, future versions, and gh exit
   zero without verified time. Add small Go table/property tests for version
   selection and evidence invariants. Avoid fixture-only tests that mirror a
   version string without reaching the owning adapter.
4. **Representative native acceptance**: provision fresh arm64 Tahoe VMs at
   `/opt/homebrew` with reviewed runtime versions and installed gh variants.
   Run the public `doctor`, actual signed metadata/fetch, real install/upgrade,
   age refusal/waiver, changed inputs, partial link, parent death and fresh
   retry for distinct changed execution cohorts. No host Homebrew mutation and
   no host credential copying. A failed old gh result or unsupported formula
   is an expected hold, not a successful execution; adjust the candidate
   envelope based on observed failure, not bypassing checks.
5. **Release/maintenance gate**: update `docs/design.md`, owning adapter
   capability records and README only for proven bounds. Run
   `./scripts/verify.sh`, affected contract/race/fuzz checks, full
   `./scripts/check.sh all`, reproducible builds, and a revision-bound native
   product gate for any changed execution path. Future releases above these
   upper bounds require a source/API delta review and affected contracts; run
   new native cases when behavior, trust, binding or platform changes, not for
   every unrelated patch. Keep unknown versions held with actionable reasons.

**Completion** means a documented *implemented* version interval that actually
passes all required contracts and representative bound-execution acceptance.
Until then, 6.0.19–7.0.6 and 2.66.0–2.101.0 are conditional targets only.
Do not publish or claim new compatibility on the strength of this plan alone.
