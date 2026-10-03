package attestation

import (
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
	for _, version := range []string{"2.66.0", "2.101.0"} {
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
				t.Fatalf("want captured result of 1..%d bytes: bytes=%d error=%v", maxResponse, len(result), err)
			}
			if got, err := oldestVerifiedTimestamp(result, artifact, now); err != nil || got != oldest {
				t.Fatalf("want verified timestamp %d for %+v, got %d: error=%v", oldest, artifact, got, err)
			}
		})
	}
}
