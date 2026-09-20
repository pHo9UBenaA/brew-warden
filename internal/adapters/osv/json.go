package osv

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

func decodeObject(data []byte, out any, keys ...string) error {
	if len(data) > maxResponse || !utf8.Valid(data) {
		return errors.New("advisory response exceeds bounds")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := validateJSON(decoder, 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("trailing advisory JSON")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return errors.New("advisory response is not an object")
	}
	for name, value := range fields {
		for _, key := range keys {
			if strings.EqualFold(name, key) && (name != key || bytes.Equal(bytes.TrimSpace(value), []byte("null"))) {
				return errors.New("ambiguous or null advisory field")
			}
		}
	}
	if err := json.Unmarshal(data, out); err != nil {
		return errors.New("invalid advisory field type")
	}
	return nil
}

// Supplement the standard parser with duplicate detection and a depth bound.
// This is not a new JSON parser or a cryptographic serialization format.
func validateJSON(d *json.Decoder, depth int) error {
	if depth > 16 {
		return errors.New("advisory nesting exceeds limit")
	}
	token, err := d.Token()
	if err != nil {
		return errors.New("malformed advisory JSON")
	}
	switch token {
	case json.Delim('{'):
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			name, ok := key.(string)
			if err != nil || !ok || seen[name] {
				return errors.New("duplicate or invalid advisory key")
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
			return errors.New("unexpected advisory delimiter")
		}
		return nil
	}
	_, err = d.Token() // The standard decoder checks the matching delimiter.
	return err
}
