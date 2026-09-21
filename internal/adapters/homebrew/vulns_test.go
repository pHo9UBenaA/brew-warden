package homebrew

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicVulnsOutput(t *testing.T) {
	candidates := metadataFixture().Formulae
	clean := `{"findings":[],"skipped_formulae":[]}`
	affected := `{"findings":[{"formula":"jq","version":"1.8.2","vulnerabilities":[{"id":"CVE-2024-23337","aliases":["GHSA-2q6r-344g-cx46"],"summary":null}],"patched":[]}],"skipped_formulae":[]}`
	patched := strings.Replace(strings.Replace(affected, `"vulnerabilities":[`, `"patched":[`, 1), `"patched":[]`, `"vulnerabilities":[]`, 1)
	for _, tc := range []struct {
		raw    string
		status int
	}{{clean, 0}, {affected, 1}, {patched, 0}} {
		if _, err := parsePublicVulns([]byte(tc.raw), tc.status, candidates); err != nil {
			t.Fatal(err)
		}
	}
	for name, tc := range map[string]struct {
		raw    string
		status int
	}{
		"failed clean": {clean, 1}, "crashed clean": {clean, 2}, "affected exit zero": {affected, 0},
		"skipped exit zero": {`{"findings":[],"skipped_formulae":["oniguruma"]}`, 0},
		"missing skipped":   {`{"findings":[]}`, 0}, "null findings": {`{"findings":null,"skipped_formulae":[]}`, 0},
		"old installed version": {strings.Replace(affected, `1.8.2`, `1.6`, 1), 1},
		"unrequested formula":   {strings.Replace(affected, `"jq"`, `"curl"`, 1), 1},
		"missing patched":       {strings.Replace(affected, `,"patched":[]`, "", 1), 1},
		"duplicate field":       {strings.Replace(clean, `"findings":[]`, `"findings":[],"findings":[]`, 1), 0},
		"mis-cased field":       {strings.Replace(clean, `findings`, `Findings`, 1), 0},
		"truncated":             {affected[:len(affected)-1], 1},
		"unsafe id":             {strings.Replace(affected, `CVE-2024-23337`, `bad\u001b`, 1), 1},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parsePublicVulns([]byte(tc.raw), tc.status, candidates); err == nil {
				t.Fatal("unverified report accepted")
			}
		})
	}
}

func TestCandidateScanRejectsInstalledOrLinkedPrefixBeforeLaunch(t *testing.T) {
	for _, mode := range []string{"keg", "opt", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			prefix := filepath.Join(root, "runtime/brew")
			if err := os.MkdirAll(prefix, 0700); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "keg":
				if err := os.MkdirAll(filepath.Join(prefix, "Cellar/jq/1.6"), 0700); err != nil {
					t.Fatal(err)
				}
			case "opt":
				if err := os.MkdirAll(filepath.Join(prefix, "opt/jq"), 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(prefix, "Cellar")); err != nil {
					t.Fatal(err)
				}
			}
			_, _, err := (workspace{root}).scanCandidateVulnerabilities(context.Background(), metadataFixture().Formulae)
			if err == nil || !strings.Contains(err.Error(), "inspection prefix") {
				t.Fatalf("expected prefix refusal before command launch, got %v", err)
			}
		})
	}
}
