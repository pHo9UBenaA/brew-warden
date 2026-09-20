package attestation

import (
	"context"
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func artifactFixture() domain.Artifact {
	return domain.Artifact{Tap: "homebrew/core", Name: "jq", Version: "1.8.2", Rebuild: 1, OS: "macos", Arch: "arm64", BottleTag: "arm64_tahoe", SHA256: domain.Digest(strings.Repeat("a", 64))}
}
func resultFixture(a domain.Artifact) string {
	return `[{"verificationResult":{"signature":{"certificate":{"subjectAlternativeName":"` + identityPrefix + `publish-commit-bottles.yml@refs/heads/main","issuer":"` + issuer + `","sourceRepositoryURI":"` + repository + `","runnerEnvironment":"github-hosted"}},"statement":{"_type":"https://in-toto.io/Statement/v1","predicateType":"https://slsa.dev/provenance/v1","subject":[{"name":"` + bottleName(a) + `","digest":{"sha256":"` + string(a.SHA256) + `"}}]}}}]`
}

func TestVerifiedSubjectMustMatchExactIdentity(t *testing.T) {
	a := artifactFixture()
	valid := resultFixture(a)
	if err := verifiedSubject([]byte(valid), a); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		`[]`, `null`, `{}`, valid + `{}`,
		strings.Replace(valid, "arm64_tahoe", "tahoe", 1),
		strings.Replace(valid, string(a.SHA256), strings.Repeat("b", 64), 1),
		strings.Replace(valid, "publish-commit-bottles", "untrusted", 1),
		strings.Replace(valid, "github-hosted", "self-hosted", 1),
		strings.Replace(valid, issuer, "https://example.invalid", 1),
		strings.Replace(valid, `"sourceRepositoryURI":"`+repository+`"`, `"sourceRepositoryURI":"https://github.com/attacker/core"`, 1),
		strings.Replace(valid, `"sha256":`, `"SHA256":`, 1),
		strings.Replace(valid, `"subject":`, `"subject":null,"subject":`, 1),
		strings.Replace(valid, "https://slsa.dev/provenance/v1", "https://unknown.invalid", 1),
		strings.Replace(valid, `"certificate":{`, `"certificate":null,"unused":{`, 1),
	} {
		if err := verifiedSubject([]byte(bad), a); err == nil {
			t.Fatal("invalid provenance accepted", bad)
		}
	}
}

func TestVerifierProcessAndIntegrityBoundary(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	a := artifactFixture()
	bottle := filepath.Join(root, bottleName(a))
	bundle := filepath.Join(root, "bundle.jsonl")
	helper := filepath.Join(root, "verifier")
	for _, file := range []string{bottle, bundle} {
		if err := os.WriteFile(file, []byte("fixture bytes"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a.SHA256, _ = hashFile(bottle, 1024)
	t.Setenv("GH_TOKEN", "must-not-inherit")
	for _, tc := range []struct {
		name, script string
		want         bool
	}{
		{"matching subject", "test -z \"${GH_TOKEN:-}\" || exit 9\nprintf '%s' '" + resultFixture(a) + "'", true},
		{"empty successful output", "printf '[]'", false},
		{"nonzero with convincing output", "printf '%s' '" + resultFixture(a) + "'; exit 1", false},
		{"tampered bottle", "printf changed > \"$1\"; printf '%s' '" + resultFixture(a) + "'", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(bottle, []byte("fixture bytes"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(helper, []byte("#!/bin/sh\n"+tc.script+"\n"), 0700); err != nil {
				t.Fatal(err)
			}
			digest, _ := hashFile(helper, 10240)
			v := Verifier{helper, digest}
			e, raw, err := v.Verify(context.Background(), a, bottle, bundle, home, time.Now().Unix())
			if (err == nil) != tc.want || (tc.want && (e.Status != domain.Verified || e.Subject != a || len(raw) == 0)) {
				t.Fatal(e, err)
			}
		})
	}
	if err := os.WriteFile(bottle, []byte("fixture bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nexec /bin/sleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	digest, _ := hashFile(helper, 10240)
	v := Verifier{helper, digest}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, _, err := v.Verify(ctx, a, bottle, bundle, home, time.Now().Unix()); err == nil || time.Since(start) > 3*time.Second {
		t.Fatal("cancellation not enforced", err)
	}
	v.SHA256 = domain.Digest(strings.Repeat("b", 64))
	if _, _, err := v.Verify(context.Background(), a, bottle, bundle, home, time.Now().Unix()); err == nil {
		t.Fatal("replaced verifier accepted")
	}
}

func TestSymlinkAndOutputBounds(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	link := filepath.Join(root, "link")
	if err := os.WriteFile(file, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	if _, err := hashFile(link, 10); err == nil {
		t.Fatal("symlink input accepted")
	}
	if _, err := hashFile(file, 3); err == nil {
		t.Fatal("oversize input accepted")
	}
	out := &boundedOutput{}
	if _, err := out.Write(make([]byte, maxResponse+1)); err == nil || !out.overflow || out.Len() != 0 {
		t.Fatal("unbounded verifier output")
	}
}

func FuzzVerifiedSubject(f *testing.F) {
	a := artifactFixture()
	f.Add(resultFixture(a))
	f.Add(`[]`)
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > maxResponse {
			return
		}
		_ = verifiedSubject([]byte(raw), a)
	})
}

func TestLiveVerifier(t *testing.T) {
	helper := os.Getenv("BREWWARDEN_LIVE_VERIFIER")
	if helper == "" {
		t.Skip("requires explicit built verifier and network")
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := hashFile(helper, 128*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	v := Verifier{helper, digest}
	a := artifactFixture()
	a.SHA256 = "ca67c64d0aaf1e5472790ec2cc081ff7972316f27095d8a8aab81b3321247036"
	inputs := filepath.Join(root, ".cache/dependency-probe-inputs")
	home := t.TempDir()
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	e, _, err := v.Verify(context.Background(), a, filepath.Join(inputs, bottleName(a)), filepath.Join(inputs, "jq-bundle.jsonl"), home, time.Now().Unix())
	if err != nil || e.Status != domain.Verified {
		t.Fatal(e, err)
	}
	t.Log("verified official jq candidate", e.RawSHA256)
}
