// Build-only packaging of explicitly pinned native dependencies. Never invoked
// by the product, hooks or offline checks.
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const brewRevision = "edb70f031e4170c780799633a1226ff73e1077f4"
const rubySHA = "e0088dff5614b39387300136ec7a5f95bf1e07589547245c919524fc9e8b4197"
const verifierSHA = "d0813b4f0e4c992036780d491e814389f8b549af5cf996406b027e946707b817"

type entry struct {
	Path   string `json:"path"`
	Mode   uint32 `json:"mode"`
	SHA256 string `json:"sha256"`
	Link   string `json:"link"`
}
type manifest struct {
	Schema       int     `json:"schema"`
	BrewRevision string  `json:"brewRevision"`
	Files        []entry `json:"files"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) != 4 {
		return fmt.Errorf("usage: runtime-pack /Homebrew/source /portable-ruby.tar.gz /verifier /new-runtime-directory")
	}
	for _, arg := range args {
		if !filepath.IsAbs(arg) {
			return fmt.Errorf("absolute paths required")
		}
	}
	if err := os.Mkdir(args[3], 0700); err != nil {
		return err
	}
	brew := filepath.Join(args[3], "brew")
	if err := os.Mkdir(brew, 0755); err != nil {
		return err
	}
	command := exec.Command("/usr/bin/git", "-C", args[0], "archive", "--format=tar", brewRevision)
	pipe, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return err
	}
	extractErr := extract(tar.NewReader(pipe), brew)
	if extractErr != nil {
		_ = command.Process.Kill()
	}
	waitErr := command.Wait()
	if extractErr != nil {
		return extractErr
	}
	if waitErr != nil {
		return waitErr
	}
	ruby, err := os.ReadFile(args[1])
	if err != nil {
		return err
	}
	if digest(ruby) != rubySHA {
		return fmt.Errorf("portable Ruby checksum mismatch")
	}
	archive, err := os.Open(args[1])
	if err != nil {
		return err
	}
	defer archive.Close()
	compressed, err := gzip.NewReader(archive)
	if err != nil {
		return err
	}
	defer compressed.Close()
	if err := extract(tar.NewReader(compressed), filepath.Join(brew, "Library/Homebrew/vendor")); err != nil {
		return err
	}
	if err := os.Symlink("4.0.7", filepath.Join(brew, "Library/Homebrew/vendor/portable-ruby/current")); err != nil {
		return err
	}
	verifier, err := os.ReadFile(args[2])
	if err != nil {
		return err
	}
	if digest(verifier) != verifierSHA {
		return fmt.Errorf("verifier checksum mismatch; rebuild and review its pin")
	}
	if err := os.WriteFile(filepath.Join(args[3], "verifier"), verifier, 0755); err != nil {
		return err
	}
	m := manifest{1, brewRevision, []entry{}}
	err = filepath.WalkDir(args[3], func(file string, item os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if item.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(args[3], file)
		if err != nil {
			return err
		}
		info, err := item.Info()
		if err != nil {
			return err
		}
		e := entry{Path: filepath.ToSlash(relative), Mode: 0644}
		if info.Mode()&os.ModeSymlink != 0 {
			e.Link, err = os.Readlink(file)
			if err != nil {
				return err
			}
			resolved, err := filepath.EvalSymlinks(file)
			if err != nil || !strings.HasPrefix(resolved, args[3]+string(filepath.Separator)) {
				return fmt.Errorf("unresolved or escaping runtime link: %s", relative)
			}
		} else {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("unsupported runtime file")
			}
			if info.Mode().Perm()&0111 != 0 {
				e.Mode = 0755
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			e.SHA256 = digest(data)
		}
		m.Files = append(m.Files, e)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(args[3], "manifest.json"), data, 0644); err != nil {
		return err
	}
	fmt.Println(digest(data))
	return nil
}
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func extract(reader *tar.Reader, root string) error {
	var total int64
	for count := 0; ; count++ {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if count >= 30000 {
			return fmt.Errorf("archive inventory exceeds limit")
		}
		if header.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		name := strings.TrimSuffix(header.Name, "/")
		if name == "" || name == "." {
			continue
		}
		if !filepath.IsLocal(name) || filepath.Clean(name) != name {
			return fmt.Errorf("unsafe archive path")
		}
		target := filepath.Join(root, name)
		// Refuse extraction through archive-supplied symlink parents.
		for parent := filepath.Dir(target); parent != root && parent != filepath.Dir(parent); parent = filepath.Dir(parent) {
			if info, err := os.Lstat(parent); err == nil && !info.IsDir() {
				return fmt.Errorf("unsafe archive parent")
			}
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if filepath.IsAbs(header.Linkname) || !filepath.IsLocal(filepath.Clean(filepath.Join(filepath.Dir(name), header.Linkname))) {
				return fmt.Errorf("unsafe archive link")
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				return err
			}
		case tar.TypeReg:
			if header.Size < 0 || header.Size > 128*1024*1024 {
				return fmt.Errorf("archive file exceeds limit")
			}
			total += header.Size
			if total > 1024*1024*1024 {
				return fmt.Errorf("archive exceeds limit")
			}
			mode := os.FileMode(0644)
			if header.Mode&0111 != 0 {
				mode = 0755
			}
			file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, reader)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("unsupported archive entry type")
		}
	}
}
