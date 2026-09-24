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
	return domain.Artifact{Tap: "homebrew/core", Name: "jq", Version: "1.8.2", Rebuild: 1, OS: "macos", Arch: "arm64", BottleTag: "arm64_tahoe", SHA256: domain.Digest(strings.Repeat("a", 64))}
}
func resultFixture(a domain.Artifact) string {
	return `[{"verificationResult":{"signature":{"certificate":{"subjectAlternativeName":"` + identityPrefix + `publish-commit-bottles.yml@refs/heads/main","issuer":"` + issuer + `","sourceRepositoryURI":"` + repository + `","runnerEnvironment":"github-hosted"}},"statement":{"_type":"https://in-toto.io/Statement/v1","predicateType":"https://slsa.dev/provenance/v1","subject":[{"name":"` + bottleName(a) + `","digest":{"sha256":"` + string(a.SHA256) + `"}}]}}}]`
}

func ghResult(a domain.Artifact, timestamps ...string) string {
	var entries []map[string]json.RawMessage
	_ = json.Unmarshal([]byte(resultFixture(a)), &entries)
	var verification map[string]json.RawMessage
	_ = json.Unmarshal(entries[0]["verificationResult"], &verification)
	var values []map[string]string
	for _, ts := range timestamps {
		values = append(values, map[string]string{"type": "Tlog", "uri": "https://rekor.sigstore.dev", "timestamp": ts})
	}
	verification["verifiedTimestamps"], _ = json.Marshal(values)
	entries[0]["verificationResult"], _ = json.Marshal(verification)
	out, _ := json.Marshal(entries[0])
	return string(out)
}

func TestOldestVerifiedTimestampBoundToEachDigest(t *testing.T) {
	a := artifactFixture()
	one := ghResult(a, "2026-09-20T12:00:00Z", "2026-09-10T12:00:00Z")
	two := ghResult(a, "2026-09-22T12:00:00Z")
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC).Unix()
	got, err := oldestVerifiedTimestamp([]byte("["+two+","+one+"]"), a, now)
	if err != nil || got != time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC).Unix() {
		t.Fatal(got, err)
	}
	for _, bad := range []string{
		"[]", "null", "[" + one + "]{}",
		"[" + strings.TrimSuffix(strings.Repeat(one+",", ghLimit), ",") + "]",
		"[" + one + "," + ghResult(func() domain.Artifact { b := a; b.SHA256 = domain.Digest(strings.Repeat("b", 64)); return b }(), "2026-01-01T00:00:00Z") + "]",
		"[" + ghResult(a) + "]",
		"[" + ghResult(a, "invalid") + "]",
		"[" + ghResult(a, "2026-09-24T00:00:00Z") + "]",
		"[" + strings.Replace(one, `"uri":"https://rekor.sigstore.dev"`, `"uri":"https://invalid.example"`, 1) + "]",
		"[" + strings.Replace(one, `"uri":"https://rekor.sigstore.dev"`, `"uri":"TODO"`, 1) + "]",
		"[" + strings.Replace(one, `"sourceRepositoryURI":"`+repository+`"`, `"sourceRepositoryURI":"https://example.invalid"`, 1) + "]",
	} {
		if _, err := oldestVerifiedTimestamp([]byte(bad), a, now); err == nil {
			t.Fatal("accepted incomplete or unrelated attestation", bad[:min(len(bad), 100)])
		}
	}
	b := a
	b.SHA256 = domain.Digest(strings.Repeat("b", 64))
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
		// Exercise the untrusted parser as well as constructed valid results.
		_ = verifiedSubject(data, a)
		valid := []byte(resultFixture(a))
		if err := verifiedSubject(valid, a); err != nil {
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
		if err := verifiedSubject(valid, other); err == nil {
			t.Fatal("different bottle digest borrowed the attestation")
		}
		wrongSigner := strings.Replace(string(valid), `"runnerEnvironment":"github-hosted"`, `"runnerEnvironment":"untrusted"`, 1)
		if err := verifiedSubject([]byte(wrongSigner), a); err == nil {
			t.Fatal("untrusted signer borrowed the attestation")
		}
		const now = int64(1800000000)
		oneTime := now - int64(len(data)%4096+1)*60
		twoTime := now - int64(sum[0]+1)*30
		one := ghResult(a, time.Unix(oneTime, 0).UTC().Format(time.RFC3339))
		two := ghResult(a, time.Unix(twoTime, 0).UTC().Format(time.RFC3339))
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
	raw := ghResult(platform, "2026-09-10T00:00:00Z")
	if _, err := oldestVerifiedTimestamp([]byte("["+raw+"]"), all, now); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		strings.Replace(raw, string(platform.SHA256), strings.Repeat("b", 64), 1),
		strings.Replace(raw, "arm64_tahoe", "arm64_linux", 1),
		strings.Replace(raw, ".bottle.1.", ".bottle.2.", 1),
	} {
		if _, err := oldestVerifiedTimestamp([]byte("["+bad+"]"), all, now); err == nil {
			t.Fatal("unbound all bottle accepted")
		}
	}
}

func TestInstalledGHInputAndOutputBounds(t *testing.T) {
	root := t.TempDir()
	file, link := filepath.Join(root, "file"), filepath.Join(root, "link")
	if err := os.WriteFile(file, []byte("data"), 0600); err != nil {
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
		t.Fatal("unbounded verifier output accepted")
	}
}

