package attestation

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

// The real gh 2.66.0 public --repo command returned this diagnostic with exit
// code 4 before guest device authorization. The unit boundary must not turn
// that missing prerequisite into evidence or expose gh's raw diagnostics.
func TestGH266OnlineCommandRequiresAuthentication(t *testing.T) {
	root := t.TempDir()
	artifact := artifactFixture()
	bottle := filepath.Join(root, bottleName(artifact))
	if err := os.WriteFile(bottle, []byte("offline-auth-refusal-fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	var err error
	artifact.SHA256, err = hashFile(bottle, 1024)
	if err != nil {
		t.Fatal(err)
	}
	version, err := filepath.Abs(filepath.Join("testdata", "gh-2.66.0-version.txt"))
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := filepath.Abs(filepath.Join("testdata", "gh-2.66.0-no-auth.stderr"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GH266_TEST_VERSION", version)
	t.Setenv("GH266_TEST_NO_AUTH", stderr)
	tool := filepath.Join(root, "gh")
	command := `#!/bin/sh
if [ "$1" = version ]; then exec /bin/cat "$GH266_TEST_VERSION"; fi
test "$1" = attestation && test "$2" = verify && test "$3" = "$GH266_TEST_BOTTLE" &&
test "$4" = --repo && test "$5" = Homebrew/homebrew-core &&
test "$6" = --predicate-type && test "$7" = https://slsa.dev/provenance/v1 &&
test "$8" = --format && test "$9" = json &&
test "${10}" = --limit && test "${11}" = 100 || exit 64
/bin/cat "$GH266_TEST_NO_AUTH" >&2
exit 4
`
	t.Setenv("GH266_TEST_BOTTLE", bottle)
	if err := os.WriteFile(tool, []byte(command), 0700); err != nil {
		t.Fatal(err)
	}
	provenance, age, raw, err := (PublicGH{Path: tool}).VerifyEvidence(context.Background(), artifact, bottle, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC).Unix())
	if err == nil || !strings.Contains(err.Error(), "gh authentication required") ||
		provenance != (domain.Evidence{}) || age != (domain.Evidence{}) || raw != nil {
		t.Fatalf("unauthenticated gh 2.66.0 produced evidence: %v", err)
	}
}
