# Fifth five-epoch readability review

## Scope and method

Applied the supplied `readable-code-review` skill and all 15 references to
tracked Go implementation and tests, shell scripts, hooks, and build/CI
configuration. Each of five epochs enumerated and scanned all 121 tracked
code/harness files, including platform-specific and VM-tagged tests, then
manually examined review candidates and the affected execution paths. Scans
included names, compound expressions, nesting, variable state, comments,
function responsibilities, test intent and diagnostics. The complete task diff
was reviewed after refactoring.

Automated signals are review aids, not definitions of a violation. The record
below lists all confirmed findings and repairs; no confirmed finding was deferred,
ranked for later work, or given an exemption. This review is not a formal proof
that no future reader can discover another readability problem.

The existing user change to `.gitignore` was preserved and is not part of this
task. No new readability gate, dependency, product capability, or security-policy
exception was introduced.

## Epoch 1

| Finding | Repair | Files |
| --- | --- | --- |
| Version-bound comparisons require decoding numeric component indices | Name major, minor and patch before comparing the supported range | `internal/adapters/attestation/gh.go` |
| Raw formula, selected candidate and bottle file are hidden behind single-letter locals | Give each representation a distinct name and separate bottle selection visually | `internal/adapters/homebrew/info.go` |
| Runtime revalidation combines acquisition, cancellation and release comparison in an initializer/else chain | Acquire once, then handle each failure with a guard | `internal/adapters/homebrew/public_session.go` |
| Success and expected-failure test conditions are interleaved in one Boolean expression | Finish the success case separately, then compare the expected error | `internal/adapters/homebrew/public_session_test.go` |
| One metadata test mixes successful selection, source-layout invariance and rejection | Separate the behaviors and report dependency/selection differences directly | `internal/adapters/homebrew/info_test.go` |

Validation: `./scripts/verify.sh` passed.

## Epoch 2

| Finding | Repair | Files |
| --- | --- | --- |
| Age-waiver lookup uses mutable bookkeeping to communicate whether a candidate exists | Use `slices.IndexFunc`, reject absence immediately, then bind the matched artifact | `internal/adapters/homebrew/engine.go` |
| Architecture checking requires tracking abbreviations for parsed files, comments, imports and graph nodes | Name these values by their roles throughout collection and validation | `tools/repo-check/architecture.go` |
| Interrupted-execution assertions are buried in the main native test's preparation retry loop | Extract the bounded fresh-plan assertion and name the process-exclusion condition | `tests/vm/public_execution_test.go` |

The waiver lookup still uses the uniquely named, validated collection closure.
The architecture rules, import graph, directive refusals and diagnostics are
unchanged. The retry retains its deadline, allowed transient failures and
new-attempt check, and closes its successful one-use session.

Validation: `./scripts/verify.sh` and VM-tagged compile-only testing passed.

## Epoch 3

| Finding | Repair | Files |
| --- | --- | --- |
| Archive validation's candidate identity is named only `f` across the whole traversal | Name the parameter `candidate` at all uses | `internal/adapters/homebrew/archive.go` |
| An installed-link test claims to reject a partial pour although it observes and marks it incomplete | Make observation and mismatched-record rejection explicit in the test name | `internal/adapters/homebrew/public_session_test.go` |
| A verifier assertion combines provenance, age, bytes and attribution in a long condition | Compare explicit expected provenance and age evidence separately, then compare raw bytes | `internal/adapters/attestation/gh_test.go` |
| One policy test mutates state across independent age, waiver-accounting and zero-assessment cases | Give each behavior its own fresh fixture and named test | `tests/policy_test.go` |

The verifier comparisons retain the existing contract checks and also make
status, provider, source and validity-interval expectations explicit. Policy
boundary values and rejection expectations were retained.

Validation: `./scripts/verify.sh` passed.

## Epoch 4

| Finding | Repair | Files |
| --- | --- | --- |
| The container probe's header describes only committed source although it also supports `--worktree` | Describe both accepted inputs | `scripts/probe-container.sh` |
| Generic `run` calls conceal that the probe runner constructs a Homebrew invocation | Rename the functions and every call to `run_brew` | `scripts/probe-homebrew-public-cli.sh`, `scripts/probe-homebrew-dependency-drift.sh` |

Command arguments, sandbox profiles and refusal behavior were not changed.
Validation: `./scripts/verify.sh` passed with pinned Go 1.27.1.

## Epoch 5

| Finding | Repair | Files |
| --- | --- | --- |
| The public-command integration test interleaves normal execution with a large fault-injection/refusal block | Extract one fault assertion owning injection, refusal, replay rejection and unchanged-payload checks | `tests/vm/public_execution_test.go` |

The installed-state fixture is restored before the caller releases its session,
as on the original return path. Cancellation still uses an already-cancelled
context; replay is checked with the live parent context. The normal and
interruption paths remain separate. Final reinspection found no additional
confirmed readability finding.

## Final validation and limits

- `./scripts/verify.sh` passed after every epoch. Epochs 1–3 used Go 1.24.0;
  epochs 4–5 and the final full suite used pinned Go 1.27.1.
- `./scripts/check.sh all` passed with Go 1.27.1: offline verification,
  shuffled tests, race, coverage, all seven required fuzz targets (10 seconds
  each), staticcheck, source vulnerability scanning and all three development
  binary scans. Vulnerability scans reported no vulnerabilities found.
- VM-tagged tests cross-compiled for `darwin/arm64`; tagged vet and staticcheck
  passed for that target.
- `git diff --check` passed.

No native VM acceptance, live advisory/attestation probe, container run,
distribution build or publication was performed. Native-only refactorings have
compile/static evidence here, not fresh end-to-end execution evidence. The
host's Homebrew installation was not modified.
