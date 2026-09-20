# Contributing

Use English for repository content and commit messages. Keep changes focused,
prefer standard-library implementations, and document any added direct,
transitive, runtime, native, or build dependency. Do not replace maintained
cryptographic verification with a custom protocol to achieve zero dependencies.

## Workflow

Read the contract relevant to the change; keep behavior at its owning boundary.
Follow [implementation rules](docs/dependencies.md) for dependencies and input
handling. Run `./scripts/verify.sh` during edits and `./scripts/check.sh all`
before requesting review; report unavailable checks accurately. See
[verification](docs/verification.md) for coverage and known gaps.

Keep current documentation focused on actionable contracts. Store historical
research and decision rationale under `docs/archive/`, without adding routine
README or agent-context links. Do not add tests solely for prose changes.

## File inclusion

The repository ignores unlisted root entries with `/*`, then allows public root
files and entire source, test, documentation and tooling directories. New files
inside these directories are eligible for tracking without changing `.gitignore`.
Only `.agents/skills/` is included from `.agents/`; other local agent configuration
remains ignored. Generated root directories such as `.cache/`, `bin/` and `dist/`
remain excluded by default.

Update `.gitignore` when introducing a new public root entry, not for each source
file. Keep generated artifacts and private local files outside the included trees,
or add a targeted ignore rule when a tool must generate them there. Avoid
`git add --force` as a substitute for correcting the inclusion rules.

Use `git status --short --untracked-files=all` to inspect included files and
`git check-ignore -v --no-index <path>` to inspect the matching rule for a path.
Ignore rules do not protect already tracked files or prevent forced additions;
always inspect the staged diff for unintended content before committing.

## Git hooks and commits

Run `./scripts/setup-hooks.sh` once per clone. It sets the repository-local
`core.hooksPath` to `.githooks`; it refuses to overwrite a different existing
setting. No global configuration is changed. Hooks run repository code: inspect
it before enabling hooks in an unfamiliar checkout.

- `pre-commit` runs the baseline offline verification suite against the worktree.
  To avoid testing a different staged version, it refuses partially staged
  tracked files. It does not automatically format or stage files.
- `commit-msg` validates the message without rewriting it.
- `pre-push` verifies the worktree and checks commit subjects reachable from the
  pushed tip but not the remote tip. A newly published ref checks its complete
  reachable history. Deletes are skipped; unavailable remote objects fail with
  instructions rather than silently reducing the range. Commits shared with
  other remote refs may be checked again.

Use this repository's stricter
[Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) subset:

```text
feat(policy): add artifact-specific age exceptions
fix(verifier)!: reject ambiguous signing identities

Explain the concrete behavior and validation when useful.

BREAKING CHANGE: describe the changed public contract
```

Allowed types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`,
`ci`, `chore`, `revert`. Type and optional scope are lowercase; scopes use letters,
digits, dots, slashes, underscores, or hyphens. A nonempty description is required;
subjects are at most 100 Unicode code points. Separate a body from the subject
with a blank line. `!` and `BREAKING CHANGE:` / `BREAKING-CHANGE:` are supported.
This small validator is not a complete commitlint replacement or a semantic
English checker. Merge, revert, fixup, and squash commits are not exempt: rename
or squash temporary commits before publication.

Local hooks are bypassable. The checked-in CI workflow repeats offline checks
and commit-message validation. Configure required checks in repository settings
when a remote exists; no server-side protection is claimed by this scaffold.

## Public harness

`AGENTS.md` and `.agents/skills/` are intentionally public. Keep private prompts,
personal paths, and assets out of them.

Changing hooks, architecture rules, CI, or agent instructions requires the same
review as code because those files execute or direct future work. Never bypass a
failing gate simply to make a change pass.
