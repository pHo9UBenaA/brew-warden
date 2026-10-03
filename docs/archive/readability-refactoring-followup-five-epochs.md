# Five-epoch readability follow-up

## Scope and method

This is a follow-up to the earlier readability refactoring, not a new set of
repository rules. The readable-code-review skill and all fifteen references
were read, together with the product, architecture, threat-model, dependency,
Homebrew integration and adapter contracts.

The initial exploration read the complete Go source and tests, including
platform files and VM-tagged acceptance, shell scripts, hooks, CI, task and
container configuration. Each subsequent exploration revisited the entire
executable-file inventory against that baseline, using repository-wide function,
expression, comment, fixture and state-lifetime searches. Candidate bodies and
changes were inspected rather than treating search matches as violations.
All fifteen topics remained in scope in every epoch; the headings below describe
findings, not restrictions on the exploration. Five exploration/repair epochs
were completed, with offline verification after each repair. The fifth epoch
also reviewed the accumulated diff and repaired issues introduced by extraction.

All identified findings were repaired without severity rankings, deferred
findings or file exemptions. Readability is a comprehension judgment: this
review does not establish that no future reader can discover another problem.
The pre-existing user edit to `.gitignore` is not part of the task.

## Findings and repairs

### Epoch 1

- `combineAdvisories` advertised an error result but could never return an error.
  Remove the impossible failure path from its interface and callers.
- Advisory collection mixed feed compression with candidate evidence collection.
  Extract `compressAdvisoryFeed` and verify decoded bytes and their digest in a
  real gzip round-trip test. Name observation fields instead of positional values.
- Homebrew JSON decoding called its tolerance flag `external`, which named the
  input's origin rather than the actual behavior. Use `allowUnknownFields`.
  Correct the comment claiming every persisted field has an explicit JSON tag.
- `ghLimit` did not distinguish the requested result limit from an inclusive
  admissible maximum. Name it `attestationResultLimit`, explain saturation and
  derive the unchanged `--limit 100` argument from that value. Share the repeated
  unsupported-version diagnostic rather than maintaining six copies.
- Evidence, artifact, metadata and prepared-plan literals crowded distinct
  fields together. Group their identities, attribution and binding fields.
- Attestation fixture JSON hid its signer and subject structure in one line.
  Expose those sections without changing their selected values.
- Installed state called an observed opt-linked active version `LinkedKeg`,
  conflating it with Homebrew's separate linked-keg record. Use `ActiveVersion`
  while retaining the original `active_version` JSON key.
- Packaging called archive entries `x` and compressed tar-header construction.
  Use `entry` and `header` and expose the deterministic header fields.
- Execution-outcome tests reset a consumed fake session to test another behavior.
  Separate changed-attempt refusal into its own fresh-session test, including
  closure. Separate CLI rejection tests from exit-code/printable-reason behavior.
- Verification still described missing product directories as intentional.
  Describe the existing formatting traversal instead.

### Epoch 2

- Attestation-age parsing wrapped each result in a JSON array, marshalled it and
  reparsed it to check the signer and subject. Extract the single-result
  `verifiedResultSubject` operation and invoke it directly from age parsing.
  Keep every-result inspection and exact signer, subject and digest conditions.
- Application execution interleaved orchestration with nested outcome mutation.
  Extract `executionOutcome` with direct returns for unknown, successful,
  partial and unchanged-failure states. Preserve the close-failure override.
- Advisory combination maintained a flag only to answer whether a matching patch
  existed. Express that search with `slices.ContainsFunc`.
- Public info fixture JSON obscured bottle fields among unrelated metadata.
  Lay out its logical sections, retaining mutation targets and irrelevant nulls.

### Epoch 3

- Fixture JSON generation discarded errors in Homebrew metadata, captured
  cohorts, in-flight records and persisted-plan tests. Use a `testing.TB` helper
  that fails at construction, including fuzz seeds. Attestation fixture building
  now checks its serialization errors consistently with its existing decode checks.
- OCI tests repeated nested JSON/string encoding mechanics. Share
  `bottleIndexFixture` so each case exposes digest, reference and dependency tab.
- Metadata, saved-plan and cache-path rejection cases were anonymous crowded
  expressions. Give mutations descriptive subtest names and diagnostic context.
