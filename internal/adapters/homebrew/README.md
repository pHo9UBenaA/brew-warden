# Homebrew boundary

The adapter uses public Homebrew commands. It does not load a Ruby bridge, call
private installer APIs or reconstruct installed payloads with Homebrew internals.

## Supported runtime

The inspected and tested implementation is Homebrew 7.0.4 at
`edb70f031e4170c780799633a1226ff73e1077f4`, using its portable Ruby 4.0.7 on Apple
Silicon macOS Tahoe at `/opt/homebrew`. The distribution's inventory SHA-256 is
`d50f6a3f967fae22b9a84cf705d59b29e15b8ee2311c984f050ce85b2f8c1ce7`.

Homebrew and Ruby are reused from the existing prefix. `tools/runtime-pack`
creates the reviewed source/file inventory and packages only the attestation
helper and inventory. Before collecting evidence, the adapter checks and copies
inventoried Homebrew files into an empty private inspection prefix. Unexpected
versions or changed required files hold. No Homebrew bootstrap or update occurs.
The empty prefix prevents an installed old SBOM from selecting the wrong version
for candidate vulnerability scanning. Execution uses `/opt/homebrew/bin/brew`.

The private HOME, cache, config, logs and temporary directory do not inherit user
credentials or Homebrew settings. Collection cannot write to the installed prefix.
Execution permits prefix writes but denies network and changes to Homebrew's
implementation or the frozen inputs. This is an execution boundary, not a claim
that older signed packages cannot contain malicious code.

## Candidate evidence

- `brew info --json=v2 --formula homebrew/core/<name>` acquires and verifies signed
  API metadata. The first snapshot is retained; subsequent dependency queries use
  that immutable snapshot offline. Every selected runtime dependency is required.
  Build-only recipe dependencies are not downloaded or evaluated.
- `brew fetch --formula --bottle-tag=arm64_tahoe` and `brew --cache` fetch and locate
  the selected bottles, including architecture-independent `all` bottles.
  Bottles may be relocatable or require the exact `/opt/homebrew/Cellar` used for
  execution; another fixed prefix is unsupported. Actual file hashes must match authenticated metadata.
  Archive paths, entry types, permissions, links and sizes are bounded before
  Homebrew extracts anything. Verified regular defaults under `.bottle/etc` and `.bottle/var` may be restored
  by Homebrew inside its prefix; shared-prefix symlinks remain unsupported.
- The OCI index cached by public fetch must identify the exact digest and bottle
  tag. Its complete runtime dependency names must match the selected signed API
  closure. Historical dependency versions are lower bounds; Homebrew owns version
  ordering and installation compatibility. Index bytes are frozen with the cache.
- The [attestation adapter](../attestation/README.md) verifies the exact bottle
  subject, expected Homebrew publisher and workflow using a maintained verifier.
- `brew vulns --json` scans explicit candidates in the empty prefix. Missing,
  skipped, untrusted or unsupported subjects hold. A clean supported scan means
  no known applicable findings. No historical advisory witness is required.
- Homebrew 7.0.4's scanner does not incorporate the Homebrew Advisory Database.
  The official advisory feed and per-formula API therefore supply that component.
  Candidate identity must match; Homebrew's API owns applicability comparisons.
  Responses are attributed HTTPS observations, not signed metadata. The compressed
  full feed, formula result and scanner result are retained by digest.
- Bottle age follows official recipe history at the authenticated tap commit,
  locating the selected digest's registration. Changed bottle bytes receive a new
  age even when the version is unchanged. A bounded older matching snapshot can
  establish a conservative lower bound on age; it is not an upload timestamp.

The signed cache location `cache/api/internal/packages.arm64_tahoe.jws.json`, OCI
annotation fields and cache naming are version-specific data contracts. They are
not private Ruby calls, nor are they presumed stable across Homebrew updates.
Review and retest them before expanding the supported inventory.

## Execution and recovery

All checks finish before any package installation. A short-lived plan binds the
policy, exact candidate artifacts, complete dependency graph, frozen inputs,
observed selected installed state and one attempt. Revalidation checks those
inputs and state again. Commands run in dependency order with `--force-bottle`;
new non-root dependencies also use `--as-dependency`. Auto-update, cleanup,
autoremove, additional confirmation and opportunistic installed-dependent
maintenance are disabled. A missing bottle cannot be replaced through a network
fetch or source download during installation.

Existing selected kegs must have supported public installed metadata, an active
opt link, no custom options and recorded bottle installation. Their receipts and
versions are observed; these flags do not prove existing payload hashes. The
user-owned installed state is trusted under the threat model. Newly introduced
artifacts are the verified cached bottles. Unrelated installed packages are not
scanned or scheduled for maintenance, except when explicitly selected by an
all-installed upgrade request.

The user must not concurrently modify the same Homebrew prefix outside
BrewWarden. A private lock coordinates BrewWarden itself; no monitoring or private
Homebrew locks are added to enforce that usage condition. A fixed startup gate
records each owned POSIX session before allowing its command to execute. Normal
cancellation interrupts Homebrew, which manages its children. Completion and
recovery confirm that the entire recorded session is absent, including children
Homebrew places in separate process groups. Recovery also requires the operation
lock; PID reuse conservatively holds rather than signalling
an unrelated process. It inventories selected racks and opt links without
following symlinks or requiring complete receipts. Reconciliation records facts,
not success, rollback or authorization to replay. Legacy internal-API attempts
require the matching older build to reconcile their different process protocol.

## Validation

Unit tests cover strict metadata, OCI digest/closure mismatch, archive extraction,
input substitution, private operation locks, process-session liveness and partial
state snapshots. Explicit VM tests exercise public collection, actual install and
upgrade, multiple targets, unchanged reruns, unrelated-package preservation,
age exceptions, tampering, cancellation, partial failure and recovery. See
[verification](../../../docs/verification.md) for repeatable entrypoints.

Validated attestation bundles and bottle registration facts are cached privately
by artifact identity. Cache hits still undergo signature verification and recipe
binding; advisory status is collected afresh. HTTP failures identify the provider
and status, with a retry time when GitHub supplies its rate-limit reset.
