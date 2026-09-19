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
| `./scripts/check.sh cover` / `task test-cover` | Coverage report at `.cache/coverage.out` | None required |
| `./scripts/check.sh fuzz` / `task fuzz` | Commit-message parser fuzzing; `FUZZTIME` defaults to 10s | None required |
| `./scripts/check.sh lint` / `task lint` | Pinned Staticcheck default checks | None required |
| `./scripts/check.sh vuln` / `task vuln` | govulncheck on source/tests and a freshly built checker binary | Advisory database |
| `./scripts/check.sh build` / `task build` | Harness binary at `bin/repo-check`; no product CLI yet | None required |

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
template comments, and control-byte rejection. Preserve discovered regressions
as seed cases or explicitly allowlisted corpus files. Add fuzz targets for actual
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

Product integration, native artifact distribution, release signing, and release
reproducibility remain unimplemented. Add their checks with the relevant code;
the development harness does not establish those guarantees.
