package homebrew

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

func TestAdvisoryArchiveRetainsExactFeedAndDecodedDigest(t *testing.T) {
	feed := []byte(`{"meta":{"count":0,"schema_version":"1.7.3"},"advisories":{}}`)
	raw, err := compressAdvisoryFeed(feed)
	if err != nil {
		t.Fatal(err)
	}
	var archive struct {
		Encoding string
		SHA256   domain.Digest
		Data     []byte
	}
	if err := json.Unmarshal(raw, &archive); err != nil {
		t.Fatal(err)
	}
	if archive.Encoding != "gzip" || archive.SHA256 != digestBytes(feed) {
		t.Fatalf("archive attribution does not identify decoded feed: %+v", archive)
	}
	reader, err := gzip.NewReader(bytes.NewReader(archive.Data))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	decoded, err := io.ReadAll(reader)
	if err != nil || !bytes.Equal(decoded, feed) {
		t.Fatalf("feed changed in compression: got %q, want %q, error=%v", decoded, feed, err)
	}
}
