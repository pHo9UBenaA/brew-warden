package githubrelease

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

const observed int64 = 1789862400 // 2026-09-20 UTC

func candidate() Candidate {
	return Candidate{
		Artifact:     domain.Artifact{Tap: "homebrew/core", Name: "jq", Version: "1.8.2", Rebuild: 1, OS: "macos", Arch: "arm64", BottleTag: "arm64_tahoe", SHA256: "ca67c64d0aaf1e5472790ec2cc081ff7972316f27095d8a8aab81b3321247036"},
		SourceURL:    "https://github.com/jqlang/jq/releases/download/jq-1.8.2/jq-1.8.2.tar.gz",
		SourceSHA256: "71b8d6e8f5fe81f6c6d0d110e3892251f6ce76ed095abd315e26e6e1193af3af",
	}
}

const validRelease = `{"id":342331441,"tag_name":"jq-1.8.2","draft":false,"prerelease":false,"published_at":"2026-06-20T14:11:27Z","assets":[{"id":1,"name":"jq-1.8.2.tar.gz","browser_download_url":"https://github.com/jqlang/jq/releases/download/jq-1.8.2/jq-1.8.2.tar.gz","digest":"sha256:71b8d6e8f5fe81f6c6d0d110e3892251f6ce76ed095abd315e26e6e1193af3af","state":"uploaded","size":2000000,"created_at":"2026-06-20T14:10:26Z","updated_at":"2026-06-20T14:10:27Z"}],"new_unrelated_field":{"nested":null}}`

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCollectExactPublication(t *testing.T) {
	c := New()
	c.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.github.com/repos/jqlang/jq/releases/tags/jq-1.8.2" || r.Method != http.MethodGet || r.Header.Get("Authorization") != "" || r.Header.Get("X-GitHub-Api-Version") != "2022-11-28" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(validRelease)), Header: make(http.Header)}, nil
	})
	e, raw, err := c.Collect(context.Background(), candidate(), observed)
	published, _ := time.Parse(time.RFC3339, "2026-06-20T14:11:27Z")
	if err != nil || e.Status != domain.Verified || e.Subject != candidate().Artifact || e.Publication != domain.UpstreamPublication || e.PublishedAt != published.Unix() || !e.RawSHA256.Valid() || e.ExpiresAt != observed+freshnessSeconds || string(raw) != validRelease {
		t.Fatalf("unexpected evidence: %+v, %v", e, err)
	}
}

func TestRejectUnboundPublication(t *testing.T) {
	replace := func(a, b string) string { return strings.Replace(validRelease, a, b, 1) }
	cases := map[string]string{
		"missing draft":       replace(`"draft":false,`, ""),
		"null draft":          replace(`"draft":false`, `"draft":null`),
		"draft release":       replace(`"draft":false`, `"draft":true`),
		"prerelease":          replace(`"prerelease":false`, `"prerelease":true`),
		"wrong version":       replace(`"tag_name":"jq-1.8.2"`, `"tag_name":"jq-1.8.1"`),
		"future publication":  replace("2026-06-20T14:11:27Z", "2030-06-20T14:11:27Z"),
		"missing publication": replace(`"published_at":"2026-06-20T14:11:27Z",`, ""),
		"invalid timestamp":   replace("2026-06-20T14:11:27Z", "not-a-time"),
		"missing digest":      replace(`"digest":"sha256:`+string(candidate().SourceSHA256)+`"`, `"digest":null`),
		"different source":    replace(string(candidate().SourceSHA256), strings.Repeat("a", 64)),
		"asset added later":   replace("2026-06-20T14:10:27Z", "2026-09-19T14:10:27Z"),
		"reversed dates":      replace("2026-06-20T14:10:26Z", "2026-06-20T14:10:28Z"),
		"upload incomplete":   replace(`"state":"uploaded"`, `"state":"starter"`),
		"empty asset":         replace(`"size":2000000`, `"size":0`),
		"different host":      replace("https://github.com/jqlang", "https://example.invalid/jqlang"),
		"different asset":     replace(`"name":"jq-1.8.2.tar.gz"`, `"name":"jq-1.8.1.tar.gz"`),
		"mis-cased key":       replace(`"draft"`, `"Draft"`),
		"duplicate key":       replace(`"draft":false`, `"draft":true,"draft":false`),
		"trailing value":      validRelease + " {}",
		"invalid UTF-8":       replace("new_unrelated_field", "bad\xff"),
		"oversized":           validRelease + strings.Repeat(" ", maxResponseBytes),
		"malformed":           `{"id":`,
		"wrong root type":     `[]`,
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := publication([]byte(data), candidate(), "jq-1.8.2", "jq-1.8.2.tar.gz", observed); err == nil {
				t.Fatal("unbound or malformed publication accepted")
			}
		})
	}
	var document releaseDocument
	if err := json.Unmarshal([]byte(validRelease), &document); err != nil {
		t.Fatal(err)
	}
	document.Assets = append(document.Assets, document.Assets[0])
	duplicate, _ := json.Marshal(document)
	if _, err := publication(duplicate, candidate(), "jq-1.8.2", "jq-1.8.2.tar.gz", observed); err == nil {
		t.Fatal("duplicate asset accepted")
	}
}

