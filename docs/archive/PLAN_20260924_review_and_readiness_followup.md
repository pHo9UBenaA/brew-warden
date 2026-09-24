# Follow-up: bounded security review and exact-revision local readiness

Date: 2026-09-24. Status: follow-up worklist; the exact-revision gate result
must be recorded separately. This document does not itself establish product
readiness. Current product rules remain in `docs/design.md`; operational checks
and their limits remain in `docs/verification.md`.

## Starting point and scope

- The public-command simplification plan is implemented for its initial tested
  Apple Silicon Tahoe / official-core bottle scope. The authenticated local
  product-ready gate passed for commit `d91ec04`, not for subsequent commits.
- Commit `dede963` fixed a real harness defect: a nonexistent attestation fuzz
  target previously returned success. It added attestation and complete-closure
  fuzz properties. Commit `2855675` separated build-tagged VM acceptance into
  `tests/vm/`. Offline full checks, tagged test compilation/vet/lint/vulnerability
  analysis, and reproducible double builds passed after these changes. The
  latest commit has **not** passed the full authenticated VM gate.
- The first subsequent gate attempt held on a guest-only directory sync error
  (`resource busy`) during `doctor`; a fresh complete `doctor` in that same guest
  succeeded. The next clone passed preparation, but its GitHub device login
  expired without approval. Both disposable guests were stopped, and neither
  attempt produced a product-ready result. This is not evidence of an execution
  bypass or a successful gate for `2855675`.
- A passing suite cannot prove an absence of security flaws. Review only the
  supported threat-model boundary; preserve conservative holds for missing
  evidence. Do not silently expand the supported Homebrew, gh, platform, tap,
  source-build or cask scope.

## Work 1: close the review with actionable evidence

Trace one candidate and its entire dependency closure through authenticated
Homebrew metadata, actual bottle digests, verified gh attestations/timestamps,
`brew vulns` subject coverage, Homebrew advisory attribution, pure policy,
short-lived plan and age waiver, frozen inputs, public-command execution, and
observed after-state. Inspect the owning adapter contracts and the inspected
upstream Homebrew/gh behavior at their supported versions. Specifically look for
missing-to-success conversions, implicit scanner skips, exceptions that waive
more than age, input changes between evaluation and execution, parent death,
partial pours, and skipped tests reported as passing.

Record each reproduced finding with trigger, consequence, exact file, test or VM
observation, and repair direction. Separate confirmed defects from trust-model
limits: direct `brew` calls, independent concurrent mutations, trusted installed
kegs, incomplete real-world advisory knowledge, and publisher authentication.
Do not claim "no holes" based solely on passing checks. If a product behavior
change is justified, first reproduce its intended failure in an isolated test,
check the actual upstream guarantee under `docs/homebrew-integration.md`, fix
only the owning boundary, and renew affected native acceptance. Do not change
host Homebrew.

## Work 2: address demonstrated readiness-harness failure handling

The `start` phase of `scripts/product-ready.sh` can leave a newly cloned guest
running if `vm-prepare` fails after cloning. Make failure cleanup explicit and
fail-closed: recognize only the VM created by this invocation, retain its local
failure log, remove any guest-only credentials if reachable, and stop that clone.
Never delete the base or another user's VM. If the guest agent cannot be reached,
report the unresolved cleanup and do not assert readiness. Handle expired device
approval with a fresh, human-approved code or a clear cancellation path; never
copy host credentials or reuse an old authorization as verification evidence.

Test the concrete after-clone failure, no-auth/expired-auth refusal, cleanup
failure, and success path using isolated driver fixtures and, where feasible, a
fresh disposable VM. Distinguish a transient filesystem `resource busy` from an
integrity failure: a retry is acceptable only if it repeats the entire `doctor`
check from fresh private scratch space and remains bounded; do not suppress a
failed sync or turn an unsupported runtime into success. Avoid extra dependencies
or adding VM runs to CI.

## Work 3: keep test ownership and properties explicit

Retain package-local adapter tests, root `tests/` external policy/application/CLI
contracts, and opt-in `tests/vm/` native acceptance. Their assertions may overlap;
strictly exclusive coverage is not a security goal. Do not duplicate shared
policy fixtures solely to move files. Confirm normal `go test ./...` excludes
VM-only cases while the tagged local runner compiles **and actually executes**
them. Keep the missing-fuzz-target regression test effective. Add further
unit/property cases only for a specific uncovered invariant or a reproduced
failure, not for an arbitrary coverage percentage. Normal coverage does not
measure opt-in native execution; report the two scopes separately.

## Work 4: commit and run the local readiness gate on the final revision

1. Run `./scripts/verify.sh` and `PATH=<pinned Go> ./scripts/check.sh all` for
   changes above; inspect staged/unstaged/untracked files and commit focused
   Conventional Commits. Do not bypass signing, hooks or failing checks.
2. From the final clean commit, use `scripts/product-ready.sh start BASE NEW_VM
   REVIEWED_BREW_TREE GH_ARM64`. It must run the pinned baseline, race, coverage,
   seven real fuzz targets, Staticcheck, govulncheck, tagged VM test
   vet/lint/vulnerability/compilation, two-build reproducibility and fresh
   `doctor`. No implicit tool installation or host Homebrew mutation.
3. Approve the short-lived `gh` device code **inside the disposable guest** and
   run `scripts/product-ready.sh complete NEW_VM`. The complete local suite must
   exercise real install/upgrade, dependency closure, age holds/exceptions,
   refusals, partial outcomes, interruption/parent death and independent
   source/build conventions. A survey's intentional hold is not an install
   success. Do not infer success from a skipped test, old gate result, or an
   interrupted session.
4. Verify the guest credentials were removed and the VM stopped. Report the
   exact commit, archive SHA-256, actual checks, native cases, intentional holds
   and limitations outside ignored `.cache`. Human account-level GitHub CLI OAuth
   revocation is separate from guest-local logout. Do not change HEAD after a
   successful gate without rerunning it for the new revision.

**Completion condition:** only a clean final commit with no unresolved
in-scope finding, passing full checks and a completed authenticated local gate
for that **same** commit may be called "locally product-ready for the tested
initial scope." Missing approval, quota, network evidence, cleanup or a test
means not ready; retain diagnosis rather than bypassing the check. Public
release signatures, notarization, publishing, new platforms and universal
formula safety are separate decisions and are not implied by this plan.
