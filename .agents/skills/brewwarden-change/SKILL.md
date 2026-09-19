---
name: brewwarden-change
description: Implement brewwarden behavior or architecture changes with evidence for policy decisions and adapter boundaries. Use for features and fixes, not routine prose edits.
---

# Implement a bounded change

Read [architecture](../../../docs/architecture.md) when ownership or imports
change, and [the product design](../../../DESIGN.md) for policy behavior.

Identify the owning layer and a realistic input that distinguishes the desired
behavior. For a defect, observe that example failing for the intended reason;
unresolved imports and broken setup are not reproductions. Fix the cause without
weakening the expectation. For new behavior, establish the contract first.

Pass time and evidence explicitly into the core. Exercise verifier, subprocess,
filesystem, and Homebrew adapters separately where fake ports hide the risk.
Emergency behavior must leave integrity and trust requirements intact. A saved
allow decision is not authorization to skip revalidation.

Run affected checks, then `./scripts/verify.sh`. For architecture gate changes,
include resolvable forbidden edges and a valid graph. Keep docs and diagnostics
in English. State what was tested and what remains unverified. This skill does
not authorize changing the host's Homebrew installation or publishing changes.
