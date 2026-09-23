# Simplify BrewWarden around public commands

Date: 2026-09-22. Status: implemented and locally accepted on 2026-09-23 for
the initial supported scope. Actual exercised cases and bounded holds are in
[verification](../verification.md). Signing and publication remain separate.
This plan supersedes the provenance, bottle-age, and recovery mechanisms recorded
in `PLAN_20260922_completion.md`. `docs/design.md` remains authoritative and must
be updated with the implementation. Repository content and diagnostics stay in
English.

## Product shape

Keep one small prefix interface:

```text
bwd [policy options] brew install|upgrade [formulae]
```

The supported initial product remains official `homebrew/core` bottles on the
tested Apple Silicon macOS/Homebrew range. Formula names do not select special
code paths, source-host conventions, or build-system recognizers. The goal is a
general-purpose tool for every official-core bottle whose required public
capabilities pass; the initial platform boundary is not a package allowlist.
Casks, source builds, third-party taps, and other platforms require separate
demonstrated capabilities.

```text
request
  -> discover the exact target and runtime dependency closure with brew
  -> fetch the exact bottles with brew
  -> verify every bottle and advisory input
  -> evaluate the whole closure
  -> revalidate the bound inputs
  -> run public brew installation commands
```

Verification may run concurrently where its inputs are independent. Installation
starts only after every requested formula and required dependency passes. Do not
mix verification with installation, and do not launch competing Homebrew mutation
commands against the same prefix. Pass a supported target set to one bound public
Homebrew invocation where its command semantics permit; otherwise serialize the
bound mutations. Never run parallel `brew install` processes against one prefix.

## Fixed decisions

- Prefer public Homebrew and GitHub CLI commands. Do not call Homebrew private
  Ruby APIs, generate Ruby installer programs, inspect upstream project tags, or
  query package-specific upstream repositories.
- Do not use source URLs, source checksums, upstream release identities, recipe
  shapes, build-system recognition, source reconstruction, or installed-payload
  reconstruction as official-bottle eligibility evidence. Keep only bounded
  archive safety checks needed to consume the selected bottle without treating
  them as build provenance.
- Trust the supported Homebrew implementation. Homebrew compromise is out of
  scope. BrewWarden still accounts for every required subject and binds verified
  inputs to its own execution.
- Use Homebrew's signed metadata, the selected platform bottle digest, and the
  complete runtime dependency closure. A successful process without complete
  subject coverage is not sufficient.
- Require Homebrew JWS metadata verification, SHA-256 verification of actual
  bottle bytes, and a valid GitHub artifact attestation. Upstream author GPG
  signatures are not required for official Homebrew bottles.
- Use `brew vulns --json` as the OSV scanner and the official Homebrew Advisory
  Database as the Homebrew-specific supplement. Do not maintain another OSV
  client or require a historical advisory as proof of database coverage. The two
  sources have overlapping provenance and do not prove exhaustive vulnerability
  coverage; report only "no known applicable findings" after both required
  lookups complete.
- Only age may be waived. Integrity, provenance, vulnerability, closure, and
  execution-binding failures remain non-waivable.
- Retain the seven-day (`168h`) default minimum age, explicit CLI override,
  optional configuration, and one-attempt artifact-bound age exceptions. A
  supported mutation invocation is authorization to proceed after verification;
  do not add a second BrewWarden confirmation prompt.
- There is no resume, rollback, or saved-plan replay. A retry performs fresh
  discovery and verification. Keep only the durable in-flight state needed to
  distinguish a still-running Homebrew process from an interrupted operation.
- BrewWarden does not intercept Homebrew commands run independently by the user.
  The user must not mutate the same prefix concurrently with a BrewWarden run.
- Do not modify the maintainer's Homebrew installation in tests. Use disposable
  environments. Do not push, publish, sign, or notarize without separate approval.
- Use the installed supported `brew` and `gh`. Remove the bundled Homebrew,
  portable Ruby, generated Ruby, and custom verifier distribution paths. The
  repository's ignored `.cache` may be used only as disposable, untracked work;
  it is not a product source or durable acceptance record. Homebrew's own cache
  may hold the exact verified bottles used by execution.

