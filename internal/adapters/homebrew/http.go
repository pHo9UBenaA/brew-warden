package homebrew

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

func publicClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &http.Client{Transport: transport, Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("metadata redirects are unsupported") }}
}

// Reject ambiguous keys and excessive depth in official advisory responses.
func unambiguousJSON(d *json.Decoder, depth int) error {
	if depth > 32 {
		return errors.New("advisory JSON nesting exceeds limit")
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
				return errors.New("ambiguous advisory field")
			}
			seen[normalized] = true
			if err := unambiguousJSON(d, depth+1); err != nil {
				return err
			}
		}
		token, err = d.Token()
		if err != nil || token != json.Delim('}') {
			return errors.New("invalid advisory object")
		}
	case json.Delim('['):
		for d.More() {
			if err := unambiguousJSON(d, depth+1); err != nil {
				return err
			}
		}
		token, err = d.Token()
		if err != nil || token != json.Delim(']') {
			return errors.New("invalid advisory array")
		}
	default:
		if _, ok := token.(json.Delim); ok {
			return errors.New("invalid advisory scalar")
		}
	}
	return nil
}
