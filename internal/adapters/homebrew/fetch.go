package homebrew

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
)

// Download the explicitly selected closure through public Homebrew commands.
// Fetch success alone is not evidence: Homebrew may skip unavailable bottles.
func (w workspace) fetchBottles(ctx context.Context, profile string, candidates []formulaMetadata) ([]byte, error) {
	if len(candidates) == 0 || len(candidates) > 128 {
		return nil, errors.New("invalid bottle selection")
	}
	names := make([]string, 0, len(candidates))
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if !candidate.artifact().Valid() || seen[candidate.Name] {
			return nil, errors.New("invalid bottle selection")
		}
		seen[candidate.Name] = true
		names = append(names, "homebrew/core/"+candidate.Name)
	}
	// The complete closure is already selected from authenticated metadata.
	// Do not ask --deps to discover additional, unassessed download targets.
	output, err := w.invoke(ctx, "fetch", profile, append([]string{"fetch", "--formula", "--bottle-tag=arm64_tahoe"}, names...)...)
	if err != nil {
		return nil, err
	}
	if err := writeNew(filepath.Join(w.root, "fetch.stdout"), output, 0600); err != nil {
		return nil, err
	}
	paths, err := w.invoke(ctx, "cache-paths", profile, append([]string{"--cache", "--formula", "--bottle-tag=arm64_tahoe"}, names...)...)
	if err != nil {
		return nil, err
	}
	data, err := w.bottleCachePaths(paths, candidates)
	if err != nil {
		return nil, err
	}
	if err := writeNew(filepath.Join(w.root, "fetch.json"), data, 0600); err != nil {
		return nil, err
	}
	return data, nil
}

func (w workspace) bottleCachePaths(output []byte, candidates []formulaMetadata) ([]byte, error) {
	lines := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	if len(candidates) == 0 || len(lines) != len(candidates) {
		return nil, errors.New("incomplete bottle cache paths")
	}
	doc := downloadDocument{Schema: 1}
	for i, line := range lines {
		if strings.ContainsAny(line, "\x00\r") || !filepath.IsAbs(line) || !strings.HasSuffix(filepath.Base(line), "--"+nativeBottleName(candidates[i])) {
			return nil, errors.New("unexpected bottle cache path")
		}
		path, err := filepath.EvalSymlinks(line)
		if err != nil || !strings.HasPrefix(path, filepath.Join(w.root, "cache")+string(filepath.Separator)) {
			return nil, errors.New("bottle cache path missing or outside workspace")
		}
		doc.Downloads = append(doc.Downloads, downloadEntry{Name: candidates[i].Name, Path: path})
	}
	// copyDownloads still verifies each authenticated digest and archive layout.
	return json.Marshal(doc)
}
