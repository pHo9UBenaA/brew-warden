package homebrew

import "testing"

func TestBottleFilenameDistinguishesRevisionAndRebuild(t *testing.T) {
	for _, tc := range []struct {
		name              string
		revision, rebuild int
		want              string
	}{
		{"original", 0, 0, "jq--1.8.2.arm64_tahoe.bottle.tar.gz"},
		{"revision", 2, 0, "jq--1.8.2_2.arm64_tahoe.bottle.tar.gz"},
		{"rebuild", 0, 3, "jq--1.8.2.arm64_tahoe.bottle.3.tar.gz"},
		{"revised rebuild", 2, 3, "jq--1.8.2_2.arm64_tahoe.bottle.3.tar.gz"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := metadataFixture().Formulae[0]
			candidate.Revision, candidate.Rebuild = tc.revision, tc.rebuild
			if got := bottleName(candidate); got != tc.want {
				t.Fatalf("bottle filename: want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestRevisedRebuiltBottleUsesSameKegInArchiveOCIAndInstalledState(t *testing.T) {
	candidates := metadataFixture().Formulae
	candidate := &candidates[0]
	candidate.Revision, candidate.Rebuild = 2, 3
	archive := bottleFixtureAtVersion(t, "1.8.2_2")
	if err := validateBottleArchive(archive, *candidate); err != nil {
		t.Fatal("revised keg archive rejected", err)
	}
	if err := validateBottleArchive(bottleFixture(t), *candidate); err == nil {
		t.Fatal("unrevised archive accepted for revised bottle")
	}

	const tab = `{"arch":"arm64","runtime_dependencies":[{"full_name":"oniguruma","version":"1.8.2","revision":0}]}`
	index := bottleIndexFixture(t, string(candidate.BottleSHA256), "1.8.2_2.arm64_tahoe.3", tab)
	if err := validateBottleMetadata(index, *candidate, candidates); err != nil {
		t.Fatal("revised rebuild OCI reference rejected", err)
	}
	wrongIndex := bottleIndexFixture(t, string(candidate.BottleSHA256), "1.8.2.arm64_tahoe.3", tab)
	if err := validateBottleMetadata(wrongIndex, *candidate, candidates); err == nil {
		t.Fatal("unrevised OCI reference accepted for revised bottle")
	}

	active := "1.8.2_2"
	state := installedFormula{
		Name: candidate.Name, ActiveVersion: &active,
		Installed: []installedVersion{{Version: active, Poured: true, Built: true}},
	}
	artifact := candidate.artifact()
	if got, err := installedAction(state, artifact, collectionInputs{Operation: "install"}); err != nil || got != "keep" {
		t.Fatalf("rebuild must retain current revised keg: operation=%q artifact=%+v error=%v", got, artifact, err)
	}
	artifact.Rebuild++
	if got, err := installedAction(state, artifact, collectionInputs{Operation: "install"}); err != nil || got != "keep" {
		t.Fatalf("new rebuild changed keg identity: operation=%q error=%v", got, err)
	}
	artifact.Revision++
	state.Outdated = true
	if got, err := installedAction(state, artifact, collectionInputs{Operation: "upgrade"}); err != nil || got != "upgrade" {
		t.Fatalf("new revision must upgrade keg: operation=%q error=%v", got, err)
	}
}
