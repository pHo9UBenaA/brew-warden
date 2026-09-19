# brew-security

An all-in-one Homebrew security CLI under design: release-age policy, artifact
and signature verification, vulnerability checks, dependency-aware installation
plans, emergency updates, and evidence history.

**Product behavior is not implemented yet.** The repository contains the design
and development harness. License and public repository identity are not yet set.

## Development

Use the Go version in `.go-version`, Git, and a POSIX shell. Full checks also
require a C compiler for race detection.

```sh
./scripts/setup-hooks.sh
./scripts/verify.sh
./scripts/setup-tools.sh  # Explicit download of pinned development tools
./scripts/check.sh all   # Includes advisory database access
```

Optional Task v3 aliases are listed by `task --list`. See
[verification](docs/verification.md) for individual checks and requirements.

## Documentation

- [Product design](DESIGN.md)
- [Contributing](CONTRIBUTING.md)
- [Architecture](docs/architecture.md)
- [Threat model](docs/threat-model.md)
