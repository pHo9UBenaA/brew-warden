package githubrelease

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"brewwarden/internal/domain"
)

const maxSourceBytes int64 = 32 * 1024 * 1024

func (c *Collector) fetchRelease(ctx context.Context, api string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return nil, errors.New("cannot construct publication request")
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "BrewWarden/1")
	response, err := c.client.Do(req)
	if err != nil {
		return nil, errors.New("publication source unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("publication source returned an unsupported status")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, errors.New("publication response incomplete")
	}
	return data, nil
}

type sourceObservation struct {
	Schema string          `json:"schema"`
	Before json.RawMessage `json:"releaseBefore"`
	After  json.RawMessage `json:"releaseAfter"`
	URL    string          `json:"sourceURL"`
	SHA256 domain.Digest   `json:"sourceSHA256"`
	Size   int64           `json:"sourceSize"`
}

func (c *Collector) bindDownloadedSource(ctx context.Context, api string, candidate Candidate, tag, asset string, now int64, binding releaseBinding, before []byte) ([]byte, error) {
	if binding.Asset.Size > maxSourceBytes {
		return nil, errors.New("publisher source exceeds download limit")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate.SourceURL, nil)
	if err != nil {
		return nil, errors.New("cannot construct source request")
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "BrewWarden/1")
	// Public GitHub release assets redirect once to GitHub's asset CDN. Never
	// forward credentials or permit a different host, port, protocol or path class.
	client := *c.client
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) != 1 || next.URL.Scheme != "https" || next.URL.Host != "release-assets.githubusercontent.com" || next.URL.User != nil || next.URL.Fragment != "" ||
			!strings.HasPrefix(next.URL.Path, "/github-production-release-asset/") || next.Header.Get("Authorization") != "" {
			return errors.New("unsupported publisher source redirect")
		}
		return nil
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, errors.New("publisher source download unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength > maxSourceBytes {
		return nil, errors.New("publisher source returned unsupported status or size")
	}
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(response.Body, binding.Asset.Size+1))
	if err != nil || size != binding.Asset.Size || size > maxSourceBytes {
		return nil, errors.New("publisher source download is incomplete or oversized")
	}
	digest := domain.Digest(hex.EncodeToString(hash.Sum(nil)))
	if digest != candidate.SourceSHA256 {
		return nil, errors.New("publisher source bytes do not match authenticated checksum")
	}
	after, err := c.fetchRelease(ctx, api)
	if err != nil {
		return nil, err
	}
	current, err := inspectPublication(after, candidate, tag, asset, now)
	if err != nil || current != binding {
		return nil, errors.New("publisher release changed during source verification")
	}
	return json.Marshal(sourceObservation{Schema: "github-source-publication-v1", Before: before, After: after, URL: candidate.SourceURL, SHA256: digest, Size: size})
}
