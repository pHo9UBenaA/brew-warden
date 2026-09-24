# Historical verification observations

Recorded: 2026-09-24. These cases are evidence for the revisions tested, not
current eligibility rules or a claim that every formula/version is safe. Local
readiness remains bound to each committed revision and its private gate record.

## Container probes

A pinned disposable OrbStack Linux arm64 container passed baseline and race
tests. The initial run revealed a fixture collision between the `brewwarden`
binary path and `XDG_CONFIG_HOME/brewwarden`; the test now separates executable
and user-state directories. The initial read-only container with a no-exec
scratch mount could not run Go's test binaries; the dedicated temporary
filesystem now allows test execution while the root stays read-only. Full-disk
fixtures use a separate 64 KiB `/full` tmpfs, not a host mount. No Linux run
constitutes native macOS Homebrew bottle acceptance.

## Native packaged execution

Authenticated disposable Apple Silicon Tahoe acceptance exercised packaged
`doctor`, fresh jq/oniguruma installation, multi-target zstd/jq with a bounded
lz4 age exception, unchanged reruns, xz 5.8.3 -> 5.8.4 explicit and all-installed
upgrade, and unaffected unrelated formulae. Fault cases covered age refusal,
changed bottle/metadata/cache/installed inputs, cancellation, parent death and
child exclusion, a real partial jq link failure, manual repair and a fully
fresh retry. An initial partially poured jq produced a false success on retry;
the linked-keg check and its regression test now reject that state.

An unauthenticated guest held before mutation. The coverage survey held `hello`
and `wget` because Homebrew skipped required advisory subjects, not because of
a formula allowlist or a determination that they were harmless. A separate
survey held `libpng` by evidence or age policy rather than installing it.
Authenticated acceptance installed `libuv` and `gmp` together under the default
policy, then verified an unchanged rerun and unrelated state preservation.
Homebrew/core recipes at `98cbed4ac9edc9e40b82ac087ddd95bd2bd2e751` have
`libuv` using a `dist.libuv.org` CMake source and `gmp` using a GNU mirror with
Autotools. These different source/release conventions exercise the same
bottle-only path; source identity is not product eligibility evidence.

The separate review records in this archive identify the historical revision
and cohort of each gate. In particular, an authenticated 17-case gate on
Homebrew 7.0.6 passed with installed gh 2.101.0 at `cf86592` and gh 2.66.0 at
`df5eb49`. Those results do not carry over to later commits. None signs or
notarizes a public release, modifies the host Homebrew, or proves an absence
of vulnerabilities.
