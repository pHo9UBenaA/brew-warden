# Verification

Scripts are the source of truth; optional Task v3 aliases call the same scripts.
Use the Go version pinned in `.go-version`, Git and a POSIX shell. Race checks
also require a C compiler; product code does not enable cgo. See the
[supported scope](support.md): Linux and x86_64 development tests do not prove
native macOS bottle compatibility.

| Command | Checks or output | Network |
| --- | --- | --- |
| `./scripts/verify.sh` / `task verify` | Formatting, shell syntax, hygiene, architecture, dependency inventory, tidy diff, module cache integrity, vet, shuffled tests | None required |
| `./scripts/setup-tools.sh` / `task tools` | Install pinned Staticcheck and govulncheck under `.cache/tools` | Go proxy and checksum database |
| `./scripts/check.sh all` / `task check` | Baseline plus every check below, using pinned Go | Advisory database |
| `./scripts/check.sh race` / `task test-race` | Race detection, shuffled uncached tests | None required |
| `./scripts/check.sh cover` / `task test-cover` | Cross-package coverage at `.cache/coverage.out` | None required |
| `./scripts/check.sh fuzz` / `task fuzz` | Active named-target fuzzing, refusing missing targets; `FUZZTIME` defaults to 10s per target | None required |
| `./scripts/check.sh lint` / `task lint` | Pinned Staticcheck default checks | None required |
| `./scripts/check.sh vuln` / `task vuln` | govulncheck on source, tests, checker and product binaries | Advisory database |
| `./scripts/check.sh build` / `task build` | Diagnostic-only development binaries in `bin/` | None required |
| `./scripts/build-product.sh darwin/arm64` | Two forced builds from committed source; compare complete archives and binaries | None required |
| `./scripts/product-ready.sh start BASE VM BREW_TREE GH` then `complete VM` | Revision-bound full checks, repeat build, authenticated native suite and guest credential cleanup | Advisory, bottle and attestation sources |
| `sh scripts/probe-container.sh [REVISION or --worktree]` | Linux arm64 baseline and race tests in a pinned disposable container | Image acquisition only; denied during tests |

## Check boundaries

Checks never implicitly install tools. Explicit setup pins their versions in
`scripts/tool-versions.env`; missing tools, unavailable advisory data or
findings fail the relevant check. Go checks use `GOTOOLCHAIN=local`,
`GOPROXY=off`, `GOSUMDB=off`, `GOWORK=off` and read-only module mode to disable
implicit downloads and workspace inheritance. These are not a network sandbox.
`go mod verify` checks cached module integrity, not publisher identity; the
product currently has no external Go modules. The module's Go 1.24 directive
is a compatibility floor, while full checks use the pinned toolchain.

[Architecture](architecture.md) defines import gates. Hygiene rejects invalid
UTF-8, control characters, trailing whitespace, missing final newlines and
symlinks. CJK rejection is a translation guard, not proof of English prose;
binary fixtures need a policy extension. Hook tests use isolated Git repositories
and replace only the recursive verification invocation. Normal coverage does
not instrument separately built subprocesses or opt-in native cases. Fuzz seeds
run in ordinary tests; active fuzzing covers only exercised properties. Preserve
discovered regressions as reviewed corpus cases. A successful test or empty
advisory response cannot prove absence of vulnerabilities.

## Test ownership and cohort contracts

Tests follow their exercised boundary, not a claim of exclusive coverage:

- `internal/<layer>/*_test.go` and `tools/<tool>/*_test.go` exercise their owning
  package. Explicit live Homebrew probes remain next to the adapter and must
  use a disposable guest, never the host prefix.
- `tests/*_test.go` covers public policy, application and CLI contracts across
  layers. Its shared fixtures include bound-execution tests.
- `tests/vm/*_test.go` uses the `vmacceptance` tag and real packaged product in
  a disposable native VM. Normal `go test ./...` and CI neither compile nor run
  these cases.

Ordinary adapter tests parse captured upstream output from representative
Homebrew and gh cohorts, including negative evidence changes. A captured JSON
result is a parser fixture, not cryptographic or signed-metadata proof. The
optional offline source-matrix test copies each reviewed Homebrew release
commit from an explicitly supplied upstream checkout into a temporary clone;
it does not run brew or require a VM:

```sh
BREWWARDEN_REVIEWED_BREW_GIT=/path/to/offline/brew-checkout \
  go test ./internal/adapters/homebrew -run '^TestReviewedReleaseGitSourceMatrix$' -count=1
```

Source inspection, unit/property contracts and representative native execution
complement each other. They do not require a full VM run for every upstream
patch release or every compatible tool combination.

## CI, distribution and native acceptance

Linux and macOS CI run full checks on pull requests, pushes to main/master,
manual dispatch and weekly schedules. Actions are pinned to commits and do not
retain checkout credentials; commit checks inspect actual PR commits. Required
branch checks must be configured separately. See [contributing](../CONTRIBUTING.md)
for local hooks. Maintain `.go-version` and tool pins as supported patched Go
releases change. Staticcheck and govulncheck are analyses, not certifications.

The baseline uses a tripwire `brew` to check refusal of mutations in
unmarked development builds. The [distribution build](distribution.md) wires
the product engine and checks both binaries plus complete license/documentation
archives for reproducibility. Only a clean committed revision can pass the
[two-phase local readiness gate](../scripts/macos-vm.md#local-product-readiness-gate).
It adds tagged-test vet/lint/vulnerability/compilation checks and an
**authenticated, disposable Apple Silicon Tahoe VM suite** for installation,
upgrade, unchanged dependencies, age exceptions, changed inputs, partial
outcomes, parent death and fresh retry. A survey completion is not a successful
installation: inspect held reasons. [The VM runner](../scripts/macos-vm.md)
owns exact modes, guest-only fixtures and cleanup instructions.

Acceptance requires a valid guest gh login; GitHub quota or unavailable
attestation evidence holds. A finished native gate applies only to its tested
source revision and inputs. Ordinary CI or prior revision results cannot grant
local product readiness. The guest must remove credentials and stop; neither
this gate nor a reproducible local archive signs, notarizes or publishes a
release. macOS may add an ad-hoc linker signature, not publisher authentication.
No host Homebrew installation or host credentials are used by the VM harness.

## Disposable Linux container

`probe-container.sh` uses a platform-specific official Go image pinned in
`tests/container/Dockerfile`. It needs an already-running Docker engine; it does
not start one or change global configuration. By default it copies committed
`HEAD`; supply another revision or `--worktree` to test tracked and non-ignored
untracked files. Every run records its source archive, base revision, harness
hashes, local image ID, stdout, stderr and exit status in `.cache/container-probe.*`.
Worktree runs also record status and tracked diffs. Private ignored caches are
never build inputs.

The container uses UID/GID 10001, a read-only root, no network, host mounts,
Docker socket, capabilities or new privileges. It has explicit CPU, memory,
process and temporary-storage limits. A temporary filesystem permits running
Go test binaries; a dedicated 64 KiB `/full` tmpfs exercises full-disk failure
without touching a host mount. Images/build cache remain reusable and run
containers are removed. This is an explicit developer check, not product Linux
support or native Homebrew acceptance.
