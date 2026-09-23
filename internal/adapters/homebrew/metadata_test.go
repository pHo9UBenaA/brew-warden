package homebrew

import (
	"encoding/json"
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"strings"
	"testing"
)

func metadataFixture() metadataDocument {
	digest := domain.Digest(strings.Repeat("a", 64))
	f := formulaMetadata{BottleTag: "arm64_tahoe", Name: "jq", Version: "1.8.2", Rebuild: 1, BottleURL: "https://ghcr.io/v2/homebrew/core/jq/blobs/sha256:" + string(digest), BottleSHA256: digest, Cellar: ":any", Dependencies: []string{"oniguruma"}}
	dep := f
	dep.Name = "oniguruma"
	dep.BottleURL = strings.Replace(f.BottleURL, "/jq/", "/oniguruma/", 1)
	dep.Dependencies = []string{}
	return metadataDocument{1, "arm64_tahoe", []formulaMetadata{f, dep}}
}
func TestMetadataClosureAndIdentity(t *testing.T) {
	doc := metadataFixture()
	raw, _ := json.Marshal(doc)
	if recipes, candidates, err := parseMetadata(raw, []string{"jq"}); err != nil || len(recipes) != 2 || len(candidates) != 2 {
		t.Fatal(recipes, candidates, err)
	}
	for _, mutate := range []func(*metadataDocument){
		func(d *metadataDocument) { d.Formulae = d.Formulae[:1] },
		func(d *metadataDocument) { d.Formulae[1].Dependencies = []string{"jq"} },
		func(d *metadataDocument) { d.Formulae[0].BottleTag = "arm64_linux" },
		func(d *metadataDocument) { d.Formulae[0].BottleURL = "https://attacker.invalid/bottle" },
		func(d *metadataDocument) { d.Formulae[0].BottleSHA256 = "" },
		func(d *metadataDocument) { d.Formulae[0].Dependencies = nil },
		func(d *metadataDocument) { d.Formulae[0].Dependencies = []string{} },
		func(d *metadataDocument) { d.Formulae = append(d.Formulae, d.Formulae[0]) },
		func(d *metadataDocument) { d.Platform = "tahoe" },
	} {
		doc := metadataFixture()
		mutate(&doc)
		raw, _ := json.Marshal(doc)
		if _, _, err := parseMetadata(raw, []string{"jq"}); err == nil {
			t.Fatal("invalid candidate metadata accepted", string(raw))
		}
	}
	for _, raw := range []string{strings.Replace(string(raw), `"revision":0,`, "", 1), strings.Replace(string(raw), `"schema":1`, `"Schema":1`, 1), strings.Replace(string(raw), `"schema":1`, `"schema":1,"schema":1`, 1)} {
		if _, _, err := parseMetadata([]byte(raw), []string{"jq"}); err == nil {
			t.Fatal("ambiguous or incomplete metadata accepted")
		}
	}
}
func FuzzNativeMetadata(f *testing.F) {
	raw, _ := json.Marshal(metadataFixture())
	f.Add(string(raw))
	f.Add(`{}`)
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > maxManifest {
			return
		}
		_, _, _ = parseMetadata([]byte(s), []string{"jq"})
	})
}

func TestBottleCellarMustMatchExecutionPrefix(t *testing.T) {
	for _, cellar := range []string{":any", ":any_skip_relocation", "/opt/homebrew/Cellar", "/usr/local/Cellar", "/tmp/Cellar"} {
		doc := metadataFixture()
		doc.Formulae[0].Cellar = cellar
		raw, _ := json.Marshal(doc)
		_, _, err := parseMetadata(raw, []string{"jq"})
		want := cellar == ":any" || cellar == ":any_skip_relocation" || cellar == "/opt/homebrew/Cellar"
		if (err == nil) != want {
			t.Fatalf("cellar %q: %v", cellar, err)
		}
	}
}
