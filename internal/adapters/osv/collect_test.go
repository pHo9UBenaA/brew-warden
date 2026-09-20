package osv

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

	"brewwarden/internal/domain"
)

const now int64 = 1789900000
const modified = "2026-08-12T15:16:59.845778Z"

func candidateFixture() Candidate {
	return Candidate{Artifact: domain.Artifact{Tap: "homebrew/core", Name: "jq", Version: "1.8.2", Rebuild: 1, OS: "macos", Arch: "arm64", BottleTag: "arm64_tahoe", SHA256: "ca67c64d0aaf1e5472790ec2cc081ff7972316f27095d8a8aab81b3321247036"}, SourceURL: "https://github.com/jqlang/jq/releases/download/jq-1.8.2/jq-1.8.2.tar.gz", SourceSHA256: "71b8d6e8f5fe81f6c6d0d110e3892251f6ce76ed095abd315e26e6e1193af3af", RecipeSHA256: "0081a3a8d8afaa165b4bfa9b222723916b56f4d04b975d7aad35e8cdad0b0296", UnmodifiedSource: true}
}

func referenceJSON(id string) string { return `{"id":"` + id + `","modified":"` + modified + `"}` }
func recordJSON(id, tag string) string {
	return `{"id":"` + id + `","modified":"` + modified + `","affected":[{"ranges":[{"type":"GIT","repo":"https://github.com/jqlang/jq","events":[{"introduced":"0"},{"fixed":"` + strings.Repeat("a", 40) + `"}]}],"versions":["` + tag + `"]}]}`
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func collectorFixture(t *testing.T, candidateRefs string, candidateRecord string) *Collector {
	t.Helper()
	c := New()
	c.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cache-Control") != "no-cache" {
			t.Fatal("unexpected request credentials or cache")
		}
		switch r.URL.String() {
		case "https://api.osv.dev/v1/querybatch":
			var body struct {
				Queries []query `json:"queries"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Queries) != 2 || body.Queries[0].Package.Name != "https://github.com/jqlang/jq" || body.Queries[0].Version != "jq-1.8.2" || body.Queries[1].Version != "jq-1.7.1" {
				t.Fatal("wrong query identity", body, err)
			}
			return response(`{"results":[` + candidateRefs + `,{"vulns":[` + referenceJSON("CVE-2024-23337") + `]}]}`), nil
		case "https://api.osv.dev/v1/vulns/CVE-2024-23337":
			return response(recordJSON("CVE-2024-23337", "jq-1.7.1")), nil
		case "https://api.osv.dev/v1/vulns/CVE-2026-EXAMPLE":
			return response(candidateRecord), nil
		default:
			t.Fatal("unexpected advisory request", r.URL)
			return nil, errors.New("unexpected request")
		}
	})
	return c
}

func TestCandidateCoverageAndApplicableFindings(t *testing.T) {
	for _, tc := range []struct {
		name, refs, record string
		want               domain.Applicability
	}{
		{"no known applicable findings", `{}`, "", domain.NoKnownApplicableFindings},
		{"known applicable", `{"vulns":[` + referenceJSON("CVE-2026-EXAMPLE") + `]}`, recordJSON("CVE-2026-EXAMPLE", "jq-1.8.2"), domain.Affected},
		{"withdrawn", `{"vulns":[` + referenceJSON("CVE-2026-EXAMPLE") + `]}`, strings.Replace(recordJSON("CVE-2026-EXAMPLE", "jq-1.8.2"), `"affected":`, `"withdrawn":"2026-08-11T00:00:00Z","affected":`, 1), domain.NoKnownApplicableFindings},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, raw, err := collectorFixture(t, tc.refs, tc.record).Collect(context.Background(), candidateFixture(), now)
			if err != nil || e.Status != domain.Verified || e.Subject != candidateFixture().Artifact || e.Applicability != tc.want || e.ExpiresAt != now+3600 {
				t.Fatal(e, err)
			}
			var obs observation
			if err := json.Unmarshal(raw, &obs); err != nil || !obs.Complete || len(obs.Queries) != 1 || len(obs.Records) == 0 {
				t.Fatal(obs, err)
			}
		})
	}
}

func TestUnresolvedRecordNeverBecomesClean(t *testing.T) {
	valid := recordJSON("CVE-2026-EXAMPLE", "jq-1.8.2")
	for _, body := range []string{
		strings.Replace(valid, modified, "2026-08-13T00:00:00Z", 1),
		strings.Replace(valid, "jqlang/jq", "attacker/jq", 1),
		strings.Replace(valid, "jq-1.8.2", "1.8.2", 1),
		strings.Replace(valid, `{"introduced":"0"},`, "", 1),
		strings.Replace(valid, `"affected":`, `"withdrawn":"2030-01-01T00:00:00Z","affected":`, 1),
		strings.Replace(valid, `"affected":`, `"withdrawn":"2026-08-13T00:00:00Z","affected":`, 1),
		strings.Replace(valid, `"affected":`, `"schema_version":"2.0.0","affected":`, 1),
		strings.Replace(valid, `"modified":`, `"Modified":`, 1),
		strings.Replace(valid, `"id":`, `"id":"duplicate","id":`, 1),
		`{"id":"CVE-2026-EXAMPLE","modified":"` + modified + `","affected":null}`,
	} {
		c := collectorFixture(t, `{"vulns":[`+referenceJSON("CVE-2026-EXAMPLE")+`]}`, body)
		if e, _, err := c.Collect(context.Background(), candidateFixture(), now); err == nil || e.Status == domain.Verified {
			t.Fatal("unresolved advisory accepted", body, e, err)
		}
	}
}

func TestIncompleteAndUncoveredBatch(t *testing.T) {
	for _, body := range []string{`{}`, `{"results":[]}`, `{"results":[{},{}]}`, `{"results":[null,{}]}`, `{"results":[{"vulns":null},{}]}`, `{"results":[{},{}],"results":[{},{}]}`, `{"Results":[{},{}]}`, `{"results":[{"vulns":[{"id":"../escape","modified":"` + modified + `"}]},{}]}`} {
		c := New()
		calls := 0
		c.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) { calls++; return response(body), nil })
		if e, _, err := c.Collect(context.Background(), candidateFixture(), now); err == nil || e.Status == domain.Verified || calls != 1 {
			t.Fatal("incomplete or uncovered query accepted", body, e, err, calls)
		}
	}
}

func TestPaginationMustFinishForEachQuery(t *testing.T) {
	for _, repeat := range []bool{false, true} {
		c := New()
		calls := 0
		c.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
			if r.Method == http.MethodGet {
				return response(recordJSON("CVE-2024-23337", "jq-1.7.1")), nil
			}
			calls++
			if calls == 1 {
				return response(`{"results":[{"next_page_token":"next"},{"vulns":[` + referenceJSON("CVE-2024-23337") + `]}]}`), nil
			}
			var body struct {
				Queries []query `json:"queries"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Queries) != 1 || body.Queries[0].PageToken != "next" || body.Queries[0].Version != "jq-1.8.2" {
				t.Fatal("pagination mixed query slots", body, err)
			}
			if repeat {
				return response(`{"results":[{"next_page_token":"next"}]}`), nil
			}
			return response(`{"results":[{}]}`), nil
		})
		e, _, err := c.Collect(context.Background(), candidateFixture(), now)
		if calls != 2 || (err != nil) != repeat || (!repeat && e.Applicability != domain.NoKnownApplicableFindings) {
			t.Fatal(e, err, calls)
		}
	}
}

