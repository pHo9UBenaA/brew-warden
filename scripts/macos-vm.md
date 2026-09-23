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

## Provision the guest

Use `tart exec` through the guest agent. Before prefix changes, check `sw_vers`,
`uname -m`, `sysctl -n hw.model`, the user and the agent's executable dependencies.
The tested image reports macOS 26.6.2, arm64, VirtualMac2,1 and user admin.

Inside the disposable guest only, preserve its original Homebrew prefix as
`/opt/brewwarden-original-homebrew`. Populate `/opt/homebrew` from `git archive`
of the reviewed Homebrew revision and extract its pinned portable Ruby archive
under `Library/Homebrew/vendor`, with the matching `portable-ruby/current` link.
The Homebrew adapter owns the installed-runtime fingerprint and supported
revision; VM provisioning must use the same reviewed source and Ruby bytes.
Do not run this provisioning against a host prefix. The running guest agent
survives the move, but its launch configuration still refers to the old path;
recreate a clone for independent experiments instead of rebooting this modified
clone and assuming the agent will restart.

Transfer explicit archives with `tart exec -i ... tar -xf -`. Do not mount host
folders. Product test inputs are a private workspace, installed supported gh
and an arm64 test binary built from `./tests`. The distribution contains no
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
