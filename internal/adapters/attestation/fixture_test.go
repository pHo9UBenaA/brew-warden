package attestation

import (
	"strings"
	"testing"
)

// Replace the first occurrence and fail at setup if the intended input vanished.
// Otherwise a fixture edit can masquerade as a verifier regression.
func replaceFixtureText(t testing.TB, original, before, after string) string {
	t.Helper()
	if !strings.Contains(original, before) {
		t.Fatalf("cannot modify fixture: missing %q", before)
	}
	return strings.Replace(original, before, after, 1)
}
