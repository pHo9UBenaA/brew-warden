package attestation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

func artifactFixture() domain.Artifact {
	return domain.Artifact{
		Tap: "homebrew/core", Name: "jq", Version: "1.8.2", Rebuild: 1,
		OS: "macos", Arch: "arm64", BottleTag: "arm64_tahoe",
		SHA256: domain.Digest(strings.Repeat("a", 64)),
	}
}

func ghResult(t testing.TB, artifact domain.Artifact, timestamps ...string) string {
	t.Helper()
	certificate := map[string]string{
		"subjectAlternativeName": identityPrefix + "publish-commit-bottles.yml@refs/heads/main",
		"issuer":                 issuer,
		"sourceRepositoryURI":    repository,
		"runnerEnvironment":      "github-hosted",
	}
	subject := map[string]any{
		"name":   bottleName(artifact),
		"digest": map[string]domain.Digest{"sha256": artifact.SHA256},
	}
	statement := map[string]any{
		"_type":         "https://in-toto.io/Statement/v1",
		"predicateType": "https://slsa.dev/provenance/v1",
		"subject":       []map[string]any{subject},
	}
	var verifiedTimestamps []map[string]string
	for _, timestamp := range timestamps {
		verifiedTimestamps = append(verifiedTimestamps, map[string]string{
			"type": "Tlog", "uri": "https://rekor.sigstore.dev", "timestamp": timestamp,
		})
	}
	verification := map[string]any{
		"signature":          map[string]any{"certificate": certificate},
		"statement":          statement,
		"verifiedTimestamps": verifiedTimestamps,
	}
	raw, err := json.Marshal(map[string]any{"verificationResult": verification})
	if err != nil {
		t.Fatalf("cannot serialize attestation fixture: %v", err)
	}
	return string(raw)
}

func TestOldestVerifiedTimestampBoundToEachDigest(t *testing.T) {
	a := artifactFixture()
	one := ghResult(t, a, "2026-09-20T12:00:00Z", "2026-09-10T12:00:00Z")
	two := ghResult(t, a, "2026-09-22T12:00:00Z")
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC).Unix()
	got, err := oldestVerifiedTimestamp([]byte("["+two+","+one+"]"), a, now)
	want := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC).Unix()
	if err != nil || got != want {
		t.Fatalf("want oldest verified timestamp %d, got %d: error=%v", want, got, err)
	}
	b := a
	b.SHA256 = domain.Digest(strings.Repeat("b", 64))
	for name, bad := range map[string]string{
		"empty results":          "[]",
		"null results":           "null",
		"trailing JSON":          "[" + one + "]{}",
		"saturated results":      "[" + strings.TrimSuffix(strings.Repeat(one+",", attestationResultLimit), ",") + "]",
		"unrelated older result": "[" + one + "," + ghResult(t, b, "2026-01-01T00:00:00Z") + "]",
		"missing timestamps":     "[" + ghResult(t, a) + "]",
		"malformed timestamp":    "[" + ghResult(t, a, "invalid") + "]",
		"future timestamp":       "[" + ghResult(t, a, "2026-09-24T00:00:00Z") + "]",
		"untrusted log":          "[" + replaceFixtureText(t, one, `"uri":"https://rekor.sigstore.dev"`, `"uri":"https://invalid.example"`) + "]",
		"unknown log":            "[" + replaceFixtureText(t, one, `"uri":"https://rekor.sigstore.dev"`, `"uri":"TODO"`) + "]",
		"untrusted repository":   "[" + replaceFixtureText(t, one, `"sourceRepositoryURI":"`+repository+`"`, `"sourceRepositoryURI":"https://example.invalid"`) + "]",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := oldestVerifiedTimestamp([]byte(bad), a, now); err == nil {
				t.Fatal("accepted incomplete or unrelated attestation", bad[:min(len(bad), 100)])
			}
		})
	}
	if _, err := oldestVerifiedTimestamp([]byte("["+one+"]"), b, now); err == nil {
		t.Fatal("rebottled bytes inherited old age")
	}
}

