package homebrew

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
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
	return domain.Artifact{
		Tap: "homebrew/core", Name: f.Name, Version: f.Version,
		Revision: f.Revision, Rebuild: f.Rebuild,
		OS: "macos", Arch: "arm64", BottleTag: f.BottleTag, SHA256: f.BottleSHA256,
	}
}

// Return the complete validated closure in deterministic formula-name order.
func parseMetadata(data []byte, targets []string) ([]formulaMetadata, error) {
	var document metadataDocument
	if err := decodeStrict(data, &document); err != nil || document.Schema != 1 || document.Platform != "arm64_tahoe" || len(document.Formulae) == 0 || len(document.Formulae) > 128 {
		return nil, errors.New("invalid authenticated metadata result")
	}
	index := map[string]formulaMetadata{}
	for _, formula := range document.Formulae {
		if !domain.ValidRequest("install", []string{formula.Name}) || index[formula.Name].Name != "" || formula.Revision < 0 || formula.Rebuild < 0 {
			return nil, errors.New("invalid formula metadata identity")
		}
		if formula.Dependencies == nil || len(formula.Dependencies) > 128 {
			return nil, errors.New("incomplete dependency metadata")
		}
		seen := map[string]bool{}
		for _, dependency := range formula.Dependencies {
			if !domain.ValidRequest("install", []string{dependency}) || seen[dependency] {
				return nil, errors.New("invalid dependency metadata")
			}
			seen[dependency] = true
		}
		index[formula.Name] = formula
	}

	const (
		visiting = iota + 1
		visited
	)
	state := map[string]int{}
	var visit func(string) error
	visit = func(name string) error {
		formula, exists := index[name]
		if !exists || state[name] == visiting {
			return errors.New("incomplete or cyclic metadata closure")
		}
		if state[name] == visited {
			return nil
		}
		state[name] = visiting
		for _, dependency := range formula.Dependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[name] = visited
		return nil
	}
	seenTargets := map[string]bool{}
	if len(targets) == 0 {
		return nil, errors.New("empty candidate selection")
	}
	for _, target := range targets {
		if seenTargets[target] {
			return nil, errors.New("duplicate target")
		}
		seenTargets[target] = true
		if err := visit(target); err != nil {
			return nil, err
		}
	}
	if len(state) != len(index) {
		return nil, errors.New("unrequested formula metadata")
	}

	candidates := []formulaMetadata{}
	for name := range state {
		formula := index[name]
		expectedURL := "https://ghcr.io/v2/homebrew/core/" + strings.ReplaceAll(formula.Name, "@", "/") + "/blobs/sha256:" + string(formula.BottleSHA256)
		supportedTag := slices.Contains([]string{"arm64_tahoe", "all"}, formula.BottleTag)
		supportedCellar := slices.Contains([]string{":any", ":any_skip_relocation", "/opt/homebrew/Cellar"}, formula.Cellar)
		if !formula.artifact().Valid() || !supportedTag || !supportedCellar || formula.BottleURL != expectedURL {
			return nil, fmt.Errorf("unsupported bottle for %s: tag %q, cellar %q", formula.Name, formula.BottleTag, formula.Cellar)
		}
		candidates = append(candidates, formula)
	}
	slices.SortFunc(candidates, func(a, b formulaMetadata) int { return strings.Compare(a.Name, b.Name) })
	return candidates, nil
}
