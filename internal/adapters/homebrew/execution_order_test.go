package homebrew

import (
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

func TestExecutionOrderWaitsForPendingDependencies(t *testing.T) {
	root := domain.Artifact{Name: "jq"}
	dependency := domain.Artifact{Name: "oniguruma"}
	nodes := []domain.Node{
		{Artifact: root, Dependencies: []domain.Artifact{dependency}},
		{Artifact: dependency},
	}
	remaining := map[string]plannedAction{
		"jq":        {Name: "jq", Operation: "install"},
		"oniguruma": {Name: "oniguruma", Operation: "upgrade"},
	}
	first := nextReadyAction(nodes, remaining)
	if first != remaining["oniguruma"] {
		t.Fatalf("dependency must execute before root: got %+v", first)
	}
	delete(remaining, first.Name)
	second := nextReadyAction(nodes, remaining)
	if second != remaining["jq"] {
		t.Fatalf("root must become ready after dependency completes: got %+v", second)
	}
	delete(remaining, second.Name)
	if got := nextReadyAction(nodes, remaining); got != (plannedAction{}) {
		t.Fatalf("completed plan selected another action: %+v", got)
	}
}

func TestExecutionOrderPreservesNodeOrderForIndependentActions(t *testing.T) {
	nodes := []domain.Node{
		{Artifact: domain.Artifact{Name: "xz"}},
		{Artifact: domain.Artifact{Name: "jq"}},
	}
	remaining := map[string]plannedAction{
		"jq": {Name: "jq", Operation: "install"},
		"xz": {Name: "xz", Operation: "install"},
	}
	if got := nextReadyAction(nodes, remaining); got.Name != "xz" {
		t.Fatalf("map enumeration changed execution order: %+v", got)
	}
}

func TestExecutionOrderCannotAdvanceACycle(t *testing.T) {
	root := domain.Artifact{Name: "jq"}
	dependency := domain.Artifact{Name: "oniguruma"}
	nodes := []domain.Node{
		{Artifact: root, Dependencies: []domain.Artifact{dependency}},
		{Artifact: dependency, Dependencies: []domain.Artifact{root}},
	}
	remaining := map[string]plannedAction{
		"jq":        {Name: "jq", Operation: "install"},
		"oniguruma": {Name: "oniguruma", Operation: "install"},
	}
	if got := nextReadyAction(nodes, remaining); got != (plannedAction{}) {
		t.Fatalf("cyclic plan selected an action: %+v", got)
	}
}
