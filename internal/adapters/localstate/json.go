// Package localstate owns the local JSON configuration and history boundary.
package localstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"
)

const maxDocumentBytes = 64 * 1024

// Decode with the standard JSON implementation, supplementing its permissive
// duplicate-key and case-insensitive field matching. This is schema validation,
// not a separate JSON parser. All persisted schema fields have explicit tags.
func decodeStrict(data []byte, out any) error {
	if len(data) > maxDocumentBytes || !utf8.Valid(data) {
		return errors.New("JSON exceeds size limit or contains invalid UTF-8")
	}
	t := reflect.TypeOf(out)
	if t.Kind() != reflect.Pointer {
		return errors.New("JSON destination must be a pointer")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := validateValue(d, t.Elem(), 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("JSON must contain exactly one value")
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	return d.Decode(out)
}

func validateValue(d *json.Decoder, t reflect.Type, depth int) error {
	if depth > 16 {
		return errors.New("JSON nesting exceeds limit")
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	token, err := d.Token()
	if err != nil {
		return errors.New("malformed JSON")
	}
	if token == nil {
		return errors.New("null is not a configuration or record value")
	}
	switch t.Kind() {
	case reflect.Struct:
		if token != json.Delim('{') {
			return errors.New("expected JSON object")
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name != "" && name != "-" {
				fields[name] = f.Type
			}
		}
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return errors.New("invalid object key")
			}
			name, ok := key.(string)
			field, known := fields[name]
			if !ok || !known || seen[name] {
				return errors.New("unknown, mis-cased or duplicate JSON field")
			}
			seen[name] = true
			if err := validateValue(d, field, depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("unterminated JSON object")
		}
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if field.Tag.Get("required") == "true" && !seen[name] {
				return errors.New("missing required JSON field")
			}
		}
	case reflect.Slice:
		if token != json.Delim('[') {
			return errors.New("expected JSON array")
		}
		for d.More() {
			if err := validateValue(d, t.Elem(), depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("unterminated JSON array")
		}
	default:
		// Type and numeric range checks are performed by the typed decode.
		if _, structured := token.(json.Delim); structured {
			return errors.New("expected scalar JSON value")
		}
	}
	return nil
}
