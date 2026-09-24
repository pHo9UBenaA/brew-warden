package homebrew

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

const advisoryFeedURL = "https://formulae.brew.sh/api/advisories.json"
const maxAdvisoryFeed = 64 * 1024 * 1024

type advisoryIndex struct {
	Records map[string][]json.RawMessage `json:"advisories"`
	Meta    struct {
		Count  int    `json:"count"`
		Schema string `json:"schema_version"`
	} `json:"meta"`
}
type brewAdvisoryStatus struct {
	Open       []brewAdvisoryEntry `json:"open" required:"true"`
	Patched    []brewAdvisoryEntry `json:"patched" required:"true"`
	FixedCount int                 `json:"fixed_count" required:"true"`
}
type brewAdvisoryEntry struct {
	ID       string   `json:"id" required:"true"`
	Upstream []string `json:"upstream"`
	Fix      string   `json:"fix"`
	FixedIn  string   `json:"fixed_in"`
}

func fetchAdvisoryJSON(ctx context.Context, client *http.Client, address string, limit int64) ([]byte, error) {
	if address != advisoryFeedURL {
		if !strings.HasPrefix(address, "https://formulae.brew.sh/api/formula/") {
			return nil, errors.New("invalid public advisory URL")
		}
		name := strings.TrimPrefix(address, "https://formulae.brew.sh/api/formula/")
		if !strings.HasSuffix(name, ".json") || !domain.ValidRequest("install", []string{strings.TrimSuffix(name, ".json")}) {
			return nil, errors.New("invalid public advisory URL")
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Cache-Control", "no-cache")
	request.Header.Set("User-Agent", "BrewWarden")
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("public advisory source unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength > limit {
		return nil, errors.New("public advisory source unavailable or oversized")
	}
	if value := response.Header.Get("Age"); value != "" {
		age, err := strconv.ParseInt(value, 10, 64)
		if err != nil || age < 0 || age > 86400 {
			return nil, errors.New("public advisory response is stale")
		}
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, errors.New("public advisory response incomplete or oversized")
	}
	return raw, nil
}

func parseAdvisoryIndex(raw []byte) (advisoryIndex, error) {
	var index advisoryIndex
	if len(raw) > maxAdvisoryFeed || !utf8.Valid(raw) {
		return index, errors.New("invalid advisory feed")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := unambiguousJSON(decoder, 0); err != nil {
		return index, errors.New("ambiguous advisory feed")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return index, errors.New("trailing advisory feed data")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return index, errors.New("invalid advisory envelope")
	}
	var meta struct {
		Count  int    `json:"count" required:"true"`
		Schema string `json:"schema_version" required:"true"`
	}
	if err := decodeSchema(fields["meta"], &meta, true); err != nil {
		return index, err
	}
	if err := json.Unmarshal(fields["advisories"], &index.Records); err != nil || index.Records == nil || meta.Schema != "1.7.3" || meta.Count < 0 || meta.Count > 100000 {
		return index, errors.New("unsupported advisory feed schema")
	}
	index.Meta.Count, index.Meta.Schema = meta.Count, meta.Schema

	count := 0
	for name, records := range index.Records {
		if !domain.ValidRequest("install", []string{name}) || records == nil {
			return advisoryIndex{}, errors.New("invalid advisory feed inventory")
		}
		count += len(records)
	}
	if count != index.Meta.Count {
		return advisoryIndex{}, errors.New("incomplete advisory feed inventory")
	}
	return index, nil
}

func parseBrewAdvisoryStatus(raw []byte, candidate formulaMetadata, records []json.RawMessage) (brewAdvisoryStatus, error) {
	// Project public formula JSON through the same identity decoder used by info.
	wrapped := append([]byte(`{"formulae":[`), raw...)
	wrapped = append(wrapped, []byte(`],"casks":[]}`)...)
	info, err := parseInfo(wrapped, []string{candidate.Name})
	if err != nil {
		return brewAdvisoryStatus{}, err
	}
	f := info[0]
	if f.artifact() != candidate.artifact() {
		return brewAdvisoryStatus{}, errors.New("public advisory metadata does not match candidate")
	}
	var doc struct {
		Vulnerabilities *brewAdvisoryStatus `json:"vulnerabilities"`
	}
	if err := decodeSchema(raw, &doc, true); err != nil {
		return brewAdvisoryStatus{}, err
	}
	if doc.Vulnerabilities == nil {
		if len(records) != 0 {
			return brewAdvisoryStatus{}, errors.New("homebrew advisory status omitted for indexed formula")
		}
		return brewAdvisoryStatus{Open: []brewAdvisoryEntry{}, Patched: []brewAdvisoryEntry{}}, nil
	}
	status := *doc.Vulnerabilities
	if status.FixedCount < 0 || status.FixedCount > 100000 {
		return brewAdvisoryStatus{}, errors.New("invalid fixed advisory count")
	}
	seen := map[string]bool{}
	for _, group := range [][]brewAdvisoryEntry{status.Open, status.Patched} {
		for _, entry := range group {
			if !validAdvisoryID(entry.ID) || seen[entry.ID] {
				return brewAdvisoryStatus{}, errors.New("ambiguous Homebrew advisory status")
			}
			seen[entry.ID] = true
			for _, id := range entry.Upstream {
				if !validAdvisoryID(id) {
					return brewAdvisoryStatus{}, errors.New("invalid upstream advisory identifier")
				}
			}
		}
	}
	for _, entry := range status.Patched {
		if entry.Fix != "patch" || entry.FixedIn == "" || len(entry.Upstream) == 0 {
			return brewAdvisoryStatus{}, errors.New("incomplete Homebrew patch evidence")
		}
	}
	return status, nil
}

// Homebrew owns version-range comparison. Only an explicit patch resolution for
// the same upstream advisory can resolve an open upstream finding. An absent
// record, unrelated fix, or lower finding count never does so.
func combineAdvisories(candidate formulaMetadata, osv publicVulnsReport, brew brewAdvisoryStatus) (domain.Applicability, error) {
	if len(brew.Open) != 0 {
		return domain.Affected, nil
	}
	var native publicFinding
	for _, f := range osv.Findings {
		if f.Formula == candidate.Name {
			native = f
		}
	}
	for _, open := range native.Open {
		ids := append([]string{open.ID}, open.Aliases...)
		fixed := false
		for _, patch := range brew.Patched {
			if slices.ContainsFunc(patch.Upstream, func(id string) bool { return slices.Contains(ids, id) }) {
				fixed = true
			}
		}
		if !fixed {
			return domain.Affected, nil
		}
	}
	// A native patch is already evaluated by the pinned scanner against the
	// candidate recipe; a contradictory open Homebrew record was handled above.
	return domain.NoKnownApplicableFindings, nil
}

func (w workspace) collectPublicAdvisories(ctx context.Context, client *http.Client, candidates []formulaMetadata, now int64, revision string) ([]domain.Evidence, error) {
	if reviewedBrewRevisions[revision] == "" {
		return nil, errors.New("unsupported Homebrew advisory source revision")
	}
	report, rawScan, err := w.scanCandidateVulnerabilities(ctx, candidates)
	if err != nil {
		return nil, err
	}
	if err := w.rawObservation(rawScan); err != nil {
		return nil, err
	}
	feed, err := fetchAdvisoryJSON(ctx, client, advisoryFeedURL, maxAdvisoryFeed)
	if err != nil {
		return nil, err
	}
	index, err := parseAdvisoryIndex(feed)
	if err != nil {
		return nil, err
	}
	// Retain the exact full response once using standard gzip, rather than repeat
	// its 40+ MB content for each subject. The raw digest identifies decoded bytes.
	var compressed bytes.Buffer
	zipper := gzip.NewWriter(&compressed)
	if _, err := zipper.Write(feed); err != nil {
		return nil, err
	}
	if err := zipper.Close(); err != nil {
		return nil, err
	}
	archive, err := json.Marshal(struct {
		Encoding string
		SHA256   domain.Digest
		Data     []byte
	}{"gzip", digestBytes(feed), compressed.Bytes()})
	if err != nil {
		return nil, err
	}
	if err := w.rawObservation(archive); err != nil {
		return nil, err
	}
	evidence := make([]domain.Evidence, 0, len(candidates))
	for _, candidate := range candidates {
		raw, err := fetchAdvisoryJSON(ctx, client, "https://formulae.brew.sh/api/formula/"+candidate.Name+".json", 2*1024*1024)
		if err != nil {
			return nil, err
		}
		status, err := parseBrewAdvisoryStatus(raw, candidate, index.Records[candidate.Name])
		if err != nil {
			return nil, err
		}
		applies, err := combineAdvisories(candidate, report, status)
		if err != nil {
			return nil, err
		}
		observation, err := json.Marshal(struct {
			Schema            int
			Candidate         domain.Artifact
			Scan, FeedArchive domain.Digest
			Formula           json.RawMessage
		}{1, candidate.artifact(), digestBytes(rawScan), digestBytes(archive), raw})
		if err != nil {
			return nil, err
		}
		e, err := domain.NewEvidence(domain.Evidence{Claim: domain.Vulnerabilities, Subject: candidate.artifact(), Status: domain.Verified, Provider: domain.Homebrew, Source: "brew vulns + Homebrew Advisory Database", ProviderVersion: "brew/" + reviewedBrewRevisions[revision], RawSHA256: digestBytes(observation), ObservedAt: now, ExpiresAt: now + 3600, Applicability: applies})
		if err != nil {
			return nil, err
		}
		if err := w.observation(e, observation); err != nil {
			return nil, err
		}
		evidence = append(evidence, e)
	}
	return evidence, nil
}
