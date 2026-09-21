package homebrew

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

// A validated registration fact does not change as it ages. Reuse it only for
// identical authenticated recipe bytes and artifact identity; advisories and
// cryptographic verification are still refreshed for each collection.
func cachedBottleRegistration(ctx context.Context, client *http.Client, directory string, candidate formulaMetadata, now int64) (domain.Evidence, []byte, error) {
	keyBytes, _ := json.Marshal(struct {
		Artifact domain.Artifact
		Recipe   domain.Digest
	}{candidate.artifact(), candidate.RecipeSHA256})
	key := digestBytes(keyBytes)
	raw, err := cachedEvidence(directory, "registrations", key)
	if err != nil {
		return domain.Evidence{}, nil, err
	}
	if raw != nil {
		var observation struct {
			Schema                int
			Candidate             domain.Artifact
			RegisteredNoLaterThan int64
			RecipeDigests         []domain.Digest
			Recipes               []string
		}
		if err := decodeSchema(raw, &observation, true); err != nil || observation.Schema != 1 || observation.Candidate != candidate.artifact() || observation.RegisteredNoLaterThan <= 0 || observation.RegisteredNoLaterThan > now || len(observation.RecipeDigests) == 0 || len(observation.Recipes) != len(observation.RecipeDigests) || observation.RecipeDigests[0] != candidate.RecipeSHA256 {
			return domain.Evidence{}, nil, errors.New("cached registration does not match authenticated candidate")
		}
		for i, encoded := range observation.Recipes {
			recipe, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil || digestBytes(recipe) != observation.RecipeDigests[i] {
				return domain.Evidence{}, nil, errors.New("cached registration recipe changed")
			}
		}
		e, err := domain.NewEvidence(domain.Evidence{Claim: domain.Publication, Subject: candidate.artifact(), Status: domain.Verified, Provider: domain.Homebrew, Source: "Homebrew bottle registration history (cached)", ProviderVersion: "github-commits-2022-11-28", RawSHA256: digestBytes(raw), ObservedAt: now, ExpiresAt: now + 3600, PublishedAt: observation.RegisteredNoLaterThan, Publication: domain.BottleRegistration})
		return e, raw, err
	}
	e, raw, err := bottleRegistration(ctx, client, candidate, now)
	if err != nil {
		return e, raw, err
	}
	if err := retainEvidence(directory, "registrations", key, raw); err != nil {
		return domain.Evidence{}, nil, err
	}
	return e, raw, nil
}
