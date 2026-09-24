package homebrew

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

const brewRevision = "edb70f031e4170c780799633a1226ff73e1077f4"
const maxManifest = 8 * 1024 * 1024

// Inspected upstream release commits with the public signed-metadata, bottle,
// scanner-coverage and installer capabilities required by this adapter. A
// version string alone cannot select an execution implementation.
var reviewedBrewRevisions = map[string]string{
	"0942cac2eda7648d4857f4e5da60f1de303b6818": "6.0.19",
	"3e3da86e4eaccd23f4ce43df9b0a31b16c707347": "6.0.20",
	"560147012b9678b42ef5e83b690f0895552d1366": "6.0.21",
	"08e85c4e42f5d8f1ea17c36cb59cf61c2ccb26c3": "6.0.22",
	"d79ef822ab8136e393ed5f86e2b56afc68d04874": "7.0.0",
	"b3625f73d3e3574c5789ee32eb7b06627b788ec4": "7.0.1",
	"83c9802fb54c60a612d52b264808c099b39a0687": "7.0.2",
	"99fd9a8eed4ff942c448da0c1f11156302441e4a": "7.0.3",
	"edb70f031e4170c780799633a1226ff73e1077f4": "7.0.4",
	"a83186e02a6c4b98cd44a290cdb56e136feaa596": "7.0.5",
	"570982948a8a194f0f42f43f4a5bce2d1c9f64cb": "7.0.6",
}

// Legacy accepted tar-only native fixture: SHA-256 of the sorted public
// Homebrew executable/Library subtree at the inspected 7.0.4 revision
// (including its installed portable Ruby). The tree is read
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

func runInstalledGit(prefix string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/usr/bin/git", append([]string{"-c", "core.fsmonitor=false", "-C", prefix}, args...)...)
	command.Env = []string{"HOME=/nonexistent", "PATH=/usr/bin:/bin", "GIT_OPTIONAL_LOCKS=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}
	command.Dir = "/"
	output, diagnostics := &processOutput{}, &processOutput{}
	command.Stdout, command.Stderr = output, diagnostics
	if err := command.Run(); err != nil || output.overflow || diagnostics.overflow || ctx.Err() != nil {
		return "", errors.New("reviewed Homebrew source is unavailable")
	}
	return strings.TrimSpace(output.String()), nil
}

// Release identity comes from the local upstream Git checkout, not from a
// printable version string. Untracked Ruby distributions and taps are data;
// tracked/extra executable Homebrew source must remain at the reviewed commit.
func reviewedBrewSource(prefix string) (string, error) {
	if !filepath.IsAbs(prefix) {
		return "", errors.New("unsupported Homebrew prefix")
	}
	revision, err := runInstalledGit(prefix, "rev-parse", "--verify", "HEAD")
	if err != nil || reviewedBrewRevisions[revision] == "" {
		return "", errors.New("unsupported Homebrew source revision")
	}
	changes, err := runInstalledGit(prefix, "status", "--porcelain=v1", "--untracked-files=all", "--", "bin/brew", "Library/Homebrew", ":!Library/Homebrew/vendor/portable-ruby", ":!Library/Taps")
	if err != nil || changes != "" {
		return "", errors.New("installed Homebrew source differs from reviewed release")
	}
	return revision, nil
}

func (r Runtime) installedRevision(digest domain.Digest) (string, error) {
	if revision, err := reviewedBrewSource("/opt/homebrew"); err == nil {
		return revision, nil
	}
	if digest == supportedRuntimeDigest {
		return brewRevision, nil // Legacy tar-only reviewed native fixture.
	}
	return "", errors.New("reviewed Homebrew release identity unavailable")
}

// Copy the reviewed public brew implementation to an empty inspection prefix.
// The copied runtime digest binds the entire implementation for this operation;
// file-by-file comparison before execution detects changes. No runtime files
// or inventory are read from the distribution or .cache.
func (r Runtime) materializeFrom(destination, prefix string) (domain.Digest, error) {
	if !filepath.IsAbs(destination) || !filepath.IsAbs(prefix) {
		return "", errors.New("invalid Homebrew inspection prefix")
	}
	if prefix == "/opt/homebrew" {
		resolved, err := filepath.EvalSymlinks(prefix)
		if err != nil || resolved != prefix {
			return "", errors.New("standard Homebrew prefix is unsafe")
		}
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
	if pin != "" && !pin.Valid() {
		return "", errors.New("invalid Homebrew runtime fixture pin")
	}
	revision := brewRevision
	sourceReviewed := false
	if pin == "" {
		// The legacy 7.0.4 tar-only acceptance fixture is matched by its full
		// fingerprint below. Normal installations select a reviewed Git release.
		if resolved, err := reviewedBrewSource(prefix); err == nil {
			revision, sourceReviewed = resolved, true
		}
	}
	if err := os.Mkdir(destination, 0700); err != nil {
		return "", err
	}
	brew := filepath.Join(destination, "brew")
	if err := os.Mkdir(brew, 0700); err != nil {
		return "", err
	}
	manifest := runtimeManifest{Schema: 2, BrewRevision: revision, Files: []runtimeEntry{}}
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
	if pin != "" && digest != pin || pin == "" && !sourceReviewed && digest != supportedRuntimeDigest {
		return digest, errors.New("installed Homebrew version or runtime differs from supported build")
	}
	if pin == "" && sourceReviewed {
		fresh, err := reviewedBrewSource(prefix)
		if err != nil || fresh != revision {
			return digest, errors.New("installed Homebrew source changed during inspection")
		}
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
