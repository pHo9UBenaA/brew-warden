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

## Guest preparation and evidence

Use `tart exec` through the image's guest agent. Before any prefix changes,
check `sw_vers`, `uname -m`, `sysctl -n hw.model`, the user, existing prefix and
agent dependencies. The tested image reports macOS 26.6.2 (25G83), arm64,
VirtualMac2,1, user `admin`, and guest-agent 0.14.1. It includes Homebrew.

Inside this disposable guest only, preserve the original prefix as
`/opt/brewwarden-original-homebrew` and make `/opt` writable to the guest admin.
The running guest agent has only system-library dependencies and remains usable
after this move. Do not reboot this modified clone: its launch configuration
still points into the original prefix. Recreate a clone from the stopped base
for a new independent experiment instead of relying on this altered boot state.

Transfer an explicit tar archive through `tart exec -i ... tar -xf -`; do not
mount host directories. Include a shallow bare copy of the pinned Homebrew
source, the probe scripts, signed API metadata, official bottle inputs and the
verifier. The bare source supports `git archive` without modifying host Homebrew.
The arm64 gh 2.62.0 release ZIP has SHA-256
`fdb77f31b8a6dd23c3fd858758d692a45f7fc76383e37d475bdcae038df92afc`,
matching the official release checksum file. Its acquisition uses public HTTPS;
this check alone does not establish an independent publisher signature.
Use the native arm64 portable Ruby already pinned in the probe.

Run the existing six-argument `jq --vm-prefix` probe in the guest. The driver
refuses an existing `/opt/homebrew` and freezes authenticated inputs before any
installation. Copy the complete emitted evidence directory back through
`tart exec ... tar -cf -` into a unique host cache directory. Retain failures as
well as successes. Stop the clone when no test is running; do not delete the
base, unrelated VMs or evidence as part of verification.

The first successful standard-prefix run is recorded in the owning
[Homebrew capability record](homebrew-probe.md#standard-prefix-macos-vm-acceptance).
It does not establish upgrade, concurrency, vulnerability or product readiness.
