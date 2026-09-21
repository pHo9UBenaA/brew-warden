package homebrew

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"
)

// This probe uses public commands in a copied prefix. The old keg is a metadata
// fixture, not an installation. No host prefix is written or used as a target.
func TestLivePublicVulnsCandidateSelection(t *testing.T) {
	source := os.Getenv("BREWWARDEN_LIVE_RUNTIME")
	if source == "" {
		t.Skip("requires explicitly built native runtime and live OSV access")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("requires Apple Silicon macOS")
	}
	raw, err := os.ReadFile(filepath.Join(source, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := workspace{root: root}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	if _, err := (Runtime{source, digestBytes(raw)}).materialize(filepath.Join(root, "runtime")); err != nil {
		t.Fatal(err)
	}
	signed, err := os.ReadFile("../../../.cache/packages.arm64_tahoe.jws.json")
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(root, metadataCachePath)
	if err := os.MkdirAll(filepath.Dir(cache), 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeNew(cache, signed, 0600); err != nil {
		t.Fatal(err)
	}
	profile, err := w.sandbox("advisory-probe", true, false, []string{filepath.Join(root, "runtime/brew/Library"), cache})
	if err != nil {
		t.Fatal(err)
	}
	type finding struct {
		Formula         string            `json:"formula"`
		Version         string            `json:"version"`
		Vulnerabilities []json.RawMessage `json:"vulnerabilities"`
	}
	type report struct {
		Findings []finding `json:"findings"`
		Skipped  []string  `json:"skipped_formulae"`
	}
	run := func(label string, args ...string) (report, int) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		cmd := w.command(ctx, profile, args...)
		cmd.Env = slices.DeleteFunc(cmd.Env, func(s string) bool { return s == "HOMEBREW_NO_INSTALL_FROM_API=1" || s == "HOMEBREW_DEVELOPER=1" })
		cmd.Env = append(cmd.Env, "HOMEBREW_CURL_RETRIES=0")
		out, stderr := &processOutput{}, &processOutput{}
		cmd.Stdout, cmd.Stderr = out, stderr
		err := cmd.Run()
		status := 0
		if err != nil {
			var ex *exec.ExitError
			if !errors.As(err, &ex) || ctx.Err() != nil {
				t.Fatalf("%s: %v: %s", label, err, stderr.String())
			}
			status = ex.ExitCode()
		}
		if out.overflow || stderr.overflow {
			t.Fatal("output limit exceeded")
		}
		t.Logf("%s: exit=%d stdout-sha256=%s stderr=%s", label, status, digestBytes(out.Bytes()), stderr.String())
		if status != 0 && out.Len() == 0 {
			return report{}, status
		}
		var r report
		if err := json.Unmarshal(out.Bytes(), &r); err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if r.Findings == nil || r.Skipped == nil {
			t.Fatal("missing result fields")
		}
		return r, status
	}
	expanded, status := run("implicit-dependency-expansion", "vulns", "--json", "--deps", "homebrew/core/jq")
	slices.Sort(expanded.Skipped)
	if status != 0 || !slices.Equal(expanded.Skipped, []string{"autoconf", "automake", "libtool", "m4"}) {
		t.Fatal("expected skipped build dependencies despite successful command exit")
	}
	clean, status := run("empty-prefix-candidate", "vulns", "--json", "homebrew/core/jq", "homebrew/core/oniguruma")
	if status != 0 || len(clean.Findings) != 0 || len(clean.Skipped) != 0 {
		t.Fatal("expected clean candidate and dependency fixture; inspect changed upstream data")
	}
	info, err := w.invokeAPI(context.Background(), "probe-info", profile, "info", "--json=v2", "--formula", "homebrew/core/jq", "homebrew/core/oniguruma")
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := parseInfo(info, []string{"jq", "oniguruma"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.scanCandidateVulnerabilities(context.Background(), candidates); err != nil {
		t.Fatal(err)
	}
	// Make the older source visible only in the copied prefix. Homebrew's public
	// scanner prefers this SBOM even when the requested formula has a newer stable.
	keg := filepath.Join(root, "runtime/brew/Cellar/jq/1.6")
	if err := os.MkdirAll(keg, 0700); err != nil {
		t.Fatal(err)
	}
	sbom := `{"packages":[{"SPDXID":"SPDXRef-Archive-jq-src","downloadLocation":"https://github.com/jqlang/jq/releases/download/jq-1.6/jq-1.6.tar.gz","versionInfo":"1.6"}]}`
	if err := writeNew(filepath.Join(keg, "sbom.spdx.json"), []byte(sbom), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.scanCandidateVulnerabilities(context.Background(), candidates); err == nil {
		t.Fatal("candidate adapter accepted installed state")
	}
	old, status := run("old-installed-sbom", "vulns", "--json", "homebrew/core/jq")
	if status != 1 || len(old.Findings) != 1 || old.Findings[0].Formula != "jq" || old.Findings[0].Version != "1.6" || len(old.Findings[0].Vulnerabilities) == 0 {
		t.Fatal("old SBOM selection not demonstrated")
	}
	t.Logf("old selected version=%s findings=%d", old.Findings[0].Version, len(old.Findings[0].Vulnerabilities))
	// Removing only the fixture proves candidate selection is recovered without
	// private Ruby calls, formula edits, or a vulnerability-query replacement.
	if err := os.RemoveAll(filepath.Join(root, "runtime/brew/Cellar/jq")); err != nil {
		t.Fatal(err)
	}
	restored, status := run("candidate-restored", "vulns", "--json", "homebrew/core/jq", "homebrew/core/oniguruma")
	if status != 0 || len(restored.Findings) != 0 || len(restored.Skipped) != 0 {
		t.Fatal("candidate result did not recover")
	}
	profile, err = w.sandbox("advisory-network-denied", false, false, []string{filepath.Join(root, "runtime/brew/Library"), cache})
	if err != nil {
		t.Fatal(err)
	}
	failed, status := run("network-failure", "vulns", "--json", "homebrew/core/jq")
	if status == 0 || failed.Findings != nil {
		t.Fatal("network failure became a clean result")
	}

}
