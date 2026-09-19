# Repository instructions

- Use English for all repository content. Match the user's language in conversation.
- Preserve user changes. Do not modify the host's Homebrew installation in tests;
  use isolated environments. Follow user authorization for commits and publishing.
- Product behavior follows [DESIGN.md](DESIGN.md). Keep execution unavailable
  until verified artifacts and the complete dependency plan are bound to execution.
  Unknown evidence never means success; emergency mode waives only bounded age
  conditions, never integrity, trust, or plan binding.
- For ownership/import changes, read [architecture](docs/architecture.md); for
  security-sensitive behavior, read [threat model](docs/threat-model.md); for
  dependencies or input handling, read [implementation rules](docs/dependencies.md).
- Use `.agents/skills/brew-security-change` for behavior/boundary changes and
  `.agents/skills/brew-security-review` for consequential reviews.
- Test behavior with realistic failure cases. Architecture fixtures must resolve
  their target packages and fail for the intended forbidden edge. Do not add
  tests that merely mirror documentation or implementation.
- Run `./scripts/verify.sh`; report actual checks and limitations. See
  [CONTRIBUTING.md](CONTRIBUTING.md) for commits, hooks, and file inclusion.
- Keep current rules in one authoritative document. Put historical investigations
  and decision rationale in `docs/archive/`; do not load or link them from routine
  entry points unless the task requires revisiting that decision.
