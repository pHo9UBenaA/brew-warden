package tests

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/adapters/attestation"
	"github.com/pHo9UBenaA/brew-warden/internal/adapters/githubrelease"
	"github.com/pHo9UBenaA/brew-warden/internal/adapters/homebrew"
	"github.com/pHo9UBenaA/brew-warden/internal/adapters/osv"
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

func TestLiveCandidateCollection(t *testing.T) {
	source := os.Getenv("BREWWARDEN_LIVE_COLLECTION_RUNTIME")
	if source == "" {
		t.Skip("requires explicit built runtime and public network")
	}
	raw, err := os.ReadFile(filepath.Join(source, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	directory, err := os.MkdirTemp(filepath.Dir(source), "live-collection-")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("retained collection", directory)
	collector := homebrew.Collector{Runtime: homebrew.Runtime{Root: source, ManifestSHA256: domain.Digest(hex.EncodeToString(sum[:]))}, Directory: directory, Publication: githubrelease.New(), Vulnerabilities: osv.New(), Verifier: func(path string, digest domain.Digest) ports.ProvenanceVerifier {
		return attestation.Verifier{Path: path, SHA256: digest}
	}}
	now := time.Now().Unix()
	result, err := collector.Collect(context.Background(), ports.Request{Operation: "install", Targets: []string{"jq"}}, now)
	if err != nil {
		walkErr := filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr == nil && !entry.IsDir() && strings.HasSuffix(path, ".stderr") {
				data, _ := os.ReadFile(path)
				t.Logf("%s: %s", filepath.Base(path), data)
			}
			return walkErr
		})
		if walkErr != nil {
			t.Log(walkErr)
		}
		t.Fatal(err)
	}
	nodes := result.Evidence()
	if len(nodes) != 2 || nodes[0].Artifact.Name != "jq" || nodes[1].Artifact.Name != "oniguruma" {
		t.Fatal("incomplete candidate closure", nodes)
	}
	for _, node := range nodes {
		if len(node.Evidence) != 5 {
			t.Fatal("incomplete claims", node)
		}
		for _, e := range node.Evidence {
			if e.Status != domain.Verified || e.Subject != node.Artifact || e.ObservedAt != now {
				t.Fatalf("unverified claim: %+v", e)
			}
		}
		t.Log("verified candidate", node.Artifact.Name, node.Artifact.Version, node.Artifact.SHA256)
	}
	nodes[0].Evidence[0].Status = domain.Failed
	if result.Evidence()[0].Evidence[0].Status != domain.Verified {
		t.Fatal("diagnostic view mutated collection")
	}
}
