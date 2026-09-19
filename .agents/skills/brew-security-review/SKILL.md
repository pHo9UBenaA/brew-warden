---
name: brew-security-review
description: Review brew-security policy, execution boundaries, dependency choices, or development harness changes. Use for requested reviews and consequential security changes.
---

# Review behavior and boundaries

Establish the actual diff or implementation scope, including staged, unstaged,
and relevant untracked files. Read [the threat model](../../../docs/threat-model.md)
and the affected contract in [the design](../../../DESIGN.md).

Trace artifact identity and evidence through policy, exceptions, plan storage,
revalidation, and execution. Look for missing-to-success conversions, weaker
verification under emergency mode, unchecked dependencies, stale approvals,
implicit tool installation, and output or path injection. Distinguish validated
metadata from an unsigned API response.

Inspect source import direction and actual wiring using
[architecture](../../../docs/architecture.md). A passing graph check is not proof
of runtime safety. Inspect meaningful tests and exercise the real boundary when
practical without changing the host's Homebrew environment.

Review harness changes as executable code: hooks, CI, checker exemptions, agent
instructions, and skills can weaken future verification. Verify negative cases
as well as a clean checkout. Do not introduce approval quotas or repeated clean
rounds without a demonstrated need.

Report actionable findings with trigger, consequence, file, evidence, and repair
direction. Separate reproduced failures from supported risks and design proposals.
Report exact checks and limitations. Do not edit or publish solely because a
review found an issue; follow the user's authorized scope.