- The unbound-session and unknown-schema in-flight cases also omitted collection
  identity, so unrelated missing-field rejection could satisfy them. Start from
  a valid complete record and change only the named property. Verify that the
  baseline is valid and that each mutation actually invalidates it.
- CLI invocation tables and Git runtime-fixture commands packed independent
  cases into single lines. Put each invocation on its own row.

### Epoch 4

- Sandbox calls used adjacent Boolean literals for network and installed-prefix
  writes. Introduce named `sandboxPermissions` fields at every caller. Keep the
  emitted confinement rules and permission values unchanged. Test the actual
  macOS sandbox with allowed diagnostic writes and denied frozen-input writes,
  using only temporary files.
- Archive path and symlink checks mixed equality, prefix membership and
  negations. Name keg membership and unsupported shared-entry conditions.
- Persisted-plan checks crowded schema alternatives, expiry boundaries and action
  operations into compound rejections. Name those predicates without changing
  the accepted schemas, intervals or operations.
- Advisory identifiers used one negated character-class puzzle. Name ASCII
  letters, digits and separators and reject characters outside those sets.
- Bottle-manifest selection copied the inventory only to delete nonmatches.
  Select matching entries directly in a simple loop.
- The startup gate compressed its read, descriptor closure and exec into a
  shell literal. Expose the same three steps without changing argument arrays.
- VM preparation, authentication, fixture removal, credential cleanup and the
  readiness runner packed assignments and actions into single lines. Separate
  them while preserving shell state, cleanup order and exit behavior.
- Homebrew source arguments and CLI doctor output hid their structure in long
  lines. Group arguments and message sections without changing bytes.

### Epoch 5

- Seven native acceptance entrypoints duplicated the disposable-machine probe.
  Share `requireDisposableMac`, retaining each explicit opt-in and checking the
  machine before any fixture mutation. Keep target-name validation separate.
- `interruptOnPour` actually interrupted at the download message, not pouring.
  Use `interruptOnFetch`; use the corresponding `killOnFetch` name for the
  parent-crash trigger.
- Live diagnostics silently discarded file-read errors. Include those errors in
  failure output, and read the metadata invocation's actual `info-0.stderr`
  rather than a nonexistent `metadata.stderr`.
- Plan binding repeated an anonymous graph-node type. Name the local projection
  while preserving serialized field order. Name and group metadata-claim fields.
- Remaining nested installed-state, execution-session and package-inventory
  fixtures crowded setup. Expand their relevant fields and rows. Expand commit
  message cases and hook environment setup; name `sourceTreeFixture` by its role.
- OCI test cleanup left a grouped single import. Use the project's single-import
  form.
- Single-result extraction left the old aggregate `verifiedSubject` wrapper used
  only by tests. Remove the orphaned production implementation. Exercise the
  actual `oldestVerifiedTimestamp` entrypoint in subject fuzzing and later-result
  regression tests, including a valid later result, an unrelated trusted subject,
  an untrusted signer and a contradictory digest.

## Verification and boundaries

- `./scripts/verify.sh` passed after all five epochs, including a final fifth-epoch
  pass after removing the orphaned parser. The first pass used installed Go
  1.24.0; subsequent passes used pinned Go 1.27.1.
- Final `./scripts/check.sh all` passed with Go 1.27.1: baseline verification,
  shuffled tests, race detection, coverage reporting, all seven fuzz targets
  (ten seconds each), pinned Staticcheck, and source/test plus three binary
  vulnerability scans. All vulnerability scans reported no findings.
- VM-tagged macOS arm64 acceptance passed vet, Staticcheck and cross-compilation.
  No native Homebrew mutation or authenticated VM acceptance was executed.
- The real macOS sandbox temporary-file test passed, including both the allowed
  diagnostic write and the refused frozen-input write.
- The optional offline Homebrew Git source matrix passed for all eleven reviewed
  release commits. It used an isolated shared clone of a cached source checkout;
  the input checkout and installed Homebrew prefix were not modified. Cached
  7.0.6 source inspection confirmed the distinction between opt and linked-keg
  records and the existing install maintenance options.
- `git diff --check` passed. Serialization keys, field order, upstream arguments,
  version bounds, policy refusal conditions, age-only exceptions and execution
  binding were reviewed for preservation. No dependency, trust expansion or
  execution capability was introduced. No push or publication was performed.
