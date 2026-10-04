package domain

import "slices"

type Outcome uint8

// Assessed outcomes are ordered by restriction: Evaluate retains the strongest.
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

// AgeException is explicitly requested by the user and bound to one current
// plan, attempt and expiry. It cannot be replayed from saved process state.
// IssuedAt and ExpiresAt are Unix seconds; validity is [IssuedAt, ExpiresAt).
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
}

// Evaluate requires the full reachable graph, exactly one current evidence item
// per claim and artifact, and explicit time. It performs no I/O.
func Evaluate(assessment Assessment) Decision {
	decision := Decision{Outcome: Allow}
	addReason := func(outcome Outcome, code string, artifact Artifact, claim Claim) {
		if outcome > decision.Outcome {
			decision.Outcome = outcome
		}
		decision.Reasons = append(decision.Reasons, Reason{Code: code, Artifact: artifact, Claim: claim})
	}
	if !assessment.Policy.Valid() || assessment.Now <= 0 || !assessment.Binding.Valid() {
		addReason(Hold, "assessment_invalid_or_replayed", Artifact{}, 0)
		return decision
	}
	if !validGraph(assessment.Targets, assessment.Nodes) {
		addReason(Hold, "dependency_graph_invalid", Artifact{}, 0)
		return decision
	}
	waivers := map[Artifact]bool{}
	if exception := assessment.Exception; exception != nil {
		validInterval := exception.IssuedAt > 0 && exception.IssuedAt <= assessment.Now && exception.ExpiresAt > assessment.Now
		if exception.Binding != assessment.Binding || !validInterval || len(exception.Waivers) == 0 {
			addReason(Hold, "age_exception_invalid", Artifact{}, Publication)
			return decision
		}
		for _, waiver := range exception.Waivers {
			inPlan := slices.ContainsFunc(assessment.Nodes, func(node Node) bool { return node.Artifact == waiver.Artifact })
			if !ValidAgeReason(waiver.Reason) || waivers[waiver.Artifact] || !inPlan {
				addReason(Hold, "age_exception_invalid", waiver.Artifact, Publication)
				return decision
			}
			waivers[waiver.Artifact] = true
		}
	}
	for _, node := range assessment.Nodes {
		byClaim := map[Claim][]Evidence{}
		for _, evidence := range node.Evidence {
			if evidence.Subject != node.Artifact || evidence.Claim < Metadata || evidence.Claim > Vulnerabilities {
				addReason(Hold, "evidence_subject_or_claim_mismatch", node.Artifact, evidence.Claim)
				continue
			}
			byClaim[evidence.Claim] = append(byClaim[evidence.Claim], evidence)
			// A duplicate successful claim must never conceal an integrity failure
			// or a known applicable vulnerability.
			integrityClaim := evidence.Claim == Metadata || evidence.Claim == Checksum || evidence.Claim == Provenance
			if evidence.Status == Failed && integrityClaim {
				addReason(Deny, "integrity_verification_failed", node.Artifact, evidence.Claim)
			}
			knownAffected := evidence.Claim == Vulnerabilities && evidence.Status == Verified && evidence.Applicability == Affected
			if knownAffected && evidence.valid() && evidence.ObservedAt <= assessment.Now {
				addReason(Deny, "known_applicable_vulnerability", node.Artifact, evidence.Claim)
			}
		}
		for claim := Metadata; claim <= Vulnerabilities; claim++ {
			code := requiredEvidenceReason(byClaim[claim], claim, assessment.Policy, assessment.Now)
			if code == "" {
				continue
			}
			if claim == Publication && code == "release_too_young" && waivers[node.Artifact] {
				decision.Waived = append(decision.Waived, node.Artifact)
			} else {
				addReason(Hold, code, node.Artifact, claim)
			}
		}
	}
	return decision
}

// requiredEvidenceReason returns an empty code only for one current, verified
// claim. Denials from any duplicate evidence are handled separately by Evaluate.
func requiredEvidenceReason(items []Evidence, claim Claim, policy Policy, now int64) string {
	if len(items) != 1 {
		return "evidence_missing_or_ambiguous"
	}
	evidence := items[0]
	switch {
	case !evidence.valid() || evidence.ObservedAt > now || evidence.ExpiresAt <= now:
		return "evidence_invalid_or_stale"
	case evidence.Status != Verified:
		return "required_evidence_unverified"
	case claim == Vulnerabilities && evidence.Applicability != NoKnownApplicableFindings:
		return "vulnerability_applicability_unresolved"
	case claim == Publication:
		if evidence.Publication != VerifiedAttestation || evidence.PublishedAt <= 0 || evidence.PublishedAt > evidence.ObservedAt {
			return "publication_unknown_or_conflicting"
		}
		if now-evidence.PublishedAt < policy.MinimumAgeSeconds() {
			return "release_too_young"
		}
	}
	return ""
}

func validGraph(targets []Artifact, nodes []Node) bool {
	if len(targets) == 0 || len(nodes) == 0 || len(nodes) > 4096 || len(targets) > len(nodes) {
		return false
	}
	nodesByArtifact := map[Artifact]Node{}
	seenNames := map[string]bool{}
	for _, node := range nodes {
		if !node.Artifact.Valid() || seenNames[node.Artifact.Name] || len(node.Dependencies) > len(nodes) || len(node.Evidence) > 32 {
			return false
		}
		seenNames[node.Artifact.Name] = true
		nodesByArtifact[node.Artifact] = node
	}
	const (
		visiting uint8 = iota + 1
		visited
	)
	visitState := map[Artifact]uint8{}
	var visit func(Artifact) bool
	visit = func(artifact Artifact) bool {
		node, exists := nodesByArtifact[artifact]
		if !exists || visitState[artifact] == visiting {
			return false
		}
		if visitState[artifact] == visited {
			return true
		}
		visitState[artifact] = visiting
		seenDependencies := map[Artifact]bool{}
		for _, dependency := range node.Dependencies {
			if seenDependencies[dependency] || !visit(dependency) {
				return false
			}
			seenDependencies[dependency] = true
		}
		visitState[artifact] = visited
		return true
	}

	seenTargets := map[Artifact]bool{}
	for _, target := range targets {
		if seenTargets[target] || !visit(target) {
			return false
		}
		seenTargets[target] = true
	}
	return len(visitState) == len(nodes)
}
