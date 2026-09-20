package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchiveDeterminismAndInventory(t *testing.T) {
	root := t.TempDir()
	first, second := filepath.Join(root, "a.tar.gz"), filepath.Join(root, "b.tar.gz")
	files := []item{{Name: "bwd", Data: []byte("executable"), Mode: 0755}, {Name: "brewwarden", Link: "bwd", Mode: 0755}, {Name: "licenses/example", Data: []byte("notice"), Mode: 0644}}
	if err := archive(first, files); err != nil {
		t.Fatal(err)
	}
	if err := archive(second, []item{files[2], files[0], files[1]}); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(first)
	b, _ := os.ReadFile(second)
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
		if h.Name == "brewwarden/SHA256SUMS" && !strings.Contains(string(data), hash([]byte("executable"))+"  bwd\n") {
			t.Fatal("missing executable digest")
		}
		if h.Name == "brewwarden/brewwarden" && (h.Typeflag != tar.TypeSymlink || h.Linkname != "bwd") {
			t.Fatal("alias not relative")
		}
	}
	if len(seen) != 4 {
		t.Fatal(seen)
	}
	if err := archive(first, files); err == nil {
		t.Fatal("overwrote an existing distribution")
	}
	if err := archive(filepath.Join(root, "duplicate.tar.gz"), []item{files[0], files[0]}); err == nil {
		t.Fatal("accepted duplicate archive paths")
	}
}
