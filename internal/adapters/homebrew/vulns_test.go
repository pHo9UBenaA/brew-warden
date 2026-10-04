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
	affected := `{"findings":[{"formula":"jq","version":"1.8.2",` +
		`"vulnerabilities":[{"id":"CVE-2024-23337","aliases":["GHSA-2q6r-344g-cx46"],"summary":null}],` +
		`"patched":[]}],"skipped_formulae":[]}`
	patched := replaceFixtureText(t, affected, `"vulnerabilities":[`, `"patched":[`)
	patched = replaceFixtureText(t, patched, `"patched":[]`, `"vulnerabilities":[]`)
	for _, tc := range []struct {
		name     string
		raw      string
		exitCode int
	}{
		{"clean", clean, 0},
		{"affected", affected, 1},
		{"patched", patched, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parsePublicVulns([]byte(tc.raw), tc.exitCode, candidates); err != nil {
				t.Fatalf("report with exit %d rejected: %v", tc.exitCode, err)
			}
		})
	}
	for name, tc := range map[string]struct {
		raw      string
		exitCode int
	}{
		"failed clean":          {clean, 1},
		"crashed clean":         {clean, 2},
		"affected exit zero":    {affected, 0},
		"skipped exit zero":     {`{"findings":[],"skipped_formulae":["oniguruma"]}`, 0},
		"missing skipped":       {`{"findings":[]}`, 0},
		"null findings":         {`{"findings":null,"skipped_formulae":[]}`, 0},
		"old installed version": {replaceFixtureText(t, affected, `1.8.2`, `1.6`), 1},
		"unrequested formula":   {replaceFixtureText(t, affected, `"jq"`, `"curl"`), 1},
		"missing patched":       {replaceFixtureText(t, affected, `,"patched":[]`, ""), 1},
		"duplicate field":       {replaceFixtureText(t, clean, `"findings":[]`, `"findings":[],"findings":[]`), 0},
		"mis-cased field":       {replaceFixtureText(t, clean, `findings`, `Findings`), 0},
		"truncated":             {affected[:len(affected)-1], 1},
		"unsafe id":             {replaceFixtureText(t, affected, `CVE-2024-23337`, `bad\u001b`), 1},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parsePublicVulns([]byte(tc.raw), tc.exitCode, candidates); err == nil {
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
			if err := os.MkdirAll(prefix, 0o700); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "keg":
				if err := os.MkdirAll(filepath.Join(prefix, "Cellar/jq/1.6"), 0o700); err != nil {
					t.Fatal(err)
				}
			case "opt":
				if err := os.MkdirAll(filepath.Join(prefix, "opt/jq"), 0o700); err != nil {
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
