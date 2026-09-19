// Package githubrelease collects narrowly mapped upstream publication evidence.
// It never treats a release timestamp as the publication date of a bottle rebuild.
package githubrelease

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"brewwarden/internal/domain"
)

const maxResponseBytes = 1024 * 1024
const freshnessSeconds int64 = 3600

// Candidate must come from authenticated Homebrew metadata and its authenticated
// recipe. This adapter does not establish that prerequisite, bottle integrity,
// provenance, vulnerability eligibility, or permission to execute a plan.
type Candidate struct {
	Artifact     domain.Artifact
	SourceURL    string
	SourceSHA256 domain.Digest
}

type Collector struct{ client *http.Client }

// New uses public GitHub REST without credentials, implicit helper installation,
// retries, or cross-host redirects. Its timeout bounds the complete request.
func New() *Collector {
	return &Collector{client: &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("publication redirects are unsupported")
		},
	}}
}

// Collect returns both the typed claim and the exact response bytes whose hash
// it records, so a caller can durably retain the observation before using it.
func (c *Collector) Collect(ctx context.Context, candidate Candidate, now int64) (domain.Evidence, []byte, error) {
	api, tag, asset, err := mapping(candidate)
	if c == nil || c.client == nil || ctx == nil || err != nil || now <= 0 || now > 1<<62 {
		return domain.Evidence{}, nil, errors.New("publication candidate is unsupported")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return domain.Evidence{}, nil, errors.New("cannot construct publication request")
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "BrewWarden-publication-probe/1")
	response, err := c.client.Do(req)
	if err != nil {
		return domain.Evidence{}, nil, errors.New("publication source unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return domain.Evidence{}, nil, errors.New("publication source returned an unsupported status")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return domain.Evidence{}, nil, errors.New("publication response incomplete")
	}
	published, err := publication(data, candidate, tag, asset, now)
	if err != nil {
		return domain.Evidence{}, nil, err
	}
	digest := sha256.Sum256(data)
	evidence, err := domain.NewEvidence(domain.Evidence{
		Claim: domain.Publication, Subject: candidate.Artifact, Status: domain.Verified,
		Provider: domain.Supplement, Source: api, ProviderVersion: "github-rest-2022-11-28/publication-v1",
		RawSHA256: domain.Digest(hex.EncodeToString(digest[:])), ObservedAt: now,
		ExpiresAt: now + freshnessSeconds, PublishedAt: published, Publication: domain.UpstreamPublication,
	})
	if err != nil {
		return domain.Evidence{}, nil, err
	}
	return evidence, data, nil
}

func mapping(c Candidate) (api, tag, asset string, err error) {
	if !c.Artifact.Valid() || !c.SourceSHA256.Valid() {
		return "", "", "", errors.New("invalid candidate")
	}
	var repository string
	switch c.Artifact.Name {
	case "jq":
		repository, tag, asset = "jqlang/jq", "jq-"+c.Artifact.Version, "jq-"+c.Artifact.Version+".tar.gz"
	case "oniguruma":
		repository, tag, asset = "kkos/oniguruma", "v"+c.Artifact.Version, "onig-"+c.Artifact.Version+".tar.gz"
	default:
		return "", "", "", errors.New("unsupported publisher mapping")
	}
	// Versions used in URL paths are deliberately narrower than Artifact's
	// generic version grammar. No escaping, repository inference or fuzzy match.
	if strings.Trim(c.Artifact.Version, "0123456789.") != "" || len(strings.Split(c.Artifact.Version, ".")) != 3 || strings.HasPrefix(c.Artifact.Version, ".") || strings.HasSuffix(c.Artifact.Version, ".") || strings.Contains(c.Artifact.Version, "..") {
		return "", "", "", errors.New("unsupported publication version")
	}
	if c.SourceURL != "https://github.com/"+repository+"/releases/download/"+tag+"/"+asset {
		return "", "", "", errors.New("publisher source mismatch")
	}
	return "https://api.github.com/repos/" + repository + "/releases/tags/" + tag, tag, asset, nil
}

type releaseDocument struct {
	ID          int64             `json:"id"`
	Tag         string            `json:"tag_name"`
	Draft       *bool             `json:"draft"`
	Prerelease  *bool             `json:"prerelease"`
	PublishedAt string            `json:"published_at"`
	Assets      []json.RawMessage `json:"assets"`
}

type assetDocument struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"browser_download_url"`
	Digest    string `json:"digest"`
	State     string `json:"state"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func publication(data []byte, candidate Candidate, tag, asset string, now int64) (int64, error) {
	if len(data) > maxResponseBytes || !utf8.Valid(data) {
		return 0, errors.New("publication response exceeds bounds")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := validateJSON(d, 0); err != nil {
		return 0, err
	}
	if _, err := d.Token(); err != io.EOF {
		return 0, errors.New("publication response has trailing values")
	}
	var release releaseDocument
	if err := exactObject(data, &release, "id", "tag_name", "draft", "prerelease", "published_at", "assets"); err != nil {
		return 0, err
	}
	if release.ID <= 0 || release.Tag != tag || release.Draft == nil || *release.Draft || release.Prerelease == nil || *release.Prerelease {
		return 0, errors.New("release is missing, unpublished or unsupported")
	}
	published, err := timestamp(release.PublishedAt)
	if err != nil || published > now {
		return 0, errors.New("release publication time is unknown or future-dated")
	}
	matches := 0
	for _, raw := range release.Assets {
		var a assetDocument
		if err := exactObject(raw, &a, "id", "name", "browser_download_url", "digest", "state", "size", "created_at", "updated_at"); err != nil {
			return 0, err
		}
		if a.Name != asset && a.URL != candidate.SourceURL {
			continue
		}
		matches++
		if a.Digest == "" {
			return 0, errors.New("publisher asset digest unavailable")
		}
		if a.Digest != "sha256:"+string(candidate.SourceSHA256) {
			return 0, errors.New("publisher asset digest mismatch")
		}
		created, e1 := timestamp(a.CreatedAt)
		updated, e2 := timestamp(a.UpdatedAt)
		if a.ID <= 0 || a.Name != asset || a.URL != candidate.SourceURL || a.State != "uploaded" || a.Size <= 0 || e1 != nil || e2 != nil || created > updated || updated > published {
			return 0, errors.New("release asset identity, digest or publication binding is unresolved")
		}
	}
	if matches != 1 {
		return 0, errors.New("release asset is missing or ambiguous")
	}
	return published, nil
}

func timestamp(value string) (int64, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil || t.Unix() <= 0 {
		return 0, errors.New("invalid publication timestamp")
	}
	return t.Unix(), nil
}

// Decision-bearing keys must match exactly. New unrelated GitHub fields remain
// compatible; encoding/json's case-insensitive matching must not fill a field.
func exactObject(data []byte, out any, keys ...string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return errors.New("publication value is not an object")
	}
	for key := range fields {
		for _, expected := range keys {
			if key != expected && strings.EqualFold(key, expected) {
				return errors.New("publication field has ambiguous casing")
			}
		}
	}
	if err := json.Unmarshal(data, out); err != nil {
		return errors.New("publication field has an invalid type")
	}
	return nil
}

// Supplement the standard parser with duplicate detection and a depth bound.
// This is not a new JSON parser or a cryptographic serialization format.
func validateJSON(d *json.Decoder, depth int) error {
	if depth > 16 {
		return errors.New("publication nesting exceeds limit")
	}
	token, err := d.Token()
	if err != nil {
		return errors.New("malformed publication JSON")
	}
	switch token {
	case json.Delim('{'):
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			name, ok := key.(string)
			if err != nil || !ok || seen[name] {
				return errors.New("duplicate or invalid publication key")
			}
			seen[name] = true
			if err := validateJSON(d, depth+1); err != nil {
				return err
			}
		}
	case json.Delim('['):
		for d.More() {
			if err := validateJSON(d, depth+1); err != nil {
				return err
			}
		}
	default:
		if _, delimiter := token.(json.Delim); delimiter {
			return errors.New("unexpected publication delimiter")
		}
		return nil
	}
	_, err = d.Token() // The standard decoder checks the matching delimiter.
	return err
}
