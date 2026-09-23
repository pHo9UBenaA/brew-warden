package domain

import "slices"

type Outcome uint8

const (
	UnassessedOutcome Outcome = iota
	Allow
	Hold
	Deny
)

type Reason struct {
	Code     string
	Artifact Artifact
	Claim    Claim
}

// Decision reports evidence eligibility only. It is never an execution permit.
type Decision struct {
	Outcome Outcome
	Reasons []Reason
	Waived  []Artifact
}

type Node struct {
	Artifact     Artifact
	Dependencies []Artifact
	Evidence     []Evidence
}

// Binding identifies an immutable plan, policy, environment and one attempt.
// Its digests must be recomputed by the owning adapters, never trusted on load.
type Binding struct {
	Plan        Digest
	Policy      Digest
	Graph       Digest
	Environment Digest
	Attempt     Digest
}

func (b Binding) Valid() bool {
	return b.Plan.Valid() && b.Policy.Valid() && b.Graph.Valid() && b.Environment.Valid() && b.Attempt.Valid()
}

type AgeWaiver struct {
	Artifact Artifact
	Reason   string
}

// An exception is explicitly requested by the user and bound to one current
// plan, attempt and expiry. It cannot be replayed from saved process state.
type AgeException struct {
	Binding   Binding
	IssuedAt  int64
	ExpiresAt int64
	Waivers   []AgeWaiver
}

type Assessment struct {
	Policy    Policy
	Binding   Binding
	Targets   []Artifact
	Nodes     []Node
	Now       int64
	Exception *AgeException
	// Supplied from the trusted attempt journal, not serialized authorization.
}

// Evaluate requires the full reachable graph, exactly one current evidence item
// per claim and artifact, and explicit time. It performs no I/O.
func Evaluate(a Assessment) Decision {
	d := Decision{Outcome: Allow}
	add := func(outcome Outcome, code string, artifact Artifact, claim Claim) {
		if outcome > d.Outcome {
			d.Outcome = outcome
		}
		d.Reasons = append(d.Reasons, Reason{code, artifact, claim})
	}
	if !a.Policy.Valid() || a.Now <= 0 || !a.Binding.Valid() {
		add(Hold, "assessment_invalid_or_replayed", Artifact{}, 0)
		return d
	}
	if !validGraph(a.Targets, a.Nodes) {
		add(Hold, "dependency_graph_invalid", Artifact{}, 0)
		return d
	}
	waivers := map[Artifact]bool{}
	if a.Exception != nil {
		e := a.Exception
		if e.Binding != a.Binding || e.IssuedAt <= 0 || e.IssuedAt > a.Now || e.ExpiresAt <= a.Now || len(e.Waivers) == 0 {
			add(Hold, "age_exception_invalid", Artifact{}, Publication)
			return d
		}
		for _, w := range e.Waivers {
			if !ValidAgeReason(w.Reason) || waivers[w.Artifact] || !slices.ContainsFunc(a.Nodes, func(n Node) bool { return n.Artifact == w.Artifact }) {
				add(Hold, "age_exception_invalid", w.Artifact, Publication)
				return d
			}
			waivers[w.Artifact] = true
		}
	}
	for _, node := range a.Nodes {
		byClaim := map[Claim][]Evidence{}
		for _, e := range node.Evidence {
			if e.Subject != node.Artifact || e.Claim < Metadata || e.Claim > Vulnerabilities {
				add(Hold, "evidence_subject_or_claim_mismatch", node.Artifact, e.Claim)
				continue
			}
			byClaim[e.Claim] = append(byClaim[e.Claim], e)
			// A duplicate successful claim must never conceal an integrity failure
			// or a known applicable vulnerability.
			if e.Status == Failed && (e.Claim == Metadata || e.Claim == Checksum || e.Claim == Provenance) {
				add(Deny, "integrity_verification_failed", node.Artifact, e.Claim)
			}
			if e.Claim == Vulnerabilities && e.Status == Verified && e.Applicability == Affected && e.valid() && e.ObservedAt <= a.Now {
				add(Deny, "known_applicable_vulnerability", node.Artifact, e.Claim)
			}
		}
		for claim := Metadata; claim <= Vulnerabilities; claim++ {
			items := byClaim[claim]
			code := ""
			if len(items) != 1 {
				code = "evidence_missing_or_ambiguous"
			} else {
				e := items[0]
				switch {
				case !e.valid() || e.ObservedAt > a.Now || e.ExpiresAt <= a.Now:
					code = "evidence_invalid_or_stale"
				case e.Status != Verified:
					code = "required_evidence_unverified"
				case claim == Vulnerabilities && e.Applicability != NoKnownApplicableFindings:
					code = "vulnerability_applicability_unresolved"
				case claim == Publication:
					if e.Publication != VerifiedAttestation || e.PublishedAt <= 0 || e.PublishedAt > e.ObservedAt {
						code = "publication_unknown_or_conflicting"
					} else if a.Now-e.PublishedAt < a.Policy.MinimumAgeSeconds() {
						code = "release_too_young"
					}
				}
			}
			if code != "" {
				if claim == Publication && code == "release_too_young" && waivers[node.Artifact] {
					d.Waived = append(d.Waived, node.Artifact)
				} else {
					add(Hold, code, node.Artifact, claim)
				}
			}
		}
	}
	return d
}

func validGraph(targets []Artifact, nodes []Node) bool {
	if len(targets) == 0 || len(nodes) == 0 || len(nodes) > 4096 || len(targets) > len(nodes) {
		return false
	}
	index := map[Artifact]Node{}
	names := map[string]bool{}
	for _, n := range nodes {
		if !n.Artifact.Valid() || names[n.Artifact.Name] || len(n.Dependencies) > len(nodes) || len(n.Evidence) > 32 {
			return false
		}
		names[n.Artifact.Name] = true
		index[n.Artifact] = n
	}
	state := map[Artifact]uint8{}
	var visit func(Artifact) bool
	visit = func(id Artifact) bool {
		n, exists := index[id]
		if !exists || state[id] == 1 {
			return false
		}
		if state[id] == 2 {
			return true
		}
		state[id] = 1
		seen := map[Artifact]bool{}
		for _, dep := range n.Dependencies {
			if seen[dep] || !visit(dep) {
				return false
			}
			seen[dep] = true
		}
		state[id] = 2
		return true
	}
	seen := map[Artifact]bool{}
	for _, target := range targets {
		if seen[target] || !visit(target) {
			return false
		}
		seen[target] = true
	}
	return len(state) == len(nodes)
}
