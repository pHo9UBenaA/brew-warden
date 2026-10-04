# Additional readability rules

## Selection

Follow-up to the readability automation task. The new rules use the already
pinned tools and do not add dependencies, file exemptions or metric quotas.
Current rule definitions and usage remain in `docs/verification.md`.

- Added Staticcheck ST1020, ST1021 and ST1022: existing exported declaration
  comments identify the function, type or variable/constant being documented.
  This supports precise reference and generated documentation. It does not
  require comments where none exist or establish factual correctness.
- Enabled gofumpt's `group_params`, `clothe_returns` and `balance_calls` extras
  by name rather than the blanket `-extra` switch. Parameter/result names remain
  visible, named-result returns expose their values, and multiline calls have
  consistent boundaries. Explicit selection prevents future tool updates from
  automatically enabling additional extras.
- Retained inherited Staticcheck checks, including existing simplification,
  unused-code, comparison-order and switch-layout checks. Mandatory package
  comments, identifier/function length limits and complexity quotas were not
  added: those are not substitutes for demonstrated understanding benefits.

The initial scans reported five existing doc-comment target mismatches and two
formatting candidates. Repaired all five comments without losing warnings,
time units or validity boundaries; moved the Clock.Now contract to its method.
Applied only call layout and named-result grouping in the two formatter
candidates. Function types, serialized keys, refusal rules and execution binding
are unchanged.

## Evidence

Before activation, the new real-tool tests failed because lint accepted all
three mismatched comment kinds and the formatter accepted implicit results and
unbalanced call layout. After activation, the fixtures are rejected for the
intended rule, corrected examples pass, and undocumented declarations still
pass without requiring filler comments.

The formatter fixture executes a Go behavior test both before and after repair.
It preserves a named result changed by a deferred closure and an empty-input
path with a bare return in a no-result callback. The same fixture verifies
parameter names, explicit result values and the multiline closing boundary.

Validation passed:

- `./scripts/verify.sh` with Go 1.24.0 and pinned Go 1.27.1.
- `./scripts/check.sh all`: offline gates, shuffled tests, race, coverage, all
  seven fuzz targets at ten seconds each, lint and vulnerability scans.
- Darwin arm64 VM-tagged vet, shared Staticcheck and compile-only tests.
- Linux arm64 cross-build and `git diff --check`.

No tool download or host Homebrew mutation occurred. Native VM execution, live
Homebrew probes, Linux test execution and distribution acceptance were not run.
The pre-existing `.gitignore` change remains outside this task's commit.