func TestUnsupportedCandidateNeverRequests(t *testing.T) {
	c := New()
	c.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("unsupported candidate contacted the network")
		return nil, errors.New("unexpected request")
	})
	for _, mutate := range []func(*Candidate){
		func(c *Candidate) { c.Artifact.Name = "../invalid" },
		func(c *Candidate) { c.SourceURL += "?redirect=attacker" },
		func(c *Candidate) {
			c.SourceURL = "https://github.com.attacker.example/jqlang/jq/releases/download/jq-1.8.2/jq-1.8.2.tar.gz"
		},
		func(c *Candidate) { c.SourceSHA256 = "" },
		func(c *Candidate) { c.SourceURL = strings.Replace(c.SourceURL, "jq-1.8.2/", "%2e%2e/", 1) },
	} {
		input := candidate()
		mutate(&input)
		if _, _, err := c.Collect(context.Background(), input, observed); err == nil {
			t.Fatal("unsupported candidate accepted")
		}
	}
}

func TestSourceFailuresCannotProduceEvidence(t *testing.T) {
	for _, status := range []int{301, 401, 403, 404, 429, 500, 503} {
		c := New()
		requests := 0
		c.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
			requests++
			return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://example.invalid/release"}}, Body: io.NopCloser(strings.NewReader(validRelease))}, nil
		})
		if e, _, err := c.Collect(context.Background(), candidate(), observed); err == nil || e.Status != domain.Unassessed || requests != 1 {
			t.Fatalf("status %d produced evidence or followed a redirect: %+v %v calls=%d", status, e, err, requests)
		}
	}
	c := New()
	c.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := c.Collect(ctx, candidate(), observed); err == nil {
		t.Fatal("cancellation accepted")
	}
}

func FuzzPublication(f *testing.F) {
	f.Add(validRelease)
	f.Add(`{"draft":false,"draft":true}`)
	f.Add(`null`)
	f.Fuzz(func(t *testing.T, input string) {
		published, err := publication([]byte(input), candidate(), "jq-1.8.2", "jq-1.8.2.tar.gz", observed)
		if err == nil && (published <= 0 || published > observed) {
			t.Fatalf("accepted invalid publication time: %d", published)
		}
	})
}

// This explicit integration test is not run by ordinary tests/hooks. It contacts
// the real public endpoint without credentials; an outage fails, never skips.
func TestLiveGitHubPublication(t *testing.T) {
	if os.Getenv("BREWWARDEN_LIVE_GITHUB") != "1" {
		t.Skip("requires explicit public-network integration run")
	}
	e, _, err := New().Collect(context.Background(), candidate(), time.Now().Unix())
	if err != nil || e.Status != domain.Verified {
		t.Fatalf("live publication: %+v %v", e, err)
	}
	t.Logf("verified upstream publication: source=%s published=%d raw_sha256=%s", e.Source, e.PublishedAt, e.RawSHA256)
	missingDigest := candidate()
	missingDigest.Artifact.Name, missingDigest.Artifact.Version = "oniguruma", "6.9.10"
	missingDigest.Artifact.SHA256 = "eb6bda3b333f497b5d294388f39fd0902a5c79a52ae16858eff711d2d104cc4d"
	missingDigest.Artifact.Rebuild = 0
	missingDigest.SourceURL = "https://github.com/kkos/oniguruma/releases/download/v6.9.10/onig-6.9.10.tar.gz"
	missingDigest.SourceSHA256 = "2a5cfc5ae259e4e97f86b68dfffc152cdaffe94e2060b770cb827238d769fc05"
	if e, raw, err := New().Collect(context.Background(), missingDigest, time.Now().Unix()); err != nil || e.Status != domain.Verified || !strings.Contains(string(raw), "github-source-publication-v1") {
		t.Fatalf("download-bound source publication failed: %+v %v", e, err)
	} else {
		t.Logf("verified downloaded source publication: source=%s published=%d raw_sha256=%s", e.Source, e.PublishedAt, e.RawSHA256)
	}
}

func TestPublicCollectorDoesNotInheritProxy(t *testing.T) {
	if New().client.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("ambient proxy inherited")
	}
}

// A formula need not share its publisher's repository name or tag convention.
func TestPublicationUsesAuthenticatedSourceIdentity(t *testing.T) {
	input := candidate()
	input.Artifact.Name = "different-formula"
	input.Artifact.Version = "2026.09"
	input.SourceURL = "https://github.com/Publisher/Project/releases/download/release-2026.09/source.tar.gz"
	document := strings.NewReplacer(candidate().SourceURL, input.SourceURL, "jq-1.8.2.tar.gz", "source.tar.gz", "jq-1.8.2", "release-2026.09").Replace(validRelease)
	c := New()
	c.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.github.com/repos/Publisher/Project/releases/tags/release-2026.09" {
			t.Fatalf("wrong publisher endpoint: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(document)), Header: make(http.Header)}, nil
	})
	e, _, err := c.Collect(context.Background(), input, observed)
	if err != nil || e.Status != domain.Verified || e.Subject != input.Artifact {
		t.Fatalf("general source identity rejected: %+v %v", e, err)
	}
}

func TestPublicationRejectsAmbiguousSourceURLs(t *testing.T) {
	for _, source := range []string{
		"http://github.com/o/r/releases/download/v1/a.tar.gz",
		"https://github.com:443/o/r/releases/download/v1/a.tar.gz",
		"https://user@github.com/o/r/releases/download/v1/a.tar.gz",
		"https://github.com/o/r/releases/download/../a.tar.gz",
		"https://github.com/o/r/releases/download/v1/a.tar.gz#fragment",
		"https://github.com/o/r/releases/download/v1/a%2ftar.gz",
		"https://github.com/o/r/releases/download/v1/a.tar.gz/extra",
		"https://github.com/o/r/archive/refs/tags/v1.tar.gz",
		"https://github.com//r/releases/download/v1/a.tar.gz",
	} {
		input := candidate()
		input.SourceURL = source
		if _, _, _, err := mapping(input); err == nil {
			t.Errorf("ambiguous source accepted: %s", source)
		}
	}
}
