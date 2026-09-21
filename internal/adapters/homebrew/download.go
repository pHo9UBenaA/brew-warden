package homebrew

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

func publicClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &http.Client{Transport: transport, Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("metadata redirects are unsupported") }}
}
func download(ctx context.Context, client *http.Client, address string, limit int64) ([]byte, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" {
		return nil, errors.New("unsupported metadata URL")
	}
	switch u.Host {
	case "raw.githubusercontent.com":
		if !strings.HasPrefix(u.Path, "/Homebrew/homebrew-core/") {
			return nil, errors.New("unsupported recipe repository")
		}
	case "api.github.com":
		if !strings.HasPrefix(u.Path, "/repos/Homebrew/homebrew-core/attestations/sha256:") {
			return nil, errors.New("unsupported attestation repository")
		}
	default:
		return nil, errors.New("unsupported metadata host")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "BrewWarden")
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("metadata request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength > limit {
		return nil, errors.New("metadata response unavailable or oversized")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("metadata response incomplete or oversized")
	}
	return data, nil
}

// The API envelope is transport, not trust. Only the pinned verifier can turn
// an extracted bundle into evidence for a bottle and expected signing identity.
func bundles(data []byte) ([]byte, error) {
	if len(data) > maxManifest || !utf8.Valid(data) {
		return nil, errors.New("invalid attestation response")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := unambiguousJSON(decoder, 0); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("trailing attestation response")
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	var attestations []map[string]json.RawMessage
	if err := json.Unmarshal(envelope["attestations"], &attestations); err != nil || len(attestations) == 0 || len(attestations) > 32 {
		return nil, errors.New("missing or oversized attestation inventory")
	}
	var out bytes.Buffer
	for _, item := range attestations {
		bundle := item["bundle"]
		if len(bundle) == 0 || bundle[0] != '{' {
			return nil, errors.New("missing attestation bundle")
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, bundle); err != nil {
			return nil, err
		}
		out.Write(compact.Bytes())
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}
func unambiguousJSON(d *json.Decoder, depth int) error {
	if depth > 32 {
		return errors.New("attestation JSON nesting exceeds limit")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	switch token {
	case json.Delim('{'):
		seen := map[string]bool{}
		for d.More() {
			token, err := d.Token()
			if err != nil {
				return err
			}
			key, ok := token.(string)
			normalized := strings.ToLower(key)
			if !ok || seen[normalized] {
				return errors.New("ambiguous attestation field")
			}
			seen[normalized] = true
			if err := unambiguousJSON(d, depth+1); err != nil {
				return err
			}
		}
		token, err = d.Token()
		if err != nil || token != json.Delim('}') {
			return errors.New("invalid attestation object")
		}
	case json.Delim('['):
		for d.More() {
			if err := unambiguousJSON(d, depth+1); err != nil {
				return err
			}
		}
		token, err = d.Token()
		if err != nil || token != json.Delim(']') {
			return errors.New("invalid attestation array")
		}
	default:
		if _, ok := token.(json.Delim); ok {
			return errors.New("invalid attestation scalar")
		}
	}
	return nil
}
