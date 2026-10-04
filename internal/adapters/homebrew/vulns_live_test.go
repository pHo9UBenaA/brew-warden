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
	if os.Getenv("BREWWARDEN_LIVE_RUNTIME") == "" {
		t.Skip("requires explicitly built native runtime and live OSV access")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("requires Apple Silicon macOS")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := workspace{root: root}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	if _, err := (Runtime{}).materialize(context.Background(), filepath.Join(root, "runtime")); err != nil {
		t.Fatal(err)
	}
	signed, err := os.ReadFile("../../../.cache/packages.arm64_tahoe.jws.json")
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(root, metadataCachePath)
	if err := os.MkdirAll(filepath.Dir(cache), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeNew(cache, signed, 0o600); err != nil {
		t.Fatal(err)
	}
	profile, err := w.sandbox("advisory-probe", sandboxPermissions{AllowNetwork: true}, []string{filepath.Join(root, "runtime/brew/Library"), cache})
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
			t.Fatalf("output limit exceeded: stdout_overflow=%t stderr_overflow=%t", out.overflow, stderr.overflow)
		}
		t.Logf("%s: exit=%d stdout-sha256=%s stderr=%s", label, status, digestBytes(out.Bytes()), stderr.String())
		if status != 0 && len(out.Bytes()) == 0 {
			return report{}, status
		}
		var r report
		if err := json.Unmarshal(out.Bytes(), &r); err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if r.Findings == nil || r.Skipped == nil {
			t.Fatalf("missing result fields: findings=%+v skipped=%+v", r.Findings, r.Skipped)
		}
		return r, status
	}
	expanded, status := run("implicit-dependency-expansion", "vulns", "--json", "--deps", "homebrew/core/jq")
	slices.Sort(expanded.Skipped)
	if status != 0 || !slices.Equal(expanded.Skipped, []string{"autoconf", "automake", "libtool", "m4"}) {
		t.Fatalf("want exit 0 with skipped autoconf, automake, libtool and m4: exit=%d report=%+v", status, expanded)
	}
	clean, status := run("empty-prefix-candidate", "vulns", "--json", "homebrew/core/jq", "homebrew/core/oniguruma")
	if status != 0 || len(clean.Findings) != 0 || len(clean.Skipped) != 0 {
		t.Fatalf("want exit 0 without findings or skipped subjects; inspect changed upstream data: exit=%d report=%+v", status, clean)
	}
	info, err := w.invoke(context.Background(), "probe-info", profile, "info", "--json=v2", "--formula", "homebrew/core/jq", "homebrew/core/oniguruma")
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
	if err := os.MkdirAll(keg, 0o700); err != nil {
		t.Fatal(err)
	}
	sbom := `{"packages":[{"SPDXID":"SPDXRef-Archive-jq-src","downloadLocation":"https://github.com/jqlang/jq/releases/download/jq-1.6/jq-1.6.tar.gz","versionInfo":"1.6"}]}`
	if err := writeNew(filepath.Join(keg, "sbom.spdx.json"), []byte(sbom), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.scanCandidateVulnerabilities(context.Background(), candidates); err == nil {
		t.Fatal("candidate adapter accepted installed state")
	}
	old, status := run("old-installed-sbom", "vulns", "--json", "homebrew/core/jq")
	if status != 1 || len(old.Findings) != 1 || old.Findings[0].Formula != "jq" || old.Findings[0].Version != "1.6" || len(old.Findings[0].Vulnerabilities) == 0 {
		t.Fatalf("want exit 1 with affected jq 1.6 SBOM: exit=%d report=%+v", status, old)
	}
	t.Logf("old selected version=%s findings=%d", old.Findings[0].Version, len(old.Findings[0].Vulnerabilities))
	// Removing only the fixture proves candidate selection is recovered without
	// private Ruby calls, formula edits, or a vulnerability-query replacement.
	if err := os.RemoveAll(filepath.Join(root, "runtime/brew/Cellar/jq")); err != nil {
		t.Fatal(err)
	}
	restored, status := run("candidate-restored", "vulns", "--json", "homebrew/core/jq", "homebrew/core/oniguruma")
	if status != 0 || len(restored.Findings) != 0 || len(restored.Skipped) != 0 {
		t.Fatalf("want exit 0 without findings or skipped subjects after fixture removal: exit=%d report=%+v", status, restored)
	}
	profile, err = w.sandbox("advisory-network-denied", sandboxPermissions{}, []string{filepath.Join(root, "runtime/brew/Library"), cache})
	if err != nil {
		t.Fatal(err)
	}
	failed, status := run("network-failure", "vulns", "--json", "homebrew/core/jq")
	if status == 0 || failed.Findings != nil {
		t.Fatalf("network failure became a clean result: exit=%d report=%+v", status, failed)
	}
}

// Exercises the agreed two-source composition with fresh signed metadata rather
// than a historical cache; all commands run in the private inspection prefix.
func TestLivePublicAdvisorySources(t *testing.T) {
	if os.Getenv("BREWWARDEN_LIVE_RUNTIME") == "" {
		t.Skip("requires explicitly built native runtime and public advisory access")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("requires Apple Silicon macOS")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := workspace{root}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	if _, err := (Runtime{}).materialize(context.Background(), filepath.Join(root, "runtime")); err != nil {
		t.Fatal(err)
	}
	profile, err := w.sandbox("acquire", sandboxPermissions{AllowNetwork: true}, []string{filepath.Join(root, "runtime/brew/Library")})
	if err != nil {
		t.Fatal(err)
	}
	targets := []string{"jq", "fzf", "ripgrep"}
	raw, err := w.metadata(context.Background(), profile, targets)
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := parseMetadata(raw, targets)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := w.collectPublicAdvisories(context.Background(), publicClient(), candidates, time.Now().Unix(), brewRevision)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != len(candidates) {
		t.Fatalf("want %d combined claims, got %d", len(candidates), len(evidence))
	}
	for i, e := range evidence {
		if e.Subject != candidates[i].artifact() {
			t.Fatalf("claim %d: want subject %+v, got %+v", i, candidates[i].artifact(), e.Subject)
		}
		t.Logf("combined advisory %s %s: %v", e.Subject.Name, e.Subject.Version, e.Applicability)
	}
}
