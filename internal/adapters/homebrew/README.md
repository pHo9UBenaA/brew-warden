# Native Homebrew boundary

Current capabilities: authenticated candidate collection and private runtime
materialization. Product execution is not wired yet. The separate developer VM
probe has demonstrated same-process formula locks across real install/upgrade.

## Runtime and source identity

The supported native implementation is Homebrew 7.0.4 at commit
`edb70f031e4170c780799633a1226ff73e1077f4`, portable Ruby 4.0.7, macOS Tahoe on
Apple Silicon, and the pinned [attestation helper](../attestation/README.md).
`tools/runtime-pack` archives that exact Homebrew source, checks the Ruby and
verifier archive/file digests, and inventories all regular files and symlinks.
It refuses unsafe archive paths, hard links, symlink parents and duplicate files.
This is an explicit build tool, never an implicit product download or host brew
installation. Runtime licensing and distribution packaging remain release work.

The distribution selects the runtime-manifest SHA-256. Materialization validates
its exact schema, version, path inventory and file hashes, rejects extra inputs,
and copies only verified bytes into a private workspace. Symlinks must resolve
inside that workspace. There is no search for an unpinned helper on PATH. The
runtime has 5,096 inventoried files/links with the current packaging inputs;
its manifest SHA-256 is
`9424050677790a1c88c66ab769c5167d59a874cd6a02074665268084c97e758b`.

All native argument arrays, environment variables and sandbox construction belong
here. Native child architecture is explicitly arm64: a Rosetta parent must not
silently select Intel bottle metadata. Candidate collection uses a private prefix,
cache, HOME, config, logs and temporary directory, with host writes denied.
Even reading bottle manifests creates native download locks; these remain private.
No user credentials, proxy variables or Homebrew settings are inherited.

## Authenticated candidate inspection

`metadata.rb` delegates JWS signature verification to the pinned Homebrew API and
its bundled trust root before selecting any recipe. It resolves both runtime and
build recipe dependencies, rejects duplicate formula identities and bounds the
inventory. The Go bridge rejects malformed schemas, missing fields, unsupported
platforms, mismatched paths/URLs, incomplete graphs and cycles. Metadata is bounded
to 80 MiB and machine-readable bridge responses to 8 MiB.

Recipes are downloaded at the authenticated tap commit and must match the signed
recipe checksum before evaluation. Candidate bottle bytes and signatures are
separate requirements. `candidate.rb` then compares the authenticated current
recipe, verified embedded bottle recipe and OCI runtime-dependency metadata.
It rejects source/graph differences, requirements, options, post-install actions,
resources, optional/test dependencies, migrations and conflicts. The selected
native bottle digest, tag and rebuild must match the authenticated candidate.
Cache files must resolve inside the private workspace and match the actual digest.

An empty patch list alone is not an unmodified-source claim. For the mapped jq
and oniguruma sources, `source.rb` compares the current and embedded install
methods with explicitly reviewed build methods using Ruby's maintained Ripper
parser. Only token source coordinates are removed; instructions, arguments and
conditions remain part of the digest. Additional instance-method overrides or
unrecognized build logic make source assessment unknown. A future version can
reuse the reviewed build procedure, but a changed patch/build method requires
review. This is a narrow recognition rule, not a general analyzer of arbitrary
Ruby or protection against compromised Homebrew (outside the threat model).

## Tested boundaries

The explicit `BREWWARDEN_LIVE_RUNTIME` test uses a built runtime and cached signed
metadata/bottles in a temporary workspace. It has verified the complete jq and
oniguruma runtime closure plus four build recipe dependencies, with no network or
host writes. Separate VM probes establish native mutations; parser and mocked
port tests do not establish installation binding. Runtime substitution, extra
files, unsafe manifests, archive traversal, ambiguous JSON and changed dependency
closures have negative tests.

`bootstrap.rb` reuses Homebrew's initialization and reexecutes Ruby with a selected
standard prefix while retaining private source paths. A disposable VM has verified
that private source can upgrade the standard prefix under retained native locks.
The product must additionally bind persisted evidence, the complete action plan,
state and attempt lifecycle before exposing that path as a CLI command.
