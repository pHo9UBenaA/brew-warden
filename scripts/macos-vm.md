# Disposable macOS verification

Linux containers exercise Go and filesystem behavior but cannot establish macOS
Homebrew bottle behavior. Use a disposable Apple Virtualization VM for standard
prefix acceptance. This is development infrastructure, not a product dependency.
Never share the host Homebrew prefix or credentials with a guest.

## Inspected tools and image

The tested [Tart release](https://github.com/openai/tart/releases/tag/2.37.0) is
2.37.0. Download its macOS archive into the workspace cache, outside host package
managers. Its SHA-256 is
`d531752c4dad5d4214ac7ff540cefc2647df1fca2338d413d3c01754f54b356b`.
The downloaded release asset digest agrees; `codesign --verify --deep --strict`
passes with Cirrus Labs Developer ID team `9M2P8L4D89`. No signature bypass or
re-signing is used. The bundled license is FSL-1.1-ALv2; review it for the intended
use. The formerly Cirrus-hosted repository redirects to the linked repository.

The tested third-party base image is pinned to
`ghcr.io/cirruslabs/macos-tahoe-base@sha256:1b093499716409d29e8b5336844528e1cae375db97d2ad8e5aeff78cf0da201e`.
Its fetched manifest bytes match that digest. This is an explicit development
image trust assumption, not an Apple publisher signature or a product trust root.
The download is approximately 27.3 GB compressed; the virtual disk is 50 GB.
Allow additional space for extraction, copy-on-write changes and evidence.

For every Tart invocation set an empty environment with a dedicated `HOME` and
`TART_HOME` under the workspace cache, `TART_NO_AUTO_PRUNE=1`, and system-only
`PATH=/usr/bin:/bin`. This keeps credentials, state and pruning separate from any
user-managed Tart installation. Keep the downloaded base stopped; clone it for
each independent test. Configure the clone with 4 CPUs and 4096 MB memory.
Run it with `--no-graphics --no-audio --no-clipboard`, without directory sharing
or attached host disks. Default NAT permits evidence acquisition; the probe's
own sandbox separately denies network access during native installation.

## Local-only acceptance runner

The optional VM cases in `tests/vm/` have the `vmacceptance` build tag and are **not**
part of `go test ./...` or GitHub Actions. `scripts/macos-vm-acceptance.sh` drives
Tart and compiles those tests only when invoked locally. It does not use
Playwright or a browser automation dependency: native subprocesses, installed
Homebrew, package files and crash behavior are the boundaries under test.

From a committed, verified tree, build the distribution, then explicitly supply
a reviewed Homebrew tree, the installed-compatible arm64 gh executable and the
archive to a fresh VM clone:

```sh
export PATH="$PWD/.cache/sdk/go/bin:$PATH" # Or another trusted Go matching .go-version.
./scripts/build-product.sh darwin/arm64
./scripts/macos-vm-acceptance.sh prepare brewwarden-tahoe-base brewwarden-local-01 \
  "$REVIEWED_HOMEBREW_TREE" "$GH_ARM64" \
  "$PWD/.cache/product-build.EXAMPLE/first/brewwarden-darwin-arm64.tar.gz"
./scripts/macos-vm-acceptance.sh auth brewwarden-local-01
# Approve the displayed short-lived device code in a browser; never paste a token.
./scripts/macos-vm-acceptance.sh suite brewwarden-local-01
./scripts/macos-vm-acceptance.sh finish brewwarden-local-01
```

Replace `product-build.EXAMPLE` with the emitted build-evidence directory.
`suite` invokes the selected cases and their explicit guest-only fixtures in a
fixed order, stopping on the first failure without inventing success. For
focused investigation, select `run public TARGETS [FAULT]`, `run native [FAULT]`,
`run survey TARGETS`, `run crash`, `run general TARGETS`, `run upgrade TARGET`
and `fixture absent-jq|absent-xz|older-xz|repair-jq` separately. Fixtures modify
**only that disposable guest**; `absent-*` deliberately uninstalls all versions
of the named fixture formulae. Do not run unrelated guest mutations concurrently.
`prepare` verifies VirtualMac arm64 before replacing the guest prefix. It
transfers an explicitly supplied reviewed Homebrew release checkout (including
its Git metadata, if present) only into the disposable guest, then runs the
packaged `doctor` against that supported release source. A tar-only 7.0.4
fixture remains bound to the legacy complete-runtime fingerprint. If
preparation fails after a successful clone, the runner attempts guest credential
removal and stops only that newly created clone; it retains private diagnostics
and reports any cleanup it cannot confirm. A failed `doctor` never counts as
success. `auth` starts the standard gh device flow inside a private guest HOME; approval
is human-mediated. Standard gh device login asks for `repo`, `read:org`, and
`gist` scopes. `finish` removes guest credentials and stops the VM; also revoke
the temporary GitHub CLI OAuth authorization from the account afterward.
Evidence under ignored `.cache/` is disposable, not the sole record of results.
Never upload VM output containing credentials or copy host credentials into it.

## Local product-readiness gate

Use the two-phase local `scripts/product-ready.sh` to reproduce the full release
readiness checks from a **clean, committed** tree. It requires pinned Go and the
explicit reviewed guest inputs above; tools are never installed implicitly:

```sh
export PATH="$PWD/.cache/sdk/go/bin:$PATH"
./scripts/product-ready.sh start brewwarden-tahoe-base brewwarden-ready-01 \
  "$REVIEWED_HOMEBREW_TREE" "$GH_ARM64"
# Human: approve the guest-only short-lived GitHub device code.
./scripts/product-ready.sh complete brewwarden-ready-01
# If approval expires, request a fresh code inside this guest:
# ./scripts/macos-vm-acceptance.sh auth brewwarden-ready-01
# Or fail this gate and stop its pending guest:
# ./scripts/product-ready.sh cancel brewwarden-ready-01
```

`start` runs `check.sh all` (baseline unit/integration tests, race, coverage,
fuzz, Staticcheck and govulncheck), then vets, lints, scans and cross-compiles
the opt-in native tests. It repeats the committed `darwin/arm64` distribution
build, fingerprints its archive, provisions a fresh guest and requests human
device approval. `complete` refuses a changed source or archive, requires guest
authentication and runs the **entire** VM suite. It removes guest credentials and
stops the VM on success or failed execution. If approval is still pending,
`complete` refuses to run and keeps the gate pending; `cancel` records failure
and stops only that gate's VM. If `start` fails while requesting a device code,
it cleans up the newly prepared guest rather than leaving it running. Only after all checks, VM cases and
cleanup pass does it write `product-ready.txt` under a private, ignored
`.cache/product-ready.VM/` evidence directory and report **local product-ready
for the tested initial scope**. A failed run is not resumable as proof of
success; use a new VM name and fresh evidence. If the guest agent is unavailable
for cleanup, intervene manually before using the VM again. Revoke the temporary
GitHub CLI OAuth grant in account settings after acceptance. This gate does not
sign, notarize or publish a release, and never runs inside GitHub Actions.

## Provision the guest

Use `tart exec` through the guest agent. Before prefix changes, check `sw_vers`,
`uname -m`, `sysctl -n hw.model`, the user and the agent's executable dependencies.
The tested image reports macOS 26.6.2, arm64, VirtualMac2,1 and user admin.

Inside the disposable guest only, preserve its original Homebrew prefix as
`/opt/brewwarden-original-homebrew`. Populate `/opt/homebrew` from `git archive`
of the reviewed Homebrew revision and extract its pinned portable Ruby archive
under `Library/Homebrew/vendor`, with the matching `portable-ruby/current` link.
The Homebrew adapter owns the reviewed release commits, copied-runtime
identity and the legacy 7.0.4 full fingerprint; VM provisioning must use the
same reviewed source and the required installed arm64 portable Ruby bytes.
Do not run this provisioning against a host prefix. The running guest agent
survives the move, but its launch configuration still refers to the old path;
recreate a clone for independent experiments instead of rebooting this modified
clone and assuming the agent will restart.

Transfer explicit archives with `tart exec -i ... tar -xf -`. Do not mount host
folders. Product test inputs are a private workspace, installed supported gh
and an arm64 test binary built from `./tests/vm`. The distribution contains no
Homebrew or Ruby bytes; the guest installation must match the reviewed runtime
fingerprint.
For final acceptance, transfer the complete reproducible distribution archive.

Run the cases in [verification](../docs/verification.md), then verify archive
checksums and exercise packaged doctor, install, upgrade, parent-death,
active-child exclusion and fresh retry. Fixtures must be confined to the disposable guest. Copy logs
and observations back with `tart exec ... tar -cf -`, preserving failures as well
as successes. Stop the clone when finished; do not delete the base or unrelated
VMs. Historical probe code remains recoverable from Git history, not as a second
maintained Ruby execution path.
