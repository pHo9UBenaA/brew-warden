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

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

const maxResponseBytes = 1024 * 1024
const freshnessSeconds int64 = 3600

// Candidate must come from authenticated Homebrew metadata and its authenticated
// recipe. This adapter does not establish that prerequisite, bottle integrity,
// provenance, vulnerability eligibility, or permission to execute a plan.
type Candidate = ports.SourceCandidate

type Collector struct{ client *http.Client }

// New uses public GitHub REST without credentials, implicit helper installation,
// retries, or API redirects. Source downloads permit only the explicit CDN hop.
func New() *Collector {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Collector{client: &http.Client{
		Timeout:   15 * time.Second,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("publication redirects are unsupported")
		},
	}}
}

// Collect returns the typed claim and exact observation bytes (API response or
// source-verification envelope) for durable storage before using the claim.
func (c *Collector) Collect(ctx context.Context, candidate Candidate, now int64) (domain.Evidence, []byte, error) {
	api, tag, asset, err := mapping(candidate)
	if c == nil || c.client == nil || ctx == nil || err != nil || now <= 0 || now > 1<<62 {
		return domain.Evidence{}, nil, errors.New("publication candidate is unsupported")
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	data, err := c.fetchRelease(ctx, api)
	if err != nil {
		return domain.Evidence{}, nil, err
	}
	binding, err := inspectPublication(data, candidate, tag, asset, now)
	if err != nil {
		return domain.Evidence{}, nil, err
	}
	if binding.Asset.Digest == "" {
		data, err = c.bindDownloadedSource(ctx, api, candidate, tag, asset, now, binding, data)
		if err != nil {
			return domain.Evidence{}, nil, err
		}
	}
	published := binding.Published
	digest := sha256.Sum256(data)
	evidence, err := domain.NewEvidence(domain.Evidence{
		Claim: domain.Publication, Subject: candidate.Artifact, Status: domain.Verified,
		Provider: domain.Supplement, Source: api, ProviderVersion: "github-rest-2022-11-28/publication-v2",
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

type releaseBinding struct {
	ReleaseID int64
	Tag       string
	Published int64
	Asset     assetDocument
}

// A timestamp-only parser cannot establish a missing digest. Collect must fetch
// the exact asset and recheck its publisher identity in that case.
func publication(data []byte, candidate Candidate, tag, asset string, now int64) (int64, error) {
	binding, err := inspectPublication(data, candidate, tag, asset, now)
	if err != nil {
		return 0, err
	}
	if binding.Asset.Digest == "" {
		return 0, errors.New("publisher asset digest unavailable")
	}
	return binding.Published, nil
}

func inspectPublication(data []byte, candidate Candidate, tag, asset string, now int64) (releaseBinding, error) {
	if len(data) > maxResponseBytes || !utf8.Valid(data) {
		return releaseBinding{}, errors.New("publication response exceeds bounds")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := validateJSON(d, 0); err != nil {
		return releaseBinding{}, err
	}
	if _, err := d.Token(); err != io.EOF {
		return releaseBinding{}, errors.New("publication response has trailing values")
	}
	var release releaseDocument
	if err := exactObject(data, &release, "id", "tag_name", "draft", "prerelease", "published_at", "assets"); err != nil {
		return releaseBinding{}, err
	}
	if release.ID <= 0 || release.Tag != tag || release.Draft == nil || *release.Draft || release.Prerelease == nil || *release.Prerelease {
		return releaseBinding{}, errors.New("release is missing, unpublished or unsupported")
	}
	published, err := timestamp(release.PublishedAt)
	if err != nil || published > now {
		return releaseBinding{}, errors.New("release publication time is unknown or future-dated")
	}
	matches := 0
	var selected assetDocument
	for _, raw := range release.Assets {
		var a assetDocument
		if err := exactObject(raw, &a, "id", "name", "browser_download_url", "digest", "state", "size", "created_at", "updated_at"); err != nil {
			return releaseBinding{}, err
		}
		if a.Name != asset && a.URL != candidate.SourceURL {
			continue
		}
		matches++
		if a.Digest != "" && a.Digest != "sha256:"+string(candidate.SourceSHA256) {
			return releaseBinding{}, errors.New("publisher asset digest mismatch")
		}
		selected = a
		created, e1 := timestamp(a.CreatedAt)
		updated, e2 := timestamp(a.UpdatedAt)
		if a.ID <= 0 || a.Name != asset || a.URL != candidate.SourceURL || a.State != "uploaded" || a.Size <= 0 || e1 != nil || e2 != nil || created > updated || updated > published {
			return releaseBinding{}, errors.New("release asset identity, digest or publication binding is unresolved")
		}
	}
	if matches != 1 {
		return releaseBinding{}, errors.New("release asset is missing or ambiguous")
	}
	return releaseBinding{ReleaseID: release.ID, Tag: release.Tag, Published: published, Asset: selected}, nil
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