func TestUnsupportedMappingAndSourceFailures(t *testing.T) {
	for _, mutate := range []func(*Candidate){func(c *Candidate) { c.Artifact.Revision = 1 }, func(c *Candidate) { c.UnmodifiedSource = false }, func(c *Candidate) { c.SourceURL += "?redirect=elsewhere" }, func(c *Candidate) { c.RecipeSHA256 = "" }, func(c *Candidate) { c.Artifact.Version = "01.8.2" }, func(c *Candidate) { c.Artifact.Name = "unmapped" }} {
		input := candidateFixture()
		mutate(&input)
		c := New()
		c.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
			t.Fatal("unsupported mapping contacted source")
			return nil, nil
		})
		if _, _, err := c.Collect(context.Background(), input, now); err == nil {
			t.Fatal("unsupported candidate accepted")
		}
	}
	for _, status := range []int{301, 401, 403, 404, 429, 500, 503} {
		c := New()
		calls := 0
		c.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://example.invalid/"}}, Body: io.NopCloser(strings.NewReader("{}"))}, nil
		})
		if _, _, err := c.Collect(context.Background(), candidateFixture(), now); err == nil || calls != 1 {
			t.Fatal("source failure accepted or redirected", err, calls)
		}
	}
}

func FuzzAdvisoryRecord(f *testing.F) {
	f.Add(recordJSON("CVE-2024-23337", "jq-1.7.1"))
	f.Add(`{"id":null}`)
	f.Fuzz(func(t *testing.T, data string) {
		if len(data) > maxResponse {
			return
		}
		a, w, err := record([]byte(data), reference{ID: "CVE-2024-23337", Modified: modified}, "https://github.com/jqlang/jq", "jq-1.7.1", now)
		if err == nil && !a && !w {
			t.Fatal("unresolved record accepted")
		}
	})
}

