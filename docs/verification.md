# Verification

Scripts are the source of truth; optional Task v3 aliases call the same scripts.
Use the Go version pinned in `.go-version`, Git, and a POSIX shell. Race detection
also requires a C compiler; it does not enable cgo in product code.

| Command | Checks or output | Network |
| --- | --- | --- |
| `./scripts/verify.sh` / `task verify` | Formatting, shell syntax, hygiene, architecture, dependency inventory, tidy diff, module cache integrity, vet, shuffled tests | None required |
| `./scripts/setup-tools.sh` / `task tools` | Install pinned Staticcheck and govulncheck under `.cache/tools` | Go proxy and checksum database |
| `./scripts/check.sh all` / `task check` | Baseline plus every check below, using pinned Go | Advisory database |
| `./scripts/check.sh race` / `task test-race` | Race detection, shuffled uncached tests | None required |
| `./scripts/check.sh cover` / `task test-cover` | Cross-package coverage report at `.cache/coverage.out`, including domain decisions exercised by integration tests | None required |
| `./scripts/check.sh fuzz` / `task fuzz` | Commit-message, strict configuration, attempt-record, publication-response, advisory-record and provenance-result and native-metadata parser fuzzing; `FUZZTIME` defaults to 10s per target | None required |
| `./scripts/check.sh lint` / `task lint` | Pinned Staticcheck default checks | None required |
| `./scripts/check.sh vuln` / `task vuln` | govulncheck on source/tests and freshly built checker plus both product binaries | Advisory database |
| `./scripts/check.sh build` / `task build` | Development binaries at `bin/repo-check`, `bin/bwd`, and `bin/brewwarden` | None required |
| `go build ./cmd/...` | Compile diagnostic-only product entrypoints | None required |
| `./scripts/build-product.sh darwin/arm64 RUNTIME VERIFIER_SOURCE NATIVE_NOTICES` | Two forced rebuilds from committed source; compare complete runtime/notice archives and binaries; see [distribution](distribution.md) | None required |
| `sh scripts/probe-container.sh [REVISION or --worktree]` | Linux arm64 baseline and race tests in a pinned disposable container | Image acquisition only; denied during tests |
| `sh scripts/probe-homebrew.sh /absolute/Homebrew/source` | Isolated macOS upstream probes; see [requirements and limits](../scripts/homebrew-probe.md) | Denied by sandbox |

## Boundaries

Tools are installed only by explicit setup, never by checks or hooks. Versions
live in `scripts/tool-versions.env`; execution checks installed module versions.
Tool dependencies live outside product `go.mod`. Missing tools, unavailable
advisories, or findings fail the relevant check, rather than silently skipping it.

Checks disable implicit Go downloads and workspace inheritance using
`GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`, `GOWORK=off`, and read-only module
mode. Build caches default to `.cache/go-build`. These settings are not a network
sandbox. `go mod tidy -diff` does not rewrite files; `go mod verify` checks cached
module integrity, not publisher identity. The product currently has no external
modules. The baseline remains compatible with the module's Go 1.24 floor; full
checks use the supported pinned toolchain.

[Architecture](architecture.md) defines import/dependency gates. Hygiene rejects
invalid UTF-8, control characters, trailing whitespace, missing final newlines,
and symlinks. CJK rejection is a translation guard, not proof of English prose.
Binary fixtures need a policy extension. Hook tests use isolated Git repositories
and substitute only the recursive verification invocation. Coverage percentages
do not include code exercised in those separately built subprocesses.

Fuzz seeds run in normal tests; active fuzzing exercises arbitrary commit text,
configuration, attempt records, publisher responses, template comments, and control-byte rejection. Preserve discovered regressions
as seed cases or reviewed corpus files under the test directories. Add fuzz targets for actual
product parsers as they appear. Coverage has no arbitrary percentage gate; race
and fuzz checks cover only exercised behavior. Do not treat a clean advisory
result as proof that code is safe.

## CI and maintenance

Linux and macOS run full checks on pull requests (the merge candidate), pushes
to main/master, manual dispatch, and weekly schedules. Actions are pinned to
commits and do not retain checkout credentials. Commit checks inspect actual PR
commits, not GitHub's generated merge message. Required branch checks must be
configured separately. See [Contributing](../CONTRIBUTING.md) for local hooks.

