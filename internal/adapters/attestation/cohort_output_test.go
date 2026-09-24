package attestation

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

// These are full outputs of real arm64 gh releases verifying public signed
// bundles for the same exact jq bottle. A saved JSON result is an output-format
// fixture, not an independently authenticated attestation: gh performs the
// cryptographic verification at collection time in the product.
func TestCapturedGHVerifiedResultCohorts(t *testing.T) {
	artifact := artifactFixture()
	artifact.SHA256 = domain.Digest("ca67c64d0aaf1e5472790ec2cc081ff7972316f27095d8a8aab81b3321247036")
	oldest := time.Date(2026, 8, 24, 22, 10, 45, 0, time.UTC).Unix()
	now := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC).Unix()
	for _, version := range []string{"2.66.0", "2.70.0", "2.74.0", "2.80.0", "2.97.0", "2.101.0"} {
		t.Run(version, func(t *testing.T) {
			output, err := os.ReadFile(filepath.Join("testdata", "gh-"+version+"-version.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if got, err := supportedGHVersion(output); err != nil || got != version {
				t.Fatalf("captured executable version rejected: %q %v", got, err)
			}
			result, err := os.ReadFile(filepath.Join("testdata", "gh-"+version+"-jq-verified.json"))
			if err != nil || len(result) == 0 || len(result) > maxResponse {
				t.Fatalf("captured result unavailable or unbounded: %v", err)
			}
			if got, err := oldestVerifiedTimestamp(result, artifact, now); err != nil || got != oldest {
				t.Fatalf("verified signer/subject/earliest time not parsed: %d %v", got, err)
			}
			// The authenticated gh 2.66.0 --repo result matched the earlier
			// offline-bundle capture byte-for-byte in the native guest. Keep the
			// online response bound to the same parser regression fixture.
			if version == "2.66.0" && EvidenceDigest(result) != domain.Digest("0faed6961f931e3c6f5fca8daedf42c3589a613fee54156ce04a593b51dabf69") {
				t.Fatal("gh 2.66.0 fixture differs from authenticated online result")
			}
			other := artifact
			other.SHA256 = domain.Digest("da67c64d0aaf1e5472790ec2cc081ff7972316f27095d8a8aab81b3321247036")
			if _, err := oldestVerifiedTimestamp(result, other, now); err == nil {
				t.Fatal("another bottle inherited the attested age")
			}
			if _, err := oldestVerifiedTimestamp(result, artifact, oldest-1); err == nil {
				t.Fatal("future attestation was accepted")
			}
			changed := bytes.Replace(result, []byte(`"uri": "https://rekor.sigstore.dev"`), []byte(`"uri": "TODO"`), 1)
			if bytes.Equal(result, changed) {
				changed = bytes.Replace(result, []byte(`"uri":"https://rekor.sigstore.dev"`), []byte(`"uri":"TODO"`), 1)
			}
			if bytes.Equal(result, changed) {
				t.Fatal("captured output did not contain a verified Rekor URI")
			}
			if _, err := oldestVerifiedTimestamp(changed, artifact, now); err == nil {
				t.Fatal("unknown transparency log URI supplied age")
			}
		})
	}
}