// Changing the subject digest or signer must never borrow the time from a
// valid result; permutation of valid results must never change the oldest time.
func FuzzVerifiedSubject(f *testing.F) {
	for _, seed := range [][]byte{nil, []byte("{"), []byte("[]"), []byte("arbitrary bottle bytes"), {0xff, 0x00}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 4096 {
			return
		}
		a := artifactFixture()
		sum := sha256.Sum256(data)
		a.SHA256 = domain.Digest(hex.EncodeToString(sum[:]))
		const now = int64(1800000000)
		// Exercise the public evidence parser, including arbitrary untrusted output.
		_, _ = oldestVerifiedTimestamp(data, a, now)
		valid := []byte("[" + ghResult(t, a, time.Unix(now-60, 0).UTC().Format(time.RFC3339)) + "]")
		if _, err := oldestVerifiedTimestamp(valid, a, now); err != nil {
			t.Fatal("matching verified subject was refused", err)
		}
		other := a
		digest := []byte(a.SHA256)
		if digest[0] == 'a' {
			digest[0] = 'b'
		} else {
			digest[0] = 'a'
		}
		other.SHA256 = domain.Digest(digest)
		if _, err := oldestVerifiedTimestamp(valid, other, now); err == nil {
			t.Fatal("different bottle digest borrowed the attestation")
		}
		wrongSigner := replaceFixtureText(t, string(valid), `"runnerEnvironment":"github-hosted"`, `"runnerEnvironment":"untrusted"`)
		if _, err := oldestVerifiedTimestamp([]byte(wrongSigner), a, now); err == nil {
			t.Fatal("untrusted signer borrowed the attestation")
		}
		oneTime := now - int64(len(data)%4096+1)*60
		twoTime := now - int64(sum[0]+1)*30
		one := ghResult(t, a, time.Unix(oneTime, 0).UTC().Format(time.RFC3339))
		two := ghResult(t, a, time.Unix(twoTime, 0).UTC().Format(time.RFC3339))
		want := min(oneTime, twoTime)
		for _, raw := range []string{"[" + one + "," + two + "]", "[" + two + "," + one + "]"} {
			got, err := oldestVerifiedTimestamp([]byte(raw), a, now)
			if err != nil || got != want {
				t.Fatalf("attestation order changed the oldest verified time: got %d, want %d: %v", got, want, err)
			}
		}
	})
}

func TestAllBottleRequiresAttestedExactPlatformBytes(t *testing.T) {
	platform := artifactFixture()
	all := platform
	all.BottleTag = "all"
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC).Unix()
	raw := ghResult(t, platform, "2026-09-10T00:00:00Z")
	if _, err := oldestVerifiedTimestamp([]byte("["+raw+"]"), all, now); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]string{
		"different digest":   replaceFixtureText(t, raw, string(platform.SHA256), strings.Repeat("b", 64)),
		"different platform": replaceFixtureText(t, raw, "arm64_tahoe", "arm64_linux"),
		"different rebuild":  replaceFixtureText(t, raw, ".bottle.1.", ".bottle.2."),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := oldestVerifiedTimestamp([]byte("["+bad+"]"), all, now); err == nil {
				t.Fatal("unbound all bottle accepted")
			}
		})
	}
}

