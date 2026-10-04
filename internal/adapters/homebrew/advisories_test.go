package homebrew

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

func TestAdvisoryInventoryAndOmittedStatus(t *testing.T) {
	feed := `{"meta":{"count":1,"schema_version":"1.7.3"},"advisories":{"jq":[{"id":"BREW-example"}]}}`
	index, err := parseAdvisoryIndex([]byte(feed))
	if err != nil || len(index.Records["jq"]) != 1 {
		t.Fatalf("want one jq advisory, got %d: error=%v", len(index.Records["jq"]), err)
	}
	for _, raw := range []string{
		replaceFixtureText(t, feed, `"count":1`, `"count":2`),
		replaceFixtureText(t, feed, `"count":1,`, ""),
		replaceFixtureText(t, feed, `,"schema_version":"1.7.3"`, ""),
		replaceFixtureText(t, feed, `"advisories":{`, `"advisories":null,"ignored":{`),
		replaceFixtureText(t, feed, `"count":1`, `"count":1,"count":1`),
		replaceFixtureText(t, feed, `"jq":[{"id":"BREW-example"}]`, `"jq":null`),
		feed + `{}`, feed[:len(feed)-1],
	} {
		if _, err := parseAdvisoryIndex([]byte(raw)); err == nil {
			t.Fatalf("incomplete feed accepted: %s", raw)
		}
	}
	var wrapper struct {
		Formulae []json.RawMessage `json:"formulae"`
	}
	if err := json.Unmarshal([]byte(infoFixture), &wrapper); err != nil {
		t.Fatal(err)
	}
	candidates, err := parseInfo([]byte(infoFixture), []string{"jq"})
	if err != nil {
		t.Fatal(err)
	}
	raw := wrapper.Formulae[0]
	if _, err := parseBrewAdvisoryStatus(raw, candidates[0], nil); err != nil {
		t.Fatal(err)
	}
	if _, err := parseBrewAdvisoryStatus(raw, candidates[0], index.Records["jq"]); err == nil {
		t.Fatal("omitted indexed status accepted")
	}
	changed := candidates[0]
	changed.Revision++
	if _, err := parseBrewAdvisoryStatus(raw, changed, nil); err == nil {
		t.Fatal("different revision accepted")
	}
	for _, status := range []string{
		`{"open":[],"patched":[],"fixed_count":8}`,
		`{"open":[],"patched":[{"id":"BREW-jq-CVE-2024-23337","upstream":["CVE-2024-23337"],"fix":"patch","fixed_in":"1.8.2"}],"fixed_count":0}`,
	} {
		withStatus := append(append([]byte{}, raw[:len(raw)-1]...), []byte(`,"vulnerabilities":`+status+`}`)...)
		if _, err := parseBrewAdvisoryStatus(withStatus, candidates[0], index.Records["jq"]); err != nil {
			t.Fatalf("supported advisory status %s rejected: %v", status, err)
		}
	}
}

func TestAdvisoryCombination(t *testing.T) {
	c := metadataFixture().Formulae[0]
	finding := publicFinding{Formula: c.Name, Version: c.Version, Open: []publicAdvisory{{ID: "GHSA-2q6r-344g-cx46", Aliases: []string{"CVE-2024-23337"}}}}
	report := publicVulnsReport{Findings: []publicFinding{finding}}
	for _, tc := range []struct {
		name   string
		status brewAdvisoryStatus
		want   domain.Applicability
	}{
		{"absent record", brewAdvisoryStatus{}, domain.Affected},
		{"unrelated fix", brewAdvisoryStatus{Patched: []brewAdvisoryEntry{{Upstream: []string{"CVE-2020-0000"}}}}, domain.Affected},
		{"matched patch", brewAdvisoryStatus{Patched: []brewAdvisoryEntry{{Upstream: []string{"CVE-2024-23337"}}}}, domain.NoKnownApplicableFindings},
		{"homebrew open overrides matching patch", brewAdvisoryStatus{
			Open:    []brewAdvisoryEntry{{ID: "BREW-example"}},
			Patched: []brewAdvisoryEntry{{Upstream: []string{"CVE-2024-23337"}}},
		}, domain.Affected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := combineAdvisories(c, report, tc.status)
			if got != tc.want {
				t.Fatalf("applicability: got %v, want %v", got, tc.want)
			}
		})
	}
}

type advisoryTransport func(*http.Request) (*http.Response, error)

func (f advisoryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPublicAdvisoryTransport(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		age, body string
		maxBytes  int64
		ok        bool
	}{
		{"complete", 200, "0", "{}", 2, true},
		{"oversized", 200, "0", "{}x", 2, false},
		{"unavailable", 503, "0", "{}", 2, false},
		{"stale", 200, "86401", "{}", 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: advisoryTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != advisoryFeedURL || r.Header.Get("Cache-Control") != "no-cache" {
					t.Fatalf("want URL %q and Cache-Control no-cache, got URL=%q Cache-Control=%q", advisoryFeedURL, r.URL.String(), r.Header.Get("Cache-Control"))
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Age": []string{tc.age}}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			_, err := fetchAdvisoryJSON(context.Background(), client, advisoryFeedURL, tc.maxBytes)
			if (err == nil) != tc.ok {
				t.Fatalf("want success=%t, got error=%v", tc.ok, err)
			}
		})
	}
}

func FuzzPublicAdvisoryStatus(f *testing.F) {
	f.Add(`{"findings":[],"skipped_formulae":[]}`)
	f.Add(`{"meta":{"count":0,"schema_version":"1.7.3"},"advisories":{}}`)
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 65536 {
			t.Skip()
		}
		_, _ = parseAdvisoryIndex([]byte(raw))
		_, _ = parsePublicVulns([]byte(raw), 0, metadataFixture().Formulae)
	})
}
