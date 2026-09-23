package homebrew

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

const brewRevision = "edb70f031e4170c780799633a1226ff73e1077f4"
const maxManifest = 8 * 1024 * 1024

// SHA-256 of the sorted public Homebrew executable/Library subtree at the
// inspected revision (including its installed portable Ruby). The tree is read
// from the installed prefix, never bundled or downloaded by BrewWarden.
const supportedRuntimeDigest domain.Digest = "e4422c589c12df8ac55953c8ddf25cdcfcabf05e6e27e141f9143e671dcd614c"

type Runtime struct {
	// An explicit pin is used only by isolated adapter fixtures. Composition
	// constructs Runtime{} so a product invocation cannot select a new pin.
	ExpectedSHA256 domain.Digest
}

type runtimeEntry struct {
	Path   string        `json:"path"`
	Mode   uint32        `json:"mode"`
	SHA256 domain.Digest `json:"sha256"`
	Link   string        `json:"link"`
}
type runtimeManifest struct {
	Schema       int            `json:"schema"`
	BrewRevision string         `json:"brewRevision"`
	Files        []runtimeEntry `json:"files"`
}

func (r Runtime) materialize(destination string) (domain.Digest, error) {
	return r.materializeFrom(destination, "/opt/homebrew")
}

// Copy the installed public brew implementation to an empty inspection prefix.
// A matching reviewed tree establishes version support; file-by-file comparison
// before execution detects changes between collection and mutation. No runtime
// files or inventory are read from the distribution or .cache.
func (r Runtime) materializeFrom(destination, prefix string) (domain.Digest, error) {
	if !filepath.IsAbs(destination) || !filepath.IsAbs(prefix) {
		return "", errors.New("invalid Homebrew inspection prefix")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(destination))
	if err != nil {
		return "", err
	}
	destination = filepath.Join(parent, filepath.Base(destination))
	if destination == prefix || strings.HasPrefix(destination, prefix+string(filepath.Separator)) {
		return "", errors.New("inspection prefix overlaps installed Homebrew")
	}
	pin := r.ExpectedSHA256
	if pin == "" {
		pin = supportedRuntimeDigest
	}
	if !pin.Valid() {
		return "", errors.New("unsupported Homebrew version pin")
	}
	if err := os.Mkdir(destination, 0700); err != nil {
		return "", err
	}
	brew := filepath.Join(destination, "brew")
	if err := os.Mkdir(brew, 0700); err != nil {
		return "", err
	}
	manifest := runtimeManifest{Schema: 2, BrewRevision: brewRevision, Files: []runtimeEntry{}}
	var total int64
	for _, root := range []string{"bin/brew", "Library/Homebrew"} {
		err := filepath.WalkDir(filepath.Join(prefix, root), func(file string, item os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(prefix, file)
			if err != nil || !safeRelative(filepath.ToSlash(relative)) {
				return errors.New("unsafe Homebrew runtime path")
			}
			if len(manifest.Files) >= 10000 {
				return errors.New("installed Homebrew runtime inventory exceeds limit")
			}
			dst := filepath.Join(brew, relative)
			if item.IsDir() {
				return os.MkdirAll(dst, 0700)
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
				return err
			}
			info, err := item.Info()
			if err != nil {
				return err
			}
			entry := runtimeEntry{Path: filepath.ToSlash(path.Join("brew", filepath.ToSlash(relative))), Mode: 0644}
			if info.Mode()&os.ModeSymlink != 0 {
				entry.Link, err = os.Readlink(file)
				if err != nil || path.IsAbs(entry.Link) || !safeRelative(path.Clean(path.Join(path.Dir(entry.Path), entry.Link))) {
					return errors.New("unsafe Homebrew runtime link")
				}
				if err := os.Symlink(entry.Link, dst); err != nil {
					return err
				}
			} else {
				if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > 128*1024*1024 {
					return errors.New("unsupported Homebrew runtime file")
				}
				total += info.Size()
				if total > 2*1024*1024*1024 {
					return errors.New("installed Homebrew runtime exceeds size limit")
				}
				if info.Mode().Perm()&0111 != 0 {
					entry.Mode = 0755
				}
				data, err := readRegular(file, 128*1024*1024)
				if err != nil {
					return err
				}
				entry.SHA256 = digestBytes(data)
				if err := writeNew(dst, data, os.FileMode(entry.Mode)); err != nil {
					return err
				}
			}
			manifest.Files = append(manifest.Files, entry)
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	for _, entry := range manifest.Files {
		if entry.Link != "" {
			resolved, err := filepath.EvalSymlinks(filepath.Join(destination, filepath.FromSlash(entry.Path)))
			if err != nil || !strings.HasPrefix(resolved, brew+string(filepath.Separator)) {
				return "", errors.New("installed Homebrew runtime link escapes or is unresolved")
			}
		}
	}
	slices.SortFunc(manifest.Files, func(a, b runtimeEntry) int { return strings.Compare(a.Path, b.Path) })
	raw, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	raw = append(raw, '\n')
	digest := digestBytes(raw)
	if digest != pin {
		return digest, errors.New("installed Homebrew version or runtime differs from supported build")
	}
	return digest, nil
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
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
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

func writeRecord(file string, data []byte) error {
	if _, err := os.Lstat(file); !errors.Is(err, os.ErrNotExist) {
		return errors.New("record already exists or is inaccessible")
	}
	pending := file + ".pending"
	if err := writeNew(pending, data, 0600); err != nil {
		return err
	}
	if err := os.Rename(pending, file); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(file))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
