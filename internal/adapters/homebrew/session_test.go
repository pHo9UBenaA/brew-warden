package homebrew

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

func planFixture() executionPlan {
	a := metadataFixture().Formulae[0].artifact()
	digest := domain.Digest(strings.Repeat("a", 64))
	return executionPlan{
		Schema: 3, MinimumAgeSeconds: 0, Targets: []domain.Artifact{a},
		Nodes:   []domain.Node{{Artifact: a, Dependencies: []domain.Artifact{}, Evidence: []domain.Evidence{}}},
		Actions: []plannedAction{{Name: "jq", Operation: "install"}}, BeforeState: digest,
		Environment: executionEnvironment{Runtime: digest, BrewRevision: brewRevision, OSVersion: "26.6.2", Prefix: "/opt/homebrew"},
		Inputs:      []frozenInput{{Path: "fixture", SHA256: digest}},
		Attempt:     digest, IssuedAt: 100, ExpiresAt: 200, Waivers: []domain.AgeWaiver{},
	}
}
func TestPersistedPlanStrictIdentityAndException(t *testing.T) {
	p := planFixture()
	p.Waivers = []domain.AgeWaiver{{Artifact: p.Targets[0], Reason: "Explicit emergency test"}}
	raw := marshalFixture(t, p)
	var restored executionPlan
	if err := decodeStrict(raw, &restored); err != nil {
		t.Fatal(err)
	}
	prepared, err := restored.prepared(digestBytes(raw))
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Assessment.Exception == nil || prepared.Assessment.Exception.Binding != prepared.Assessment.Binding || !prepared.ExceptionID.Valid() {
		t.Fatal("exception not bound", prepared)
	}
	for name, test := range map[string]struct {
		change    func(*executionPlan)
		component func(domain.Binding) domain.Digest
	}{
		"policy":         {func(p *executionPlan) { p.MinimumAgeSeconds = 42 }, func(b domain.Binding) domain.Digest { return b.Policy }},
		"OS version":     {func(p *executionPlan) { p.Environment.OSVersion = "26.6.3" }, func(b domain.Binding) domain.Digest { return b.Environment }},
		"bottle rebuild": {func(p *executionPlan) { p.Nodes[0].Artifact.Rebuild++ }, func(b domain.Binding) domain.Digest { return b.Graph }},
		"waiver reason":  {change: func(p *executionPlan) { p.Waivers[0].Reason = "Another reason" }},
	} {
		t.Run(name, func(t *testing.T) {
			var changed executionPlan
			if err := decodeStrict(raw, &changed); err != nil {
				t.Fatal(err)
			}
			test.change(&changed)
			// Hold the plan ID fixed so it cannot mask a missing component binding.
			next, err := changed.prepared(prepared.Assessment.Binding.Plan)
			if err != nil {
				t.Fatal(err)
			}
			if test.component != nil && test.component(next.Assessment.Binding) == test.component(prepared.Assessment.Binding) {
				t.Fatalf("changed component reused binding: before=%+v after=%+v", prepared, next)
			}
			if next.ExceptionID == prepared.ExceptionID {
				t.Fatal("changed binding or waiver reused exception identity")
			}
		})
	}
	t.Run("installed state changes serialized plan and exception identity", func(t *testing.T) {
		p.BeforeState = domain.Digest(strings.Repeat("b", 64))
		changedID := digestBytes(marshalFixture(t, p))
		if changedID == prepared.Assessment.Binding.Plan {
			t.Fatal("installed state omitted from serialized plan identity")
		}
		next, err := p.prepared(changedID)
		if err != nil || next.Assessment.Binding.Plan != changedID || next.ExceptionID == prepared.ExceptionID {
			t.Fatalf("changed plan ID did not bind the exception: %+v: %v", next, err)
		}
	})
	for name, bad := range map[string]string{
		"missing revision":      replaceFixtureText(t, string(raw), `"Revision":0,`, ""),
		"mis-cased revision":    replaceFixtureText(t, string(raw), `"Revision":0`, `"revision":0`),
		"duplicate minimum age": replaceFixtureText(t, string(raw), `"minimumAge":0`, `"minimumAge":0,"minimumAge":1`),
	} {
		t.Run(name, func(t *testing.T) {
			if err := decodeStrict([]byte(bad), &executionPlan{}); err == nil {
				t.Fatal("ambiguous or incomplete plan accepted")
			}
		})
	}
}
func TestExecutionPlanBindsReviewedHomebrewRevision(t *testing.T) {
	plan := planFixture()
	for _, schema := range []int{1, 2, 4} {
		legacy := plan
		legacy.Schema = schema
		if _, err := legacy.prepared(plan.BeforeState); err == nil {
			t.Fatalf("accepted obsolete or unknown schema %d", schema)
		}
	}
	for _, revision := range []string{"", strings.Repeat("0", 40)} {
		plan.Environment.BrewRevision = revision
		if _, err := plan.prepared(plan.BeforeState); err == nil {
			t.Fatal("unreviewed Homebrew revision authorized an execution plan")
		}
	}
	plan.Environment.BrewRevision = brewRevision
	if _, err := plan.prepared(plan.BeforeState); err != nil {
		t.Fatal("reviewed Homebrew revision could not bind a plan", err)
	}
}

func TestSavedPlanAndFrozenInputRevalidation(t *testing.T) {
	root := t.TempDir()
	p := planFixture()
	state := []byte("observed installed state")
	p.BeforeState = digestBytes(state)
	if err := os.Mkdir(filepath.Join(root, "states"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeNew(filepath.Join(root, "states", string(p.BeforeState)+".json"), state, 0600); err != nil {
		t.Fatal(err)
	}
	data := []byte("authenticated candidate bytes")
	p.Inputs = []frozenInput{{"bottle", digestBytes(data)}}
	if err := writeNew(filepath.Join(root, "bottle"), data, 0600); err != nil {
		t.Fatal(err)
	}
	raw := marshalFixture(t, p)
	if err := writeNew(filepath.Join(root, "plan.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	prepared, err := p.prepared(digestBytes(raw))
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspace{root}
	if _, err := workspace.readPrepared(prepared.Assessment.Binding.Plan); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bottle"), []byte("substitution"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.readPrepared(prepared.Assessment.Binding.Plan); err == nil {
		t.Fatal("changed input accepted")
	}
	if err := os.WriteFile(filepath.Join(root, "bottle"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plan.json"), append(raw, ' '), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.readPrepared(prepared.Assessment.Binding.Plan); err == nil {
		t.Fatal("changed exact plan bytes accepted")
	}
}
