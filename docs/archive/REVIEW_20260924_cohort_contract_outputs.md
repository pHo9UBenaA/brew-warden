# Captured compatibility-cohort contract evidence

Date: 2026-09-24. This follow-up responds to the distinction between a
bounded, fail-closed implementation and *individual* release testing. It
adds no new platform, privilege, Homebrew or gh installation. The host's
Homebrew and the host's gh authentication were not touched.

## Homebrew

Official upstream release commits from the preexisting local Git object store
were exercised through the installed-runtime copy/identity adapter in a
separate temporary shared clone. All 11 reviewed commits in `runtime.go`
materialized with distinct release identities; an extra untracked executable
source was rejected at every commit. This offline source-matrix test is
opt-in (`BREWWARDEN_REVIEWED_BREW_GIT`) because routine CI must not fetch an
entire upstream Git history or assume it is installed. It takes approximately
three minutes with the local source objects. The checks do not execute brew
or prove signed metadata for all eleven commits.

Native read-only adapter probes in fresh, disposable arm64 Tahoe guests passed
at **7.0.2**, **7.0.3** and **7.0.5** as well as the previously probed **6.0.19**,
**6.0.22** and **7.0.6**. The installed Homebrew command authenticated its
signed metadata index and resolved the actual jq/oniguruma dependency closure.
The adapter fetched and hashed both exact bottles, checked OCI dependency
evidence and rejected skipped scanner subjects. The added probes target the
7.0.1→7.0.2 API/info/keg, 7.0.2→7.0.3 vulnerability and 7.0.4→7.0.5 installer
source transitions without requiring a complete product VM suite per patch.
Actual bound install/upgrade remained represented by 6.0.19 and 7.0.6;
`cf86592` passed the authenticated 17-case distribution gate on 7.0.6.

`internal/adapters/homebrew/testdata/` records **unaltered public CLI JSON**
for jq info and scanner from 6.0.19, 7.0.2, 7.0.3, 7.0.5 and 7.0.6. Routine Go
tests parse each capture and reject omitted dependency inventories or an
omitted/skipped scanner subject. Some responses are identical: these are real
captured outputs, not a proof that an unsigned JSON object was signed or that
every intermediate installation was attempted. The separate live probes
established the signature/cache and execution boundaries for representative
cohorts.

## gh

Native arm64 gh binaries **2.66.0**, **2.70.0**, **2.74.0**, **2.80.0**,
**2.97.0**, **2.101.0** each cryptographically verified public signed bundles
for the *same* exact Tahoe jq bottle, SHA-256
`ca67c64d0aaf1e5472790ec2cc081ff7972316f27095d8a8aab81b3321247036`.
These samples cross sigstore-go 0.7, 1.0, 1.1, 1.2 and 1.3 output cohorts.
2.66.0 ran inside a disposable guest; the others ran as isolated arm64 binaries
with private temporary HOME/config and no host gh token. Each result contains
verified Homebrew signing identity, the exact subject, and two trusted Rekor
Tlog timestamps; the oldest is 2026-08-24T22:10:45Z. `2.70.0` and later
serialize timestamps with a local offset on this Mac; the parser normalizes
them correctly. The complete verified CLI JSON outputs are stored under
`internal/adapters/attestation/testdata/`. Routine Go tests
exercise *these actual outputs* and reject wrong subject, future time or an
untrusted transparency-log URI. Captured CLI output is **not** independent
cryptographic proof: real gh must verify again for every product operation.

The normal authenticated `gh attestation verify --repo ...` acquisition path
has only been covered in the final gate with gh 2.101.0. Offline verification
with `--bundle` for the other cohorts proves signature and output contracts,
not GitHub API availability/authentication for each version. Any API error,
missing result or changed output holds; do not silently promote an unavailable
old client to successful evidence. No new release was built, pushed or published
by this follow-up. A repository commit containing these tests is a *different
revision* from the `cf86592` native gate and does not inherit that gate's
product-ready status.
