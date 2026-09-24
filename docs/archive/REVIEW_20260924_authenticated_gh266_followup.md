# Authenticated gh 2.66.0 online-path follow-up

Date: 2026-09-24. Revision under test: `df5eb495d70071328b3a33b2d4ec8d049a79c4fc`.
This follow-up resolves the earlier uncertainty about the *normal* gh 2.66.0
GitHub API path; it does not widen supported versions or authorize publishing.

In the initial gh 2.66.0 guest, device authorization never completed. Signed
bundle verification with `--bundle` succeeded, but that does not establish
online acquisition. In the new disposable native arm64 Tahoe guest, the same
2.66.0 `gh attestation verify <bottle> --repo Homebrew/homebrew-core
--predicate-type https://slsa.dev/provenance/v1 --format json --limit 100`
**before login** exited 4 with an explicit `gh auth login` requirement and
empty stdout. This is a reproducible missing-credential hold, not evidence of
signature failure or a transient Homebrew bottle failure. A device code for
the new guest expired; after a fresh code was approved by the user, `gh auth
status` succeeded inside that guest. No host GitHub credentials were copied.

After guest authorization, the same gh 2.66.0 command without `--bundle`
acquired and verified the *exact* jq 1.8.2 arm64 Tahoe bottle (SHA-256
`ca67c64d0aaf1e5472790ec2cc081ff7972316f27095d8a8aab81b3321247036`)
via the normal GitHub API. It exited 0 with two verified Homebrew-signer
results, both including a trusted Tlog URI `https://rekor.sigstore.dev` and a
matching exact subject. The CLI output was checked for these fields without
copying authentication diagnostics into public test fixtures.

The packaged distribution at the unchanged `df5eb49` revision then passed
`product-ready.sh complete` using reviewed Homebrew 7.0.6, the real installed
gh 2.66.0, and guest-only GitHub device authentication: all 17 VM cases passed,
with zero skips. `start` had passed `check.sh all`, VM test vet/lint/vulnerability
checks and compilation, and two reproducible product builds. The archive SHA-256
was `7593f82d755603aeb40aa05f9a048c5c048f06cf57278ff1b60a7fff0f7c2d71`;
private local gate evidence is under
`.cache/product-ready.brewwarden-gh266-online-01/`. Guest credentials were
removed and the VM stopped. Revoke the guest's temporary GitHub CLI OAuth
authorization in account settings when no longer needed.

Conclusion: the observed old-client failure was missing/expired guest device
authorization; it is repeatable when unauthenticated, and the same version
succeeds in the authenticated online product path. This does not prove GitHub
API availability forever, every intermediate gh version, or absence of
vulnerabilities. Offline output fixtures and 2.101.0's earlier authenticated
gate remain separate evidence. The readiness result applies **only** to
`df5eb49`, not to a subsequent documentation-only commit.
