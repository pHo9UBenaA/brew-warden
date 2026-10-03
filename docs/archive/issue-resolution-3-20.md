# Resolution work for Issues #3–20

Base: `494ed92c6650f8c9ca97e95958d3b335d23a323c`. Implementation commit:
`23b2902ddb181e75b621dbf360b18a46c40297af`. The initial verification below
preceded native acceptance and issue closure. The follow-up records readiness
only for that exact tested revision; nothing was pushed or published.

## Scope and ownership

- #3: archive current worktree files with NUL-safe names, omitting unstaged
  deletions while retaining dangling symlinks. A temporary Git/archive fixture
  checks modified, new, deleted and ignored files without running Docker.
- #4: package the linked VM Markdown guide, not VM tooling. Check relative
  Markdown document links against the actual generated archive inventory.
- #5: preserve safe cancellation, deadline, output-limit and command-exit
  classifications after workspace removal, without exposing stderr or saving
  execution history.
- #6: the three Task descriptions were already corrected at the base revision;
  `task --list` agrees with the owning check script. No redundant edit was made.
- #7: give active CLI refusal tests a clock and require `invocation_invalid`.
  Retain control-character output safety, wrapper-option rejection and the
  independent binary/PATH and no-state-creation contracts.
- #8: execute guest log filtering and file removal on temporary fixture files,
  rather than returning canned outcomes. Reach the unauthenticated-gh boundary
  with a reachable guest and retain owned-clone cancellation checks.
- #9: isolate OCI digest and Homebrew-open advisory refusals from unrelated
  negative inputs. The in-flight fixture already started from a complete record;
  require its invalid-record diagnostic instead of accepting later failures.
- #10: consolidate equivalent captured formats and the old-gh authentication
  subprocess into existing boundary tests. Remove the unused `brew verify`
  credential probe. Duplicate lock and changed-attempt checks were already
  consolidated at the base revision and remain intact.
- #11: use an independent known checksum and hold plan ID fixed when checking
  policy, graph, environment and waiver identities. Separately retain serialized
  installed-state binding and propagation of changed plan ID into exception ID.
- #12: require schema 3 and a reviewed Homebrew revision for live private plans.
  No legacy-plan import or replay mechanism was introduced.
- #13–15: validate the collected closure at its consumer, carry the checked gh
  version into evidence, and remove inactive localstate/scanner scaffolding.
  Selected attestation-object reuse and the single result-limit constant were
  already implemented at the base revision; their checks remain intact.
- #16: remove repeated architecture/history narration while preserving the exact
  candidate presentation and complete-session cleanup contracts. The orphan
  domain journal comment was already removed at the base revision.
- #17: bind public `installed_on_request` observations and root install actions.
  Use the existing confined public install to promote current dependency-only
  roots, then require ownership in the final observation. No receipt editing or
  second installer was added. A tagged packaged-binary VM case checks promotion,
  unchanged payload/time, and unchanged non-root dependency ownership.
- #18: choose gh's explicit private-file credential storage for fresh disposable
  guests. Retained legacy storage, logout failure or removal failure cannot
  certify cleanup. Stop pending device login before deleting files; still attempt
  VM stop on cleanup failure. Fresh and empty-config guests are legitimate cases.
- #19: pass caller context through materialization, Git inspection and execution
  comparison. Cancellation cannot select the legacy fingerprint fallback. File
  boundaries observe cancellation; blocking filesystem syscalls are not promised
  to be immediately interruptible. Cleanup/liveness contexts remain independent.
- #20: clarify the two existing skills' affected scope, readability, regression
  value, authoritative documentation ownership and complete-diff review criteria.
  Names, triggers, fail-closed guarantees and host/publishing restrictions remain.

The output-limit regression for #5 also reproduced a real boundary bug:
embedding `bytes.Buffer` exposed `ReadFrom`, allowing subprocess `io.Copy` to
bypass the bounded `Write`. Both Homebrew and gh now contain a private buffer;
real child stdout/stderr overflow cases verify the bound.

## Upstream and capture evidence

Homebrew's reviewed 6.0.19 and 7.0.6 `install/check.rb` promote
`installed_on_request` even when installation is unnecessary; `formula.rb`
exposes that flag in public info. The 7.0.6 source was compared byte-for-byte
with upstream commit `570982948a8a194f0f42f43f4a5bce2d1c9f64cb`.
GitHub CLI login/config at `0cf1092493af067646fc5f3db9421c6a6ec9c938`
explains both the explicit `--insecure-storage` choice and why default-store
logout success is insufficient proof of Keychain deletion. The login option
also exists at the admitted 2.66.0 endpoint. Current contracts stay with the
Homebrew adapter and VM guide, not this record.

