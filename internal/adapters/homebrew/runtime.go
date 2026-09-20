package homebrew

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"brewwarden/internal/domain"
)

const brewRevision = "edb70f031e4170c780799633a1226ff73e1077f4"
const maxManifest = 8 * 1024 * 1024

type Runtime struct {
	Root           string
	ManifestSHA256 domain.Digest
}
type runtimeEntry struct {
	Path   string        `json:"path" required:"true"`
	Mode   uint32        `json:"mode" required:"true"`
	SHA256 domain.Digest `json:"sha256" required:"true"`
	Link   string        `json:"link" required:"true"`
}
type runtimeManifest struct {
	Schema       int            `json:"schema" required:"true"`
	BrewRevision string         `json:"brewRevision" required:"true"`
	Files        []runtimeEntry `json:"files" required:"true"`
}

// The manifest digest is selected by the distribution, not by package metadata.
// Copy only inventoried inputs; unexpected executable/search-path content fails.
func (r Runtime) materialize(destination string) (domain.Digest, error) {
	if !filepath.IsAbs(r.Root) || !r.ManifestSHA256.Valid() || !filepath.IsAbs(destination) {
		return "", errors.New("trusted runtime is unavailable")
	}
	raw, err := readRegular(filepath.Join(r.Root, "manifest.json"), maxManifest)
	if err != nil || digestBytes(raw) != r.ManifestSHA256 {
		return "", errors.New("runtime manifest integrity mismatch")
	}
	var manifest runtimeManifest
	if err := decodeStrict(raw, &manifest); err != nil || manifest.Schema != 1 || manifest.BrewRevision != brewRevision || len(manifest.Files) == 0 || len(manifest.Files) > 30000 {
		return "", errors.New("unsupported runtime manifest")
	}
	entries := map[string]runtimeEntry{}
	for _, entry := range manifest.Files {
		if !safeRelative(entry.Path) || entry.Path == "manifest.json" || entry.Mode != 0644 && entry.Mode != 0755 || entries[entry.Path].Path != "" {
			return "", errors.New("unsafe runtime inventory")
		}
		if entry.Link != "" {
			if entry.SHA256 != "" || path.IsAbs(entry.Link) || !safeRelative(path.Clean(path.Join(path.Dir(entry.Path), entry.Link))) {
				return "", errors.New("unsafe runtime symlink")
			}
		} else if !entry.SHA256.Valid() {
			return "", errors.New("missing runtime file digest")
		}
		entries[entry.Path] = entry
	}
	for _, entry := range entries {
		for parent := path.Dir(entry.Path); parent != "."; parent = path.Dir(parent) {
			if _, exists := entries[parent]; exists {
				return "", errors.New("runtime file used as parent")
			}
		}
	}
	err = filepath.WalkDir(r.Root, func(file string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(r.Root, file)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == "manifest.json" {
			return nil
		}
		if _, ok := entries[relative]; !ok {
			return errors.New("unlisted runtime input")
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if err := os.Mkdir(destination, 0700); err != nil {
		return "", err
	}
	for _, entry := range manifest.Files {
		src, dst := filepath.Join(r.Root, filepath.FromSlash(entry.Path)), filepath.Join(destination, filepath.FromSlash(entry.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
			return "", err
		}
		if entry.Link != "" {
			info, err := os.Lstat(src)
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				return "", errors.New("runtime symlink replaced")
			}
			link, err := os.Readlink(src)
			if err != nil || link != entry.Link {
				return "", errors.New("runtime link changed")
			}
			if err := os.Symlink(link, dst); err != nil {
				return "", err
			}
		} else {
			data, err := readRegular(src, 128*1024*1024)
			if err != nil || digestBytes(data) != entry.SHA256 {
				return "", errors.New("runtime file integrity mismatch")
			}
			if err := writeNew(dst, data, os.FileMode(entry.Mode)); err != nil {
				return "", err
			}
		}
	}
	for _, entry := range manifest.Files {
		if entry.Link != "" {
			resolved, err := filepath.EvalSymlinks(filepath.Join(destination, filepath.FromSlash(entry.Path)))
			if err != nil || !strings.HasPrefix(resolved, destination+string(filepath.Separator)) {
				return "", errors.New("runtime link escapes or is unresolved")
			}
		}
	}
	verifier, ok := entries["verifier"]
	if !ok || !verifier.SHA256.Valid() || verifier.Mode != 0755 {
		return "", errors.New("runtime verifier is missing")
	}
	return verifier.SHA256, nil
}

func safeRelative(value string) bool {
	if value == "" || value == "." || value == ".." || strings.HasPrefix(value, "../") || path.IsAbs(value) || path.Clean(value) != value || strings.Contains(value, "\\") {
		return false
	}
	for _, c := range value {
		if c < 32 || c > 126 {
			return false
		}
	}
	return true
}
func digestBytes(data []byte) domain.Digest {
	sum := sha256.Sum256(data)
	return domain.Digest(hex.EncodeToString(sum[:]))
}
func readRegular(file string, limit int64) ([]byte, error) {
	info, err := os.Lstat(file)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("input is not a bounded regular file")
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errors.New("input changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("input exceeds limit")
	}
	return data, nil
}
func writeNew(file string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	dir, err := os.Open(filepath.Dir(file))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
