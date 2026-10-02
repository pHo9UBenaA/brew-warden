# Five-epoch readability refactoring

## Scope and method

The requested review covered product Go code, unit and integration tests,
platform-tagged VM tests, development tools, shell scripts, Git hooks, CI YAML,
and task/container configuration. The readable-code-review skill and all fifteen
references were read. Every topic was considered: understanding cost, names,
ambiguity, visual structure, comment value and precision, control flow,
expressions, variable lifetime, extraction, task separation, plain-language
logic, unnecessary code, test readability, and design tradeoffs.

The first exploration read the complete source bodies. Subsequent explorations
revisited the complete file inventory and examined the changed code and remaining
candidates against that baseline. Source-wide searches for crowded expressions,
long functions, positional evidence mutations, numeric traversal states, and
compressed statements supplemented the review; these searches are candidate
finders, not automatic readability verdicts. Five exploration/repair epochs were
completed, with offline verification after each epoch. Findings were not assigned
priorities or deferred as exceptions. The list below records the identified
problems and their repairs, not a guarantee that subjective readability review
can discover every possible future improvement.

## Findings and repairs

### Epoch 1

- Standard-library and project imports were mixed in attestation results,
  Homebrew metadata and metadata tests, and evidence/planning ports. Group them
  consistently.
- Homebrew process environments, HTTP settings, evidence records, execution
  controls, plans, process records, and plan/assessment fixtures hid their
  structure in single lines. Expose related fields and arguments in groups.
- The attestation signer condition required readers to distribute multiple
  negations. Name the allowed workflow condition positively.
- Bottle eligibility mixed URL construction with platform and cellar rules.
  Name the expected URL and supported tag/cellar conditions.
- `Assessment` retained a comment about an obsolete attempt journal with no
  corresponding field. Remove the misleading comment.
- The public-session comment used an unexplained pronoun. Name the session's
  ownership and the operation lock's scope.
- Sandbox quoting, digest formatting, and fatal reporting compressed several
  operations into one line. Expand the operations.
- A single import had an unnecessary group, and a VM-driver test had a stray
  leading blank line. Match surrounding presentation.

### Epoch 2

- Public state inspection interleaved metadata collection with installed-receipt
  path checks, decoding, attribution checks, and hashing. Extract
  `installedReceiptDigest`, preserving the trusted-user observation boundary.
- `publicActions` interleaved inventory validation with nested installed-version
  selection and a separate `found` flag. Extract `installedAction`, use guards,
  and retain inspection of every matching installed entry.
- Policy evaluation interleaved single-claim interpretation with waiver and
  decision accumulation. Extract `requiredEvidenceReason` with direct returns;
  leave duplicate integrity/vulnerability denials in the evaluator.
- CLI routing interleaved plan presentation and duplicated help-output mechanics.
  Extract `presentPlan`, expose help text in readable blocks, and share the
  output operation without changing output bytes.

### Epoch 3

- Policy and execution tests encoded evidence meaning as slice positions.
  Locate fixture claims by their semantic claim identifier instead.
- Timestamp failure cases used anonymous inputs and an immediate anonymous
  artifact-building function. Name the cases and construct the other digest
  explicitly before using it.
- Metadata mutations and plan-binding mutations were anonymous sequences.
  Give each mutation a named subtest and check serialization setup.
- In-flight test names used a temporary Boolean-keyed map. Use a direct choice.
- Two operation-lock tests exercised the same exclusion/reacquisition behavior.
  Keep the test that checks close errors; preserve cleanup on unexpected success.
- Some attestation and archive test setup discarded filesystem, hashing, or
  decoding errors. Report or fail immediately rather than obscure setup failure.
- The configuration test name did not describe its behavior. Name defaults and
  explicit zero age.
- Add installed-action cases for a missing active version and a duplicate active
  entry with unsupported receipt flags, protecting the extracted loop's behavior.

### Epoch 4

- Dependency and architecture traversals encoded visiting/visited states as
  unexplained integers. Name the states locally at each owning boundary.
- The minimum-age duration ceiling was an unexplained repeated literal. Name and
  document the existing nanosecond-duration-derived bound and reuse it in
  configuration and VM acceptance.
- Runtime comparisons called both inputs `a`/`b`. Distinguish copied and installed
  links/bytes.
- Runtime fixture selection, persisted environment/inventory validation, and
  execution plan/exception checks required readers to reconstruct compound
  meanings. Name their predicates without changing refusal conditions.
- Shared-prefix archive validation mixed location and entry-type restrictions.
  Name the locations and check the two restrictions separately.
- Collection's initial guard crowded services, time, platform, and request checks.
  Separate the guard groups while retaining the same failure.
- The guest-command wrapper changed the shell's global `vm` unnecessarily.
  Forward its argument array directly.
- Guest device-login setup was one long shell literal. Expose its existing
  startup, environment, logging, and PID-recording steps; expand failure helpers.

### Epoch 5

- Execution interleaved dependency readiness scanning with launching and expiry
  checks. Extract `nextReadyAction`; return immediately when an action is ready,
  keep node-order selection, and test dependency completion, independent actions,
  and inability to advance a cycle.
- Remaining multi-action test lambdas and session-close methods compressed setup
  and state transitions. Expand them into separate statements.
- The VM survey's eligibility expression crowded three distinct conditions.
  Lay out each condition independently without altering short-circuit order.
- Task descriptions still claimed the CLI was unimplemented and understated fuzz
  and vulnerability checks. Describe the actual existing commands.
- Add explicit policy-duration boundary cases for negative, zero, maximum, and
  one-above-maximum inputs.

## Verification and limits

- `./scripts/verify.sh` passed after each epoch. The final pass used pinned
  Go 1.27.1; earlier passes also exercised the installed Go 1.24.0 compatibility
  floor.
- `./scripts/check.sh all` passed with pinned Go 1.27.1 and the existing pinned
  tools: baseline checks, shuffled tests, race detection, coverage reporting,
  all seven fuzz targets (default ten seconds each), Staticcheck, and source/test
  plus three binary vulnerability scans. No vulnerabilities were reported.
- macOS arm64 VM acceptance tests passed vet and cross-compilation with their
  build tag. No native mutation or authenticated VM acceptance was executed.
- `git diff --check` passed. Imports, upstream options, persisted schema fields,
  output text, policy refusal rules, startup gating, and execution binding were
  reviewed for preservation. No dependency or execution capability was added.
- The host Homebrew installation was not modified. The existing user change to
  `.gitignore` is outside this task and must remain outside its commit.
