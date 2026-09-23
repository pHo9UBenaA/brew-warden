package tests

import (
	"fmt"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

// Every required claim of every reachable dependency is necessary, including
// when a valid exception waives only the age of a verified bottle.
func FuzzPolicyRequiresCompleteEvidence(f *testing.F) {
	for _, seed := range [][]byte{
		{0, 0, 0, 0, 0}, {1, 1, 1, 3, 0}, {7, 1, 8, 4, 4}, {5, 0, 3, 2, 2},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 5 || len(data) > 128 {
			return
		}
		a := eligibleAssessment()
		count := 2 + int(data[0]%8)
		for i := 2; i < count; i++ {
			node := a.Nodes[1]
			node.Artifact.Name = fmt.Sprintf("runtime-%d", i)
			node.Dependencies = nil
			node.Evidence = append([]domain.Evidence(nil), node.Evidence...)
			for j := range node.Evidence {
				node.Evidence[j].Subject = node.Artifact
			}
			a.Nodes[i-1].Dependencies = []domain.Artifact{node.Artifact}
			a.Nodes = append(a.Nodes, node)
		}
		if data[1]&1 != 0 {
			waiveYoung(&a)
		}
		if got := domain.Evaluate(a); got.Outcome != domain.Allow {
			t.Fatalf("complete chain of %d formulae was not eligible: %+v", count, got)
		}
		victim := int(data[2]) % len(a.Nodes)
		claim := int(data[3]) % int(domain.Vulnerabilities-domain.Metadata+1)
		evidence := &a.Nodes[victim].Evidence
		switch data[4] % 5 {
		case 0:
			*evidence = append((*evidence)[:claim], (*evidence)[claim+1:]...)
		case 1:
			(*evidence)[claim].Status = domain.Unavailable
		case 2:
			(*evidence)[claim].ExpiresAt = a.Now
		case 3:
			(*evidence)[claim].Subject.Rebuild++
		case 4:
			*evidence = append(*evidence, (*evidence)[claim])
		}
		if got := domain.Evaluate(a); got.Outcome != domain.Hold || len(got.Reasons) == 0 {
			t.Fatalf("missing, stale or ambiguous evidence on node %d claim %d was accepted: %+v", victim, claim, got)
		}
	})
}
