---
name: brewwarden-change
description: Implement brewwarden behavior or architecture changes with evidence for policy decisions and adapter boundaries. Use for features and fixes, not routine prose edits.
---

# Implement a bounded change

Read [architecture](../../../docs/architecture.md) when ownership or imports
change, and [the product design](../../../docs/design.md) for policy behavior.
Follow [repository instructions](../../../AGENTS.md); keep each rule at its
authoritative owner rather than copying it into another layer or document.

For each security feature, follow [Homebrew integration](../../../docs/homebrew-integration.md).
Inspect upstream support first; record the delegated guarantee, gaps, exact
options, supported versions, and owning adapter/tests. Add only the required
supplement, and keep upstream command details out of core policy.

Identify the owning layer and a realistic input that distinguishes the desired
behavior. For a defect, observe that example failing for the intended reason;
unresolved imports and broken setup are not reproductions. Fix the cause without
weakening the expectation. For new behavior, establish the contract first.

Prefer deleting unnecessary code and consolidating duplicate processing at its
owning boundary before adding files or abstractions. Add structure only when it
makes the required behavior easier to follow; preserve dependency boundaries.
Use names that expose artifact identity and evidence status; keep collection,
policy decisions, and execution easy to trace without requiring more helpers.
Measure a performance problem before adding complexity to optimize it.

Pass time and evidence explicitly into the core. Exercise verifier, subprocess,
filesystem, and Homebrew adapters separately where fake ports hide the risk.
Emergency behavior must leave integrity and trust requirements intact. A saved
allow decision is not authorization to skip revalidation.

Choose tests by the realistic regression they catch versus execution time,
flakiness, and maintenance cost. Remove implementation copies, mock self-checks,
redundant type/library checks, duplicate cases, and internal coupling only after
identifying what bug detection would be lost. Keep independent contract and
real-boundary evidence, including strict input rejection; do not mistake these
for library self-tests. Do not optimize for test counts or coverage percentages.

Run affected checks, then `./scripts/verify.sh`. For architecture gate changes,
include resolvable forbidden edges and a valid graph. Keep docs and diagnostics
in English; update docs where readers need them to decide or act. Inspect the
implementation and complete diff, including new files; passing tests do not
replace review. State what was tested and what remains unverified. This skill
does not authorize changing the host's Homebrew installation or publishing changes.