Review Go security releases and update `.go-version` and tool pins promptly;
rerun full checks and review tool transitive dependencies. Dependabot proposes
GitHub Action updates; it does not maintain the custom Go/tool pin files.
The [Go security guidance](https://go.dev/doc/security/best-practices) covers
vulnerability scans, supported toolchains, fuzzing, races, vet, and release notices.
Staticcheck is an additional analysis tool, not a Go security certification.

The baseline tests development CLI binaries against a tripwire `brew` executable
and checks refusal without a trusted runtime. Distribution builds additionally
wire the real execution engine. Their native acceptance tests are explicit and
separate from the baseline; see Native product acceptance below.

The distribution build checks repeatability of both binaries and complete native
runtime/license archives. Product installation enforcement and distribution
packaging are implemented for the supported scope. Public release signing and
notarization have not been performed. Go may add a linker ad-hoc signature on
macOS; this is not publisher authentication. Building does not publish, tag,
notarize, or establish provenance of the compiler.

## Disposable Linux container

`probe-container.sh` uses the platform-specific official Go image pinned in
`tests/container/Dockerfile`. It requires a running Docker engine; it does not
start one or change its global configuration. The default source is committed
`HEAD`; pass a Git revision to reproduce another commit, or `--worktree` to test
tracked files and non-ignored untracked files. The current container harness
is copied explicitly for pre-commit validation. Each run records its source archive,
base revision, harness hashes, local image ID, stdout, stderr and exit status under
a unique `.cache/container-probe.*` directory. Worktree runs also record status
and the tracked diff. Ignored caches and private files are not build inputs.

The container runs as UID/GID 10001, with a read-only root filesystem, no network,
no host mounts, no passed credentials, no Docker socket, no capabilities, and no
new privileges. CPU, memory, process and temporary-storage limits are explicit in
the driver. Its temporary filesystem permits execution because Go tests build and
execute child binaries there; the first no-exec run correctly failed this boundary.
Images/build cache are retained for reuse; containers are removed after each run.
This is an explicit developer command, never invoked by product code or hooks.

Verified on OrbStack's Linux arm64 engine: offline baseline and race tests pass.
The first Linux run exposed a real fixture collision between the `brewwarden`
binary path and `XDG_CONFIG_HOME/brewwarden`; the integration test now separates
executable and user-state directories. These results validate exercised Linux
code paths, not macOS Homebrew bottle compatibility or a Linux product release.

The container also provides a dedicated 64 KiB `/full` tmpfs, with no execution
permission or host mount. The journal test checks its size before exhausting it,
requires an actual `ENOSPC` failure, and verifies that no start was committed.
The host suite skips only this explicit full-filesystem test. Both host and
container suites kill a writer after a durable start and require reconciliation
before any new attempt. Application tests use the real journal to verify launch
ordering, replay rejection, stale evidence, cancellation, partial outcomes and
unknown outcome durability. They do not substitute for native Homebrew execution.

## Native product acceptance

Use `tests/native_execution_test.go` only inside a disposable VirtualMac with
`BREWWARDEN_VM_RUNTIME` pointing to the pinned bundle. The test checks the hardware
model before any prefix mutation. `BREWWARDEN_VM_OPERATION=upgrade` selects upgrade;
fixtures must provision an older target and the intended existing dependencies.
The default path checks actual execution, native lock exclusion, candidate payload
identity, durable outcomes and unchanged installed dependency bytes.

`BREWWARDEN_VM_FAULT` selects `age`, `age-exception`, `changed-input`,
`exception-changed-input`, `affected-dependent`, `recovery` or `link-conflict`.
Recovery attempts reconciliation while the native locks are held, terminates the
owned session, then requires a durable observed-state reconciliation with no keg
changes. Link conflict requires absent jq/oniguruma and exercises a real partial
installation while preserving an existing shared-prefix file. These tests never
silently prepare or reset the host Homebrew installation.

The packaged CLI must also pass `doctor`, a real install/upgrade, `history` and
`status` in that VM. Offline tests, Docker tests and payload-only probes cannot
substitute for this native product-path acceptance. Signing/notarization and
public release provenance are not implied by local test success.
