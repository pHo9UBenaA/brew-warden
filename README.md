# BrewWarden

Check Homebrew packages before installing them. BrewWarden verifies bottle
checksums and publisher provenance, checks known vulnerabilities through
Homebrew's public commands and advisory data, and waits seven days from the
oldest verified attestation for the exact bottle digest. New bytes start a new age.

```sh
bwd doctor
bwd brew install jq
bwd brew upgrade jq
```

The public-gh migration has not passed native Apple Silicon acceptance or
removed the legacy distribution/state workflow; do not treat it as product-ready.
All required checks finish before installation starts. Missing or unsupported
evidence stops the command. Successful checks lead directly to ordinary public
`brew install` or `brew upgrade`, using the verified downloads and dependency plan.

## Install

Extract a verified distribution archive into a user-owned directory and add that
directory to `PATH`. Keep `bwd` and its `brewwarden` alias together.
Users need an existing supported Homebrew installation and an installed supported
`gh` (currently 2.101.0), but not Go. BrewWarden does not install or update `gh`.
Homebrew, Ruby and the attestation verifier are not bundled. BrewWarden
checks the installed Homebrew tree before copying it to an isolated inspection
prefix; an unsupported or changed implementation holds execution.

The current target is **Apple Silicon macOS Tahoe**, with Homebrew at
`/opt/homebrew`. See the [Homebrew contract](internal/adapters/homebrew/README.md)
for the tested version and boundaries, and [distribution](docs/distribution.md)
for building and verifying an archive. Local build results are not published or
notarized releases.

Official core bottles are eligible when their complete dependency plan has the
required evidence. There is no package-name allowlist. Casks, third-party taps,
source builds and unsupported evidence are held. An empty advisory result means
no known applicable findings, not proof that the package is harmless. Homebrew's
scanner does not cover every formula; an unsupported dependency also holds its
parent. `doctor` checks the environment, not individual package eligibility.

Do not run another command that changes the same Homebrew installation while
BrewWarden is running. BrewWarden does not monitor or intercept ordinary `brew`
commands. It does not claim to authenticate files previously installed outside
its own verified execution.

## Policy and history

Set a different minimum age or make an explicit, one-attempt age exception:

```sh
bwd --minimum-release-age 336h brew install jq
bwd --age-exception 'jq=Urgent upstream fix' brew upgrade jq
bwd history
bwd status
```

An age exception waives only a verified but too-young timestamp. Missing or
invalid timestamps, checksums, provenance, advisory checks and dependency
binding cannot be waived. Dependencies need their own exception reasons. Formulae and versions are
shown before execution; exceptions also show the exact digest and reason.
`bwd brew upgrade` without names checks the installed formula inventory.

Optional policy configuration lives at
`~/Library/Application Support/brewwarden/config.json`:

```json
{"schemaVersion":1,"age":{"minimumHours":168}}
```

Use `--config PATH` to select another policy file. It does not redirect history.
Verification records are private and retained beside the default configuration.
Collections can be large; preserve records while an attempt is unresolved.

If `status` reports an interrupted attempt, run `bwd reconcile`. It waits for
BrewWarden's owned operation to stop and records the actual selected-package
state. It does not rerun installation, invent a successful exit or roll back
packages. A fresh command performs fresh checks. Inconsistent installations may
need ordinary Homebrew repair while BrewWarden is idle.

## Development

Use the Go version in `.go-version`, then:

```sh
./scripts/setup-hooks.sh
./scripts/verify.sh
./scripts/setup-tools.sh
./scripts/check.sh all
```

Development binaries are diagnostic-only until built with a trusted distribution
inventory. Mutation acceptance requires a disposable macOS VM; tests never change
the maintainer's Homebrew installation. See [verification](docs/verification.md),
[design](docs/design.md), [architecture](docs/architecture.md),
[threat model](docs/threat-model.md) and [contributing](CONTRIBUTING.md).

Licensed under [MIT](LICENSE). Distributed dependencies retain their own licenses.
