# BrewWarden

Security policies and verification for Homebrew.

BrewWarden is an all-in-one CLI under design: release-age policy, artifact
and signature verification, vulnerability checks, dependency-aware installation
plans, emergency updates, and evidence history.

**Installation is disabled.** The initial CLI provides local help and a `doctor`
diagnostic that reports the unresolved execution-binding gate. It does not launch
Homebrew. Pure evidence eligibility rules exist, but live evidence collection
and installation enforcement are not implemented.
The intended interface prefixes supported commands:

```sh
bwd brew install wget
bwd brew upgrade
```

The planned behavior is to execute after checks pass, without an extra BrewWarden
prompt, and stop when checks fail. Currently both commands above fail closed.
`brewwarden` is the full executable name; `bwd` is its short form.
Plain `brew` calls are not intercepted. The public repository URL and license
are not yet set.

## Development

Use the Go version in `.go-version`, Git, and a POSIX shell. Full checks also
require a C compiler for race detection.

```sh
./scripts/setup-hooks.sh
./scripts/verify.sh
./scripts/setup-tools.sh  # Explicit download of pinned development tools
./scripts/check.sh all   # Includes advisory database access
go run ./cmd/bwd --help
go run ./cmd/bwd doctor  # Exits nonzero: execution binding is unverified
```

Optional Task v3 aliases are listed by `task --list`. See
[verification](docs/verification.md) for individual checks and requirements.

## Documentation

- [Product design](docs/design.md)
- [Contributing](CONTRIBUTING.md)
- [Architecture](docs/architecture.md)
- [Threat model](docs/threat-model.md)
