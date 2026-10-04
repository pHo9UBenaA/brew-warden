# Seventh five-epoch readability review

## Scope and method

This task reviewed all 121 tracked executable-source and harness files: Go
production code, adapter and external tests, tagged VM tests, shell scripts,
Git hooks, CI YAML and Taskfile. Fixture data and prose were consulted as
context rather than treated as executable code. The pre-existing `.gitignore`
edit was preserved and excluded from the task's commit.

The review used all 15 topics in the readable-code-review skill. Each epoch
searched the whole source inventory, repaired the findings from that search,
and ran offline verification. The first search read every source file in full;
subsequent searches rescanned every file, reconsidered all topics against that
source, and inspected changed code and its callers. Inventory hashes and
mechanical candidates supported the review; they were not an automatic
readability gate or proof that no future reader could find another issue.
No finding was deferred through a severity category or a readability exemption.

## Epoch results

| Epoch | Findings repaired after the whole-source search |
| --- | --- |
| 1 | Ambiguous graph and advisory bookkeeping names; attestation fixture setup failures without caller-attributed diagnostics; positional metadata/input literals; oversized configuration failure output; visually crowded changed declarations and version-check precedence. |
| 2 | Remaining crowded declaration boundaries across product, tests, tools and VM scripts; opaque advisory subject conditions; assessment and execution fixture construction without test-helper attribution; scanner comments implying one exact pin rather than reviewed release support. |
| 3 | Attestation fixtures unnecessarily decoding and re-encoding their own generated JSON; runtime file-copy details interleaved with release selection and fingerprint acceptance; missing artifact/claim context when a fixture claim lookup fails. |
| 4 | Inconsistent field layout in the newly simplified fixture; unstated timestamp units and inclusive/exclusive validity boundaries. Added real-filesystem regression evidence for the extracted runtime copy and link-resolution boundary. |
| 5 | Whole-source search and complete diff review found no additional actionable readability issue. No artificial code change was made to fill this epoch. |

## Resulting changes

- Graph maps distinguish artifact lookup, visit state and duplicate names,
  dependencies and targets. Advisory variables distinguish candidates,
  expected versions, formula findings and advisory identifiers.
- The gh fixture constructs certificate, subject, statement and verified
  timestamps directly, with one serialization and test-helper diagnostics.
  Existing rejection tests and captured real outputs remain intact.
- `Runtime.materializeFrom` retains release selection and fingerprint
  acceptance. `copyRuntimeFiles` owns the bounded filesystem copy, inventory
  and resolved-link checks. Traversal order, cancellation checks, entry/byte
  bounds, modes and serialized fingerprint fields are preserved.
- Relative-link tests exercise the real materialization boundary. A lexical
  in-tree link is copied and bound; a dangling link is refused even when its
  hypothetical manifest would match the supplied fixture pin.
- Named literals, bounded failure messages, declaration separation and precise
  time comments reduce interpretation work without changing execution policy,
  persisted schemas, import direction, command arguments or harness gates.

## Verification and limitations

- Baseline and every epoch: `./scripts/verify.sh` passed. Initial checks used
  the ambient Go 1.24.0 compatibility-floor toolchain; epochs 2 onward used the
  existing pinned Go 1.27.1 SDK.
- Final `./scripts/check.sh all` with Go 1.27.1 passed: offline gates, shuffled
  tests, race detection, coverage reporting, all seven fuzz targets at their
  default ten-second budgets, pinned Staticcheck, source/test vulnerability
  scanning, builds and vulnerability scans of all three binaries.
- Darwin arm64 tagged VM tests passed vet, compilation and tagged Staticcheck.
  Linux arm64 production/tool packages cross-built successfully.
- In an isolated temporary copy, removing runtime resolved-link validation made
  the dangling-link test fail at its intended refusal assertion with a matching
  fingerprint and no error. The repository implementation was not mutated.
- `git diff --check` passed. No host Homebrew installation was modified.

Tagged native VM acceptance, optional live evidence/source-matrix probes and
Linux test execution were not run. Cross-compilation does not establish those
runtime guarantees. This readability task neither expands supported Homebrew
capabilities nor establishes new release-readiness or publication evidence.
