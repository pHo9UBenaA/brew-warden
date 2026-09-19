# Homebrew integration contract

For every feature, investigate Homebrew's current commands, flags, environment
variables, APIs, and implementation before writing a replacement. Prefer reusing
verified capabilities and implementing the missing guarantees in BrewWarden.
Do not equate delegation with trust in every upstream behavior, or reimplement a
check solely to make it appear independent.

## Capability records

Maintain this inventory as implementation progresses. These are investigation
candidates, not currently supported capabilities or proven guarantees.

| Requirement | Homebrew candidate | Boundary to establish; BrewWarden responsibility |
| --- | --- | --- |
| Artifact integrity | Native checksum verification | Authenticate expected metadata; bind results to actual bytes and the complete execution closure |
| Metadata authenticity | Signed API metadata handling | Establish which input was verified and whether settings can bypass it |
| Bottle provenance | `HOMEBREW_VERIFY_ATTESTATIONS=1` | Test covered paths, identities, cache behavior, exclusions, authentication, and gh bootstrap |
| Minimum release age | Metadata/API fields and any native age controls available in the supported version | Bind publication evidence to the candidate and enforce the requested duration where upstream does not |
| Known vulnerabilities | Native vulnerability commands and supported advisory interfaces | Establish revision-aware coverage, freshness, unknowns, dependency coverage, and refusal rules |
| Execution binding | Native resolution, fetch, and install controls | Prove verified targets are consumed; an unrestricted install after preflight is insufficient |

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
