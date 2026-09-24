# Supported product scope

This is the current user-facing compatibility and eligibility matrix. The
[product design](design.md) owns policy and execution rules; the
[Homebrew](../internal/adapters/homebrew/README.md) and
[attestation](../internal/adapters/attestation/README.md) adapters own upstream
contracts. Their source files contain the exact admitted release identities and
verifier version gate. A version banner alone never authorizes execution.

## Mutation compatibility

| Component | Supported scope | What holds |
| --- | --- | --- |
| Host | Native Apple Silicon arm64 macOS Tahoe (26.x) | Intel Macs, Rosetta/x86_64 Homebrew, Linux and other macOS releases |
| Homebrew | Installed at `/opt/homebrew`, at a reviewed official release commit from 6.0.19–6.0.22 or 7.0.0–7.0.6; executable and Library source clean at that commit | A different prefix, unreviewed Git commit, changed code or missing required installed portable Ruby |
| Legacy Homebrew fixture | Tar-only 7.0.4 with the complete reviewed runtime fingerprint | Any different tar-only tree; a version string or tag is insufficient |
| GitHub CLI | Already-installed `gh` 2.66.0–2.101.0, authenticated to github.com for online attestation checks | Missing login, prerelease, out-of-range or incompatible verified output |
| Packages | Official `homebrew/core` bottles for the selected native platform, including byte-identical `all` bottles when the verified subject matches | Casks, third-party taps, source builds or any incomplete required evidence |

BrewWarden bundles none of Homebrew, portable Ruby or `gh` and never installs or
updates them. It does not replace Homebrew or intercept a direct `brew` call.
The user's shell, including zsh, does not change platform or version support.
Do not move an unsupported Homebrew installation to `/opt/homebrew` or change
its implementation merely to bypass these checks.

Compatibility is **not** per-formula approval. Each requested operation must
verify authenticated metadata, the exact bottle and signer, trusted publication
time, vulnerability coverage and the complete dependency closure. An unavailable
or skipped check holds the entire operation, even if another check succeeds.
The default minimum verified bottle age is seven days; an explicit exception
can waive **only** the age of specifically identified verified bytes. A clean
advisory result does not prove that a package is harmless. `bwd doctor` checks
the environment, not whether a particular formula will pass every check.
See [usage](usage.md) for commands and failure handling.

The admitted bounds come from upstream source review, contract tests and
representative native execution, **not** a full VM run of every version or
combination. The lower and upper gh cohorts (2.66.0 and 2.101.0) passed
normal authenticated native acceptance; intermediate verifier cohorts have
signed-result and adapter contract coverage. New releases beyond these bounds
require review before admission, not an automatic assumption of compatibility.
[Verification](verification.md) describes repeatable checks and their limits.
Passing tests do not establish an absence of vulnerabilities or publisher
signing/notarization of the local distribution.
