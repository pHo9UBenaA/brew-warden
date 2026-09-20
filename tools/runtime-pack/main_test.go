package main

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func archiveFixture(t *testing.T, headers []*tar.Header) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := tar.NewWriter(&out)
	for _, header := range headers {
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Size > 0 {
			if _, err := writer.Write(bytes.Repeat([]byte{'x'}, int(header.Size))); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func TestArchiveBoundary(t *testing.T) {
	for _, tc := range []struct {
		name    string
		headers []*tar.Header
		want    bool
	}{
		{"regular nested file", []*tar.Header{{Name: "dir/file", Typeflag: tar.TypeReg, Mode: 0644, Size: 3}}, true},
		{"traversal", []*tar.Header{{Name: "../outside", Typeflag: tar.TypeReg, Mode: 0644, Size: 3}}, false},
		{"absolute", []*tar.Header{{Name: "/outside", Typeflag: tar.TypeReg, Mode: 0644, Size: 3}}, false},
		{"absolute symlink", []*tar.Header{{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}}, false},
		{"symlink parent", []*tar.Header{{Name: "dir", Typeflag: tar.TypeSymlink, Linkname: "other"}, {Name: "dir/file", Typeflag: tar.TypeReg, Mode: 0644, Size: 3}}, false},
		{"duplicate file", []*tar.Header{{Name: "file", Typeflag: tar.TypeReg, Mode: 0644, Size: 3}, {Name: "file", Typeflag: tar.TypeReg, Mode: 0644, Size: 3}}, false},
		{"hard link", []*tar.Header{{Name: "file", Typeflag: tar.TypeLink, Linkname: "other"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "root")
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			err := extract(tar.NewReader(bytes.NewReader(archiveFixture(t, tc.headers))), root)
			if (err == nil) != tc.want {
				t.Fatal(err)
			}
		})
	}
}
