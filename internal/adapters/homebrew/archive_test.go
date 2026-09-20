package homebrew

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"
)

func bottleFixture(t *testing.T, extra ...*tar.Header) []byte {
	t.Helper()
	var data bytes.Buffer
	compressed := gzip.NewWriter(&data)
	archive := tar.NewWriter(compressed)
	headers := append([]*tar.Header{{Name: "jq/1.8.2/", Typeflag: tar.TypeDir, Mode: 0755}, {Name: "jq/1.8.2/bin/jq", Typeflag: tar.TypeReg, Mode: 0555, Size: 3}}, extra...)
	for _, header := range headers {
		if err := archive.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Size > 0 {
			if _, err := archive.Write(bytes.Repeat([]byte{'x'}, int(header.Size))); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}
func TestBottleExtractionBoundaries(t *testing.T) {
	candidate := formulaMetadata{Name: "jq", Version: "1.8.2"}
	if err := validateBottleArchive(bottleFixture(t), candidate); err != nil {
		t.Fatal(err)
	}
	for _, header := range []*tar.Header{
		{Name: "../outside", Typeflag: tar.TypeReg, Size: 1},
		{Name: "jq/1.8.2/../other", Typeflag: tar.TypeReg, Size: 1},
		{Name: "jq/1.8.2/.bottle/etc/config", Typeflag: tar.TypeReg, Size: 1},
		{Name: "jq/1.8.2/INSTALL_RECEIPT.json", Typeflag: tar.TypeReg, Size: 1},
		{Name: "jq/1.8.2/escape", Typeflag: tar.TypeSymlink, Linkname: "../../../etc"},
		{Name: "jq/1.8.2/escape", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"},
		{Name: "jq/1.8.2/suid", Typeflag: tar.TypeReg, Mode: 04755, Size: 1},
		{Name: "jq/1.8.2/fifo", Typeflag: tar.TypeFifo, Mode: 0600},
		{Name: "jq/1.8.2/hard", Typeflag: tar.TypeLink, Linkname: "jq/1.8.2/bin/jq"},
		{Name: "jq/1.8.2/bin/jq", Typeflag: tar.TypeReg, Size: 1},
		{Name: "jq/1.8.2/bin", Typeflag: tar.TypeSymlink, Linkname: "lib"},
	} {
		if err := validateBottleArchive(bottleFixture(t, header), candidate); err == nil {
			t.Fatalf("unsafe archive accepted: %+v", header)
		}
	}
	data := bottleFixture(t)
	data[len(data)-5] ^= 0x01
	if err := validateBottleArchive(data, candidate); err == nil {
		t.Fatal("damaged gzip footer accepted")
	}
}