func TestLiveOSVCandidates(t *testing.T) {
	if os.Getenv("BREWWARDEN_LIVE_OSV") != "1" {
		t.Skip("requires explicit public-network integration run")
	}
	jq := candidateFixture()
	onig := jq
	onig.Artifact.Name = "oniguruma"
	onig.Artifact.Version = "6.9.10"
	onig.Artifact.Rebuild = 0
	onig.Artifact.SHA256 = "eb6bda3b333f497b5d294388f39fd0902a5c79a52ae16858eff711d2d104cc4d"
	onig.SourceURL = "https://github.com/kkos/oniguruma/releases/download/v6.9.10/onig-6.9.10.tar.gz"
	onig.SourceSHA256 = "2a5cfc5ae259e4e97f86b68dfffc152cdaffe94e2060b770cb827238d769fc05"
	onig.RecipeSHA256 = "2656eda555be128035d8dcf82bb04d094f96b06b7f5dd1b65f967bc76aa0b1c3"
	for _, candidate := range []Candidate{jq, onig} {
		e, _, err := New().Collect(context.Background(), candidate, time.Now().Unix())
		if err != nil || e.Status != domain.Verified || e.Applicability != domain.NoKnownApplicableFindings {
			t.Fatal(candidate.Artifact.Name, e, err)
		}
		t.Logf("%s %s: supported complete candidate lookup with positive project coverage control, raw_sha256=%s", candidate.Artifact.Name, candidate.Artifact.Version, e.RawSHA256)
	}
}

// Real OSV conversion records may include both a last-known affected commit and
// a subsequently discovered fix. We use their explicit affected tags only.
func TestExplicitAffectedTagWithNonlinearGitEvents(t *testing.T) {
	data := recordJSON("CVE-2024-23337", "jq-1.7.1")
	data = strings.Replace(data, `{"fixed":`, `{"last_affected":"`+strings.Repeat("b", 40)+`"},{"fixed":`, 1)
	applies, _, err := record([]byte(data), reference{"CVE-2024-23337", modified}, "https://github.com/jqlang/jq", "jq-1.7.1", now)
	if err != nil || !applies {
		t.Fatal(applies, err)
	}
	if _, _, err := record([]byte(data), reference{"CVE-2024-23337", modified}, "https://github.com/jqlang/jq", "jq-1.8.2", now); err == nil {
		t.Fatal("range inferred an unaffected tag")
	}
}

func TestKnownFindingSurvivesIncompleteInventory(t *testing.T) {
	c := collectorFixture(t, `{"vulns":[`+referenceJSON("CVE-2026-EXAMPLE")+`,`+referenceJSON("CVE-2026-MISSING")+`]}`, recordJSON("CVE-2026-EXAMPLE", "jq-1.8.2"))
	base := c.client.Transport
	c.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "CVE-2026-MISSING") {
			return nil, errors.New("unavailable record")
		}
		return base.RoundTrip(r)
	})
	e, raw, err := c.Collect(context.Background(), candidateFixture(), now)
	var obs observation
	if err != nil || e.Applicability != domain.Affected || json.Unmarshal(raw, &obs) != nil || obs.Complete {
		t.Fatal(e, obs, err)
	}
}
