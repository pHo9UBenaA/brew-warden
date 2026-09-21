package homebrew

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

// Homebrew's public fetch caches the OCI index used by install. Bind the selected
// tab to the authenticated bottle digest and reconcile its full dependency names
// with the signed API closure. Homebrew owns minimum-version comparison at install.
func validateBottleMetadata(raw []byte, candidate formulaMetadata, candidates []formulaMetadata) error {
	var index struct {
		Schema    int `json:"schemaVersion" required:"true"`
		Manifests []struct {
			Annotations struct {
				Digest string `json:"sh.brew.bottle.digest"`
				Ref    string `json:"org.opencontainers.image.ref.name"`
				Tab    string `json:"sh.brew.tab"`
			} `json:"annotations"`
		} `json:"manifests" required:"true"`
	}
	if err := decodeSchema(raw, &index, true); err != nil || index.Schema != 2 || len(index.Manifests) == 0 || len(index.Manifests) > 128 {
		return errors.New("invalid bottle OCI index")
	}
	version := candidate.Version
	if candidate.Revision > 0 {
		version += "_" + strconv.Itoa(candidate.Revision)
	}
	ref := version + "." + candidate.BottleTag
	if candidate.Rebuild > 0 {
		ref += "." + strconv.Itoa(candidate.Rebuild)
	}
	selected := ""
	for _, manifest := range index.Manifests {
		a := manifest.Annotations
		if a.Digest != string(candidate.BottleSHA256) || a.Ref != ref {
			continue
		}
		if selected != "" || a.Tab == "" {
			return errors.New("ambiguous bottle OCI selection")
		}
		selected = a.Tab
	}
	if selected == "" {
		return errors.New("OCI index does not identify selected bottle")
	}
	var tab struct {
		Dependencies []installedDependency `json:"runtime_dependencies" required:"true"`
		Arch         string                `json:"arch"`
	}
	if err := decodeSchema([]byte(selected), &tab, true); err != nil || (candidate.BottleTag != "all" && tab.Arch != "arm64") || len(tab.Dependencies) > 128 {
		return errors.New("invalid bottle dependency metadata")
	}
	known := map[string]formulaMetadata{}
	for _, c := range candidates {
		known[c.Name] = c
	}
	wanted := map[string]bool{}
	queue := append([]string{}, candidate.Dependencies...)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if wanted[name] {
			continue
		}
		c, ok := known[name]
		if !ok || name == candidate.Name {
			return errors.New("invalid selected dependency closure")
		}
		wanted[name] = true
		queue = append(queue, c.Dependencies...)
	}
	seen := map[string]bool{}
	for _, dependency := range tab.Dependencies {
		if !wanted[dependency.Name] || seen[dependency.Name] || dependency.Version == "" || dependency.Revision < 0 || !domain.ValidRequest("install", []string{dependency.Name}) {
			return errors.New("bottle dependency graph differs from selected plan")
		}
		seen[dependency.Name] = true
	}
	// A newly selected dependency may add its own dependencies after this bottle
	// was built. Its own index is checked separately, and the full selected API
	// closure is verified and installed in dependency order.
	for _, name := range candidate.Dependencies {
		if !seen[name] {
			return errors.New("bottle dependency metadata omits a direct dependency")
		}
	}
	return nil
}

func (w workspace) checkBottleMetadata(candidates []formulaMetadata) error {
	entries, err := os.ReadDir(filepath.Join(w.root, "cache/downloads"))
	if err != nil || len(entries) > 10000 {
		return errors.New("bottle cache inventory unavailable")
	}
	for _, candidate := range candidates {
		version := candidate.Version
		if candidate.Revision > 0 {
			version += "_" + strconv.Itoa(candidate.Revision)
		}
		if candidate.Rebuild > 0 {
			version += "-" + strconv.Itoa(candidate.Rebuild)
		}
		suffix := "--" + candidate.Name + "-" + version + ".bottle_manifest.json"
		matches := slices.DeleteFunc(append([]os.DirEntry{}, entries...), func(e os.DirEntry) bool { return !strings.HasSuffix(e.Name(), suffix) })
		if len(matches) != 1 {
			return errors.New("selected bottle manifest missing or ambiguous")
		}
		raw, err := readRegular(filepath.Join(w.root, "cache/downloads", matches[0].Name()), maxManifest)
		if err != nil {
			return err
		}
		if err := validateBottleMetadata(raw, candidate, candidates); err != nil {
			return err
		}
	}
	return nil
}
