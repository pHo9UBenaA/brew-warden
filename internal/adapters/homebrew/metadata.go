package homebrew

import (
	"errors"
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"path"
	"slices"
	"strings"
)

type formulaMetadata struct {
	Name              string        `json:"name" required:"true"`
	Version           string        `json:"version" required:"true"`
	Revision          int           `json:"revision" required:"true"`
	Rebuild           int           `json:"rebuild" required:"true"`
	SourceURL         string        `json:"sourceURL" required:"true"`
	SourceSHA256      domain.Digest `json:"sourceSHA256" required:"true"`
	RecipeSHA256      domain.Digest `json:"recipeSHA256" required:"true"`
	RecipePath        string        `json:"recipePath" required:"true"`
	TapCommit         string        `json:"tapCommit" required:"true"`
	BottleURL         string        `json:"bottleURL" required:"true"`
	BottleSHA256      domain.Digest `json:"bottleSHA256" required:"true"`
	Cellar            string        `json:"cellar" required:"true"`
	Dependencies      []string      `json:"dependencies" required:"true"`
	BuildDependencies []string      `json:"buildDependencies" required:"true"`
}
type metadataDocument struct {
	Schema   int               `json:"schema" required:"true"`
	Platform string            `json:"platform" required:"true"`
	Formulae []formulaMetadata `json:"formulae" required:"true"`
}

func (f formulaMetadata) artifact() domain.Artifact {
	return domain.Artifact{Tap: "homebrew/core", Name: f.Name, Version: f.Version, Revision: f.Revision, Rebuild: f.Rebuild, OS: "macos", Arch: "arm64", BottleTag: "arm64_tahoe", SHA256: f.BottleSHA256}
}
func parseMetadata(data []byte, targets []string) ([]formulaMetadata, []formulaMetadata, error) {
	var doc metadataDocument
	if err := decodeStrict(data, &doc); err != nil || doc.Schema != 1 || doc.Platform != "arm64_tahoe" || len(doc.Formulae) == 0 || len(doc.Formulae) > 128 {
		return nil, nil, errors.New("invalid authenticated metadata result")
	}
	index := map[string]formulaMetadata{}
	for _, f := range doc.Formulae {
		if !domain.ValidRequest("install", []string{f.Name}) || index[f.Name].Name != "" || !f.SourceSHA256.Valid() || !f.RecipeSHA256.Valid() || f.Revision < 0 || f.Rebuild < 0 || len(f.TapCommit) != 40 || strings.Trim(f.TapCommit, "0123456789abcdef") != "" {
			return nil, nil, errors.New("invalid formula metadata identity")
		}
		directory := f.Name[:1]
		if strings.HasPrefix(f.Name, "lib") {
			directory = "lib"
		}
		if f.RecipePath != path.Join("Formula", directory, f.Name+".rb") || !strings.HasPrefix(f.SourceURL, "https://") {
			return nil, nil, errors.New("unsupported formula source")
		}
		for _, deps := range [][]string{f.Dependencies, f.BuildDependencies} {
			seen := map[string]bool{}
			if deps == nil || len(deps) > 128 {
				return nil, nil, errors.New("incomplete dependency metadata")
			}
			for _, dep := range deps {
				if !domain.ValidRequest("install", []string{dep}) || seen[dep] {
					return nil, nil, errors.New("invalid dependency metadata")
				}
				seen[dep] = true
			}
		}
		index[f.Name] = f
	}
	state := map[string]int{}
	reachable := map[string]bool{}
	runtimeNames := map[string]bool{}
	var visit func(string, bool) error
	visit = func(name string, runtime bool) error {
		f, ok := index[name]
		if !ok || state[name] == 1 {
			return errors.New("incomplete or cyclic metadata closure")
		}
		if runtime {
			runtimeNames[name] = true
		}
		if state[name] == 2 && (!runtime || reachable[name]) {
			return nil
		}
		state[name] = 1
		for _, dep := range f.Dependencies {
			if err := visit(dep, runtime); err != nil {
				return err
			}
		}
		for _, dep := range f.BuildDependencies {
			if err := visit(dep, false); err != nil {
				return err
			}
		}
		state[name] = 2
		if runtime {
			reachable[name] = true
		}
		return nil
	}
	seenTargets := map[string]bool{}
	if len(targets) == 0 {
		return nil, nil, errors.New("empty candidate selection")
	}
	for _, target := range targets {
		if seenTargets[target] {
			return nil, nil, errors.New("duplicate target")
		}
		seenTargets[target] = true
		if err := visit(target, true); err != nil {
			return nil, nil, err
		}
	}
	if len(state) != len(index) {
		return nil, nil, errors.New("unrequested formula metadata")
	}
	candidates := []formulaMetadata{}
	for name := range runtimeNames {
		f := index[name]
		if !f.artifact().Valid() || !slices.Contains([]string{":any", ":any_skip_relocation"}, f.Cellar) || f.BottleURL != "https://ghcr.io/v2/homebrew/core/"+strings.ReplaceAll(f.Name, "@", "/")+"/blobs/sha256:"+string(f.BottleSHA256) {
			return nil, nil, errors.New("unsupported candidate bottle")
		}
		candidates = append(candidates, f)
	}
	slices.SortFunc(candidates, func(a, b formulaMetadata) int { return strings.Compare(a.Name, b.Name) })
	return doc.Formulae, candidates, nil
}
