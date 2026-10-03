package tests

import (
	"slices"
	"strings"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

func eligibleAssessment() domain.Assessment {
	const now = int64(2000000)
	digest := domain.Digest(strings.Repeat("a", 64))
	root := domain.Artifact{Tap: "homebrew/core", Name: "wget", Version: "1.0", OS: "macos", Arch: "arm64", BottleTag: "arm64_tahoe", SHA256: digest}
	dependency := root
	dependency.Name = "openssl@3"
	nodes := []domain.Node{{Artifact: root, Dependencies: []domain.Artifact{dependency}}, {Artifact: dependency}}
	for i := range nodes {
		for claim := domain.Metadata; claim <= domain.Vulnerabilities; claim++ {
			evidence, err := domain.NewEvidence(domain.Evidence{
				Claim: claim, Subject: nodes[i].Artifact, Status: domain.Verified,
				Provider: domain.Homebrew, Source: "fixture-provider", ProviderVersion: "test-1",
				RawSHA256: digest, ObservedAt: now - 60, ExpiresAt: now + 60,
				PublishedAt: now - 10*86400, Publication: domain.VerifiedAttestation,
				Applicability: domain.NoKnownApplicableFindings,
			})
			if err != nil {
				panic(err)
			}
			nodes[i].Evidence = append(nodes[i].Evidence, evidence)
		}
	}
	return domain.Assessment{
		Policy:  domain.DefaultPolicy(),
		Binding: domain.Binding{Plan: digest, Policy: digest, Graph: digest, Environment: digest, Attempt: digest},
		Targets: []domain.Artifact{root}, Nodes: nodes, Now: now,
	}
}

// Fixture mutations identify claims by meaning rather than their slice position.
func evidenceFor(node *domain.Node, claim domain.Claim) *domain.Evidence {
	for i := range node.Evidence {
		if node.Evidence[i].Claim == claim {
			return &node.Evidence[i]
		}
	}
	panic("required claim missing from assessment fixture")
}

func waiveYoung(a *domain.Assessment) {
	a.Exception = &domain.AgeException{Binding: a.Binding, IssuedAt: a.Now - 1, ExpiresAt: a.Now + 60}
	for i := range a.Nodes {
		evidenceFor(&a.Nodes[i], domain.Publication).PublishedAt = a.Now - 120
		a.Exception.Waivers = append(a.Exception.Waivers, domain.AgeWaiver{Artifact: a.Nodes[i].Artifact, Reason: "Explicit incident response"})
	}
}

func TestEvidenceDecisions(t *testing.T) {
	tests := []struct {
		name string
		edit func(*domain.Assessment)
		want domain.Outcome
	}{
		{"old attestation on first observation", func(*domain.Assessment) {}, domain.Allow},
		{"young dependency", func(a *domain.Assessment) { evidenceFor(&a.Nodes[1], domain.Publication).PublishedAt = a.Now - 120 }, domain.Hold},
		{"missing publication", func(a *domain.Assessment) { evidenceFor(&a.Nodes[1], domain.Publication).PublishedAt = 0 }, domain.Hold},
		{"future publication", func(a *domain.Assessment) { evidenceFor(&a.Nodes[0], domain.Publication).PublishedAt = a.Now + 1 }, domain.Hold},
		{"observed before publication", func(a *domain.Assessment) { evidenceFor(&a.Nodes[0], domain.Publication).PublishedAt = a.Now - 1 }, domain.Hold},
		{"legacy registration cannot age a bottle", func(a *domain.Assessment) {
			evidenceFor(&a.Nodes[0], domain.Publication).Publication = domain.BottleRegistration
		}, domain.Hold},
		{"unsupported publication event", func(a *domain.Assessment) { evidenceFor(&a.Nodes[0], domain.Publication).Publication = 99 }, domain.Hold},
		{"unknown advisory applicability", func(a *domain.Assessment) {
			evidenceFor(&a.Nodes[1], domain.Vulnerabilities).Applicability = domain.Unknown
		}, domain.Hold},
		{"known dependency vulnerability", func(a *domain.Assessment) {
			evidenceFor(&a.Nodes[1], domain.Vulnerabilities).Applicability = domain.Affected
		}, domain.Deny},
		{"stale clean advisory response", func(a *domain.Assessment) { evidenceFor(&a.Nodes[1], domain.Vulnerabilities).ExpiresAt = a.Now }, domain.Hold},
		{"missing provenance", func(a *domain.Assessment) {
			a.Nodes[0].Evidence = slices.DeleteFunc(a.Nodes[0].Evidence, func(e domain.Evidence) bool { return e.Claim == domain.Provenance })
		}, domain.Hold},
		{"duplicate provenance", func(a *domain.Assessment) {
			a.Nodes[0].Evidence = append(a.Nodes[0].Evidence, *evidenceFor(&a.Nodes[0], domain.Provenance))
		}, domain.Hold},
		{"duplicate conceals failed signature", func(a *domain.Assessment) {
			failed := *evidenceFor(&a.Nodes[0], domain.Provenance)
			failed.Status = domain.Failed
			a.Nodes[0].Evidence = append(a.Nodes[0].Evidence, failed)
		}, domain.Deny},
		{"replaced bytes", func(a *domain.Assessment) {
			evidenceFor(&a.Nodes[1], domain.Checksum).Subject.SHA256 = domain.Digest(strings.Repeat("b", 64))
		}, domain.Hold},
		{"same version rebottle", func(a *domain.Assessment) { evidenceFor(&a.Nodes[0], domain.Publication).Subject.Rebuild++ }, domain.Hold},
		{"wrong platform", func(a *domain.Assessment) { evidenceFor(&a.Nodes[0], domain.Provenance).Subject.Arch = "amd64" }, domain.Hold},
		{"different macOS bottle", func(a *domain.Assessment) {
			evidenceFor(&a.Nodes[0], domain.Provenance).Subject.BottleTag = "arm64_sonoma"
		}, domain.Hold},
		{"missing dependency", func(a *domain.Assessment) { a.Nodes = a.Nodes[:1] }, domain.Hold},
		{"cycle", func(a *domain.Assessment) { a.Nodes[1].Dependencies = a.Targets }, domain.Hold},
		{"extra unrequested node", func(a *domain.Assessment) { a.Nodes[0].Dependencies = nil }, domain.Hold},
		{"duplicate package versions", func(a *domain.Assessment) { a.Nodes = append(a.Nodes, a.Nodes[0]) }, domain.Hold},
		{"zero policy", func(a *domain.Assessment) { a.Policy = domain.Policy{} }, domain.Hold},
		{"zero evidence", func(a *domain.Assessment) { *evidenceFor(&a.Nodes[0], domain.Metadata) = domain.Evidence{} }, domain.Hold},
		{"unknown status", func(a *domain.Assessment) { evidenceFor(&a.Nodes[0], domain.Provenance).Status = 99 }, domain.Hold},
		{"missing provider attribution", func(a *domain.Assessment) { evidenceFor(&a.Nodes[0], domain.Provenance).Source = "" }, domain.Hold},
		{"explicit bounded age waiver", waiveYoung, domain.Allow},
		{"emergency invalid signature", func(a *domain.Assessment) {
			waiveYoung(a)
			evidenceFor(&a.Nodes[0], domain.Provenance).Status = domain.Failed
		}, domain.Deny},
		{"emergency integrity unavailable", func(a *domain.Assessment) {
			waiveYoung(a)
			evidenceFor(&a.Nodes[0], domain.Checksum).Status = domain.Unavailable
		}, domain.Hold},
		{"emergency vulnerability", func(a *domain.Assessment) {
			waiveYoung(a)
			evidenceFor(&a.Nodes[1], domain.Vulnerabilities).Applicability = domain.Affected
		}, domain.Deny},
		{"emergency missing attestation time", func(a *domain.Assessment) {
			waiveYoung(a)
			evidenceFor(&a.Nodes[0], domain.Publication).Status = domain.Unavailable
		}, domain.Hold},
		{"dependency not waived", func(a *domain.Assessment) {
			waiveYoung(a)
			a.Exception.Waivers = a.Exception.Waivers[:1]
		}, domain.Hold},
		{"expired waiver", func(a *domain.Assessment) {
			waiveYoung(a)
			a.Exception.ExpiresAt = a.Now
		}, domain.Hold},
		{"policy changed", func(a *domain.Assessment) {
			waiveYoung(a)
			a.Binding.Policy = domain.Digest(strings.Repeat("b", 64))
		}, domain.Hold},
		{"dependency graph changed", func(a *domain.Assessment) {
			waiveYoung(a)
			a.Binding.Graph = domain.Digest(strings.Repeat("b", 64))
		}, domain.Hold},
		{"attempt changed", func(a *domain.Assessment) {
			waiveYoung(a)
			a.Binding.Attempt = domain.Digest(strings.Repeat("b", 64))
		}, domain.Hold},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := eligibleAssessment()
			tc.edit(&a)
			decision := domain.Evaluate(a)
			if decision.Outcome != tc.want || (decision.Outcome != domain.Allow && len(decision.Reasons) == 0) {
				t.Fatalf("want %v, got %+v", tc.want, decision)
			}
		})
	}
}

