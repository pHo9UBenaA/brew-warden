package homebrew

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// This explicit guest-only probe exercises the actual public Homebrew commands
// in a private inspection prefix. It never installs into the guest or host.
func TestLiveReviewedHomebrewPublicEvidenceContract(t *testing.T) {
	if os.Getenv("BREWWARDEN_LIVE_PREFIX") == "" {
		t.Skip("requires an explicitly provisioned disposable native guest")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" || os.Getenv("BREWWARDEN_LIVE_PREFIX") != "/opt/homebrew" {
		t.Fatal("requires a disposable native arm64 guest at the standard prefix")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := workspace{root: root}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	digest, err := (Runtime{}).materialize(filepath.Join(root, "runtime"))
	if err != nil || !digest.Valid() {
		t.Fatalf("reviewed Homebrew runtime unavailable: %v", err)
	}
	profile, err := w.sandbox("cohort-collect", true, false, []string{filepath.Join(root, "runtime/brew/Library")})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	metadata, err := w.metadata(ctx, profile, []string{"jq"})
	if err != nil {
		t.Fatalf("signed public metadata and closure unavailable: %v", err)
	}
	if _, err := readRegular(filepath.Join(root, metadataCachePath), 80*1024*1024); err != nil {
		t.Fatalf("authenticated Homebrew snapshot absent: %v", err)
	}
	_, candidates, err := parseMetadata(metadata, []string{"jq"})
	if err != nil || len(candidates) < 2 {
		t.Fatalf("public signed dependency closure unavailable: %v", err)
	}
	downloads, err := w.fetchBottles(ctx, profile, candidates)
	if err != nil {
		t.Fatalf("exact bottles could not be fetched: %v", err)
	}
	if err := w.copyDownloads(downloads, candidates); err != nil {
		t.Fatalf("downloaded bytes do not match signed bottle identities: %v", err)
	}
	if err := w.checkBottleMetadata(candidates); err != nil {
		t.Fatalf("bottle OCI dependency evidence unavailable: %v", err)
	}
	report, _, err := w.scanCandidateVulnerabilities(ctx, candidates)
	if err != nil || len(report.Skipped) != 0 {
		t.Fatalf("public scanner did not cover the complete candidate closure: %v", err)
	}
	t.Logf("reviewed native runtime %s supplied signed metadata, exact bottles and advisory subjects for %d candidates", digest, len(candidates))
}