## Evidence sources

| Decision | Source | BrewWarden responsibility |
| --- | --- | --- |
| Candidate identity and dependency closure | `brew info --json=v2 --formula` with Homebrew's signed API path | Require exact formula, version, revision, rebuild, platform, digest, and complete runtime closure |
| Bottle acquisition and checksum | `brew fetch --formula --bottle-tag=...` and `brew --cache` | Hash the actual bytes and match authenticated metadata; reject omissions, extras, source fallback, and cache escape |
| Provenance | `gh attestation verify` on each local bottle | Require successful verification for every exact digest and validate bounded JSON coverage |
| Minimum age | Verified attestation timestamps for the exact bottle digest | Use the chronologically earliest valid timestamp; a new digest starts a new age |
| Upstream vulnerabilities | `brew vulns --json` on the explicit candidate closure in an isolated inspection prefix | Prove the command scans planned candidates rather than installed versions; treat findings as deny and skipped, malformed, incomplete, or unavailable results as hold |
| Homebrew-specific vulnerabilities | `https://formulae.brew.sh/api/advisories.json` and authenticated formula identity | Apply Homebrew version/revision and patch status; retain source attribution |
| Execution | Public `brew install` / `brew upgrade` with frozen verified inputs | Revalidate the plan and deny network, source fallback, new dependencies, or changed inputs during execution |

Do not use `generated_date`, recipe Git history, source archive timestamps, OCI
creation time, or GitHub Release dates for the age decision.

## Attestation and age contract

For each downloaded bottle, invoke the installed GitHub CLI with fixed arguments
equivalent to:

```text
gh attestation verify BOTTLE \
  --repo Homebrew/homebrew-core \
  --predicate-type https://slsa.dev/provenance/v1 \
  --format json \
  --limit 100
```

Use the normal GitHub CLI verifier and trust roots. BrewWarden does not install,
upgrade, bundle, or reimplement `gh`; a missing or incompatible command is an
actionable hold. Do not download bundles through a separate GitHub API client.

Parse every verified result and every `verificationResult.verifiedTimestamps`
entry. Require the verified subject digest to equal the locally hashed bottle.
Select the oldest timestamp across all accepted results. This measures how long
the exact bytes have had trusted Homebrew provenance, not the first HTTP download
or formula-API publication time. Re-attesting unchanged bytes does not reset age;
changing the digest does.

Empty results, missing timestamps, invalid time, wrong repository, wrong digest,
unsupported predicate, command failure, timeout, or malformed/oversized output
hold the complete operation. A result count reaching the configured fetch limit
also holds because an older attestation may have been omitted. Record the `gh`
version and evidence digest in diagnostics.

Legacy bottles available only through Homebrew's backfill repository are not
silently trusted by a new custom fallback. First measure whether supported
current candidate closures encounter this case. If they do, record the exact
Homebrew behavior and obtain a separate policy decision before adding another
trust identity.

## Simpler state and interface

Retain policy configuration and one-attempt age exceptions. Remove product state
whose only purpose is replay, rollback, long-lived plan application, custom
attestation caching, or Git-history age caching.

Replace the current hash-linked attempt/reconciliation workflow with the smallest
durable record that can safely handle process loss:

1. Before launching Homebrew, atomically record the operation identity and owned
   process/session identity.
2. While that process may still be active, reject another BrewWarden mutation.
3. If BrewWarden restarts and the owned process is gone, mark/remove the stale
   record and require a complete fresh run.
4. Never infer success, replay the old plan, restore an exception, or roll back.

Remove `history`, `status`, and `reconcile` from the normal interface when their
remaining behavior only supports saved attempts, replay, or reconciliation that
the new contract removes. Retain a command only if it has demonstrated user value
independent of that old workflow. Report a current interruption directly and keep
the minimum evidence needed for diagnosis in bounded output.

## Code migration

