# Threat model

Trust the OS, the running tool, selected trust roots, supported Homebrew and
verifier versions, and user-owned state. A same-user attacker who can replace
these components is outside the local wrapper's enforcement boundary.
Defending against a compromised Homebrew implementation is explicitly out of
scope. Trusting that implementation does not establish that every invocation
covered all required subjects or that preflight and execution use the same inputs.

| Input or boundary | Required handling |
| --- | --- |
| Artifact bytes and metadata | Bind a real digest to authenticated metadata and expected identity |
| API and verifier output | Bound size/time/depth, validate schemas, distinguish missing and failed |
| Package names and URLs | Validate identifiers; no command strings; constrain network destinations |
| Saved plans and exceptions | Revalidate contents, policy, evidence, expiry, environment, and identity |
| Filesystem state | Private paths, no traversal/symlink escape, atomic records, crash reconciliation |
| Terminal and logs | Escape control sequences, redact credentials, preserve useful failure evidence |
| Dependencies | Evaluate the complete execution closure; reject unsupported resolution paths |
| Toolchain and CI | Record versions, minimize dependencies, avoid implicit downloads and secrets |

An emergency exception applies only to age checks for exact artifacts in one
plan/attempt. It cannot repair a bad signature or substitute for missing evidence.
An API outage is not evidence that a signature was removed or a vulnerability was
fixed. A valid signature, old release, or empty advisory response is not proof of
harmless software.

Existing packages deliberately installed or changed outside BrewWarden remain
trusted user-owned state. Public receipt flags are observations, not proof of
installed payload hashes. Exact bottle integrity and provenance claims apply to
the acquired artifacts and BrewWarden's own new installation actions.

Only operations routed through BrewWarden are in scope. A proposed PATH shim
can be bypassed by direct Homebrew paths or a different environment; installing
the binary alone does not intercept commands.

Do not run an independent Homebrew mutation against the same prefix while a
BrewWarden operation is in progress. The wrapper does not monitor, intercept or
prevent operations the user deliberately runs outside it. A preflight recheck
does not establish exclusion against those operations. Do not add private API
hooks or installed-prefix locking solely to enforce this usage condition.
Product mutation execution remains unavailable until the relevant execution path
is demonstrated under this condition.
Post-install inspection cannot prevent code that already ran during installation.

Do not test updates in a maintainer's normal Homebrew prefix. Use fixtures and
isolated integration environments. No live exploits or automatic outbound
reports are part of the repository harness.
