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
| Metadata authenticity | Public Homebrew JWS verification; strict selected artifact and dependency identities | [Homebrew](../internal/adapters/homebrew/README.md) |
| Bottle provenance | Reviewed installed gh 2.66.0–2.101.0 public attestation verifier; exact downloaded digest, signer, verified subject and trusted time checks | [Attestation](../internal/adapters/attestation/README.md) |
| Minimum release age | Earliest trusted attestation timestamp for the exact digest; missing evidence holds even with an age exception | [Attestation](../internal/adapters/attestation/README.md) |
| Known vulnerabilities | Public OSV scanning plus Homebrew advisory status for the exact candidate; unavailable or skipped checks hold | [Homebrew](../internal/adapters/homebrew/README.md) |
| Execution binding | Public install/upgrade, fixed metadata and cache, verified bottle bytes, complete closure and observed opt/linked-keg state; partial pours cannot be mistaken for linked installations | [Homebrew](../internal/adapters/homebrew/README.md) |

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

The public product path requires disposable macOS VM acceptance for installation,
upgrade, unchanged dependencies, age exceptions, input refusal, partial outcomes
and interruption with fresh retry. Representative authenticated final-path
cases passed in a disposable Tahoe VM; see [verification](verification.md) for
exercised scope and remaining release boundaries. The [Homebrew adapter](../internal/adapters/homebrew/README.md) owns
exact guarantees and pins; [verification](verification.md) lists the entrypoints.

Before expanding support, inspect the selected upstream revision and test both
successful and skipped verification paths. A native preview or zero exit code
does not establish coverage of every planned artifact. Likewise, enabling native
attestation checks does not by itself constrain helper bootstrap, credentials,
signer identity or later dependency resolution. Reuse only the guarantees actually
established, and supplement missing claims through the owning adapters above.

Verified attestation time dates the selected digest rather than the upstream
version. A completed supported advisory lookup can report no known findings
without a historical-advisory witness. Neither claim alone grants execution. The public-command session binds new bottle installations and the selected
existing state to the evaluated plan; builds without the distribution build marker remain diagnostic-only.

## Code and evidence ownership

Organize the Homebrew adapter by capability so command construction, parsing,
and compatibility changes have an identifiable owner. Centralize each upstream
option and environment setting there. Do not scatter them through use cases or
hide them in generic shell helpers. No capability framework is needed in advance.

Return typed evidence identifying the claim, subject/digest, provider, provider
version, status, and evidence reference. Record whether a claim is established by
Homebrew or an additional BrewWarden check; expose that attribution in diagnostics
without saving a history. "Option enabled" and process exit zero are not, by themselves,
proof that every required check covered every artifact.

Core policy decides which evidence is required. A replacement provider must
satisfy the same contract; it must not silently weaken policy. A second invocation
of the same upstream check is not an independent trust source.

## Compatibility and verification

Test success, rejection, skipped/unsupported paths, conflicting environment,
cached artifacts, helper bootstrap, and dependency changes where relevant. Use
isolated integration environments; mocks alone do not demonstrate upstream
behavior. Source inspection, unit and adapter contract tests, and representative
native execution for materially different binding behavior must support the
claimed scope. This does not require a full VM suite for every upstream patch
release or every combination of otherwise compatible tool versions. Conversely,
unit tests with synthetic success cannot prove an upstream installer consumed
the frozen artifacts and complete plan.

On brew updates, review the upstream delta at affected capability boundaries
and run the relevant contracts before expanding supported versions. Add native
cases for a new platform or demonstrated change to trust or execution binding,
not as an arbitrary per-version quota. Removed flags, deprecated commands,
changed output, or unrecognized versions must not silently produce successful
evidence. Make unsupported
capabilities visible through `doctor`; fail only operations requiring those
capabilities. Retire redundant BrewWarden checks when an upstream improvement
satisfies their complete contract and equivalent tests pass.

### Interrupted operations

A private operation lock coordinates BrewWarden sessions. The startup gate records
an owned process session before launching a public Homebrew command. A new
mutation refuses an active or unobservable session. After the whole session
stops, a retry inspects installed state and starts fresh candidate collection;
the old record cannot authorize success, replay, rollback or an age exception.
Ordinary independent Homebrew mutations remain outside the wrapper's control.
