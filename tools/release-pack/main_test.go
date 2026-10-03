package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestPackagedDocumentationLinksResolve(t *testing.T) {
	t.Chdir("../..")
	root := t.TempDir()
	binary := filepath.Join(root, "bwd")
	license := filepath.Join(root, "Go-LICENSE")
	for _, file := range []string{binary, license} {
		if err := os.WriteFile(file, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	output := filepath.Join(root, "release.tar.gz")
	if err := run([]string{binary, license, strings.Repeat("a", 40), output}); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	documents := map[string][]byte{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(header.Name, ".md") {
			documents[header.Name], err = io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	links := regexp.MustCompile(`\]\(([^)#]+\.md)(?:#[^)]*)?\)`)
	for name, raw := range documents {
		for _, link := range links.FindAllSubmatch(raw, -1) {
			target := string(link[1])
			if strings.Contains(target, "://") {
				continue
			}
			resolved := path.Clean(path.Join(path.Dir(name), target))
			if _, exists := documents[resolved]; !exists {
				t.Errorf("archive document %s links to missing %s", name, resolved)
			}
		}
	}
}

func TestArchiveDeterminismAndInventory(t *testing.T) {
	root := t.TempDir()
	first, second := filepath.Join(root, "a.tar.gz"), filepath.Join(root, "b.tar.gz")
	files := []item{
		{Name: "bwd", Data: []byte("executable"), Mode: 0755},
		{Name: "brewwarden", Link: "bwd", Mode: 0755},
		{Name: "licenses/example", Data: []byte("notice"), Mode: 0644},
	}
	if err := archive(first, files); err != nil {
		t.Fatal(err)
	}
	if err := archive(second, []item{files[2], files[0], files[1]}); err != nil {
		t.Fatal(err)
	}
	a, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("archive depends on enumeration or time")
	}
	gz, err := gzip.NewReader(bytes.NewReader(a))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	seen := map[string]bool{}
	for {
		h, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		seen[h.Name] = true
		if h.Uid != 0 || h.Gid != 0 || h.ModTime.Unix() != 0 {
			t.Fatal("host metadata leaked", h)
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		const executableChecksum = "a29c2123a01df1d0febb9d308a20d8a2fe3d40a91ca8b1294f59f08f9773ebea  bwd\n"
		if h.Name == "brewwarden/SHA256SUMS" && !strings.Contains(string(data), executableChecksum) {
			t.Fatalf("want executable digest %q in SHA256SUMS, got %q", executableChecksum, data)
		}
		if h.Name == "brewwarden/brewwarden" && (h.Typeflag != tar.TypeSymlink || h.Linkname != "bwd") {
			t.Fatalf("want symlink alias to bwd, got type=%d link=%q", h.Typeflag, h.Linkname)
		}
	}
	if len(seen) != 4 {
		t.Fatalf("want four packaged entries including SHA256SUMS: entries=%v", seen)
	}
	if err := archive(first, files); err == nil {
		t.Fatal("overwrote an existing distribution")
	}
	if err := archive(filepath.Join(root, "duplicate.tar.gz"), []item{files[0], files[0]}); err == nil {
		t.Fatal("accepted duplicate archive paths")
	}
}