1. Add a command adapter for the installed `gh attestation verify` and contract
   tests for exact digest coverage, multiple attestations, multiple timestamps,
   deterministic oldest-time selection, limits, malformed output, failures, and
   cancellation.
2. Change the domain age evidence from bottle registration history to exact
   digest attestation time. Keep explicit time injection and age-only waivers.
3. Route provenance and age through the same verified `gh` response. Re-hash the
   bottle before and after verification and retain only evidence required by the
   pending operation.
4. Delete the custom attestation-only helper, its build template and packaging,
   direct attestation-bundle downloader/cache, GitHub commits registration
   provider/cache, bundled Homebrew/portable-Ruby runtime and inventory builder,
   generated Ruby paths, and their obsolete tests and documentation.
5. Keep the accepted public Homebrew metadata, fetch, advisory, and execution
   paths. Remove source-release, source-checksum, recipe-shape, build-recognition,
   reconstruction, GPG, and obsolete payload-proof fields and providers from the
   candidate contract. Remove duplicate interfaces and compatibility machinery
   after their final callers disappear; keep the enforced inward dependency
   direction.
6. Reduce recovery state and CLI commands to the in-flight contract above. Test
   actual parent death where Homebrew continues and where it has already stopped.
7. Update README, design, architecture, threat model, dependency inventory,
   Homebrew integration contract, verification instructions, distribution
   contents, help text, and examples to describe only the final implementation.

## Acceptance

- Unit and integration tests prove that the selected oldest verified timestamp
  is bound to the exact bottle digest and that later attestations of unchanged
  bytes do not reset age.
- A same-version rebottle with a new digest receives a new age.
- Every requested formula and required runtime dependency has metadata, bytes,
  provenance/age, OSV, and Homebrew advisory evidence before mutation.
- Public `brew vulns` acceptance proves planned-version selection in an isolated
  inspection prefix and accounts for every requested closure subject; an empty
  findings list or zero exit status alone is insufficient.
- Missing `gh`, authentication/rate-limit failure, missing attestation, saturated
  results, skipped vulnerability subjects, network failure, and malformed output
  all stop before installation.
- Fresh install, explicit upgrade, upgrade-all, multiple targets, unchanged
  dependencies, an age hold, and an explicit age exception work in a disposable
  standard-prefix macOS environment.
- Changed metadata, dependencies, bottle bytes, cache paths, or installed state
  between verification and execution stop the operation.
- Cancellation, parent death, child continuation, partial Homebrew failure, and a
  fresh retry behave according to the minimal in-flight contract.
- The distribution contains no Homebrew/Ruby runtime and no custom attestation
  verifier. It uses the installed supported `brew` and `gh` commands.
- Representative official-core formulae with unrelated source hosts and build
  systems follow the same bottle-only path without source-specific providers,
  build fingerprints, formula-name exceptions, or payload reconstruction.
- Repository verification confirms ignored `.cache` contents are disposable and
  untracked, and are neither source inputs nor the only copy of acceptance
  evidence.
- `./scripts/verify.sh`, broader race/fuzz/lint/vulnerability checks, isolated
  native acceptance, and two-build reproducibility pass before product-ready is
  claimed.

## Commit sequence

Use coherent Conventional Commits after each verified boundary:

1. `docs(plan): define public attestation age migration`
2. `feat(attestation): derive bottle age from verified gh timestamps`
3. `refactor(attestation): remove bundled verifier and history providers`
4. `refactor(state): reduce interrupted execution recovery`
5. `docs: align product contracts with the simplified flow`
6. `test: record final native distribution acceptance`

Do not preserve obsolete code solely for compatibility with unreleased local
records. If a safe automatic migration is not smaller than rejecting the old
schema with an actionable message, reject it explicitly.

## Previously completed repository decisions

The root `.gitignore` remains ignore-by-default while unignoring tracked source
trees recursively as whole directories. It does not use a global `!*/` traversal
rule or enumerate every `internal` subdirectory. This is already implemented and
is not another migration work package. README and current design documents must
still be rewritten in work package 7 because they describe the superseded
runtime, age, and recovery behavior.
