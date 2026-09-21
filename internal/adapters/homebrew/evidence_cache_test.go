package homebrew

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

func TestEvidenceCacheRoundTripAndCorruption(t *testing.T) {
	directory := t.TempDir()
	id := digestBytes([]byte("subject"))
	if raw, err := cachedEvidence(directory, "attestations", id); err != nil || raw != nil {
		t.Fatal(raw, err)
	}
	data := []byte(`{"attestations":[]}`)
	if err := retainEvidence(directory, "attestations", id, data); err != nil {
		t.Fatal(err)
	}
	raw, err := cachedEvidence(directory, "attestations", id)
	if err != nil || string(raw) != string(data) {
		t.Fatal(raw, err)
	}
	path, err := evidenceCachePath(directory, "attestations", id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"SHA256":"`+string(digestBytes(data))+`","Data":"changed"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := cachedEvidence(directory, "attestations", id); err == nil {
		t.Fatal("corrupted cached input accepted")
	}
}

func TestRegistrationCacheBindsRecipeAndArtifact(t *testing.T) {
	directory := t.TempDir()
	candidate := metadataFixture().Formulae[0]
	recipe := []byte("selected bottle " + string(candidate.BottleSHA256))
	candidate.RecipeSHA256 = digestBytes(recipe)
	keyBytes, _ := json.Marshal(struct {
		Artifact domain.Artifact
		Recipe   domain.Digest
	}{candidate.artifact(), candidate.RecipeSHA256})
	raw, _ := json.Marshal(registrationObservation{Schema: 1, Candidate: candidate.artifact(), RegisteredNoLaterThan: 100, RecipeDigests: []domain.Digest{candidate.RecipeSHA256}, Recipes: [][]byte{recipe}})
	if err := retainEvidence(directory, "registrations", digestBytes(keyBytes), raw); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: cacheRejectNetwork{}}
	evidence, _, err := cachedBottleRegistration(context.Background(), client, directory, candidate, 1000)
	if err != nil || evidence.PublishedAt != 100 || evidence.ObservedAt != 1000 || evidence.Subject != candidate.artifact() {
		t.Fatal(evidence, err)
	}
	candidate.Rebuild++
	if _, _, err := cachedBottleRegistration(context.Background(), client, directory, candidate, 1000); err == nil {
		t.Fatal("different artifact reused cached registration")
	}
}

type cacheRejectNetwork struct{}

func (cacheRejectNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, os.ErrPermission
}
