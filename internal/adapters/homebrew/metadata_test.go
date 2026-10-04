package homebrew

import (
	"strings"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

func metadataFixture() metadataDocument {
	digest := domain.Digest(strings.Repeat("a", 64))
	root := formulaMetadata{
		BottleTag: "arm64_tahoe", Name: "jq", Version: "1.8.2", Rebuild: 1,
		BottleURL:    "https://ghcr.io/v2/homebrew/core/jq/blobs/sha256:" + string(digest),
		BottleSHA256: digest, Cellar: ":any", Dependencies: []string{"oniguruma"},
	}
	dependency := root
	dependency.Name = "oniguruma"
	dependency.BottleURL = strings.Replace(root.BottleURL, "/jq/", "/oniguruma/", 1)
	dependency.Dependencies = []string{}
	return metadataDocument{Schema: 1, Platform: "arm64_tahoe", Formulae: []formulaMetadata{root, dependency}}
}

func TestMetadataClosureAndIdentity(t *testing.T) {
	doc := metadataFixture()
	raw := marshalFixture(t, doc)
	if candidates, err := parseMetadata(raw, []string{"jq"}); err != nil || len(candidates) != 2 {
		t.Fatalf("want two candidates in complete jq closure: candidates=%+v error=%v", candidates, err)
	}
	for name, mutate := range map[string]func(*metadataDocument){
		"missing dependency":    func(d *metadataDocument) { d.Formulae = d.Formulae[:1] },
		"dependency cycle":      func(d *metadataDocument) { d.Formulae[1].Dependencies = []string{"jq"} },
		"wrong platform bottle": func(d *metadataDocument) { d.Formulae[0].BottleTag = "arm64_linux" },
		"untrusted bottle host": func(d *metadataDocument) { d.Formulae[0].BottleURL = "https://attacker.invalid/bottle" },
		"missing digest":        func(d *metadataDocument) { d.Formulae[0].BottleSHA256 = "" },
		"null dependencies":     func(d *metadataDocument) { d.Formulae[0].Dependencies = nil },
		"unreachable formula":   func(d *metadataDocument) { d.Formulae[0].Dependencies = []string{} },
		"duplicate formula":     func(d *metadataDocument) { d.Formulae = append(d.Formulae, d.Formulae[0]) },
		"unsupported platform":  func(d *metadataDocument) { d.Platform = "tahoe" },
	} {
		t.Run(name, func(t *testing.T) {
			doc := metadataFixture()
			mutate(&doc)
			raw := marshalFixture(t, doc)
			if _, err := parseMetadata(raw, []string{"jq"}); err == nil {
				t.Fatal("invalid candidate metadata accepted", string(raw))
			}
		})
	}
	for name, invalid := range map[string]string{
		"missing revision": replaceFixtureText(t, string(raw), `"revision":0,`, ""),
		"mis-cased schema": replaceFixtureText(t, string(raw), `"schema":1`, `"Schema":1`),
		"duplicate schema": replaceFixtureText(t, string(raw), `"schema":1`, `"schema":1,"schema":1`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseMetadata([]byte(invalid), []string{"jq"}); err == nil {
				t.Fatal("ambiguous or incomplete metadata accepted")
			}
		})
	}
}

func FuzzNativeMetadata(f *testing.F) {
	raw := marshalFixture(f, metadataFixture())
	f.Add(string(raw))
	f.Add(`{}`)
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > maxManifest {
			return
		}
		_, _ = parseMetadata([]byte(s), []string{"jq"})
	})
}

func TestBottleCellarMustMatchExecutionPrefix(t *testing.T) {
	for _, cellar := range []string{":any", ":any_skip_relocation", "/opt/homebrew/Cellar", "/usr/local/Cellar", "/tmp/Cellar"} {
		doc := metadataFixture()
		doc.Formulae[0].Cellar = cellar
		raw := marshalFixture(t, doc)
		_, err := parseMetadata(raw, []string{"jq"})
		want := cellar == ":any" || cellar == ":any_skip_relocation" || cellar == "/opt/homebrew/Cellar"
		if (err == nil) != want {
			t.Fatalf("cellar %q: want supported=%t, got error=%v", cellar, want, err)
		}
	}
}
