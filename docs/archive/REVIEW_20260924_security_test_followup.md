# Scoped security and test follow-up

Date: 2026-09-24. Review scope: the supported Apple Silicon Tahoe,
Homebrew 7.0.4, installed gh 2.101.0, official-core bottle path. This is a
review record, not a change to `docs/design.md` or a claim that tests establish
an absence of vulnerabilities.

## Traced boundary

- `internal/adapters/homebrew/collect.go`, `metadata.go`, `vulns.go` and
  `advisories.go`: signed candidate and closure identity, downloaded-bottle
  digest, explicit scanner subjects, skipped subjects, attributed formula
  advisory applicability, and missing-evidence holds. The pinned Homebrew
  `Scanner#scan` partitions its requested formulae into queryable subjects and
  `skipped_formulae`; its JSON output does not expose `checked`. The adapter
  checks the skip list and the requested closure under that pinned guarantee,
  rather than treating an arbitrary empty JSON response as independent proof
  that all formulae are safe. A new scanner version needs a fresh review.
- `internal/adapters/attestation/gh.go` and `result.go`: the actual bottle bytes
  and installed verifier are hashed before and after the pinned public `gh`
  invocation. Every accepted result must have a matching verified signer,
  subject/digest and Rekor timestamp; saturated, missing or future timestamps
  hold, including when bottle age is waived.
- `internal/domain/decision.go`, `internal/adapters/homebrew/session.go`,
  `public_session.go`, `inflight.go`, and `internal/application/execute.go`:
  the full reachable graph and exactly one current claim of each kind are
  evaluated; an age exception does not repair integrity or advisory failure.
  Prepared plan, frozen inputs and observed installed state are revalidated
  before a one-use public command. An incomplete link, lost parent or
  unobservable owned process is not reported as successful execution.
- `tests/policy_fuzz_test.go`, `internal/adapters/attestation/gh_test.go`,
  Homebrew boundary tests and `tests/vm/` exercise the above at different
  boundaries. Baseline tests do not execute the tagged VM suite. The named fuzz
  targets are checked for actual existence before active fuzzing.

## Findings and limits

No reproducible in-scope product execution bypass was found in this pass. One
**reproduced readiness-harness failure** remained: when guest setup failed
*after* Tart created the clone, `prepare` exited without stopping that clone.
An isolated driver test failed against the prior implementation. The runner now
stops only its own successfully created clone, attempts guest credential cleanup
when reachable, keeps the private failure log and reports unresolved cleanup.
An expired device approval cannot be treated as a completed gate; a pending run
can be explicitly cancelled without stopping an unrelated VM. The auth restart
clears the previous device log before requesting a fresh code. These are harness
repairs, not evidence of a product execution bypass.

Residual scope limits remain: independent unwrapped `brew` operations are not
intercepted; a user-owned existing keg is an observation, not a payload-hash
proof; vendor-authenticated attestations and known advisory feeds do not prove
benign code or complete vulnerability knowledge; signing, notarization and
public distribution are separate. Source inspection, unit/property tests and
one native suite on an earlier revision do not transfer local product readiness
to the revision containing these changes. Record the final revision's actual
gate result separately after human-approved guest authentication.
