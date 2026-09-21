# Homebrew integration contract

For every feature, investigate Homebrew's current commands, flags, environment
variables, APIs, and implementation before writing a replacement. Prefer reusing
verified capabilities and implementing the missing guarantees in BrewWarden.
Do not equate delegation with trust in every upstream behavior, or reimplement a
check solely to make it appear independent.
Reuse is a preference, not a requirement to depend on brittle private APIs or
human-readable output. Compare guarantee coverage and maintenance cost; use a
maintained library for a justified gap. BrewWarden owns the policy decision even
when Homebrew supplies evidence. Reimplementing all checks and using Homebrew as
a final check does not, by itself, bind preflight results to installation.

## Capability records

The implemented capabilities below apply to the initial supported official-bottle
scope. Exact upstream pins, options and boundary tests live beside each adapter.

| Requirement | Delegated guarantee and BrewWarden boundary | Owning contract |
| --- | --- | --- |
| Artifact integrity | Native checksum verification plus authenticated digest, frozen bottle inputs and complete closure binding | [Homebrew](../internal/adapters/homebrew/README.md) |
| Metadata authenticity | Pinned Homebrew JWS verification before recipe selection; strict metadata and recipe identity checks | [Homebrew](../internal/adapters/homebrew/README.md) |
| Bottle provenance | Maintained attestation-only verifier; exact subject, signer and workflow checks without native gh bootstrap | [Attestation](../internal/adapters/attestation/README.md) |
| Minimum release age | Official recipe history bound to the selected bottle digest; rebuilds receive their own age | [Homebrew](../internal/adapters/homebrew/README.md) |
| Known vulnerabilities | Public OSV scanning plus Homebrew advisory status for the exact candidate; unavailable or skipped checks hold | [Homebrew](../internal/adapters/homebrew/README.md) |
| Execution binding | Pinned native install APIs, frozen inputs, retained formula locks, installer guards and reconstructed installed payload checks | [Homebrew](../internal/adapters/homebrew/README.md) |

For each implemented row, link to its owning adapter and contract tests, and record:

- A stable capability identifier and the exact security claim and artifact scope.
- Upstream documentation and source at an inspected revision, tested brew/OS
  versions, the selected command/flags/environment, and conflicting settings.
- Preconditions, dependencies and credentials, side effects, exclusions, and how
  successful verification can be distinguished from a skipped check.
- Guarantees delegated to Homebrew, additional BrewWarden checks, remaining gaps,
  and handling of missing, failed, or unsupported evidence.
- Tests, known upstream deprecations/replacements, and the last verified version.

Keep detailed operational records beside the owning adapter once it exists;
this document remains the short index. Do not duplicate command definitions or
version tables across core policy, CLI, documentation, and multiple adapters.

## Preflight investigation

The supported product path has demonstrated native install, upgrade with an
unchanged dependency, age exceptions, changed-input refusal, affected-dependent
refusal, lock contention, partial failure and stopped-session reconciliation in
a disposable macOS VM. The [native adapter](../internal/adapters/homebrew/README.md)
owns the guarantees and exact supported pins; [verification](verification.md)
describes the repeatable acceptance entrypoints.

Before expanding support, inspect the selected upstream revision and test both
successful and skipped verification paths. A native preview or zero exit code
does not establish coverage of every planned artifact. Likewise, enabling native
attestation checks does not by itself constrain helper bootstrap, credentials,
signer identity or later dependency resolution. Reuse only the guarantees actually
established, and supplement missing claims through the owning adapters above.

Bottle registration history dates the selected digest rather than the upstream
version. A completed supported advisory lookup can report no known findings
without a historical-advisory witness. Neither claim alone grants execution. The native session binds the complete evaluated closure to actual
installation; builds without a trusted runtime digest remain diagnostic-only.

## Code and evidence ownership

Organize the Homebrew adapter by capability so command construction, parsing,
and compatibility changes have an identifiable owner. Centralize each upstream
option and environment setting there. Do not scatter them through use cases or
hide them in generic shell helpers. No capability framework is needed in advance.

Return typed evidence identifying the claim, subject/digest, provider, provider
version, status, and evidence reference. Record whether a claim is established by
Homebrew or an additional BrewWarden check; expose that attribution in diagnostics
and history. "Option enabled" and process exit zero are not, by themselves,
proof that every required check covered every artifact.

Core policy decides which evidence is required. A replacement provider must
satisfy the same contract; it must not silently weaken policy. A second invocation
of the same upstream check is not an independent trust source.

## Compatibility and verification

Test success, rejection, skipped/unsupported paths, conflicting environment,
cached artifacts, helper bootstrap, and dependency changes where relevant. Use
isolated integration environments; mocks alone do not demonstrate upstream
behavior. Source inspection and empirical tests must support the claimed scope.

On brew updates, revisit affected records and tests before expanding supported
versions. Removed flags, deprecated commands, changed output, or unrecognized
versions must not silently produce successful evidence. Make unsupported
capabilities visible through `doctor`; fail only operations requiring those
capabilities. Retire redundant BrewWarden checks when an upstream improvement
satisfies their complete contract and equivalent tests pass.

### Reconciliation

The pinned `FormulaLock` implementation provides nonblocking cross-process
exclusion for each candidate formula. Reconciliation uses fresh trusted scripts
and runtime bytes, reacquires every original candidate lock, inventories the
actual candidate racks and activation links, and retains the snapshot by digest.
An active original session prevents observation. This records current state only;
it does not rerun installers, signal remembered process IDs, infer an exit code,
restore exceptions, or roll back installed packages. The application appends a
`reconciled` journal transition and requires a new plan for further mutations.