The retained Homebrew 6.0.19 info/scanner pair is byte-equivalent to removed
scanner captures; info differs only in ignored `tap_git_head`. The retained gh
2.66.0 and 2.101.0 results cover both observed output shapes and UTC/offset times.
Consumed signer, statement/subject and timestamp fields of the four removed
intermediate gh captures were compared with the retained representative.
Original captures and historical provenance remain available at the base
revision and in the existing archived cohort investigation. The old runtime
fixture-checksum assertion was consciously removed: replay tests do not
cryptographically authenticate captured JSON.

## Executed checks and limits

Using the existing Go 1.27.1 toolchain and pinned development tools:

- `./scripts/verify.sh`: formatting, hygiene, architecture, dependency checks,
  shell syntax, vet and shuffled Go tests passed.
- `./scripts/check.sh all`: baseline, race, coverage, all seven fuzz targets,
  Staticcheck, source/test vulnerability scan and all three binary scans passed.
  No vulnerability findings were reported. Coverage is not an acceptance target.
- Tagged VM tests passed vet and Staticcheck and cross-compiled for darwin/arm64.
- The opt-in offline Git source matrix passed for all eleven reviewed releases,
  including copied-runtime binding and extra-source rejection in isolated clones.
- Eleven temporary-copy mutation probes were detected by their intended tests:
  missing packaged guide, incorrect checksum, constant policy binding, bypassed
  CLI age validation, broken device-log filter, removed credential deletion,
  ignored logout failure, missing ownership promotion, detached runtime context,
  discarded command-failure reason, and a second gh version probe. Compilation
  failures were not counted as mutation detection.
- Skill frontmatter/relative links, Task descriptions and the complete diff were
  inspected. The user's unrelated `.gitignore` edit was preserved and excluded.

At the initial implementation commit, native authenticated Homebrew execution
and the fresh private-file login/cleanup flow had not yet been exercised. The
offline/source/shell checks did not modify host Homebrew or host authentication
and were not treated as a replacement for native release evidence.

## Authenticated native follow-up, 2026-10-03

After the user approved guest-only device authorization, the complete local
readiness gate passed for `23b2902ddb181e75b621dbf360b18a46c40297af`:

- Artifact: `brewwarden-darwin-arm64.tar.gz`, SHA-256
  `6ad228661dcdededdfd6c07937f6a4b651df0786299757e55e6b02214c2ec2b9`.
- Runtime: reviewed Homebrew 7.0.6 at
  `570982948a8a194f0f42f43f4a5bce2d1c9f64cb`, installed gh 2.101.0,
  disposable Apple Silicon Tahoe VM `brewwarden-issues-17-18-02`.
- A clean detached worktree preserved the user's unrelated `.gitignore` edit.
  The initial cache-root symlink was refused by hygiene before VM creation;
  replacing it with a real ignored cache directory allowed normal checks.
  No check was bypassed.
- `product-ready.sh start` passed full offline/race/coverage/fuzz/lint/vulnerability
  checks, tagged native vet/lint/vulnerability checks and arm64 compilation,
  two matching distribution builds, and guest preparation/doctor.
- `product-ready.sh complete` passed the entire fixed-order native suite:
  18 tagged test invocations, no failures or skips, plus the packaged commands.
  Survey completion was not interpreted as eligibility for every surveyed bottle.
- For #17, `TestLiveDistributionPromotesExplicitInstallOwnership` passed against
  the packaged binary. An unchanged jq install retained dependency-only
  oniguruma ownership; explicit oniguruma install promoted `installed_on_request`
  while preserving payload bytes and installation time. Install/upgrade,
  refusal, exception, interruption, partial-link, parent-death and fresh-retry
  cases also passed.
- For #18, real guest gh device login used explicit private-file storage. After
  the suite, pending-login cleanup, logout, credential/config/log removal and
  Tart stop succeeded. An independent Tart inventory showed the retained VM
  stopped. The gate wrote its readiness record only after successful cleanup,
  at `2026-10-03T12:08:17Z`.

Private evidence is retained under the detached worktree's ignored
`.cache/product-ready.brewwarden-issues-17-18-02/` directory. This is native
private-file credential cleanup evidence, not proof of deleting a legacy
Keychain entry. OAuth grant revocation remains an account-owner action; no host
credentials were forwarded to the guest and no host Homebrew prefix was changed.

The user authorized issue closure: #3–16 and #19–20 were closed after their
respective checks; #17–18 were closed after native acceptance. GitHub reported
no remaining open issues. The later documentation-only commit recording these
results does not inherit the tested revision's readiness certification.
