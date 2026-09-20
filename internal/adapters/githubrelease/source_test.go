package githubrelease

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"brewwarden/internal/domain"
)

func sourceFixture() (Candidate, string, string) {
	input := candidate()
	body := "publisher archive bytes"
	sum := sha256.Sum256([]byte(body))
	input.SourceSHA256 = domain.Digest(hex.EncodeToString(sum[:]))
	document := strings.Replace(validRelease, `"digest":"sha256:`+string(candidate().SourceSHA256)+`"`, `"digest":null`, 1)
	document = strings.Replace(document, `"size":2000000`, `"size":`+strconv.Itoa(len(body)), 1)
	return input, document, body
}

func TestDownloadBindsPublicationWithoutPublisherDigest(t *testing.T) {
	input, document, body := sourceFixture()
	apiCalls, sourceCalls := 0, 0
	c := New()
	c.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cache-Control") != "no-cache" {
			t.Fatal("unexpected credentials or cache use")
		}
		content := document
		if r.URL.String() == input.SourceURL {
			sourceCalls++
			content = body
		} else {
			apiCalls++
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(content))}, nil
	})
	e, raw, err := c.Collect(context.Background(), input, observed)
	if err != nil || e.Status != domain.Verified || e.Subject != input.Artifact || apiCalls != 2 || sourceCalls != 1 {
		t.Fatal(e, err, apiCalls, sourceCalls)
	}
	var record sourceObservation
	if err := json.Unmarshal(raw, &record); err != nil || record.SHA256 != input.SourceSHA256 || record.Size != int64(len(body)) || record.URL != input.SourceURL || len(record.Before) == 0 || len(record.After) == 0 {
		t.Fatal(record, err)
	}
	sum := sha256.Sum256(raw)
	if e.RawSHA256 != domain.Digest(hex.EncodeToString(sum[:])) {
		t.Fatal("raw evidence hash does not bind source observation")
	}
}

func TestDownloadedSourceCannotRepairConflictingEvidence(t *testing.T) {
	for _, problem := range []string{"wrong bytes", "short body", "long body", "changed asset", "changed release", "changed publication", "oversized metadata", "oversized response", "publisher mismatch", "source error", "recheck error"} {
		t.Run(problem, func(t *testing.T) {
			input, document, body := sourceFixture()
			if problem == "oversized metadata" {
				document = strings.Replace(document, `"size":`+strconv.Itoa(len(body)), `"size":33554433`, 1)
			}
			if problem == "publisher mismatch" {
				document = strings.Replace(document, `"digest":null`, `"digest":"sha256:`+strings.Repeat("a", 64)+`"`, 1)
			}
			apiCalls, sourceCalls := 0, 0
			c := New()
			c.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				content, status, size := document, 200, int64(-1)
				if r.URL.String() == input.SourceURL {
					sourceCalls++
					content = body
					switch problem {
					case "wrong bytes":
						content = strings.Repeat("x", len(body))
					case "short body":
						content = body[:len(body)-1]
					case "long body":
						content = body + "x"
					case "oversized response":
						size = maxSourceBytes + 1
					case "source error":
						status = 503
					}
				} else {
					apiCalls++
					if apiCalls == 2 {
						switch problem {
						case "changed asset":
							content = strings.Replace(content, `"id":1,`, `"id":2,`, 1)
						case "changed release":
							content = strings.Replace(content, `"id":342331441`, `"id":342331442`, 1)
						case "changed publication":
							content = strings.Replace(content, "14:11:27Z", "14:11:28Z", 1)
						case "recheck error":
							status = 503
						}
					}
				}
				return &http.Response{StatusCode: status, ContentLength: size, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(content))}, nil
			})
			if e, raw, err := c.Collect(context.Background(), input, observed); err == nil || e.Status != domain.Unassessed || raw != nil {
				t.Fatal("unbound source accepted", e, err)
			}
			if (problem == "oversized metadata" || problem == "publisher mismatch") && sourceCalls != 0 {
				t.Fatal("download attempted despite conflicting or oversized metadata")
			}
		})
	}
}

func TestSourceRedirectBoundary(t *testing.T) {
	for _, destination := range []string{
		"https://release-assets.githubusercontent.com/github-production-release-asset/1/archive",
		"https://example.invalid/archive",
		"http://release-assets.githubusercontent.com/github-production-release-asset/1/archive",
		"https://release-assets.githubusercontent.com:443/github-production-release-asset/1/archive",
		"https://user@release-assets.githubusercontent.com/github-production-release-asset/1/archive",
		"https://release-assets.githubusercontent.com/unrelated/archive",
	} {
		t.Run(destination, func(t *testing.T) {
			input, document, body := sourceFixture()
			followed := false
			c := New()
			c.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() == input.SourceURL {
					return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{destination}}, Body: io.NopCloser(strings.NewReader(""))}, nil
				}
				content := document
				if r.URL.Host != "api.github.com" {
					followed = true
					content = body
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(content))}, nil
			})
			e, _, err := c.Collect(context.Background(), input, observed)
			allowed := destination == "https://release-assets.githubusercontent.com/github-production-release-asset/1/archive"
			if followed != allowed || (err == nil) != allowed || (e.Status == domain.Verified) != allowed {
				t.Fatal("redirect boundary failure", followed, e, err)
			}
		})
	}
}
