package homebrew

import (
	"encoding/json"
	"testing"
)

func TestBottleMetadataBindsDigestAndCompleteClosure(t *testing.T) {
	candidates := metadataFixture().Formulae
	root := candidates[0]
	root.Revision, root.Rebuild = 0, 0
	candidates[0] = root
	for _, tc := range []struct {
		name         string
		digest       string
		dependencies string
		want         bool
	}{
		{"matching", string(root.BottleSHA256), `[{"full_name":"oniguruma","version":"6.9.9","revision":0}]`, true},
		{"different bottle", "wrong", `[]`, false},
		{"missing dependency", string(root.BottleSHA256), `[]`, false},
		{"additional dependency", string(root.BottleSHA256), `[{"full_name":"unplanned","version":"1","revision":0}]`, false},
		{"duplicate dependency", string(root.BottleSHA256), `[{"full_name":"oniguruma","version":"6.9.9","revision":0},{"full_name":"oniguruma","version":"6.9.9","revision":0}]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tab, _ := json.Marshal(`{"arch":"arm64","runtime_dependencies":` + tc.dependencies + `}`)
			raw := []byte(`{"schemaVersion":2,"manifests":[{"annotations":{"sh.brew.bottle.digest":"` + tc.digest + `","org.opencontainers.image.ref.name":"` + root.Version + `.arm64_tahoe","sh.brew.tab":` + string(tab) + `}}]}`)
			err := validateBottleMetadata(raw, root, candidates)
			if (err == nil) != tc.want {
				t.Fatal("unexpected metadata decision", err)
			}
		})
	}
}

func TestHistoricalBottleCanUseExpandedVerifiedDependencyClosure(t *testing.T) {
	candidates := metadataFixture().Formulae
	root := candidates[0]
	root.Rebuild = 0
	candidates[0] = root
	extra := candidates[1]
	extra.Name = "additional"
	candidates[1].Dependencies = []string{"additional"}
	candidates = append(candidates, extra)
	tab, _ := json.Marshal(`{"arch":"arm64","runtime_dependencies":[{"full_name":"oniguruma","version":"6.9.9","revision":0}]}`)
	raw := []byte(`{"schemaVersion":2,"manifests":[{"annotations":{"sh.brew.bottle.digest":"` + string(root.BottleSHA256) + `","org.opencontainers.image.ref.name":"` + root.Version + `.arm64_tahoe","sh.brew.tab":` + string(tab) + `}}]}`)
	if err := validateBottleMetadata(raw, root, candidates); err != nil {
		t.Fatal(err)
	}
}

func TestAllBottleMayOmitArchitecture(t *testing.T) {
	candidate := metadataFixture().Formulae[1]
	candidate.BottleTag, candidate.Rebuild = "all", 0
	raw := []byte(`{"schemaVersion":2,"manifests":[{"annotations":{"sh.brew.bottle.digest":"` + string(candidate.BottleSHA256) + `","org.opencontainers.image.ref.name":"` + candidate.Version + `.all","sh.brew.tab":"{\"runtime_dependencies\":[]}"}}]}`)
	if err := validateBottleMetadata(raw, candidate, []formulaMetadata{candidate}); err != nil {
		t.Fatal(err)
	}
}
