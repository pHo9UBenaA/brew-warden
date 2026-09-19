package localstate

import (
	"strings"
	"testing"

	"brewwarden/internal/domain"
)

func TestConfig(t *testing.T) {
	for _, data := range []string{
		`{"schemaVersion":1}`,
		`{"schemaVersion":1,"age":{"minimumHours":168},"trust":{"allowedTaps":["homebrew/core"]},"verification":{"requireChecksum":true,"requireBottleAttestation":true},"emergency":{"mode":"suggest","waivableRules":["age"]}}`,
	} {
		p, err := ParseConfig(strings.NewReader(data))
		if err != nil || p.MinimumAgeSeconds() != domain.DefaultMinimumAgeSeconds || !p.Valid() {
			t.Fatalf("%v: %+v", err, p)
		}
	}
	p, err := ParseConfig(strings.NewReader(`{"schemaVersion":1,"age":{"minimumHours":0}}`))
	if err != nil || !p.Valid() || p.MinimumAgeSeconds() != 0 {
		t.Fatalf("explicit zero: %+v %v", p, err)
	}
}

func TestRejectAmbiguousOrWeakenedConfig(t *testing.T) {
	for _, data := range []string{
		``, `{}`, `null`, `[]`, `{"schemaVersion":2}`, `{"SchemaVersion":1}`,
		`{"schemaVersion":1,"schemaVersion":1}`, `{"schemaVersion":1,"schema\u0056ersion":1}`,
		`{"schemaVersion":1,"age":{"MinimumHours":0}}`,
		`{"schemaVersion":1,"age":{"minimumHours":168,"minimumHours":0}}`,
		`{"schemaVersion":1} {"schemaVersion":1}`, `{"schemaVersion":1} garbage`,
		`{"schemaVersion":1,"age":null}`, `{"schemaVersion":1,"age":{"minimumHours":null}}`,
		`{"schemaVersion":1,"age":{"minimumHours":-1}}`, `{"schemaVersion":1,"age":{"minimumHours":1.5}}`,
		`{"schemaVersion":1,"age":{"minimumHours":"168"}}`, `{"schemaVersion":1,"age":{"minimumHours":1e30}}`,
		`{"schemaVersion":1,"age":{"minimumHours":9223372036854775807}}`,
		`{"schemaVersion":1,"verification":{"requireChecksum":false}}`,
		`{"schemaVersion":1,"verification":{"requireBottleAttestation":false}}`,
		`{"schemaVersion":1,"trust":{"allowedTaps":["attacker/tap"]}}`,
		`{"schemaVersion":1,"emergency":{"mode":"auto","waivableRules":["age"]}}`,
		`{"schemaVersion":1,"emergency":{"mode":"suggest","waivableRules":["age","provenance"]}}`,
		`{"schemaVersion":1,"unknown":true}`, "{\"schemaVersion\":1,\"x\":\"\xff\"}",
		strings.Repeat(" ", maxDocumentBytes) + `{"schemaVersion":1}`,
	} {
		if p, err := ParseConfig(strings.NewReader(data)); err == nil || p.Valid() {
			t.Fatalf("accepted invalid config %q: %+v", data, p)
		}
	}
}

func FuzzConfig(f *testing.F) {
	for _, seed := range []string{`{"schemaVersion":1}`, `{"schemaVersion":1,"age":{"minimumHours":0}}`, `{"schemaVersion":1,"schemaVersion":1}`, `null`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		p, err := ParseConfig(strings.NewReader(text))
		if err == nil && (!p.Valid() || p.MinimumAgeSeconds() < 0) {
			t.Fatal("invalid policy returned without error")
		}
	})
}
