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
	if len(args) != 7 {
		return fmt.Errorf("usage: release-pack BINARY RUNTIME MODULES_JSON GO_LICENSE NATIVE_NOTICES SOURCE_REVISION OUTPUT.tar.gz")
	}
	binary, runtimeRoot, modulesFile, goLicense, nativeNotices, revision, output := args[0], args[1], args[2], args[3], args[4], args[5], args[6]
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
	const pinned = "9424050677790a1c88c66ab769c5167d59a874cd6a02074665268084c97e758b"
	if hash(manifestRaw) != pinned {
		return fmt.Errorf("unsupported runtime manifest")
	}
	var manifest struct {
		Files []struct {
			Path         string
			Mode         uint32
			SHA256, Link string
		}
	}
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return err
	}
	for _, f := range manifest.Files {
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
	notice := "BrewWarden runtime dependencies\n\nSource revision: " + revision + "\nRuntime manifest SHA-256: " + pinned + "\n\nHomebrew 7.0.4: https://github.com/Homebrew/brew/tree/edb70f031e4170c780799633a1226ff73e1077f4\nPortable Ruby 4.0.7 original distribution: https://cache.ruby-lang.org/pub/ruby/4.0/ruby-4.0.7.tar.gz\nRuby source SHA-256: 911ace20f90d068ca0e4dda6d0e4f0f81e52e52f2dd4f4004c721e253412e82d\nRuby statically includes OpenSSL 4.0.2 and libyaml 0.2.5.\nNative dependency licenses and Ruby LEGAL are in licenses/native.\nHomebrew and bundled gem licenses are retained in runtime/brew.\nSystem dependencies: macOS libSystem, libobjc, libresolv, Security and CoreFoundation.\n\nAttestation-only helper: github.com/cli/cli/v2 v2.101.0\nThe following Go modules are recorded in the actual helper binary; licenses\ninclude additional notices supplied by each module's source distribution.\n\n"
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
	expectedNotices := map[string]string{
		"Ruby-LEGAL":          "a74812486cffbdc55141a5d9f165d782cbb202660d827622ec966237d4717b99",
		"Ruby-BSDL":           "36a9a6e7347214bbba599a412617204e65bff065dcbe5c46f5cb454c80de9eb0",
		"OpenSSL-LICENSE.txt": "7d5450cb2d142651b8afa315b5f238efc805dad827d91ba367d8516bc9d49e7a",
		"libyaml-LICENSE.txt": "c40112449f254b9753045925248313e9270efa36d226b22d82d4cc6c43c57f29",
	}
	for name, want := range expectedNotices {
		raw, err := os.ReadFile(filepath.Join(nativeNotices, name))
		if err != nil || hash(raw) != want {
			return fmt.Errorf("native license input mismatch: %s", name)
		}
		if err := add("licenses/native/"+name, filepath.Join(nativeNotices, name), 0644); err != nil {
			return err
		}
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
