package homebrew

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

func planFixture() executionPlan {
	a := metadataFixture().Formulae[0].artifact()
	digest := domain.Digest(strings.Repeat("a", 64))
	return executionPlan{Schema: 1, MinimumAge: 0, Targets: []domain.Artifact{a}, Nodes: []domain.Node{{Artifact: a, Dependencies: []domain.Artifact{}, Evidence: []domain.Evidence{}}}, Actions: []plannedAction{{"jq", "install"}}, BeforeState: digest, Environment: executionEnvironment{digest, "26.6.2", "/opt/homebrew"}, Inputs: []frozenInput{{"fixture", digest}}, Attempt: digest, IssuedAt: 100, ExpiresAt: 200, Waivers: []domain.AgeWaiver{}}
}
func TestPersistedPlanStrictIdentityAndException(t *testing.T) {
	p := planFixture()
	p.Waivers = []domain.AgeWaiver{{Artifact: p.Targets[0], Reason: "Explicit emergency test"}}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
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
	for _, change := range []func(*executionPlan){
		func(p *executionPlan) { p.MinimumAge = 42 }, func(p *executionPlan) { p.Environment.OSVersion = "26.6.3" }, func(p *executionPlan) { p.Nodes[0].Artifact.Rebuild++ }, func(p *executionPlan) { p.Waivers[0].Reason = "Another reason" }, func(p *executionPlan) { p.BeforeState = domain.Digest(strings.Repeat("b", 64)) },
	} {
		var changed executionPlan
		if err := decodeStrict(raw, &changed); err != nil {
			t.Fatal(err)
		}
		change(&changed)
		modified, _ := json.Marshal(changed)
		next, err := changed.prepared(digestBytes(modified))
		if err != nil {
			t.Fatal(err)
		}
		if next.Assessment.Binding == prepared.Assessment.Binding || next.ExceptionID == prepared.ExceptionID {
			t.Fatal("changed plan reused binding")
		}
	}
	for _, bad := range []string{strings.Replace(string(raw), `"Revision":0,`, "", 1), strings.Replace(string(raw), `"Revision":0`, `"revision":0`, 1), strings.Replace(string(raw), `"minimumAge":0`, `"minimumAge":0,"minimumAge":1`, 1)} {
		if err := decodeStrict([]byte(bad), &executionPlan{}); err == nil {
			t.Fatal("ambiguous or incomplete plan accepted")
		}
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
	raw, _ := json.Marshal(p)
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
