package attestation

import (
	"strings"
	"testing"
)

func TestAttestationAgeChecksEachResultAfterMatchingCandidate(t *testing.T) {
	const now = int64(1800000000)
	artifact := artifactFixture()
	matching := ghResult(artifact, "2026-09-10T00:00:00Z")
	other := artifact
	other.Name = "other"
	unrelated := ghResult(other, "2026-09-01T00:00:00Z")
	for _, tc := range []struct {
		name, later string
		wantError   bool
	}{
		{"matching later subject", matching, false},
		{"unrelated trusted subject", unrelated, true},
		{"untrusted later signer", replaceFixtureText(t, matching, `"runnerEnvironment":"github-hosted"`, `"runnerEnvironment":"untrusted"`), true},
		{"contradictory later digest", replaceFixtureText(t, matching, string(artifact.SHA256), strings.Repeat("b", 64)), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := oldestVerifiedTimestamp([]byte("["+matching+","+tc.later+"]"), artifact, now)
			if (err != nil) != tc.wantError {
				t.Fatalf("later result: want error=%t, got %v", tc.wantError, err)
			}
		})
	}
}