func TestInstalledGHInputAndOutputBounds(t *testing.T) {
	root := t.TempDir()
	file, link := filepath.Join(root, "file"), filepath.Join(root, "link")
	if err := os.WriteFile(file, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	if _, err := hashFile(link, 10); err == nil {
		t.Fatal("symlink verifier input accepted")
	}
	if _, err := hashFile(file, 3); err == nil {
		t.Fatal("oversized verifier input accepted")
	}
	out := &boundedOutput{}
	if _, err := out.Write(make([]byte, maxResponse+1)); err == nil || !out.overflow {
		t.Fatalf("want output overflow and write error: overflow=%t error=%v", out.overflow, err)
	}
}

func TestPublicGHVersionCohortUsesSameVerifiedResult(t *testing.T) {
	root := t.TempDir()
	artifact := artifactFixture()
	bottle := filepath.Join(root, bottleName(artifact))
	if err := os.WriteFile(bottle, []byte("cohort-bottle"), 0o600); err != nil {
		t.Fatal(err)
	}
	var err error
	artifact.SHA256, err = hashFile(bottle, 1000)
	if err != nil {
		t.Fatal("cannot hash cohort bottle fixture", err)
	}
	verified := "[" + ghResult(t, artifact, "2026-09-10T00:00:00Z") + "]"
	for _, tc := range []struct {
		version string
		allowed bool
	}{
		{"2.65.0", false},
		{"2.66.0", true},
		{"2.70.0", true},
		{"2.101.0", true},
		{"2.102.0", false},
		{"3.0.0", false},
		{"2.66.0-rc1", false},
	} {
		t.Run(tc.version, func(t *testing.T) {
			tool := filepath.Join(t.TempDir(), "gh")
			script := "#!/bin/sh\nif [ \"$1\" = version ]; then printf '%s\\n' 'gh version " + tc.version + " (fixture)'; exit 0; fi\nprintf '%s' '" + verified + "'\n"
			if err := os.WriteFile(tool, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			provenance, age, _, err := (PublicGH{Path: tool}).VerifyEvidence(context.Background(), artifact, bottle, time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC).Unix())
			if (err == nil) != tc.allowed {
				t.Fatalf("gh %s: want allowed=%t, got error=%v", tc.version, tc.allowed, err)
			}
			if tc.allowed && (age.ProviderVersion != "gh/"+tc.version || provenance.ProviderVersion != age.ProviderVersion || age.Publication != domain.VerifiedAttestation) {
				t.Fatalf("verified claims lost verifier identity: %#v %#v", provenance, age)
			}
		})
	}
}

func ghCommandFixture(t *testing.T) (domain.Artifact, string, string) {
	t.Helper()
	root := t.TempDir()
	artifact := artifactFixture()
	bottle := filepath.Join(root, bottleName(artifact))
	if err := os.WriteFile(bottle, []byte("bottle"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := hashFile(bottle, 1000)
	if err != nil {
		t.Fatal("cannot hash command-boundary bottle fixture", err)
	}
	artifact.SHA256 = digest
	return artifact, bottle, filepath.Join(root, "gh")
}

func TestPublicGHCommandBoundary(t *testing.T) {
	a, bottle, tool := ghCommandFixture(t)
	verified := "[" + ghResult(t, a, "2026-09-10T00:00:00Z") + "]"
	version, err := filepath.Abs("testdata/gh-2.66.0-version.txt")
	if err != nil {
		t.Fatal(err)
	}
	diagnostic, err := filepath.Abs("testdata/gh-2.66.0-no-auth.stderr")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GH_TEST_VERSION", version)
	t.Setenv("GH_TEST_NO_AUTH", diagnostic)
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC).Unix()
	for _, tc := range []struct {
		name, script, reason string
		ok                   bool
	}{
		{"verified", "printf '%s' '" + verified + "'", "", true},
		{"empty result", "printf '[]'", "", false},
		{"stdout overflow", "/usr/bin/head -c 9000000 /dev/zero", "gh output exceeded limit", false},
		{"stderr overflow", "/usr/bin/head -c 9000000 /dev/zero >&2", "gh output exceeded limit", false},
		{"failed request", "printf '%s' '" + verified + "'; exit 2", "", false},
		{"authentication", "/bin/cat \"$GH_TEST_NO_AUTH\" >&2; echo secret-marker >&2; exit 4", "gh authentication required", false},
		{"rate limit", "echo 'API rate limit exceeded. secret-marker' >&2; exit 1", "rate limit reached", false},
		{"bottle changed", "printf changed > \"$3\"; printf '%s' '" + verified + "'", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(bottle, []byte("bottle"), 0o600); err != nil {
				t.Fatal("cannot restore bottle fixture", err)
			}
			versionChecked := filepath.Join(filepath.Dir(tool), "version-checked")
			if err := os.Remove(versionChecked); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			t.Setenv("GH_TEST_VERSION_CHECKED", versionChecked)
			script := `#!/bin/sh
if [ "$1" = version ]; then
  test ! -e "$GH_TEST_VERSION_CHECKED" || exit 17
  touch "$GH_TEST_VERSION_CHECKED"
  exec /bin/cat "$GH_TEST_VERSION"
fi
test "$1" = attestation && test "$2" = verify &&
test "$3" = '` + bottle + `' && test "$4" = --repo &&
test "$5" = Homebrew/homebrew-core && test "$6" = --predicate-type &&
test "$7" = https://slsa.dev/provenance/v1 && test "$8" = --format &&
test "$9" = json && test "${10}" = --limit && test "${11}" = 100 || exit 5
` + tc.script + "\n"
			if err := os.WriteFile(tool, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			provenance, age, raw, err := (PublicGH{Path: tool}).VerifyEvidence(context.Background(), a, bottle, now)
			if (err == nil) != tc.ok {
				t.Fatalf("want verification success=%t, got error=%v", tc.ok, err)
			}
			if !tc.ok && (provenance != (domain.Evidence{}) || age != (domain.Evidence{}) || raw != nil) {
				t.Fatalf("failed verification returned evidence: %+v %+v %q", provenance, age, raw)
			}
			if tc.reason != "" && (err == nil || !strings.Contains(err.Error(), tc.reason) || strings.Contains(err.Error(), "secret-marker")) {
				t.Fatalf("want redacted gh error containing %q, got %v", tc.reason, err)
			}
			if tc.ok {
				wantProvenance := domain.Evidence{
					Claim: domain.Provenance, Subject: a, Status: domain.Verified,
					Provider: domain.Supplement, Source: repository, ProviderVersion: "gh/2.66.0",
					RawSHA256: EvidenceDigest([]byte(verified)), ObservedAt: now, ExpiresAt: now + 3600,
				}
				if provenance != wantProvenance {
					t.Fatalf("provenance=%+v, want %+v", provenance, wantProvenance)
				}
				wantAge := wantProvenance
				wantAge.Claim = domain.Publication
				wantAge.Publication = domain.VerifiedAttestation
				wantAge.PublishedAt = time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC).Unix()
				if age != wantAge {
					t.Fatalf("age=%+v, want %+v", age, wantAge)
				}
				if string(raw) != verified {
					t.Fatalf("raw verification response=%q, want %q", raw, verified)
				}
			}
		})
	}
}

func TestPublicGHRejectsUnsupportedVersionBeforeAttestation(t *testing.T) {
	artifact, bottle, tool := ghCommandFixture(t)
	started := filepath.Join(filepath.Dir(tool), "attestation-started")
	t.Setenv("GH_TEST_VERIFY_STARTED", started)
	script := `#!/bin/sh
if [ "$1" = version ]; then
  echo 'gh version 2.62.0 (unsupported)'
  exit 0
fi
printf started > "$GH_TEST_VERIFY_STARTED"
exit 42
`
	if err := os.WriteFile(tool, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	const now = int64(1800000000)
	_, _, _, err := (PublicGH{Path: tool}).VerifyEvidence(context.Background(), artifact, bottle, now)
	if err == nil || !strings.Contains(err.Error(), "unsupported installed gh version") {
		t.Fatalf("want unsupported gh version refusal, got %v", err)
	}
	if _, err := os.Stat(started); !os.IsNotExist(err) {
		t.Fatal("unsupported gh reached attestation command", err)
	}
}

func TestPublicGHCancellationStopsRunningAttestation(t *testing.T) {
	artifact, bottle, tool := ghCommandFixture(t)
	started := filepath.Join(filepath.Dir(tool), "attestation-started")
	t.Setenv("GH_TEST_VERIFY_STARTED", started)
	script := `#!/bin/sh
if [ "$1" = version ]; then
  echo 'gh version 2.101.0 (fixture)'
  exit 0
fi
printf started > "$GH_TEST_VERIFY_STARTED"
exec /bin/sleep 30
`
	if err := os.WriteFile(tool, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	completed := make(chan error, 1)
	go func() {
		_, _, _, err := (PublicGH{Path: tool}).VerifyEvidence(ctx, artifact, bottle, 1800000000)
		completed <- err
	}()
	// Wait for the actual attestation invocation, not an arbitrary sleep that
	// could expire during file hashing or the separate version probe.
	for {
		if _, err := os.Stat(started); err == nil {
			break
		} else if !os.IsNotExist(err) {
			t.Fatal("cannot observe attestation start", err)
		}
		select {
		case err := <-completed:
			t.Fatalf("verification ended before attestation started: %v", err)
		case <-ctx.Done():
			t.Fatal("attestation did not start before test deadline")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-completed:
		if err == nil || !strings.Contains(err.Error(), "gh command timed out or was cancelled") {
			t.Fatalf("want attestation cancellation refusal, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("running attestation did not stop within three seconds of cancellation")
	}
}
