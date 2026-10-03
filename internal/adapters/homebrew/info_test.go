package homebrew

import (
	"slices"
	"testing"
)

// Captured public info output, trimmed to selected fields and an irrelevant null.
const infoFixture = `{"formulae":[{
	"name":"jq","full_name":"jq","tap":"homebrew/core",
	"versions":{"stable":"1.8.2","head":"HEAD","bottle":true},
	"urls":{
		"stable":{
			"url":"https://github.com/jqlang/jq/releases/download/jq-1.8.2/jq-1.8.2.tar.gz",
			"tag":null,"revision":null,"using":null,
			"checksum":"71b8d6e8f5fe81f6c6d0d110e3892251f6ce76ed095abd315e26e6e1193af3af"
		},
		"head":{"url":"https://github.com/jqlang/jq.git","branch":"master","using":null}
	},
	"revision":0,
	"bottle":{"stable":{
		"rebuild":1,"root_url":"https://ghcr.io/v2/homebrew/core",
		"files":{"arm64_tahoe":{
			"cellar":":any",
			"url":"https://ghcr.io/v2/homebrew/core/jq/blobs/sha256:ca67c64d0aaf1e5472790ec2cc081ff7972316f27095d8a8aab81b3321247036",
			"sha256":"ca67c64d0aaf1e5472790ec2cc081ff7972316f27095d8a8aab81b3321247036"
		}}
	}},
	"dependencies":["oniguruma"],"build_dependencies":[],
	"tap_git_head":"65464f0db62b68517e0b6509173d405b7398b095",
	"ruby_source_path":"Formula/j/jq.rb",
	"ruby_source_checksum":{"sha256":"0081a3a8d8afaa165b4bfa9b222723916b56f4d04b975d7aad35e8cdad0b0296"},
	"desc":null
}],"casks":[]}`

func TestInfoSelectsRebuiltBottleAndDependencies(t *testing.T) {
	candidates, err := parseInfo([]byte(infoFixture), []string{"jq"})
	if err != nil || len(candidates) != 1 {
		t.Fatalf("want one jq candidate: candidates=%+v error=%v", candidates, err)
	}
	candidate := candidates[0]
	if !slices.Equal(candidate.Dependencies, []string{"oniguruma"}) {
		t.Fatalf("dependencies=%v, want [oniguruma]", candidate.Dependencies)
	}
	if candidate.Rebuild != 1 || !candidate.BottleSHA256.Valid() {
		t.Fatalf("want rebuild 1 with a valid bottle digest: candidate=%+v", candidate)
	}
}

func TestInfoBottleSelectionIgnoresSourceLayout(t *testing.T) {
	baseline, err := parseInfo([]byte(infoFixture), []string{"jq"})
	if err != nil || len(baseline) != 1 {
		t.Fatalf("cannot parse baseline: candidates=%+v error=%v", baseline, err)
	}
	// Source hosts and recipe paths are not bottle eligibility inputs. A
	// different project layout must follow the same selected bottle path.
	unrelatedSource := replaceFixtureText(t, infoFixture, "https://github.com/jqlang/jq/releases/download/jq-1.8.2/jq-1.8.2.tar.gz", "https://downloads.example.org/archive")
	unrelatedSource = replaceFixtureText(t, unrelatedSource, `"ruby_source_path":"Formula/j/jq.rb"`, `"ruby_source_path":"Formula/other/layout.rb"`)
	other, err := parseInfo([]byte(unrelatedSource), []string{"jq"})
	if err != nil || len(other) != 1 {
		t.Fatalf("cannot parse changed source layout: candidates=%+v error=%v", other, err)
	}
	if other[0].artifact() != baseline[0].artifact() || !slices.Equal(other[0].Dependencies, baseline[0].Dependencies) {
		t.Fatalf("source-specific metadata changed bottle selection: got=%+v want=%+v", other, baseline)
	}
}

func TestInfoRejectsMissingOrAmbiguousEvidence(t *testing.T) {
	for name, raw := range map[string]string{
		"missing revision":     replaceFixtureText(t, infoFixture, `"revision":0,`, ""),
		"missing dependencies": replaceFixtureText(t, infoFixture, `"dependencies":["oniguruma"],`, ""),
		"null dependencies":    replaceFixtureText(t, infoFixture, `"dependencies":["oniguruma"]`, `"dependencies":null`),
		"duplicate name":       replaceFixtureText(t, infoFixture, `"name":"jq"`, `"name":"evil","name":"jq"`),
		"ambiguous case":       replaceFixtureText(t, infoFixture, `"revision":0`, `"Revision":0`),
		"nested case":          replaceFixtureText(t, infoFixture, `"sha256":"ca67c64`, `"SHA256":"ca67c64`),
		"wrong tap":            replaceFixtureText(t, infoFixture, `"tap":"homebrew/core"`, `"tap":"other/core"`),
		"wrong identity":       replaceFixtureText(t, infoFixture, `"full_name":"jq"`, `"full_name":"other/jq"`),
		"cask result":          replaceFixtureText(t, infoFixture, `"casks":[]`, `"casks":["jq"]`),
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
