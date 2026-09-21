package homebrew

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

type registrationCommit struct {
	SHA    string `json:"sha" required:"true"`
	Commit struct {
		Committer struct {
			Date string `json:"date" required:"true"`
		} `json:"committer" required:"true"`
	} `json:"commit" required:"true"`
}
type registrationObservation struct {
	Schema                int
	Candidate             domain.Artifact
	MetadataCommit        string
	DateCommit            string
	RegisteredNoLaterThan int64
	ExactChange           bool
	History               []json.RawMessage
	RecipeDigests         []domain.Digest
	Recipes               [][]byte
}

// A registered digest observed in official history establishes an upper bound
// on its introduction. Unrelated recipe edits cannot make it appear older; a
// different/rebuilt bottle digest must establish its own bound. GitHub/Homebrew
// history is trusted here, not presented as an independently signed timestamp.
func bottleRegistration(ctx context.Context, client *http.Client, candidate formulaMetadata, now int64) (domain.Evidence, []byte, error) {
	fail := func(err error) (domain.Evidence, []byte, error) { return domain.Evidence{}, nil, err }
	if !candidate.artifact().Valid() || len(candidate.TapCommit) != 40 || strings.Trim(candidate.TapCommit, "0123456789abcdef") != "" || !candidate.RecipeSHA256.Valid() || now <= 0 {
		return fail(errors.New("invalid bottle registration candidate"))
	}
	// Reuse the recipe path validation rather than accepting arbitrary API paths.
	if !strings.HasPrefix(candidate.RecipePath, "Formula/") || !strings.HasSuffix(candidate.RecipePath, "/"+candidate.Name+".rb") || strings.Contains(candidate.RecipePath, "..") {
		return fail(errors.New("invalid bottle registration path"))
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	observation := registrationObservation{Schema: 1, Candidate: candidate.artifact(), MetadataCommit: candidate.TapCommit, History: []json.RawMessage{}, RecipeDigests: []domain.Digest{}}
	seen := map[string]bool{}
	finished := false
	var previousDate int64
	for page := 1; page <= 4 && !finished; page++ {
		query := url.Values{"path": {candidate.RecipePath}, "sha": {candidate.TapCommit}, "per_page": {"20"}, "page": {strconv.Itoa(page)}}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/Homebrew/homebrew-core/commits?"+query.Encode(), nil)
		if err != nil {
			return fail(err)
		}
		request.Header.Set("Accept", "application/vnd.github+json")
		request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		request.Header.Set("User-Agent", "BrewWarden")
		response, err := client.Do(request)
		if err != nil {
			return fail(errors.New("bottle registration history unavailable"))
		}
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024+1))
		response.Body.Close()
		if readErr != nil || response.StatusCode != 200 || len(raw) > 2*1024*1024 {
			return fail(errors.New("bottle registration history incomplete"))
		}
		var commits []registrationCommit
		if err := decodeSchema(raw, &commits, true); err != nil || len(commits) > 20 {
			return fail(errors.New("invalid bottle registration history"))
		}
		observation.History = append(observation.History, raw)
		if len(commits) == 0 {
			finished = true
			break
		}
		for _, commit := range commits {
			if len(commit.SHA) != 40 || strings.Trim(commit.SHA, "0123456789abcdef") != "" || seen[commit.SHA] {
				return fail(errors.New("invalid registration commit identity"))
			}
			seen[commit.SHA] = true
			date, err := time.Parse(time.RFC3339, commit.Commit.Committer.Date)
			if err != nil || date.Unix() <= 0 || date.Unix() > now {
				return fail(errors.New("invalid bottle registration date"))
			}
			if previousDate != 0 && date.Unix() > previousDate {
				return fail(errors.New("nonmonotonic registration history"))
			}
			previousDate = date.Unix()
			recipe, err := download(ctx, client, "https://raw.githubusercontent.com/Homebrew/homebrew-core/"+commit.SHA+"/"+candidate.RecipePath, 1024*1024)
			if err != nil {
				return fail(err)
			}
			hash := digestBytes(recipe)
			if len(observation.RecipeDigests) == 0 && hash != candidate.RecipeSHA256 {
				return fail(errors.New("registration history does not match authenticated recipe"))
			}
			observation.RecipeDigests = append(observation.RecipeDigests, hash)
			observation.Recipes = append(observation.Recipes, recipe)
			if !strings.Contains(string(recipe), string(candidate.BottleSHA256)) {
				if observation.RegisteredNoLaterThan == 0 {
					return fail(errors.New("bottle digest absent from registration history"))
				}
				observation.ExactChange = true
				finished = true
				break
			}
			// Retain the oldest consecutive matching snapshot. The history above
			// rejects clock inversions rather than sorting commits by their dates.
			if observation.RegisteredNoLaterThan == 0 || date.Unix() < observation.RegisteredNoLaterThan {
				observation.RegisteredNoLaterThan = date.Unix()
				observation.DateCommit = commit.SHA
			}
		}
		if len(commits) < 20 {
			finished = true
		}
	}
	if observation.RegisteredNoLaterThan == 0 {
		return fail(errors.New("bottle registration date unavailable"))
	}
	raw, err := json.Marshal(observation)
	if err != nil {
		return fail(err)
	}
	e, err := domain.NewEvidence(domain.Evidence{Claim: domain.Publication, Subject: candidate.artifact(), Status: domain.Verified, Provider: domain.Homebrew, Source: "Homebrew bottle registration history", ProviderVersion: "github-commits-2022-11-28", RawSHA256: digestBytes(raw), ObservedAt: now, ExpiresAt: now + 3600, PublishedAt: observation.RegisteredNoLaterThan, Publication: domain.BottleRegistration})
	return e, raw, err
}
