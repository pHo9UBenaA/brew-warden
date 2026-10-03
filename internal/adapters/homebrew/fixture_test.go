package homebrew

import (
	"encoding/json"
	"strings"
	"testing"
)

// Fail at fixture construction, rather than diagnose malformed test setup as a
// product refusal. testing.TB supports both ordinary cases and fuzz seeds.
func marshalFixture(t testing.TB, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("cannot serialize fixture: %v", err)
	}
	return raw
}

// Replace the first occurrence and fail at setup if the intended input vanished.
// Otherwise a fixture edit can masquerade as a parser or policy regression.
func replaceFixtureText(t testing.TB, original, before, after string) string {
	t.Helper()
	if !strings.Contains(original, before) {
		t.Fatalf("cannot modify fixture: missing %q", before)
	}
	return strings.Replace(original, before, after, 1)
}
