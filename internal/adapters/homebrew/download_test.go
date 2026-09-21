package homebrew

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPublicMetadataBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, address, body string
		status              int
		limit               int64
		want                bool
	}{
		{"complete", "https://raw.githubusercontent.com/Homebrew/homebrew-core/commit/Formula/j/jq.rb", "signed", 200, 6, true},
		{"oversized", "https://raw.githubusercontent.com/Homebrew/homebrew-core/commit/Formula/j/jq.rb", "too large", 200, 6, false},
		{"unavailable", "https://raw.githubusercontent.com/Homebrew/homebrew-core/commit/Formula/j/jq.rb", "error", 503, 100, false},
		{"credentials", "https://secret@raw.githubusercontent.com/Homebrew/homebrew-core/commit/Formula/j/jq.rb", "", 200, 100, false},
		{"other host", "https://evil.example/recipe.rb", "", 200, 100, false},
		{"other metadata", "https://formulae.brew.sh/api/cask.jws.json", "", 200, 100, false},
		{"query", "https://raw.githubusercontent.com/Homebrew/homebrew-core/commit/Formula/j/jq.rb?token=secret", "", 200, 100, false},
		{"other repo", "https://api.github.com/repos/other/repo/attestations/sha256:abc", "", 200, 100, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				called = true
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Fatal("credentials sent")
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), ContentLength: -1}, nil
			})}
			_, err := download(context.Background(), client, tc.address, tc.limit)
			if (err == nil) != tc.want {
				t.Fatal(err)
			}
			if !tc.want && tc.body == "" && called {
				t.Fatal("invalid destination reached transport")
			}
		})
	}
	if publicClient().Transport.(*http.Transport).Proxy != nil {
		t.Fatal("ambient proxy inherited")
	}
	if publicClient().CheckRedirect(&http.Request{}, nil) == nil {
		t.Fatal("metadata redirect accepted")
	}
}
func TestAttestationEnvelopeAmbiguity(t *testing.T) {
	for _, data := range []string{
		`{}`, `{"attestations":[]}`, `{"attestations":[{"bundle":null}]}`,
		`{"Attestations":[{"bundle":{}}]}`,
		`{"attestations":[],"attestations":[{"bundle":{}}]}`,
		`{"attestations":[{"bundle":{},"Bundle":{}}]}`,
		`{"attestations":[{"bundle":{"verificationMaterial":{"x":1,"x":2}}}]}`,
		`{"attestations":[{"bundle":{}}]} {}`,
	} {
		if _, err := bundles([]byte(data)); err == nil {
			t.Fatal("ambiguous/missing envelope accepted", data)
		}
	}
	got, err := bundles([]byte(`{"attestations":[{"bundle":{"mediaType":"test"},"repository_id":1}],"other":null}`))
	if err != nil || string(got) != "{\"mediaType\":\"test\"}\n" {
		t.Fatal(string(got), err)
	}
}
func TestDownloadedBytesAndObservationBinding(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := workspace{root}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "cache/bottle")
	original := bottleFixture(t)
	if err := writeNew(file, original, 0600); err != nil {
		t.Fatal(err)
	}
	candidate := formulaMetadata{Name: "jq", Version: "1.8.2"}
	candidate.BottleSHA256 = digestBytes(original)
	raw := []byte(`{"schema":1,"downloads":[{"name":"jq","path":"` + file + `"}]}`)
	if err := os.WriteFile(file, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := w.copyDownloads(raw, []formulaMetadata{candidate}); err == nil {
		t.Fatal("changed bottle accepted")
	}
	if err := os.WriteFile(file, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := w.copyDownloads(raw, []formulaMetadata{candidate}); err != nil {
		t.Fatal(err)
	}
	e := domain.Evidence{RawSHA256: digestBytes(original)}
	if err := w.observation(e, []byte("substituted")); err == nil {
		t.Fatal("substituted observation accepted")
	}
	if err := w.observation(e, original); err != nil {
		t.Fatal(err)
	}
	if err := w.observation(e, original); err != nil {
		t.Fatal(err)
	}
	stored := filepath.Join(root, "observations", string(e.RawSHA256)+".json")
	if err := os.WriteFile(stored, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := w.observation(e, original); err == nil {
		t.Fatal("changed stored observation accepted")
	}
}
