---
name: brewwarden-review
description: Review brewwarden policy, execution boundaries, dependency choices, or development harness changes. Use for requested reviews and consequential security changes.
---

# Review behavior and boundaries

Establish the actual diff or implementation scope, including staged, unstaged,
and relevant untracked files. Read [the threat model](../../../docs/threat-model.md)
and the affected contract in [the design](../../../docs/design.md).

For affected Homebrew capabilities, review the
[capability records](../../../docs/homebrew-integration.md) against upstream
source and tests. Check for duplicate implementations, implicit
trust in enabled flags, skipped checks reported as verified, scattered upstream
options, and unhandled version/deprecation changes. Verify attribution between
Homebrew guarantees and BrewWarden's additional checks.

Trace artifact identity and evidence through policy, exceptions, plan storage,
revalidation, and execution. Look for missing-to-success conversions, weaker
verification under emergency mode, unchecked dependencies, stale approvals,
implicit tool installation, and output or path injection. Distinguish validated
metadata from an unsigned API response.

When ownership, imports, or composition are affected, inspect source import
direction and actual wiring using [architecture](../../../docs/architecture.md).
A passing graph check is not proof of runtime safety. Inspect the implementation
and complete diff, including new files, as well as meaningful tests; passing
tests do not establish readability or complete correctness. Exercise the real
boundary when practical without changing the host's Homebrew environment.

Check whether deletion or consolidated processing would make the affected code
easier to understand than added files, helpers, or layers. Require a concrete
benefit for extra structure and measurements for performance claims. Keep rules
at their authoritative owner and docs focused on reader decisions and actions.

For test changes, identify the realistic bugs each case detects and what removal
would lose. Flag implementation copies, mock self-checks, redundant type/library
checks, duplicate cases, and coupling to internal details when their execution
time, flakiness, and maintenance cost exceed their regression value. Preserve
independent contract and real-boundary evidence, including strict input rejection;
these are not library self-tests. Test counts and coverage percentages are not
acceptance targets.

Review harness changes as executable code: hooks, CI, checker exemptions, agent
instructions, and skills can weaken future verification. For affected harness
rules, verify meaningful negative cases as well as a clean checkout. Do not
introduce approval quotas or repeated clean rounds without a demonstrated need.

Report actionable findings with trigger, consequence, file, evidence, and repair
direction. Distinguish required contract fixes, recommendations, and hypotheses;
separate reproduced failures from supported risks and design proposals.
Report exact checks and limitations. Do not edit or publish solely because a
review found an issue; follow the user's authorized scope.
