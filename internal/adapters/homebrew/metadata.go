package homebrew

import (
	"errors"
	"fmt"
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"slices"
	"strings"
)

type formulaMetadata struct {
	BottleTag    string        `json:"bottleTag" required:"true"`
	Name         string        `json:"name" required:"true"`
	Version      string        `json:"version" required:"true"`
	Revision     int           `json:"revision" required:"true"`
	Rebuild      int           `json:"rebuild" required:"true"`
	BottleURL    string        `json:"bottleURL" required:"true"`
	BottleSHA256 domain.Digest `json:"bottleSHA256" required:"true"`
	Cellar       string        `json:"cellar" required:"true"`
	Dependencies []string      `json:"dependencies" required:"true"`
}
type metadataDocument struct {
	Schema   int               `json:"schema" required:"true"`
	Platform string            `json:"platform" required:"true"`
	Formulae []formulaMetadata `json:"formulae" required:"true"`
}

func (f formulaMetadata) artifact() domain.Artifact {
	return domain.Artifact{Tap: "homebrew/core", Name: f.Name, Version: f.Version, Revision: f.Revision, Rebuild: f.Rebuild, OS: "macos", Arch: "arm64", BottleTag: f.BottleTag, SHA256: f.BottleSHA256}
}
func parseMetadata(data []byte, targets []string) ([]formulaMetadata, []formulaMetadata, error) {
	var doc metadataDocument
	if err := decodeStrict(data, &doc); err != nil || doc.Schema != 1 || doc.Platform != "arm64_tahoe" || len(doc.Formulae) == 0 || len(doc.Formulae) > 128 {
		return nil, nil, errors.New("invalid authenticated metadata result")
	}
	index := map[string]formulaMetadata{}
	for _, f := range doc.Formulae {
		if !domain.ValidRequest("install", []string{f.Name}) || index[f.Name].Name != "" || f.Revision < 0 || f.Rebuild < 0 {
			return nil, nil, errors.New("invalid formula metadata identity")
		}
		if f.Dependencies == nil || len(f.Dependencies) > 128 {
			return nil, nil, errors.New("incomplete dependency metadata")
		}
		seen := map[string]bool{}
		for _, dep := range f.Dependencies {
			if !domain.ValidRequest("install", []string{dep}) || seen[dep] {
				return nil, nil, errors.New("invalid dependency metadata")
			}
			seen[dep] = true
		}
		index[f.Name] = f
	}
	state := map[string]int{}
	var visit func(string) error
	visit = func(name string) error {
		f, ok := index[name]
		if !ok || state[name] == 1 {
			return errors.New("incomplete or cyclic metadata closure")
		}
		if state[name] == 2 {
			return nil
		}
		state[name] = 1
		for _, dep := range f.Dependencies {
			if err := visit(dep); err != nil {
				return err
			}
		}
		state[name] = 2
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
		if err := visit(target); err != nil {
			return nil, nil, err
		}
	}
	if len(state) != len(index) {
		return nil, nil, errors.New("unrequested formula metadata")
	}
	candidates := []formulaMetadata{}
	for name := range state {
		f := index[name]
		if !f.artifact().Valid() || !slices.Contains([]string{"arm64_tahoe", "all"}, f.BottleTag) || !slices.Contains([]string{":any", ":any_skip_relocation", "/opt/homebrew/Cellar"}, f.Cellar) || f.BottleURL != "https://ghcr.io/v2/homebrew/core/"+strings.ReplaceAll(f.Name, "@", "/")+"/blobs/sha256:"+string(f.BottleSHA256) {
			return nil, nil, fmt.Errorf("unsupported bottle for %s: tag %q, cellar %q", f.Name, f.BottleTag, f.Cellar)
		}
		candidates = append(candidates, f)
	}
	slices.SortFunc(candidates, func(a, b formulaMetadata) int { return strings.Compare(a.Name, b.Name) })
	return doc.Formulae, candidates, nil
}
