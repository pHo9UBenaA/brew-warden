# Automating recurring readability repairs

## Investigation and choice

Reviewed all seven archived five-epoch readability records against the existing
verification harness. Many repairs were comprehension judgments; a smaller set
were repeatedly mechanical. Rules and usage now belong to `docs/verification.md`,
not these historical notes.

| Earlier repair category | Automation selected | Boundary |
| --- | --- | --- |
| Import grouping, declaration gaps, literal layout, redundant grouping | Pinned gofumpt default formatting | Does not choose semantic field groups or break arbitrary long expressions |
| Crowded shell branches and inconsistent layout | Pinned shfmt with POSIX parsing and existing indentation/redirection conventions | No shell simplification; no recursive rewriting of quoted commands or YAML |
| Redundant Go syntax | Baseline gofmt simplification plus existing Staticcheck simplification checks | Does not discover unnecessary product requirements or duplicate workflows |
| Conventional identifier spelling and receiver consistency | Additional Staticcheck ST1003, ST1016 and ST1023 checks | Does not establish meaningful units, artifact identities or accurate names |
| Fixture failure locations lost inside named helpers | Standard-library AST gate requiring an initial Helper call on directly qualified testing handles | Build-tag independent; not type-alias resolution or call-graph inference |
| Misleading comments, mixed responsibilities, test state leakage and wrong-reason rejection | Retained semantic review and independent behavior tests | No blanket name-length, complexity or decomposition quota |

Reused maintained formatters rather than reimplementing their syntax/layout
rules. Tool setup is explicit, version-pinned and isolated from product modules.
The narrow helper gate uses the existing all-source traversal and Go parser,
without adding another external linter/framework. Ordinary and VM-tagged
Staticcheck invocations share one rule selection and existing pin validation.
Baseline verification remains usable without separately installed formatters.

The first gofumpt scan identified 39 existing Go files that ordinary gofmt had
accepted. Applied formatting to the source tree; preserved literal string bytes,
numeric permission values, serialized fields, refusal rules and upstream command
arguments. Extra gofumpt rules were not enabled. Shfmt options preserve the
existing two-space, indented-case and spaced-redirection style to avoid imposing
unrelated layout churn.

## Regression evidence and validation

- Real pinned-tool fixtures reject formatting drift, malformed source and missing
  or incorrectly pinned tools. Check mode preserves bytes; write mode repairs
  tagged/generated/testdata Go files, shell scripts and Git hooks.
- A shell fixture retains spaces and semicolons within separate arguments after
  formatting. Named-helper fixtures reject missing, late and wrong-handle Helper
  calls, including VM-tagged files and renamed testing imports. Valid test,
  benchmark/fuzz entrypoints and anonymous callbacks are accepted.
- The real Staticcheck fixture reports all three added rule IDs and accepts the
  corrected implementation. Existing hook, missing-tool, fuzz-target and VM
  cleanup/refusal tests still pass.
- `./scripts/verify.sh` passed with the Go 1.24 compatibility-floor toolchain and
  pinned Go 1.27.1. `./scripts/check.sh all` passed with pinned Go: offline gates,
  shuffled tests, race, coverage, all seven fuzz targets at ten seconds each,
  full lint and source/test plus three binary vulnerability scans.
- Both new formatter binaries passed additional govulncheck binary scans.
  Darwin arm64 VM-tagged vet, shared Staticcheck and compilation passed; ordinary
  packages cross-built for Linux arm64. `git diff --check` passed.
- Formatter-only shell scripts were compared as parsed ASTs with source positions
  removed; no structural or literal changes were found.

No host Homebrew installation was modified. Native authenticated VM acceptance,
live Homebrew/network probes, Linux test execution and distribution packaging
were not run. Installed-tool integration cases skip when tools are absent in an
ordinary baseline checkout; full checks require all tool pins before tests.
The user's existing `.gitignore` edit is preserved outside this task's commit.
