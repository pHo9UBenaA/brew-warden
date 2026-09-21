// Build-only deterministic distribution packaging. No network or installation.
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type module struct{ Path, Version, Dir, Sum string }
type item struct {
	Name string
	Data []byte
	Mode int64
	Link string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func hash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func run(args []string) error {
	if len(args) != 6 {
		return fmt.Errorf("usage: release-pack BINARY RUNTIME MODULES_JSON GO_LICENSE SOURCE_REVISION OUTPUT.tar.gz")
	}
	binary, runtimeRoot, modulesFile, goLicense, revision, output := args[0], args[1], args[2], args[3], args[4], args[5]
	if len(revision) != 40 || strings.Trim(revision, "0123456789abcdef") != "" {
		return fmt.Errorf("invalid source revision")
	}
	items := []item{}
	add := func(name, file string, mode int64) error {
		raw, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		items = append(items, item{Name: name, Data: raw, Mode: mode})
		return nil
	}
	if err := add("bwd", binary, 0755); err != nil {
		return err
	}
	items = append(items, item{Name: "brewwarden", Link: "bwd", Mode: 0755})
	docs, err := filepath.Glob("docs/*.md")
	if err != nil {
		return err
	}
	adapterDocs, err := filepath.Glob("internal/adapters/*/README.md")
	if err != nil {
		return err
	}
	docs = append(docs, adapterDocs...)
	docs = append(docs, "CONTRIBUTING.md")
	for _, file := range docs {
		if err := add(filepath.ToSlash(file), file, 0644); err != nil {
			return err
		}
	}

	if err := add("LICENSE", "LICENSE", 0644); err != nil {
		return err
	}
	if err := add("README.md", "README.md", 0644); err != nil {
		return err
	}
	if err := add("licenses/Go-LICENSE", goLicense, 0644); err != nil {
		return err
	}
	manifestRaw, err := os.ReadFile(filepath.Join(runtimeRoot, "manifest.json"))
	if err != nil {
		return err
	}
	const pinned = "d50f6a3f967fae22b9a84cf705d59b29e15b8ee2311c984f050ce85b2f8c1ce7"
	if hash(manifestRaw) != pinned {
		return fmt.Errorf("unsupported runtime inventory digest")
	}

	var manifest struct {
		Schema       int
		BrewRevision string
		Files        []struct {
			Path         string
			Mode         uint32
			SHA256, Link string
		}
	}
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return err
	}
	if manifest.Schema != 2 || manifest.BrewRevision != "edb70f031e4170c780799633a1226ff73e1077f4" {
		return fmt.Errorf("unsupported runtime inventory")
	}
	for _, f := range manifest.Files {
		if strings.HasPrefix(f.Path, "brew/") {
			continue
		}
		if f.Path != "verifier" {
			return fmt.Errorf("unexpected bundled runtime input")
		}
		file := filepath.Join(runtimeRoot, filepath.FromSlash(f.Path))
		info, err := os.Lstat(file)
		if err != nil {
			return err
		}
		if f.Link != "" {
			link, err := os.Readlink(file)
			if err != nil || link != f.Link {
				return fmt.Errorf("runtime link mismatch")
			}
			items = append(items, item{Name: "runtime/" + f.Path, Link: link, Mode: int64(f.Mode)})
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("runtime input is not regular")
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if hash(data) != f.SHA256 {
			return fmt.Errorf("runtime checksum mismatch: %s", f.Path)
		}
		items = append(items, item{Name: "runtime/" + f.Path, Data: data, Mode: int64(f.Mode)})
	}
	items = append(items, item{Name: "runtime/manifest.json", Data: manifestRaw, Mode: 0644})
	modules := map[string]module{}
	stream, err := os.Open(modulesFile)
	if err != nil {
		return err
	}
	defer stream.Close()
	decoder := json.NewDecoder(stream)
	for {
		var m module
		err := decoder.Decode(&m)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		modules[m.Path] = m
	}
	info, err := buildinfo.ReadFile(filepath.Join(runtimeRoot, "verifier"))
	if err != nil {
		return err
	}
	notice := "BrewWarden dependencies\n\nSource revision: " + revision + "\nRuntime inventory SHA-256: " + pinned + "\n\nHomebrew is reused from the existing installation and is not bundled.\nAttestation-only helper: github.com/cli/cli/v2 v2.101.0\nThe following Go modules are recorded in the helper binary.\n\n"
	linked := append(info.Deps, &info.Main)
	sort.Slice(linked, func(i, j int) bool { return linked[i].Path < linked[j].Path })
	for _, dependency := range linked {
		m, ok := modules[dependency.Path]
		if !ok || m.Dir == "" {
			return fmt.Errorf("missing source module %s", dependency.Path)
		}
		// The helper's main module is built from a reviewed local copy.
		if dependency.Path != info.Main.Path && (m.Version != dependency.Version || m.Sum != dependency.Sum) {
			return fmt.Errorf("module version mismatch")
		}
		count := 0
		err := filepath.WalkDir(m.Dir, func(file string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if entry.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			name := strings.ToLower(entry.Name())
			if !(strings.Contains(name, "license") || strings.HasPrefix(name, "copying") || strings.HasPrefix(name, "notice")) {
				return nil
			}
			stat, err := entry.Info()
			if err != nil {
				return err
			}
			if !stat.Mode().IsRegular() || stat.Size() > 2*1024*1024 {
				return nil
			}
			relative, err := filepath.Rel(m.Dir, file)
			if err != nil {
				return err
			}
			count++
			return add("licenses/go/"+m.Path+"/"+filepath.ToSlash(relative), file, 0644)
		})
		if err != nil {
			return err
		}
		if count == 0 {
			return fmt.Errorf("no license found for linked module %s", m.Path)
		}
		notice += m.Path + " " + m.Version + " " + m.Sum + "\n"
	}
	items = append(items, item{Name: "THIRD_PARTY_NOTICES.txt", Data: []byte(notice), Mode: 0644})
	return archive(output, items)
}
func archive(output string, items []item) error {
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	sums := ""
	for i, x := range items {
		if i > 0 && x.Name == items[i-1].Name {
			return fmt.Errorf("duplicate package path")
		}
		if x.Link == "" {
			sums += hash(x.Data) + "  " + x.Name + "\n"
		}
	}
	items = append(items, item{Name: "SHA256SUMS", Data: []byte(sums), Mode: 0644})
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	compressed := gzip.NewWriter(file)
	writer := tar.NewWriter(compressed)
	for _, x := range items {
		h := &tar.Header{Name: "brewwarden/" + x.Name, Mode: x.Mode, Size: int64(len(x.Data)), ModTime: time.Unix(0, 0), Format: tar.FormatPAX, Typeflag: tar.TypeReg}
		if x.Link != "" {
			h.Typeflag = tar.TypeSymlink
			h.Linkname = x.Link
			h.Size = 0
		}
		if err := writer.WriteHeader(h); err != nil {
			return err
		}
		if x.Link == "" {
			if _, err := writer.Write(x.Data); err != nil {
				return err
			}
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	if err := compressed.Close(); err != nil {
		return err
	}
	return file.Sync()
}
