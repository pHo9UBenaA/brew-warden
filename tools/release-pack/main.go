// Build-only deterministic distribution packaging. No network or installation.
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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
	if len(args) != 4 {
		return fmt.Errorf("usage: release-pack BINARY GO_LICENSE SOURCE_REVISION OUTPUT.tar.gz")
	}
	binary, goLicense, revision, output := args[0], args[1], args[2], args[3]
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
	// Installed Homebrew is checked at collection and again before execution;
	// the distribution contains no runtime files or generated inventory.
	notice := "BrewWarden dependencies\n\nSource revision: " + revision + "\n\nHomebrew and GitHub CLI are installed separately and are not bundled.\n"
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
