package homebrew

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

type candidateVerifier struct {
	mutate func(*domain.Evidence)
}

func (candidateVerifier) Check(context.Context) error { return nil }

func (v candidateVerifier) VerifyEvidence(_ context.Context, artifact domain.Artifact, bottle string, now int64) (domain.Evidence, domain.Evidence, []byte, error) {
	if filepath.Base(bottle) != bottleName(metadataFixture().Formulae[0]) {
		return domain.Evidence{}, domain.Evidence{}, nil, errors.New("unexpected bottle path")
	}
	raw := []byte("verified candidate response")
	provenance := domain.Evidence{
		Claim: domain.Provenance, Subject: artifact, Status: domain.Verified,
		Provider: domain.Supplement, Source: "fixture", ProviderVersion: "test-1",
		RawSHA256: digestBytes(raw), ObservedAt: now, ExpiresAt: now + 3600,
	}
	age := provenance
	age.Claim, age.Publication, age.PublishedAt = domain.Publication, domain.VerifiedAttestation, now-60
	if v.mutate != nil {
		v.mutate(&provenance)
	}
	return provenance, age, raw, nil
}

func TestCandidateEvidenceRequiresBoundVerifierClaims(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*domain.Evidence)
	}{
		{"verified", nil},
		{"different subject", func(e *domain.Evidence) { e.Subject.Rebuild++ }},
		{"different observation", func(e *domain.Evidence) { e.ObservedAt-- }},
		{"excess validity", func(e *domain.Evidence) { e.ExpiresAt++ }},
		{"unavailable provenance", func(e *domain.Evidence) { e.Status = domain.Unavailable }},
		{"wrong claim", func(e *domain.Evidence) { e.Claim = domain.Metadata }},
		{"different response", func(e *domain.Evidence) { e.RawSHA256 = metadataFixture().Formulae[0].BottleSHA256 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := workspace{t.TempDir()}
			if err := w.initialize(); err != nil {
				t.Fatal(err)
			}
			collection := &Collection{root: w.root, observedAt: 1000, runtimeRevision: brewRevision}
			collector := Collector{BottleVerifier: candidateVerifier{mutate: tc.mutate}}
			candidates := metadataFixture().Formulae
			node, err := collector.collectCandidateEvidence(context.Background(), collection, candidates[0], candidates, []byte("authenticated metadata response"))
			if tc.mutate != nil {
				if err == nil || len(node.Evidence) != 0 {
					t.Fatalf("unbound verifier claim produced a candidate: node=%+v error=%v", node, err)
				}
				return
			}
			if err != nil || len(node.Evidence) != 4 || len(node.Dependencies) != 1 || node.Dependencies[0] != candidates[1].artifact() {
				t.Fatalf("candidate identity, closure or evidence lost: node=%+v error=%v", node, err)
			}
			for _, evidence := range node.Evidence {
				path := filepath.Join(w.root, "observations", string(evidence.RawSHA256)+".json")
				raw, err := os.ReadFile(path)
				if err != nil || digestBytes(raw) != evidence.RawSHA256 {
					t.Fatalf("claim %v observation at %s: want digest=%s got=%s error=%v", evidence.Claim, path, evidence.RawSHA256, digestBytes(raw), err)
				}
			}
		})
	}
}
