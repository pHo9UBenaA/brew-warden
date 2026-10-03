package homebrew

import (
	"encoding/json"
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