func TestPolicyDurationRange(t *testing.T) {
	for _, seconds := range []int64{-1, 0, domain.MaximumMinimumAgeSeconds, domain.MaximumMinimumAgeSeconds + 1} {
		policy, err := domain.NewPolicy(seconds)
		wantValid := seconds >= 0 && seconds <= domain.MaximumMinimumAgeSeconds
		if (err == nil) != wantValid || policy.Valid() != wantValid {
			t.Fatalf("minimum age %d: want valid=%t, got %+v, error=%v", seconds, wantValid, policy, err)
		}
	}
}

func TestAgeBoundaryAndWaiverAccounting(t *testing.T) {
	a := eligibleAssessment()
	publication := evidenceFor(&a.Nodes[0], domain.Publication)
	publication.PublishedAt = a.Now - a.Policy.MinimumAgeSeconds()
	if got := domain.Evaluate(a); got.Outcome != domain.Allow {
		t.Fatalf("exact minimum age must allow: got %+v", got)
	}
	publication.PublishedAt++
	if got := domain.Evaluate(a); got.Outcome != domain.Hold {
		t.Fatalf("one second young must hold: got %+v", got)
	}
	waiveYoung(&a)
	if got := domain.Evaluate(a); got.Outcome != domain.Allow || len(got.Waived) != 2 {
		t.Fatalf("missing waiver accounting: %+v", got)
	}
	if got := domain.Evaluate(domain.Assessment{}); got.Outcome != domain.Hold {
		t.Fatalf("zero assessment must hold: got %+v", got)
	}
}
