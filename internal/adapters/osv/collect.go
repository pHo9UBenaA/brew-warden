// Package osv collects exact upstream-tag vulnerability observations for
// authenticated, unmodified Homebrew sources. Patched revisions remain unknown.
package osv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

const maxResponse = 2 * 1024 * 1024
const maxObservation = 16 * 1024 * 1024
const maxFindings = 128
const maxCoverageReferences = 1024
const maxCoverageRecords = 8

type Candidate = ports.SourceCandidate

type Collector struct{ client *http.Client }

func New() *Collector {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Collector{client: &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("advisory redirects are unsupported") }}}
}

type packageKey struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}
type query struct {
	Package   packageKey `json:"package"`
	Version   string     `json:"version,omitempty"`
	PageToken string     `json:"page_token,omitempty"`
}
type reference struct {
	ID       string `json:"id"`
	Modified string `json:"modified"`
}
type page struct {
	Vulns []json.RawMessage `json:"vulns"`
	Next  string            `json:"next_page_token"`
}
type exchange struct {
	Request  json.RawMessage `json:"request"`
	Response json.RawMessage `json:"response"`
}
type observation struct {
	Schema       string            `json:"schema"`
	Complete     bool              `json:"complete"`
	SourceSHA256 domain.Digest     `json:"sourceSHA256"`
	RecipeSHA256 domain.Digest     `json:"recipeSHA256"`
	Queries      []exchange        `json:"queries"`
	Records      []json.RawMessage `json:"records"`
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var version = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// Source identity comes from the authenticated recipe, never a name search.
var sourceRelease = regexp.MustCompile(`^https://github\.com/([A-Za-z0-9_.+-]+)/([A-Za-z0-9_.+-]+)/releases/download/([A-Za-z0-9_.+-]+)/([A-Za-z0-9_.+-]+)$`)

func mapping(c Candidate) (repo, tag string, err error) {
	if !c.Artifact.Valid() || c.Artifact.Revision != 0 || !c.UnmodifiedSource || !c.SourceSHA256.Valid() || !c.RecipeSHA256.Valid() || len(c.SourceURL) > 2048 {
		return "", "", errors.New("candidate source or revision mapping is unsupported")
	}
	parts := sourceRelease.FindStringSubmatch(c.SourceURL)
	if len(parts) != 5 {
		return "", "", errors.New("unsupported advisory source mapping")
	}
	for _, part := range parts[1:] {
		if part == "." || part == ".." {
			return "", "", errors.New("ambiguous advisory source identity")
		}
	}
	return "https://github.com/" + parts[1] + "/" + parts[2], parts[3], nil
}

func (c *Collector) Collect(ctx context.Context, candidate Candidate, now int64) (domain.Evidence, []byte, error) {
	repo, tag, err := mapping(candidate)
	if c == nil || c.client == nil || ctx == nil || err != nil || now <= 0 || now > 1<<62 {
		return domain.Evidence{}, nil, errors.New("advisory candidate is unsupported")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	obs := observation{Schema: "osv-git-candidate-v2", SourceSHA256: candidate.SourceSHA256, RecipeSHA256: candidate.RecipeSHA256, Queries: []exchange{}, Records: []json.RawMessage{}}
	queries := []query{{Package: packageKey{repo, "GIT"}, Version: tag}, {Package: packageKey{repo, "GIT"}}}
	refs, err := c.queryAll(ctx, queries, &obs, now)
	if err != nil {
		return domain.Evidence{}, nil, err
	}
	if err := c.verifyCoverage(ctx, refs[1], repo, &obs, now); err != nil {
		return domain.Evidence{}, nil, err
	}
	applicability := domain.NoKnownApplicableFindings
	ids := make([]string, 0, len(refs[0]))
	for id := range refs[0] {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var unresolved error
	for _, id := range ids {
		data, err := c.request(ctx, http.MethodGet, "https://api.osv.dev/v1/vulns/"+id, nil)
		if err != nil {
			unresolved = err
			continue
		}
		obs.Records = append(obs.Records, json.RawMessage(data))
		if observationSize(obs) > maxObservation {
			return domain.Evidence{}, nil, errors.New("advisory observation exceeds limit")
		}
		applies, withdrawn, err := record(data, refs[0][id], repo, tag, now)
		if err != nil {
			unresolved = err
			continue
		}
		if applies && !withdrawn {
			applicability = domain.Affected
		}
	}
	if unresolved != nil && applicability != domain.Affected {
		return domain.Evidence{}, nil, unresolved
	}
	obs.Complete = unresolved == nil
	data, err := json.Marshal(obs)
	if err != nil || len(data) > maxObservation {
		return domain.Evidence{}, nil, errors.New("cannot retain complete advisory observation")
	}
	sum := sha256.Sum256(data)
	evidence, err := domain.NewEvidence(domain.Evidence{Claim: domain.Vulnerabilities, Subject: candidate.Artifact, Status: domain.Verified, Provider: domain.Supplement, Source: "https://api.osv.dev/v1/querybatch", ProviderVersion: "osv-v1/git-candidate-v2", RawSHA256: domain.Digest(hex.EncodeToString(sum[:])), ObservedAt: now, ExpiresAt: now + 3600, Applicability: applicability})
	return evidence, data, err
}

// A complete versionless repository query discovers a positive coverage control.
// One valid non-withdrawn record with explicit affected tags proves that the
// provider covers this repository. It does not prove database completeness.
func (c *Collector) verifyCoverage(ctx context.Context, refs map[string]reference, repo string, obs *observation, now int64) error {
	ids := make([]string, 0, len(refs))
	for id := range refs {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	if len(ids) > maxCoverageRecords {
		ids = ids[:maxCoverageRecords]
	}
	for _, id := range ids {
		data, err := c.request(ctx, http.MethodGet, "https://api.osv.dev/v1/vulns/"+id, nil)
		if err != nil {
			return err
		}
		obs.Records = append(obs.Records, json.RawMessage(data))
		if observationSize(*obs) > maxObservation {
			return errors.New("advisory observation exceeds limit")
		}
		covered, withdrawn, err := record(data, refs[id], repo, "", now)
		if err == nil && covered && !withdrawn {
			return nil
		}
	}
	return errors.New("advisory project coverage is unverified")
}

func observationSize(o observation) int {
	size := 0
	for _, q := range o.Queries {
		size += len(q.Request) + len(q.Response)
	}
	for _, r := range o.Records {
		size += len(r)
	}
	return size
}

func (c *Collector) queryAll(ctx context.Context, queries []query, obs *observation, now int64) ([]map[string]reference, error) {
	results := make([]map[string]reference, len(queries))
	tokens := make([]map[string]bool, len(queries))
	pending := []int{}
	for i := range queries {
		results[i] = map[string]reference{}
		tokens[i] = map[string]bool{}
		pending = append(pending, i)
	}
	for n := 0; len(pending) > 0; n++ {
		if n >= 8 {
			return nil, errors.New("advisory pagination exceeds limit")
		}
		batch := []query{}
		for _, i := range pending {
			batch = append(batch, queries[i])
		}
		body, err := json.Marshal(struct {
			Queries []query `json:"queries"`
		}{batch})
		if err != nil {
			return nil, err
		}
		data, err := c.request(ctx, http.MethodPost, "https://api.osv.dev/v1/querybatch", body)
		if err != nil {
			return nil, err
		}
		obs.Queries = append(obs.Queries, exchange{body, data})
		if observationSize(*obs) > maxObservation {
			return nil, errors.New("advisory observation exceeds limit")
		}
		var response struct {
			Results []json.RawMessage `json:"results"`
		}
		if err := decodeObject(data, &response, "results"); err != nil {
			return nil, err
		}
		if len(response.Results) != len(pending) {
			return nil, errors.New("incomplete advisory batch")
		}
		continued := []int{}
		for slot, raw := range response.Results {
			index := pending[slot]
			var p page
			if err := decodeObject(raw, &p, "vulns", "next_page_token"); err != nil {
				return nil, err
			}
			for _, rawRef := range p.Vulns {
				var ref reference
				if err := decodeObject(rawRef, &ref, "id", "modified"); err != nil {
					return nil, err
				}
				if !identifier.MatchString(ref.ID) || !validTime(ref.Modified, now) {
					return nil, errors.New("invalid advisory reference")
				}
				if old, exists := results[index][ref.ID]; exists && old != ref {
					return nil, errors.New("advisory changed during pagination")
				}
				results[index][ref.ID] = ref
				limit := maxFindings
				if queries[index].Version == "" {
					limit = maxCoverageReferences
				}
				if len(results[index]) > limit {
					return nil, errors.New("advisory inventory exceeds limit")
				}
			}
			if p.Next != "" {
				if len(p.Next) > 4096 || tokens[index][p.Next] {
					return nil, errors.New("invalid or repeated advisory page token")
				}
				tokens[index][p.Next] = true
				queries[index].PageToken = p.Next
				continued = append(continued, index)
			}
		}
		pending = continued
	}
	return results, nil
}

func validTime(value string, now int64) bool {
	t, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && t.Unix() > 0 && t.Unix() <= now
}

func (c *Collector) request(ctx context.Context, method, url string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "BrewWarden/1")
	response, err := c.client.Do(req)
	if err != nil {
		return nil, errors.New("advisory source unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("advisory source returned unsupported status")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil || len(data) > maxResponse {
		return nil, errors.New("advisory response incomplete or oversized")
	}
	return data, nil
}

func record(data []byte, ref reference, repo, tag string, now int64) (affected, withdrawn bool, err error) {
	var v struct {
		ID        string            `json:"id"`
		Modified  string            `json:"modified"`
		Withdrawn string            `json:"withdrawn"`
		Schema    string            `json:"schema_version"`
		Affected  []json.RawMessage `json:"affected"`
	}
	if err := decodeObject(data, &v, "id", "modified", "withdrawn", "affected", "schema_version"); err != nil {
		return false, false, err
	}
	if v.Schema != "" && (!version.MatchString(v.Schema) || !strings.HasPrefix(v.Schema, "1.")) {
		return false, false, errors.New("unsupported advisory schema")
	}
	if v.ID != ref.ID || v.Modified != ref.Modified || !validTime(v.Modified, now) || len(v.Affected) == 0 {
		return false, false, errors.New("advisory identity, freshness or applicability is unresolved")
	}
	if v.Withdrawn != "" {
		withdrawal, _ := time.Parse(time.RFC3339Nano, v.Withdrawn)
		modification, _ := time.Parse(time.RFC3339Nano, v.Modified)
		if !validTime(v.Withdrawn, now) || withdrawal.After(modification) {
			return false, false, errors.New("invalid withdrawal time")
		}
		withdrawn = true
	}
	matched := false
	for _, raw := range v.Affected {
		var a struct {
			Ranges   []json.RawMessage `json:"ranges"`
			Versions []string          `json:"versions"`
		}
		if err := decodeObject(raw, &a, "ranges", "versions"); err != nil {
			return false, false, err
		}
		for _, rawRange := range a.Ranges {
			var r struct {
				Type   string            `json:"type"`
				Repo   string            `json:"repo"`
				Events []json.RawMessage `json:"events"`
			}
			if err := decodeObject(rawRange, &r, "type", "repo", "events"); err != nil {
				return false, false, err
			}
			if r.Type != "GIT" || strings.TrimSuffix(r.Repo, ".git") != repo {
				continue
			}
			if len(r.Events) == 0 {
				return false, false, errors.New("incomplete advisory range")
			}
			introduced := false
			for _, event := range r.Events {
				var e map[string]string
				if err := json.Unmarshal(event, &e); err != nil || len(e) != 1 {
					return false, false, errors.New("invalid advisory range event")
				}
				for kind, value := range e {
					if !slices.Contains([]string{"introduced", "fixed", "last_affected", "limit"}, kind) || (value == "0" && kind != "introduced") || (value != "0" && (len(value) != 40 || strings.Trim(value, "0123456789abcdef") != "")) {
						return false, false, errors.New("unsupported advisory range event")
					}
					introduced = introduced || kind == "introduced"
				}
			}
			if !introduced {
				return false, false, errors.New("incomplete advisory range")
			}
			// An empty requested tag is used only to establish project coverage.
			// Only explicit affected tags establish candidate applicability. GIT event
			// order is not necessarily linear (for example, cherry-picked fixes).
			// Never infer that a tag is unaffected from these ranges.
			matched = true
			if slices.Contains(a.Versions, tag) && tag != "" || tag == "" && slices.ContainsFunc(a.Versions, func(v string) bool { return v != "" }) {
				affected = true
			}
		}
	}
	if !matched || (!affected && !withdrawn) {
		return false, false, errors.New("exact advisory tag applicability is unresolved")
	}
	return affected, withdrawn, nil
}
