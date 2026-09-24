# Homebrew boundary

The adapter uses public Homebrew commands. It does not load a Ruby bridge, call
private installer APIs or reconstruct installed payloads with Homebrew internals.

## Supported runtime

The supported source cohort comprises the inspected official Homebrew release
commits **6.0.19 through 7.0.6** listed in `runtime.go`, with an installed
native Apple Silicon arm64 Tahoe prefix at `/opt/homebrew`. Git HEAD must be a
reviewed release; installed executable/Library source must be clean at that
revision. A printable version or a forged local tag does not select a cohort.
The existing tar-only 7.0.4 native fixture remains accepted only with its exact
installed `bin/brew` and `Library/Homebrew` fingerprint (SHA-256
`e4422c589c12df8ac55953c8ddf25cdcfcabf05e6e27e141f9143e671dcd614c`).
Intel macOS, x86_64/Rosetta Homebrew on Apple Silicon, Linux, and unreviewed
Git commits are not supported execution targets. The full installed
executable/Library tree is still copied into a private inspection prefix and
bound to the one-use plan by a digest. Changed, extra executable source or
unsafe files hold. Its Git release identity and file bytes are checked again
before Homebrew is allowed to mutate the prefix. No runtime inventory or
Homebrew binaries are distributed. The installed portable Ruby is reused;
missing required Ruby cannot be installed implicitly under the immutable
inspection sandbox. The empty prefix prevents an installed old SBOM from
selecting the wrong version for candidate scanning.

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
- The [attestation adapter](../attestation/README.md) uses installed public gh
  to verify the exact bottle digest, Homebrew publisher and workflow.
- `brew vulns --json` scans explicit candidates in the empty prefix. Missing,
  skipped, untrusted or unsupported subjects hold. A clean supported scan means
  no known applicable findings. No historical advisory witness is required.
- The reviewed Homebrew scanner does not incorporate the Homebrew Advisory Database.
  The official advisory feed and per-formula API therefore supply that component.
  Candidate identity must match; Homebrew's API owns applicability comparisons.
  Responses are attributed HTTPS observations, not signed metadata. The compressed
  full feed, formula result and scanner result are retained by digest.
- Bottle age uses the chronologically earliest verified attestation timestamp
  for the selected digest. Missing timestamps hold, including under age waivers;
  changed bottle bytes require new age evidence even at the same version.

The signed cache location `cache/api/internal/packages.arm64_tahoe.jws.json`, OCI
annotation fields and cache naming are version-specific data contracts. They are
not private Ruby calls, nor are they presumed stable across Homebrew updates.
Review and retest them before expanding the supported inventory.

## Execution and interruption

All checks finish before any package installation. A short-lived plan binds the
policy, exact candidate artifacts, complete dependency graph, frozen inputs,
observed selected installed state and one attempt. Revalidation checks those
inputs and state again. Commands run in dependency order with `--force-bottle`;
new non-root dependencies also use `--as-dependency`. Auto-update, cleanup,
autoremove, additional confirmation and opportunistic installed-dependent
maintenance are disabled. A missing bottle cannot be replaced through a network
fetch or source download during installation.

Existing selected kegs must have supported public installed metadata, an active
opt link, no custom options and recorded bottle installation. Non-keg-only
formulae must also have Homebrew's matching `var/homebrew/linked` record:
Homebrew's public `brew link --dry-run` distinguishes this from a partial
pour that created an opt link but failed to link files into the prefix. This
version-specific record is read as a symlink, not through a private Ruby API.
Receipts and versions are observed; neither link proves existing payload hashes. The
user-owned installed state is trusted under the threat model. Newly introduced
artifacts are the verified cached bottles. Unrelated installed packages are not
scanned or scheduled for maintenance, except when explicitly selected by an
all-installed upgrade request.

The user must not concurrently modify the same Homebrew prefix outside
BrewWarden. A private lock coordinates BrewWarden itself; no monitoring or private
Homebrew locks are added to enforce that usage condition. A fixed startup gate
records each owned POSIX session before allowing its command to execute. Normal
cancellation interrupts Homebrew, which manages its children. A private,
durably synchronized in-flight record binds the process session and collection
to the pending attempt before the gate opens. Completion and fresh retry check
the whole session across process groups and user-ID changes. An active or
unobservable session holds new mutations; PID reuse conservatively holds rather
than signalling another process. Normal closure removes the workspace; after
parent death a new invocation removes stale state only once the child stops.
This never infers success, replays the plan or restores an exception. Older
attempt journals require the matching older build for recovery before mutation.

## Validation

Unit tests cover strict metadata, OCI digest/closure mismatch, archive extraction,
input substitution, private operation locks, process-session liveness and partial
execution outcomes. Isolated arm64 guest probes exercised signed metadata, exact
bottles and complete scanner subjects on 6.0.19, 6.0.22 and 7.0.6; actual bound
jq/dependency installation and unchanged rerun on 6.0.19, and jq installation
and xz upgrade on 7.0.6, used a guest-only verifier wrapper delegating to real
gh cryptographic verification of public bundles. They do not establish a
completed authenticated final-path product gate on the changed source revision.
The earlier 7.0.4 packaged VM suite covered age holds, tampering, cancellation,
partial failure and fresh retry; do not transfer its revision-bound readiness
result to this compatibility change. See [verification](../../../docs/verification.md)
for repeatable entrypoints.

The collector retains the verified gh response only within the pending workspace
and does not use attestation or registration caches. Advisory status is
collected afresh. No execution history, cached provider proof, or saved-plan reconciliation is
used.
