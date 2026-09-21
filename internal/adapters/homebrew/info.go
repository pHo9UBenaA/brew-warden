package homebrew

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

const metadataCachePath = "cache/api/internal/packages.arm64_tahoe.jws.json"

// These are selected fields of the public info --json=v2 contract. Homebrew
// authenticates the API snapshot; we do not parse or verify its JWS ourselves.
type infoFormula struct {
	Name     string `json:"name" required:"true"`
	FullName string `json:"full_name" required:"true"`
	Tap      string `json:"tap" required:"true"`
	Versions struct {
		Stable string `json:"stable" required:"true"`
	} `json:"versions" required:"true"`
	URLs struct {
		Stable struct {
			URL      string        `json:"url" required:"true"`
			Checksum domain.Digest `json:"checksum" required:"true"`
		} `json:"stable" required:"true"`
	} `json:"urls" required:"true"`
	Revision int `json:"revision" required:"true"`
	Bottle   struct {
		Stable struct {
			Rebuild int `json:"rebuild" required:"true"`
			Files   struct {
				Tahoe *infoBottleFile `json:"arm64_tahoe"`
				All   *infoBottleFile `json:"all"`
			} `json:"files" required:"true"`
		} `json:"stable" required:"true"`
	} `json:"bottle" required:"true"`
	Dependencies      []string `json:"dependencies" required:"true"`
	BuildDependencies []string `json:"build_dependencies" required:"true"`
	TapCommit         string   `json:"tap_git_head" required:"true"`
	RecipePath        string   `json:"ruby_source_path" required:"true"`
	RecipeChecksum    struct {
		SHA256 domain.Digest `json:"sha256" required:"true"`
	} `json:"ruby_source_checksum" required:"true"`
}

type infoBottleFile struct {
	URL    string        `json:"url" required:"true"`
	SHA256 domain.Digest `json:"sha256" required:"true"`
	Cellar string        `json:"cellar" required:"true"`
}

func parseInfo(raw []byte, names []string) ([]formulaMetadata, error) {
	var doc struct {
		Formulae []infoFormula `json:"formulae" required:"true"`
		Casks    []string      `json:"casks" required:"true"`
	}
	if err := decodeSchema(raw, &doc, true); err != nil || len(doc.Casks) != 0 || len(doc.Formulae) != len(names) {
		return nil, errors.New("incomplete Homebrew info result")
	}
	selected := map[string]bool{}
	result := make([]formulaMetadata, 0, len(names))
	for _, f := range doc.Formulae {
		if !slices.Contains(names, f.Name) || selected[f.Name] || f.FullName != f.Name || f.Tap != "homebrew/core" {
			return nil, errors.New("homebrew info identity mismatch")
		}
		selected[f.Name] = true
		m := formulaMetadata{Name: f.Name, Version: f.Versions.Stable, Revision: f.Revision, Rebuild: f.Bottle.Stable.Rebuild, SourceURL: f.URLs.Stable.URL, SourceSHA256: f.URLs.Stable.Checksum, RecipeSHA256: f.RecipeChecksum.SHA256, RecipePath: f.RecipePath, TapCommit: f.TapCommit, Dependencies: f.Dependencies, BuildDependencies: f.BuildDependencies}
		b := f.Bottle.Stable.Files.Tahoe
		m.BottleTag = "arm64_tahoe"
		if b == nil {
			b = f.Bottle.Stable.Files.All
			m.BottleTag = "all"
		}
		if b != nil {
			m.BottleURL, m.BottleSHA256, m.Cellar = b.URL, b.SHA256, b.Cellar
		}
		result = append(result, m)
	}
	return result, nil
}

func (w workspace) metadata(ctx context.Context, acquireProfile string, targets []string) ([]byte, error) {
	if !domain.ValidRequest("install", targets) || len(targets) == 0 {
		return nil, errors.New("invalid metadata request")
	}
	cache := filepath.Join(w.root, metadataCachePath)
	profile := acquireProfile
	queue := append([]string{}, targets...)
	seen := map[string]bool{}
	doc := metadataDocument{Schema: 1, Platform: "arm64_tahoe", Formulae: []formulaMetadata{}}
	for len(queue) != 0 {
		names := []string{}
		for _, name := range queue {
			if !domain.ValidRequest("install", []string{name}) {
				return nil, errors.New("invalid metadata dependency")
			}
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
		if len(seen) > 128 {
			return nil, errors.New("formula inventory exceeds limit")
		}
		if len(names) == 0 {
			break
		}
		args := []string{"info", "--json=v2", "--formula"}
		for _, name := range names {
			args = append(args, "homebrew/core/"+name)
		}
		raw, err := w.invoke(ctx, fmt.Sprintf("info-%d", len(doc.Formulae)), profile, args...)
		if err != nil {
			return nil, err
		}
		if len(doc.Formulae) == 0 {
			// Homebrew owns acquisition and authentication. Pin the actual signed
			// snapshot it used before querying further dependency batches.
			if _, err := readRegular(cache, 80*1024*1024); err != nil {
				return nil, err
			}
			profile, err = w.sandbox("metadata", false, false, []string{cache, filepath.Join(w.root, "runtime/brew/Library")})
			if err != nil {
				return nil, err
			}
		}
		batch, err := parseInfo(raw, names)
		if err != nil {
			return nil, err
		}
		doc.Formulae = append(doc.Formulae, batch...)
		queue = nil
		for _, f := range batch {
			queue = append(queue, f.Dependencies...)
		}
	}
	slices.SortFunc(doc.Formulae, func(a, b formulaMetadata) int { return strings.Compare(a.Name, b.Name) })
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	if _, _, err := parseMetadata(raw, targets); err != nil {
		return nil, err
	}
	if err := writeNew(filepath.Join(w.root, "metadata.json"), raw, 0600); err != nil {
		return nil, err
	}
	return raw, nil
}
