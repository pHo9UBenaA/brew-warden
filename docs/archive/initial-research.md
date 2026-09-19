# Homebrew security tooling research

Research date: 2026-09-19. This is the initial investigation, retained as evidence.
The subsequent product decision is an all-in-one tool; overlap with existing OSS
is acceptable. [Product design](../design.md) supersedes the earlier preference for a
narrow audit-only starting product. [Language evaluation](language-evaluation.md) covers language.

Public documentation and source at fixed commits were inspected. No competing
tool was installed, no live upgrade was performed, and no security vulnerability
was demonstrated. Source observations are not confirmed exploit reports.

## Existing capabilities

| Project | Observed or documented capabilities |
| --- | --- |
| Homebrew | SHA-256, signed JSON metadata, opt-in bottle provenance, brew vulns |
| sharkyger/homebrew-safe-upgrade | Pre-install/upgrade checks, age policy, dependency checks, comparative findings, temporary pins, guarded self-upgrade |
| ahokinson/cold-brew | Age holds, per-package policy, CVSS/KEV/EPSS exceptions, intermediate-version upgrades, heuristic change inspection |
| damovsky/brew-upgrade-grace-period | Estimate age from commit history and hold upgrades |
| Workbrew | Vendor-described enterprise management and pre-command policy checks |
| EnterpriseBrew | Vendor-described allow/deny, minimum-version, and vulnerability policy |

Enterprise features are vendor claims, not independently verified enforcement,
customer counts, or evidence of willingness to pay. Search also surfaced
homegrew/grew lockfile/hash features, but its repository content was unavailable
in this investigation; it remains an unassessed related project.

## Source observations

1. safe-upgrade's current code compares old/new findings before waiving freshness;
   its simplified README description understates that implementation.
2. Its age cache key contains type/name/version, not the bottle digest. A
   same-key artifact change deserves a regression experiment; actual bypass
   feasibility was not established.
3. Git author/committer dates are configurable by the author. Reading them from
   GitHub does not make them independently certified publication times.
4. The inspected Homebrew installer has different attestation paths for local
   bottles, gh bootstrapping, and cache presence. An enabled environment variable
   is not by itself a per-artifact verification receipt. No cache exploit was
   reproduced or inferred as proven.
5. Core attestation checks constrain the repository but do not universally pin
   one workflow, because multiple workflows produce bottles. A stricter identity
   policy adds maintenance obligations for legitimate changes.
6. cold-brew's inspected provenance module examines commits/authors/code patterns;
   that heuristic evidence is distinct from cryptographic artifact provenance.

## Design implications retained

Record artifact identity, metadata identity, and dependency identity separately.
Keep raw evidence digests, verifier versions, observation dates, policy, and exact
exceptions. Distinguish verified, failed, unavailable, and unsupported. An API
error must not be described as a revoked signature.

Digest-specific observation age avoids trusting arbitrary old metadata dates,
but creates first-run friction, depends on observation frequency and the local
clock, and cannot establish that a release is benign. Rebuilds can be legitimate.

A wrapper cannot assume that filtering command-line package names constrains
Homebrew's eventual dependency resolution. Auto-update suppression and a final
check are not sufficient proof of immutable execution. Test exact artifact/plan
binding before publishing enforcement claims; post-install detection is too late
to prevent install-time execution.

Upstream signatures require key lifecycle management and subject mapping. Casks
add changing URLs, no_check, installers, and self-updates. Vulnerability matching
must account for package identity, affected ranges, data gaps, and patched brew
revisions. Avoid vague AI safety scores or unsupported safe/unsafe labels.

## Evaluation proposal, not completed work

Evaluate representative formulae over several weeks, measuring evidence coverage,
initial/repeated runtime, request count, legitimate rebuild warnings, exception
frequency, and whether explanations support a decision. The earlier proposal was
20-30 formulae over 2-4 weeks; it was never run. No schedule was created.

Useful tests include same-version digest changes, unexpected signing identities,
API failure versus bad signatures, invalid/backdated timestamps, harmless metadata
changes versus dependency changes, and unknown evidence in new dependencies.

Commercial viability, market size, and detection effectiveness are unknown.
Competition does not prevent proceeding with the agreed all-in-one design.

## Primary sources and inspected revisions

- [safe-upgrade implementation](https://github.com/sharkyger/homebrew-safe-upgrade/blob/b41389eff2be6ed79be71e89de94eba291261f51/brew-safe-upgrade): age cache, freshness exceptions, dependency and pin handling.
- [Git commit timestamps](https://git-scm.com/docs/git-commit#_commit_information).
- [Homebrew installer](https://github.com/Homebrew/brew/blob/2f1c682db046d37c4b6c09aa43837be6ff270c39/Library/Homebrew/formula_installer.rb).
- [Homebrew attestation](https://github.com/Homebrew/brew/blob/2f1c682db046d37c4b6c09aa43837be6ff270c39/Library/Homebrew/attestation.rb).
- [Homebrew manual](https://github.com/Homebrew/brew/blob/2f1c682db046d37c4b6c09aa43837be6ff270c39/docs/Manpage.md).
- [cold-brew provenance](https://github.com/ahokinson/cold-brew/blob/d5344047412b08f726feab335fc9219efb6a9cb6/src/brew/provenance.ts), [policy](https://github.com/ahokinson/cold-brew/blob/d5344047412b08f726feab335fc9219efb6a9cb6/src/brew/policy.ts), [stepping](https://github.com/ahokinson/cold-brew/blob/d5344047412b08f726feab335fc9219efb6a9cb6/src/brew/stepping.ts).
- [grace-period implementation](https://github.com/damovsky/brew-upgrade-grace-period/blob/49e39e3849f507de21c5357dfe5af99170f88ed6/brew-safe-upgrade).
- [Homebrew security model](https://docs.brew.sh/Homebrew-Security-and-Supply-Chain), [minimum-age request](https://github.com/Homebrew/brew/issues/21421).
- [Sigstore verification example](https://blog.sigstore.dev/cosign-verify-bundles/): historical 2024 instructions, not a current command compatibility test.
- [Workbrew](https://workbrew.com/blog/how-workbrew-works), [EnterpriseBrew](https://enterprisebrew.com/compare/managed-homebrew): vendor statements.

Fixed revisions were obtained through the GitHub API. Search caches exposed
older source as well, so code claims use the fixed revisions above.
