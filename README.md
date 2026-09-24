# BrewWarden

Check official Homebrew bottles before installing or upgrading them. BrewWarden
verifies the selected bytes and publisher, checks known vulnerabilities, and
applies a seven-day age policy to the exact bottle digest. It evaluates the
complete dependency plan before allowing Homebrew to run; missing required
evidence holds rather than becoming success.

```sh
bwd doctor
bwd brew install jq
bwd brew upgrade jq
```

The supported product path is **native Apple Silicon macOS Tahoe** with an
existing reviewed Homebrew installation and authenticated installed `gh`.
See the [current compatibility and eligibility matrix](docs/support.md) for
version bounds, unsupported platforms and required evidence. `doctor` checks
the environment, not individual package eligibility. BrewWarden does not
replace `brew` or intercept calls made directly to Homebrew.

## Use

Extract a verified distribution archive into a user-owned directory and add
its directory to `PATH`; keep `bwd` and its `brewwarden` alias together. The
archive does not bundle Homebrew, portable Ruby or GitHub CLI, and no Go
installation is needed to use the product. Local archives are not signed,
notarized or published releases. See [distribution](docs/distribution.md) for
build and trust requirements.

To change the minimum age or explicitly waive **only** the age rule for one
verified artifact:

```sh
bwd --minimum-release-age 336h brew install jq
bwd --age-exception 'jq=Urgent upstream fix' brew upgrade jq
```

Unknown or skipped evidence, including a missing verified timestamp, cannot be
waived. BrewWarden does not infer success or replay approval after an interrupted
or partial installation. See [usage](docs/usage.md) for login, configuration,
held operations and repair instructions. Previously installed payloads are not
cryptographically authenticated by invoking BrewWarden.

## Development

Use the Go version in `.go-version`, then:

```sh
./scripts/setup-hooks.sh
./scripts/verify.sh
./scripts/setup-tools.sh
./scripts/check.sh all
```

Development binaries are diagnostic-only until built as a distribution from
reviewed source. The [local product-readiness gate](scripts/macos-vm.md#local-product-readiness-gate)
uses reproducible builds and an authenticated disposable Apple Silicon VM; it
never mutates the host Homebrew installation. For implementation and review,
see [design](docs/design.md), [verification](docs/verification.md),
[architecture](docs/architecture.md), [threat model](docs/threat-model.md)
and [contributing](CONTRIBUTING.md).

Licensed under [MIT](LICENSE). Distributed dependencies retain their own licenses.
