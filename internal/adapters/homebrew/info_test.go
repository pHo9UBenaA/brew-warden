package homebrew

import (
	"strings"
	"testing"
)

// Captured public info output, trimmed to selected fields and an irrelevant null.
const infoFixture = `{"formulae":[{"name":"jq","full_name":"jq","tap":"homebrew/core","versions":{"stable":"1.8.2","head":"HEAD","bottle":true},"urls":{"stable":{"url":"https://github.com/jqlang/jq/releases/download/jq-1.8.2/jq-1.8.2.tar.gz","tag":null,"revision":null,"using":null,"checksum":"71b8d6e8f5fe81f6c6d0d110e3892251f6ce76ed095abd315e26e6e1193af3af"},"head":{"url":"https://github.com/jqlang/jq.git","branch":"master","using":null}},"revision":0,"bottle":{"stable":{"rebuild":1,"root_url":"https://ghcr.io/v2/homebrew/core","files":{"arm64_tahoe":{"cellar":":any","url":"https://ghcr.io/v2/homebrew/core/jq/blobs/sha256:ca67c64d0aaf1e5472790ec2cc081ff7972316f27095d8a8aab81b3321247036","sha256":"ca67c64d0aaf1e5472790ec2cc081ff7972316f27095d8a8aab81b3321247036"}}}},"dependencies":["oniguruma"],"build_dependencies":[],"tap_git_head":"65464f0db62b68517e0b6509173d405b7398b095","ruby_source_path":"Formula/j/jq.rb","ruby_source_checksum":{"sha256":"0081a3a8d8afaa165b4bfa9b222723916b56f4d04b975d7aad35e8cdad0b0296"},"desc":null}],"casks":[]}`

func TestInfoRejectsMissingOrAmbiguousEvidence(t *testing.T) {
	f, err := parseInfo([]byte(infoFixture), []string{"jq"})
	if err != nil || len(f) != 1 || f[0].Dependencies[0] != "oniguruma" || f[0].Rebuild != 1 || !f[0].BottleSHA256.Valid() {
		t.Fatal(f, err)
	}
	// Source hosts and recipe paths are not bottle eligibility inputs. A
	// different project layout must follow the same selected bottle path.
	unrelatedSource := strings.Replace(infoFixture, "https://github.com/jqlang/jq/releases/download/jq-1.8.2/jq-1.8.2.tar.gz", "https://downloads.example.org/archive", 1)
	unrelatedSource = strings.Replace(unrelatedSource, `"ruby_source_path":"Formula/j/jq.rb"`, `"ruby_source_path":"Formula/other/layout.rb"`, 1)
	other, err := parseInfo([]byte(unrelatedSource), []string{"jq"})
	if err != nil || len(other) != 1 || other[0].artifact() != f[0].artifact() || other[0].Dependencies[0] != f[0].Dependencies[0] {
		t.Fatal("source-specific metadata changed bottle selection", err)
	}
	for name, raw := range map[string]string{
		"missing revision":     strings.Replace(infoFixture, `"revision":0,`, "", 1),
		"missing dependencies": strings.Replace(infoFixture, `"dependencies":["oniguruma"],`, "", 1),
		"null dependencies":    strings.Replace(infoFixture, `"dependencies":["oniguruma"]`, `"dependencies":null`, 1),
		"duplicate name":       strings.Replace(infoFixture, `"name":"jq"`, `"name":"evil","name":"jq"`, 1),
		"ambiguous case":       strings.Replace(infoFixture, `"revision":0`, `"Revision":0`, 1),
		"nested case":          strings.Replace(infoFixture, `"sha256":"ca67c64`, `"SHA256":"ca67c64`, 1),
		"wrong tap":            strings.Replace(infoFixture, `"tap":"homebrew/core"`, `"tap":"other/core"`, 1),
		"wrong identity":       strings.Replace(infoFixture, `"full_name":"jq"`, `"full_name":"other/jq"`, 1),
		"cask result":          strings.Replace(infoFixture, `"casks":[]`, `"casks":["jq"]`, 1),
		"trailing JSON":        infoFixture + `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseInfo([]byte(raw), []string{"jq"}); err == nil {
				t.Fatal("incomplete or ambiguous evidence accepted")
			}
		})
	}
	if _, err := parseInfo([]byte(infoFixture), []string{"jq", "oniguruma"}); err == nil {
		t.Fatal("missing result accepted")
	}
	if _, err := parseInfo([]byte(infoFixture), []string{"oniguruma"}); err == nil {
		t.Fatal("unrequested formula accepted")
	}
}
