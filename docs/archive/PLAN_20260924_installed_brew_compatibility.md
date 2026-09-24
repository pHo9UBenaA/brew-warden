# Expand support for existing Homebrew installations without weakening binding

Date: 2026-09-24. Status: investigated; additional product compatibility is **not
implemented or accepted** by this document. `docs/design.md` remains the
product contract. This is a bounded investigation and an implementation/acceptance
plan, not an authorization to remove version, prefix or verifier checks.
Follow-up user direction on 2026-09-24 supersedes this plan's x86_64/Rosetta
support proposals: retain native arm64-only product support, do not modify the
host Homebrew installation, and treat the x86_64 observations below as a
read-only explanation of why that host installation is not supported. The
verified-timestamp version analysis is in a separate archived review.
The per-tuple full-VM requirement below is also superseded by
`PLAN_20260924_arm_compatibility_ranges.md`: source deltas and adapter
contracts cover version cohorts, with representative native cases for
materially different execution behavior rather than one full VM per patch.

## Read-only integration probes

On the maintainer's Apple Silicon MacBookPro17,1 (macOS 26.6.2, zsh 5.9),
`PATH` finds `/usr/local/bin/brew` and `/usr/local/bin/gh`. Public `brew
--prefix`, `brew --repository`, and `brew config` report Homebrew 7.0.6 at
`/usr/local`, executable source at `/usr/local/Homebrew`, cellar at
`/usr/local/Cellar`, **x86_64 under Rosetta 2**, and signed API cache
`packages.tahoe.jws.json`. The installed portable Ruby is x86_64. Installed
gh is x86_64 2.62.0. The `brew config` probe used a private HOME/cache and
`HOMEBREW_NO_AUTO_UPDATE=1`; no install, upgrade, symlink or host-prefix
mutation was performed. This environment is not the previously accepted
Apple Silicon arm64 Tahoe `/opt/homebrew` + Homebrew 7.0.4 + gh 2.101.0 VM
configuration.

At Homebrew revisions `edb70f031e4170c780799633a1226ff73e1077f4`
(7.0.4) and `570982948a8a194f0f42f43f4a5bce2d1c9f64cb` (7.0.6), the
checked `cmd/vulns.rb` and `vulns/scanner.rb` source files are unchanged, but
`formula_installer.rb`, `formulary.rb`, `keg.rb`, the sandbox and vulnerability
matching/history components changed. Upstream source diff is not acceptance of
all their behavior. Homebrew's `bin/brew` and `brew.sh` distinguish repository,
prefix and cellar; these cannot be derived by replacing `/opt/homebrew` in one
string. The existing code also fixes the native inspection architecture,
`arm64_tahoe` bottle tag and signed-cache name, eligible `cellar` values,
receipt/link paths, sandbox permissions and frozen plan's prefix.

The installed gh 2.62.0 accepts the public `attestation verify --format json`
syntax, but its upstream `v2.62.0` verify help does not state the later
`verifiedTimestamps` trust contract. Later source inspection established the
specific incompatibility: gh 2.62.0's sigstore-go 0.6.2 serializes even a
verified Tlog time with `uri: "TODO"`. gh 2.66.0 first includes sigstore-go
0.7.0 with a verified Tlog URI; gh 2.70.0 first documents this JSON contract.
These milestones do not establish a safe continuous product version range. The product uses verified Rekor time for
the **exact digest**; a process exit code, unsigned JSON or an attestation
predicate's time cannot replace it. No guest or host credential was copied or
used for this investigation. Sources inspected: the locally installed official
[Homebrew checkout](https://github.com/Homebrew/brew/compare/edb70f031e4170c780799633a1226ff73e1077f4...570982948a8a194f0f42f43f4a5bce2d1c9f64cb)
and GitHub CLI tagged verify sources at
[`v2.62.0`](https://github.com/cli/cli/blob/v2.62.0/pkg/cmd/attestation/verify/verify.go),
[`v2.80.0`](https://github.com/cli/cli/blob/v2.80.0/pkg/cmd/attestation/verify/verify.go)
and [`v2.101.0`](https://github.com/cli/cli/blob/v2.101.0/pkg/cmd/attestation/verify/verify.go).

## Compatibility dimensions and required change

1. **Discovery:** Resolve the user's selected `brew` from `PATH` to a stable
   absolute executable; obtain `brew --prefix`, `brew --repository`, `brew
   --cellar` and the process's actual OS/architecture under a private read-only
   environment. Reject inconsistent paths, symlink escapes, ambiguous binaries,
   unsafe directories, unsupported arch combinations and changes between
   discovery, collection and execution. `PATH` is selection, not verification.
   Never silently install, update or redirect Homebrew or gh.
2. **Owned Homebrew adapter:** Introduce a single reviewed installation identity
   carrying executable, repository, prefix, cellar, architecture, OS/bottle tag,
   and supported runtime identity. Replace fixed paths in the adapter's runtime
   copy/inspection, sandbox, fetch, signed metadata, public state/receipt and
   linked-keg checks, command execution, plan environment and revalidation.
   Maintain an empty inspection prefix and complete candidate-closure checks.
   Scope operation locks and state to the selected installation. Unknown signed
   metadata paths, bottle tag/cellar, skipped advisory subjects and unexpected
   installer effects must hold, not fall through to a plain `brew` command.
3. **Version compatibility:** Review each Homebrew cohort's actual signed API,
   bottle resolution, OSV scanner skip contract, installer sandbox/no-network
   behavior and linking/partial-state semantics against upstream source and
   tests. Prefer verified capability checks over an unbounded version wildcard;
   retain an explicit supported-version ceiling unless newer behavior is
   demonstrated. For gh, inspect trust-root and verification changes per
   candidate release, then exercise real `--repo`, signer/subject and all
   verified-timestamp outcomes. Negative tests must catch exit-zero with missing
   evidence. A broad `2.x` acceptance rule is not a substitute for this work.
4. **Native acceptance per tuple:** Keep the already-tested arm64 Tahoe
   `/opt/homebrew` case as a regression. Exercise 7.0.6 and additional reviewed
   versions in fresh disposable macOS VMs: native arm64 prefix, and separately
   `/usr/local` x86_64/Rosetta or a real Intel Mac. Do not mutate the host
   installation for tests. Confirm signed snapshot acquisition, exact bottles,
   full dependency plan, advisory skip/age holds, install/upgrade, source
   fallback refusal, tampering, partial links, interruption and fresh retry.
   A previous version's VM result cannot certify the new revision or tuple.
5. **Additional platforms:** Intel macOS, Linux, casks, taps and source builds
   have different commands, bottle tags, sandboxes and policy claims. Linux
   cannot reuse `/usr/bin/sandbox-exec`. Enable each only with its own upstream
   guarantee record and actual isolated native acceptance. Do not promise every
   `brew` installation is supported merely because it is discoverable on PATH.

## Acceptance and stopping conditions

First add isolated negative tests that show a selected executable, repository,
prefix, cellar or architecture changing after verification stops mutation. Test
unrecognized versions and gh output that omits trusted time. Before changing
execution, run the integration probes in `docs/design.md` and the upstream
review in `docs/homebrew-integration.md`; keep the owning adapter record current.
Use `./scripts/verify.sh`, `./scripts/check.sh all`, reproducible distribution
builds, and full authenticated local VM acceptance **for each new supported
execution tuple on the final clean commit**. Do not treat compilation, a
preview, a doc comparison, a read-only `doctor`, or a clean vulnerability feed
as execution-binding proof. Commit only validated behavior; otherwise leave the
unsupported configuration held and report the exact missing environment or
evidence. Public release signing/notarization is a separate decision.
