# Bottle provenance boundary

Capability: `homebrew.core.provenance.v1`. Cryptographic verification is delegated
to the maintained GitHub CLI v2.101.0 attestation command, built with an isolated
entrypoint by `scripts/build-verifier.sh`. The entrypoint changes no upstream
verification code. It supplies bounded HTTP clients and process IO, and exposes
only the attestation verifier. It does not install gh or use Homebrew to bootstrap
its own trusted helper. Distribution wiring remains separate from this adapter.

## Guarantee and checks

Homebrew's native attestation path depends on gh, can bootstrap helpers and has
paths which skip unavailable bottles. The adapter verifies each actual candidate
bottle explicitly. It passes a local bundle, `--repo Homebrew/homebrew-core`,
anchored `--cert-identity-regex` for the inspected publish-commit-bottles and
dispatch-build-bottle workflows on `refs/heads/main`, the GitHub Actions OIDC
issuer, `--deny-self-hosted-runners`, the SLSA v1 predicate and JSON output.
Upstream owns Sigstore, certificate, transparency-log and TUF verification.
The [upstream command](https://github.com/cli/cli/tree/v2.101.0/pkg/cmd/attestation/verify)
and verification library are retained unchanged. Homebrew itself is trusted by
the threat model; this does not claim safety against a compromised Homebrew build.

BrewWarden additionally validates the distribution-selected verifier hash, actual
bottle digest, exact filename/platform/revision/rebuild, verified certificate
identity, repository, issuer, runner environment, statement type and exact subject
digest. Exit zero or empty output cannot establish provenance. Inputs are rehashed
after verification, and a missing or changed input fails. This is evidence
collection, not an execution lock; the native session must retain frozen bytes.

The helper gets an empty credential environment, system-only PATH, private HOME,
no inherited proxy or GH tokens, and a 90-second total deadline. Input files must
be regular, not symlinks. Bundles/output are bounded to 8 MiB, bottles to 2 GiB and
the helper to 128 MiB. Exact raw verifier output is returned with its digest and
a one-hour observation lifetime. The application must retain it before execution.
Invalid signatures, unavailable TUF services, unsupported output and input changes
all fail. No emergency override changes this boundary. TUF refresh requires network;
local bundles do not imply fully offline verification.

## Dependency and build inventory

The Go wrapper still has no third-party module imports. The verifier is an explicit
runtime dependency, not a zero-dependency claim. Its build starts with module
`github.com/cli/cli/v2@v2.101.0`, checksum
`h1:zJ+YxyhomQ9PHDjB5wwv4tCFnjq6vEbfI0pRdOsJ0xo=`, and the repository's pinned Go.
The upstream module graph contains 465 entries including the main module; the
arm64 linked build metadata identifies 138 dependency modules. Build evidence
retains both inventories and all module checksums. This includes maintained
Sigstore, TUF and in-toto implementations and upstream CLI transitive dependencies.
The source template is reviewed and compiled by this separate build, not included
in the standard-library-only application graph. It is never downloaded by a hook,
check or product invocation. Updating the upstream pin requires this full review.

On 2026-09-20, two forced darwin/arm64 builds were byte-identical, SHA-256
`d0813b4f0e4c992036780d491e814389f8b549af5cf996406b027e946707b817`.
Native links are system libSystem, libresolv, CoreFoundation and Security only.
The binary vulnerability scan reported no affected symbols or imported packages.
The full module graph includes GO-2026-5932 in unused OpenPGP code; the attestation
entrypoint does not link/call that package. The full gh release did report that
finding at package level, which is why its unrelated command registry is excluded.
A clean scan is not a general security proof. Release licensing/notice packaging
and signatures must cover this dependency before distribution.

## Tests

Subprocess tests check credential isolation, nonzero exits despite convincing
output, empty success, changed inputs, cancellation, helper substitution and
bounds. Parser tests reject wrong signers, subjects, platforms, predicates,
digests, malformed or ambiguous JSON. The explicit live test accepts a prebuilt
helper path in `BREWWARDEN_LIVE_VERIFIER` and verifies the actual jq bottle and
bundle in the isolated probe cache. The built helper also rejected a wrong
workflow identity against the same official bundle. No host Homebrew was modified.