func TestPublicGHVersionCohortUsesSameVerifiedResult(t *testing.T) {
	root := t.TempDir()
	artifact := artifactFixture()
	bottle := filepath.Join(root, bottleName(artifact))
	if err := os.WriteFile(bottle, []byte("cohort-bottle"), 0600); err != nil {
		t.Fatal(err)
	}
	artifact.SHA256, _ = hashFile(bottle, 1000)
	verified := "[" + ghResult(artifact, "2026-09-10T00:00:00Z") + "]"
	for _, tc := range []struct {
		version string
		allowed bool
	}{
		{"2.65.0", false}, {"2.66.0", true}, {"2.70.0", true}, {"2.101.0", true},
		{"2.102.0", false}, {"3.0.0", false}, {"2.66.0-rc1", false},
	} {
		t.Run(tc.version, func(t *testing.T) {
			tool := filepath.Join(t.TempDir(), "gh")
			script := "#!/bin/sh\nif [ \"$1\" = version ]; then printf '%s\\n' 'gh version " + tc.version + " (fixture)'; exit 0; fi\nprintf '%s' '" + verified + "'\n"
			if err := os.WriteFile(tool, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			provenance, age, _, err := (PublicGH{Path: tool}).VerifyEvidence(context.Background(), artifact, bottle, time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC).Unix())
			if (err == nil) != tc.allowed {
				t.Fatalf("gh %s eligibility mismatch: %v", tc.version, err)
			}
			if tc.allowed && (age.ProviderVersion != "gh/"+tc.version || provenance.ProviderVersion != age.ProviderVersion || age.Publication != domain.VerifiedAttestation) {
				t.Fatalf("verified claims lost verifier identity: %#v %#v", provenance, age)
			}
		})
	}
}

func TestPublicGHCommandBoundary(t *testing.T) {
	root := t.TempDir()
	a := artifactFixture()
	bottle := filepath.Join(root, bottleName(a))
	if err := os.WriteFile(bottle, []byte("bottle"), 0600); err != nil {
		t.Fatal(err)
	}
	a.SHA256, _ = hashFile(bottle, 1000)
	tool := filepath.Join(root, "gh")
	verified := "[" + ghResult(a, "2026-09-10T00:00:00Z") + "]"
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC).Unix()
	for _, tc := range []struct {
		name, script, reason string
		ok                   bool
	}{
		{"verified", "printf '%s' '" + verified + "'", "", true},
		{"empty result", "printf '[]'", "", false},
		{"failed request", "printf '%s' '" + verified + "'; exit 2", "", false},
		{"authentication", "echo 'To get started, run gh auth login. secret-marker' >&2; exit 4", "gh authentication required", false},
		{"rate limit", "echo 'API rate limit exceeded. secret-marker' >&2; exit 1", "rate limit reached", false},
		{"bottle changed", "printf changed > \"$3\"; printf '%s' '" + verified + "'", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = os.WriteFile(bottle, []byte("bottle"), 0600)
			script := "#!/bin/sh\nif [ \"$1\" = version ]; then printf 'gh version 2.101.0 (fixture)\\n'; exit 0; fi\n" +
				"test \"$1\" = attestation && test \"$2\" = verify && test \"$3\" = '" + bottle + "' && test \"$4\" = --repo && test \"$5\" = Homebrew/homebrew-core && test \"$6\" = --predicate-type && test \"$8\" = --format && test \"${10}\" = --limit && test \"${11}\" = 100 || exit 5\n" + tc.script + "\n"
			if err := os.WriteFile(tool, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			got, raw, err := (PublicGH{Path: tool}).VerifyBottle(context.Background(), a, bottle, now)
			if (err == nil) != tc.ok || tc.ok && (got == 0 || string(raw) != verified) {
				t.Fatal(got, err)
			}
			if tc.reason != "" && (err == nil || !strings.Contains(err.Error(), tc.reason) || strings.Contains(err.Error(), "secret-marker")) {
				t.Fatal("gh failure lost its actionable redacted reason", err)
			}
			if tc.ok {
				provenance, age, evidenceRaw, err := (PublicGH{Path: tool}).VerifyEvidence(context.Background(), a, bottle, now)
				if err != nil || provenance.Claim != domain.Provenance || age.Claim != domain.Publication ||
					age.Publication != domain.VerifiedAttestation || age.PublishedAt != got ||
					provenance.RawSHA256 != EvidenceDigest(evidenceRaw) || age.RawSHA256 != provenance.RawSHA256 ||
					provenance.Subject != a || age.Subject != a {
					t.Fatal("provenance and age not bound to the same bytes", err)
				}
			}
		})
	}
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nif [ \"$1\" = version ]; then echo 'gh version 2.62.0 (unsupported)'; else exit 42; fi\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := (PublicGH{Path: tool}).VerifyBottle(context.Background(), a, bottle, now); err == nil {
		t.Fatal("unsupported gh accepted")
	}
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nif [ \"$1\" = version ]; then echo 'gh version 2.101.0 (fixture)'; else exec /bin/sleep 30; fi\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, _, err := (PublicGH{Path: tool}).VerifyBottle(ctx, a, bottle, now); err == nil || time.Since(start) > 3*time.Second {
		t.Fatal("cancellation failed", err)
	}
}
