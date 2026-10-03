# Readability refactoring: third five-epoch review

## Scope and method

The requested work was five epochs of whole-tree discovery followed by repair
of every confirmed readability finding. Each epoch enumerated production Go,
Go tests (including platform and VM-tagged files), shell scripts, Git hooks,
CI configuration and Task aliases. The initial inventory contained 116 files;
the added candidate-evidence test brought it to 117. Captured upstream responses
were inspected through their consumers, not rewritten as hand-maintained code.

All 15 readable-code-review reference topics were considered. Source reading,
whole-tree searches and diff review were used together: search matches were
navigation aids, not readability rules or exemptions. Findings were not assigned
severity, priority or a repair quota. The five epochs are a record of this
requested investigation, not a new repository gate or clean-round requirement.

## Epoch 1: discovery, repair and verification

- `internal/adapters/homebrew/collect.go`: per-candidate dependency assembly,
  evidence construction and verifier binding obscured the collection sequence.
  Extracted `collectCandidateEvidence`, preserving acquisition, observation and
  OCI/advisory validation order. Added real observation-file assertions and
  wrong-subject, wrong-time, excess-validity, unavailable-status, wrong-claim
  and substituted-response failures in `collect_test.go`.
- `internal/adapters/homebrew/public_session.go`: matching execution metadata
  against the verified graph was interleaved with installed-state observation.
  Extracted `matchExecutionMetadata`; added changed-artifact, missing-dependency
  and additional-dependency regressions in `public_session_test.go`.
- `internal/adapters/homebrew/json.go`: two copies of field-name interpretation
  and an inline required-field expression made exact schema handling harder to
  inspect. Shared `schemaFieldName` and named required-field conditions.
- `internal/adapters/localstate/config.go`: compounded optional-section checks
  obscured supported trust, mandatory verification and age-only emergency rules.
  Grouped each section and named its relevant conditions.
- `tools/repo-check/architecture_test.go`: multi-file fixtures were compressed
  onto single rows. Expanded them so their resolved forbidden edges are visible.

Validation: `./scripts/verify.sh` passed.

## Epoch 2: whole-tree rediscovery, repair and verification

- `internal/adapters/homebrew/session.go`, `public_session.go`, `session_test.go`:
  `MinimumAge` hid its unit. Renamed the plan field to `MinimumAgeSeconds` while
  retaining the `minimumAge` JSON key and the binding's serialized `MinimumAge`
  key. Renamed the selected installed-version text to `candidateKegVersion`.
- `internal/adapters/attestation/input.go`, `gh.go` and
  `internal/adapters/homebrew/runtime.go`, `advisories.go`: size parameters and
  input records used ambiguous `limit` names. Changed them to `maxBytes`;
  named the counted bytes `bytesRead`. Inclusive bounds are unchanged.
- `internal/adapters/homebrew/vulns.go`: process `status` and `ex` did not carry
  their meaning through the parsing and invocation stages. Named them
  `exitCode` and `exitErr`.
- `tests/service_test.go`: planner members `p` and `s` required readers to
  recover their meaning across tests. Named them `prepared` and `session`;
  named captured request and override parameters.

Validation: `./scripts/verify.sh` passed; searched all callers for stale names.

## Epoch 3: whole-tree rediscovery, repair and verification

Comparison failures sometimes printed only a fixed sentence or a nil error,
leaving the actual failed value unavailable. Added actual/expected information
for advisory requests, feed inventory, captured cohorts, process IDs/groups,
in-flight records, selected metadata, copied runtime bytes, plan bindings,
configuration, CLI output, archive entries and VM state comparisons.

The affected suites are attestation `cohort_output_test.go` and `gh_test.go`;
Homebrew `advisories_test.go`, `cohort_output_test.go`, `inflight_test.go`,
`info_test.go`, `process_liveness_test.go`, `runtime_live_test.go`,
`runtime_test.go`, `session_test.go`, `vulns_live_test.go`; local-state
`config_test.go` and `files_test.go`; CLI `run_test.go`; `tests/service_test.go`;
VM `distribution_execution_test.go`, `general_execution_test.go`,
`native_execution_test.go`, `native_upgrade_test.go`, `public_execution_test.go`;
and release-pack `main_test.go`.

Separated the CLI exit, preserved age reason and candidate-presentation
assertions so each failure identifies the broken contract independently.

Validation: `./scripts/verify.sh` passed. VM-tagged tests passed darwin/arm64
vet and compilation; no VM mutation was run.

## Epoch 4: whole-tree rediscovery, repair and verification

- `scripts/check.sh`: `require_tool` silently populated a global variable for
  later callers. `checked_tool_path` now returns the validated path explicitly,
  with temporary state confined to a POSIX subshell. Tool pins and refusal
  behavior are unchanged.
- `scripts/probe-homebrew-public-cli.sh` and
  `scripts/probe-homebrew-dependency-drift.sh`: crowded precondition and failure
  branches obscured guards around disposable-prefix probes. Expanded those
  branches without changing commands, sandbox profiles or refusal conditions.

Validation: `./scripts/verify.sh` passed, including missing/wrong-tool and
isolated VM-driver failure tests. The initial `check.sh all` invocation correctly
refused the default Go 1.24.0; cached Go 1.27.1 was then located for final checks.

## Epoch 5: whole-tree rediscovery, repair and verification

- Homebrew `runtime.go` and `inflight.go`: three copies of parent-directory
  synchronization repeated the same storage operation. Reused `syncParent`
  and placed it with filesystem helpers; synchronization ordering and errors
  are unchanged.
- Attestation `cohort_output_test.go`: repeated the authenticated response digest
  in a comparison and diagnostic. Named the digest once.
- Homebrew `bottle_metadata_test.go`: repeated historical dependency JSON hid
  the duplicate-entry case. Named that input once and improved its diagnostic.
- Attestation `gh_test.go`: success/error comparison and response comparison
  shared an opaque failure. Split them and displayed the expected response.
- Local-state `config_test.go`: a complete valid configuration was one crowded
  literal. Grouped its JSON sections without changing values.
- `scripts/product-ready.sh`: pending-run guards were long single lines.
  Used matching layouts for cancellation and completion guards.

Reviewed the final diff for input order, unchanged serialized keys, short-circuit
safety, evidence attribution, execution binding, synchronization and cleanup.
All confirmed findings from these searches were repaired. This review does not
establish an objective proof that no future readability finding is possible.

## Final validation and limits

Using cached Go 1.27.1, `./scripts/check.sh all` passed: offline verification,
race detection, shuffled uncached tests, coverage reporting, all seven active
fuzz targets (10 seconds each), pinned Staticcheck, and source/test plus checker
and both CLI binary vulnerability scans. No vulnerabilities were reported.

VM-tagged darwin/arm64 vet, compilation, Staticcheck and source/test vulnerability
scanning also passed with the pinned toolchain. `git diff --check` passed.

No tools or product dependencies were added or downloaded. No host Homebrew
installation was modified. Live upstream probes, native VM execution, container
acceptance, distribution packaging and publishing were not run for this task.
The user's existing `.gitignore` edit is outside the task and is not included.
