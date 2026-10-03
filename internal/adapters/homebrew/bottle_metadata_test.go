package homebrew

import "testing"

func bottleIndexFixture(t *testing.T, digest, ref, tab string) []byte {
	t.Helper()
	encodedTab := marshalFixture(t, tab)
	return []byte(`{"schemaVersion":2,"manifests":[{"annotations":{
		"sh.brew.bottle.digest":"` + digest + `",
		"org.opencontainers.image.ref.name":"` + ref + `",
		"sh.brew.tab":` + string(encodedTab) + `
	}}]}`)
}

func TestBottleMetadataBindsDigestAndCompleteClosure(t *testing.T) {
	candidates := metadataFixture().Formulae
	root := candidates[0]
	root.Revision, root.Rebuild = 0, 0
	candidates[0] = root
	const historicalDependency = `{"full_name":"oniguruma","version":"6.9.9","revision":0}`
	for _, tc := range []struct {
		name         string
		digest       string
		dependencies string
		want         bool
	}{
		{"matching", string(root.BottleSHA256), "[" + historicalDependency + "]", true},
		{"different bottle", "wrong", `[]`, false},
		{"missing dependency", string(root.BottleSHA256), `[]`, false},
		{"additional dependency", string(root.BottleSHA256), `[{"full_name":"unplanned","version":"1","revision":0}]`, false},
		{"duplicate dependency", string(root.BottleSHA256), "[" + historicalDependency + "," + historicalDependency + "]", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := bottleIndexFixture(t, tc.digest, root.Version+".arm64_tahoe",
				`{"arch":"arm64","runtime_dependencies":`+tc.dependencies+`}`)
			err := validateBottleMetadata(raw, root, candidates)
			if (err == nil) != tc.want {
				t.Fatalf("want metadata success=%t for dependencies %s, got error=%v", tc.want, tc.dependencies, err)
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
	raw := bottleIndexFixture(t, string(root.BottleSHA256), root.Version+".arm64_tahoe",
		`{"arch":"arm64","runtime_dependencies":[{"full_name":"oniguruma","version":"6.9.9","revision":0}]}`)
	if err := validateBottleMetadata(raw, root, candidates); err != nil {
		t.Fatal(err)
	}
}

func TestAllBottleMayOmitArchitecture(t *testing.T) {
	candidate := metadataFixture().Formulae[1]
	candidate.BottleTag, candidate.Rebuild = "all", 0
	raw := bottleIndexFixture(t, string(candidate.BottleSHA256), candidate.Version+".all",
		`{"runtime_dependencies":[]}`)
	if err := validateBottleMetadata(raw, candidate, []formulaMetadata{candidate}); err != nil {
		t.Fatal(err)
	}
}
