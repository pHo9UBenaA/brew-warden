# Using BrewWarden

BrewWarden is a prefix command, not a replacement for `brew`. Extract a
verified distribution archive into a user-owned directory, keep `bwd` and its
`brewwarden` alias together, and add that directory to `PATH`. The archive is
not itself a published, signed or notarized release; see
[distribution](distribution.md) for build and trust requirements.

An existing supported Homebrew and GitHub CLI are required; Go is not required
to use the product. Check the [supported scope](support.md) before attempting a
mutation. `gh auth status` checks whether the installed CLI is authenticated
to github.com; if not, complete the normal `gh auth login` flow. BrewWarden does
not install or authenticate `gh`, read host credentials into a VM, or silently
fall back to unverified execution.

```sh
bwd doctor
bwd brew install jq
bwd brew upgrade jq
bwd brew upgrade
```

`doctor` is read-only and checks platform/runtime support, not the eligibility
of individual bottles. Install and upgrade evaluate every requested target and
its complete dependency closure before any mutation. If required evidence is
missing, stale, skipped or unsupported, the command holds rather than running
an unchecked `brew`. `bwd brew upgrade` without names considers the installed
formula inventory. Only supported command forms are routed; a direct `brew`
command is not intercepted. A different shell such as zsh requires no special
BrewWarden setup.

## Age and configuration

The default minimum release age is 168 hours, measured from the earliest
verified attestation for the **exact selected bottle digest**. Use a different
threshold or explicitly waive only the age of one identified candidate:

```sh
bwd --minimum-release-age 336h brew install jq
bwd --age-exception 'jq=Urgent upstream fix' brew upgrade jq
```

An exception is bound to one attempt and its verified artifact; each dependency
needing one requires its own reason. It cannot waive a missing or invalid
signature, checksum, timestamp, vulnerability check or dependency plan. Reasons
and waived digests appear before execution. A rebuild with a new digest needs
fresh evidence even if the version number is unchanged.

Optional JSON policy lives at
`~/Library/Application Support/brewwarden/config.json`:

```json
{"schemaVersion":1,"age":{"minimumHours":168}}
```

Use `--config PATH` before `brew` to select a different file. See the
[product design](design.md#configuration-and-storage) for the strict schema and
supported settings; configuration cannot disable mandatory verification.

## Holds, interruptions and repair

A hold stops before mutation. Correct the missing prerequisite (for example,
gh login or a supported Homebrew runtime), then retry for fresh evidence;
`doctor` can help diagnose environment support. A failed or interrupted
installation can leave a **partial** state. BrewWarden does not infer success,
automatically roll back or replay a saved plan or age exception. It refuses a
new mutation while its owned Homebrew child may still be running. Once the
child stops, inspect the installation, perform ordinary Homebrew repair while
BrewWarden is idle if needed, and rerun the whole command. Do not run a separate
Homebrew mutation against the same prefix *during* a BrewWarden operation.

Previously installed packages changed outside BrewWarden remain user-owned
state; BrewWarden does not claim to authenticate those installed payloads.
Use the [verification guide](verification.md) only for development or local
release-readiness checks, not as a prerequisite for ordinary use.
