# Initial product acceptance, 2026-09-20

This records the tested initial capability, not support for every Homebrew
formula or a public release. Host Homebrew was not changed. The supported product
path is official jq/oniguruma bottles on Apple Silicon macOS Tahoe at
`/opt/homebrew`; unknown required evidence and unsupported paths hold execution.

## Exact distribution

Source: `d82cc65e1b6c2f92794f271ffa11375442dd9e7f`.
Version: `0.1.0+d82cc65e1b6c2f92794f271ffa11375442dd9e7f`.

- Binary SHA-256:
  `97180f67261a9aef403d15d20459d3f2e72500bcb1468eb84c7f297dee95943a`.
- Complete archive SHA-256:
  `9a10e28f66169341c9ed1354441c040907f479b438adb9fe3c277ed1b0b45aa9`.
- Runtime manifest SHA-256:
  `9424050677790a1c88c66ab769c5167d59a874cd6a02074665268084c97e758b`.

Two forced builds from separate committed-source extractions matched exactly,
including the compressed runtime/license distribution. Every inner archive file
checksum passed after extraction in the disposable VM. The full-name executable
alias reported the expected version. No publisher signing, notarization, push,
tag or release publication was performed.

## Native acceptance

The disposable VirtualMac used macOS 26.6.2 arm64 and the normal Homebrew prefix.
The final archive upgraded jq 1.8.1 to 1.8.2 using the product CLI. Installed jq
reported 1.8.2. Complete oniguruma keg tar snapshots before and after the operation
were byte-identical. Both `status` and `history` reported the durable success. The final archive
also passed `doctor` runtime/platform validation.

Final product plan:
`9c2c14a5f0bbd147b816290e27274e9fbd5e3b220471f818d063812a249a7afb`.
Attempt:
`b8788d62c8237f191748cef3f19a5400ba792fd0084903e32f914898675ec95a`.

Verified candidates:

| Artifact | Version / rebuild | Bottle SHA-256 |
| --- | --- | --- |
| jq | 1.8.2 / 1 | `ca67c64d0aaf1e5472790ec2cc081ff7972316f27095d8a8aab81b3321247036` |
| oniguruma | 6.9.10 / 0 | `eb6bda3b333f497b5d294388f39fd0902a5c79a52ae16858eff711d2d104cc4d` |

The same real collector/session/application path additionally passed:

- Fresh installation of both packages, with actual payload reconstruction.
- Upgrade while preserving the already-installed dependency byte for byte.
- Explicit age-waiver upgrade; a too-young policy without waivers held unchanged.
- Changed bottle input held even with explicit age waivers.
- Unverified affected external dependent held before package mutation.
- Native lock contention against a separately launched Homebrew process.
- Recovery refused while the original session retained candidate locks; after
  stopping that owned session, it recorded observed state without changing kegs
  or claiming a child exit status or success.
- Real partial installation: oniguruma installed, then jq encountered a preexisting
  shared-prefix file. That file was preserved and the durable outcome was partial
  with a nonzero exit and retained actual state. No automatic rollback occurred.
- Modified, extra, missing and mode-changed installed payloads, modified SBOM
  package identity and missing receipt were rejected by native payload probes.
- Actual native source-build and unplanned-installer guards rejected those paths.

## Other verification and limits

The offline suite, race detector, coverage run, fuzz targets and staticcheck
passed. The isolated OrbStack Linux container passed baseline/race checks and
real full-filesystem journal checks with a read-only root, no network or host
mounts, and no capabilities. Linux container success is not native Mac support.

The full check's advisory request was initially blocked by the sandbox network;
the networked vulnerability step was then rerun successfully. Product source and
binaries had no reported affected vulnerabilities. The final attestation helper
had no affected symbols or imported packages. Its module inventory contains
`golang.org/x/crypto@v0.57.0`, associated with GO-2026-5932 in the unused OpenPGP
package; that package is not linked by the attestation-only entrypoint. This is
not a claim of zero unknown vulnerabilities or an audit of all native code.

Native links were inspected. Go product/helper use libSystem, libresolv,
CoreFoundation and Security. Portable Ruby uses CoreFoundation, libSystem,
libobjc and system libz, with OpenSSL 4.0.2 and libyaml 0.2.5 statically included.
Original license documents and linked-module notices are included in the archive.

Broader formula mappings, casks, external taps, source builds, other architectures
and operating systems remain unsupported rather than silently unchecked.
