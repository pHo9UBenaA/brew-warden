# Sixth five-epoch readability review

This follow-up reviewed the repository against every topic in the
`readable-code-review` skill. Findings were repaired without severity rankings,
waivers or deferred findings. The initial review read the product, adapter,
test, script and harness implementations; subsequent epochs rescanned every
repository-owned executable/configuration file and reviewed candidates and the
accumulated changes. The inventory grew from 120 files to 122 after adding
focused doctor contracts and an attestation fixture helper.

The review considered names, visual structure, comment value and precision,
control flow, expressions, variable lifetime, unrelated work, task separation,
plain-language logic, unnecessary code, test clarity and design costs. Search
patterns identified candidates, not mandatory identifier lengths or line limits.
No new readability gate, repeated-review requirement, dependency or execution
capability was added.

## Epochs and resolved findings

### 1

- `internal/domain/decision.go`: assessment, decision, exception and evidence
  names obscured their roles across the evaluator. Name those values directly,
  name the denial conditions and construct reasons with explicit fields.
- `internal/adapters/homebrew/public_session.go`: receipt-file mechanics were
  mixed into state collection. Bind every installed receipt in one named
  operation; separate receipt decoding from its official-core identity check.
- `internal/adapters/homebrew/collect.go`: evidence and artifact parameters,
  along with advisory subjects, required decoding one-letter names. Name the
  identity and evidence roles at those boundaries.
- `tests/policy_fuzz_test.go`: claim numbers were actually array positions and
  mutations required pointer-to-slice bookkeeping. Select typed claims by their
  meaning and use collection operations for removal.
- Execution, service, runtime, metadata, installed-link and archive tests:
  aggregate failures hid expected values or printed pointer addresses. Report
  actual outcomes, launch/release flags, digests, inventories and link values.
- `tests/vm/ownership_execution_test.go`: generic reflection obscured a string-map
  comparison; use `maps.Equal` and display both snapshots on failure.
- `internal/adapters/homebrew/vulns_live_test.go`: an unused local and filtering
  environment flags absent from the private environment added work with no
  effect. Remove them without changing the constructed command environment.
- `internal/composition/main.go`: visually separate process entry and composition.

### 2

- `internal/adapters/homebrew/metadata.go`: two equivalent inventories were
  returned although callers needed only the validated closure. Return one
  deterministically ordered closure and update all consumers; retain cycle,
  missing-dependency, identity and unrequested-formula rejection tests.
- `internal/cli/runtime.go`: runtime diagnostics occupied the routing function,
  and its copied service was named as an action. Extract doctor reporting and
  identify the invocation-local service. Add public CLI contracts for supported,
  missing and unsupported diagnostics, output failure and no mutation planning.
- `internal/adapters/homebrew/runtime.go`: distinguish the copied destination
  file from the inspection destination directory.
- VM execution tests: report policy and execution fields, and name the
  continued-process condition instead of negating multiple message searches.

### 3

- Homebrew and local configuration JSON decoders: one-letter decoder/type names
  and, for Homebrew, rediscovery of required schema fields mixed schema inspection
  with input traversal. Use explicit decoder/type names and collect Homebrew's
  required field names during the initial schema inspection.
- Bottle metadata, advisory reconciliation, request validation and CLI reporting:
  name annotations, findings, operation eligibility, platform eligibility and
  refusal reasons; separate decoding from conditional identity validation.
- Adapter tests and VM scripts: separate long calls from their assertions,
  expose the affected/open advisory data and group sandbox restrictions and
  guest arguments so their distinct responsibilities can be scanned.
- The verification run detected a fixture-formatting change that made the
  missing-patched-field mutation ineffective. Restore the original serialized
  bytes using readable literal segments and rerun verification successfully.

### 4

- Negative fixtures across Homebrew and attestation tests silently depended on
  replacement text being present. Small package-owned helpers now fail at fixture
  construction when the intended text disappeared, before parser/verifier
  assertions. Preserve all negative cases and their original changed bytes.
- The patched-advisory fixture packed two transformations into one expression.
  Express the field changes sequentially.

### 5

- The formatter moved argument comments beside the wrong Boolean in the link
  assertion calls. Replace fragile inline labels with precise scenario comments.
- The new doctor contract combined stdout and stderr absence expectations into
  one condition. Check each stream separately with its own failure explanation.
- Inspect the complete accumulated diff and new files for altered refusal rules,
  evidence attribution, serialized binding inputs, import direction and lost
  regression assertions. No further actionable finding remained in this review.

## Verification

Each epoch ran `./scripts/verify.sh`; the first used the Go 1.24.0 compatibility
floor, and subsequent runs used the existing pinned Go 1.27.1 toolchain. The
third epoch's initial failing fixture check was repaired before its successful
rerun. Final checks passed with Go 1.27.1:

- `./scripts/check.sh all`: offline verification, race detection, cross-package
  coverage, all seven active fuzz targets at 10 seconds each, pinned Staticcheck,
  source/test vulnerability analysis and all three diagnostic binary scans.
- Darwin/arm64 tagged VM tests: vet, pinned Staticcheck, govulncheck and test-binary
  compilation with `-tags=vmacceptance`.
- Linux/arm64 test-binary compilation for all ordinary packages.
- `git diff --check`.

Local epoch inventories, search reports and check logs are under `.cache/` and
are not distribution or publisher evidence.

No test modified the host's Homebrew installation. Native authenticated VM
execution, live network Homebrew probes and distribution acceptance were not
performed; offline tests and cross-compilation do not establish those guarantees.
The user's existing `.gitignore` changes are not part of this task's commit.
