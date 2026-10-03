// Strict schemas for persisted Homebrew and execution data.
package homebrew

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"
)

const maxDocumentBytes = maxManifest

// Decode with the standard JSON implementation, supplementing its permissive
// duplicate-key and case-insensitive field matching. This is schema validation,
// not a separate JSON parser. Untagged exported fields use their exact Go names.
func decodeStrict(data []byte, out any) error {
	return decodeSchema(data, out, false)
}

// External command output may contain irrelevant fields, but all selected fields
// retain exact spelling, uniqueness, required presence and non-null values.
func decodeSchema(data []byte, out any, allowUnknownFields bool) error {
	if len(data) > maxDocumentBytes || !utf8.Valid(data) {
		return errors.New("JSON exceeds size limit or contains invalid UTF-8")
	}
	destinationType := reflect.TypeOf(out)
	if destinationType.Kind() != reflect.Pointer {
		return errors.New("JSON destination must be a pointer")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := validateValue(decoder, destinationType.Elem(), 0, allowUnknownFields); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("JSON must contain exactly one value")
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	if !allowUnknownFields {
		decoder.DisallowUnknownFields()
	}
	return decoder.Decode(out)
}

// Untagged exported fields retain their Go name; explicitly tagged fields use
// the tag's exact spelling, without options such as omitempty.
func schemaFieldName(field reflect.StructField) string {
	name := strings.Split(field.Tag.Get("json"), ",")[0]
	if name == "" && field.PkgPath == "" {
		return field.Name
	}
	return name
}

func validateValue(decoder *json.Decoder, valueType reflect.Type, depth int, allowUnknownFields bool) error {
	if depth > 16 {
		return errors.New("JSON nesting exceeds limit")
	}
	for valueType.Kind() == reflect.Pointer {
		valueType = valueType.Elem()
	}
	token, err := decoder.Token()
	if err != nil {
		return errors.New("malformed JSON")
	}
	if token == nil {
		return errors.New("null is not a configuration or record value")
	}
	switch valueType.Kind() {
	case reflect.Struct:
		if token != json.Delim('{') {
			return errors.New("expected JSON object")
		}
		fields := map[string]reflect.Type{}
		var requiredFields []string
		for i := 0; i < valueType.NumField(); i++ {
			field := valueType.Field(i)
			name := schemaFieldName(field)
			if name != "" && name != "-" {
				fields[name] = field.Type
			}
			untaggedExported := field.Tag.Get("json") == "" && field.PkgPath == ""
			if field.Tag.Get("required") == "true" || untaggedExported {
				requiredFields = append(requiredFields, name)
			}
		}

		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return errors.New("invalid object key")
			}
			name, isString := key.(string)
			fieldType, known := fields[name]
			if !isString || (!known && !allowUnknownFields) || seen[name] {
				return errors.New("unknown, mis-cased or duplicate JSON field")
			}
			seen[name] = true
			if !known {
				for expected := range fields {
					if strings.EqualFold(name, expected) {
						return errors.New("mis-cased JSON field")
					}
				}
				var ignored json.RawMessage
				if err := decoder.Decode(&ignored); err != nil {
					return err
				}
				continue
			}
			if err := validateValue(decoder, fieldType, depth+1, allowUnknownFields); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("unterminated JSON object")
		}
		for _, name := range requiredFields {
			if !seen[name] {
				return errors.New("missing required JSON field")
			}
		}
	case reflect.Slice:
		if token != json.Delim('[') {
			return errors.New("expected JSON array")
		}
		for decoder.More() {
			if err := validateValue(decoder, valueType.Elem(), depth+1, allowUnknownFields); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
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
