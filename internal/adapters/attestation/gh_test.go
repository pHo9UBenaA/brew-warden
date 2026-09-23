package attestation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

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
		name, script string
		ok           bool
	}{
		{"verified", "printf '%s' '" + verified + "'", true},
		{"empty result", "printf '[]'", false},
		{"failed request", "printf '%s' '" + verified + "'; exit 2", false},
		{"bottle changed", "printf changed > \"$3\"; printf '%s' '" + verified + "'", false},
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
