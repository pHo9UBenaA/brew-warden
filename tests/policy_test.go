package tests

import (
	"strings"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

func eligibleAssessment() domain.Assessment {
	const now = int64(2000000)
	digest := domain.Digest(strings.Repeat("a", 64))
	root := domain.Artifact{Tap: "homebrew/core", Name: "wget", Version: "1.0", OS: "macos", Arch: "arm64", BottleTag: "arm64_tahoe", SHA256: digest}
	dep := root
	dep.Name = "openssl@3"
	nodes := []domain.Node{{Artifact: root, Dependencies: []domain.Artifact{dep}}, {Artifact: dep}}
	for i := range nodes {
		for claim := domain.Metadata; claim <= domain.Vulnerabilities; claim++ {
			e, err := domain.NewEvidence(domain.Evidence{
				Claim: claim, Subject: nodes[i].Artifact, Status: domain.Verified,
				Provider: domain.Homebrew, Source: "fixture-provider", ProviderVersion: "test-1",
				RawSHA256: digest, ObservedAt: now - 60, ExpiresAt: now + 60,
				PublishedAt: now - 10*86400, Publication: domain.BottleRegistration,
				Applicability: domain.NoKnownApplicableFindings,
			})
			if err != nil {
				panic(err)
			}
			nodes[i].Evidence = append(nodes[i].Evidence, e)
		}
	}
	return domain.Assessment{Policy: domain.DefaultPolicy(), Binding: domain.Binding{Plan: digest, Policy: digest, Graph: digest, Environment: digest, Attempt: digest}, Targets: []domain.Artifact{root}, Nodes: nodes, Now: now}
}

func waiveYoung(a *domain.Assessment) {
	a.Exception = &domain.AgeException{Binding: a.Binding, IssuedAt: a.Now - 1, ExpiresAt: a.Now + 60}
	for i := range a.Nodes {
		a.Nodes[i].Evidence[3].PublishedAt = a.Now - 120
		a.Exception.Waivers = append(a.Exception.Waivers, domain.AgeWaiver{Artifact: a.Nodes[i].Artifact, Reason: "Explicit incident response"})
	}
}

func TestEvidenceDecisions(t *testing.T) {
	tests := []struct {
		name string
		edit func(*domain.Assessment)
		want domain.Outcome
	}{
		{"old release on first observation", func(*domain.Assessment) {}, domain.Allow},
		{"young dependency", func(a *domain.Assessment) { a.Nodes[1].Evidence[3].PublishedAt = a.Now - 120 }, domain.Hold},
		{"missing publication", func(a *domain.Assessment) { a.Nodes[1].Evidence[3].PublishedAt = 0 }, domain.Hold},
		{"future publication", func(a *domain.Assessment) { a.Nodes[0].Evidence[3].PublishedAt = a.Now + 1 }, domain.Hold},
		{"observed before publication", func(a *domain.Assessment) { a.Nodes[0].Evidence[3].PublishedAt = a.Now - 1 }, domain.Hold},
		{"legacy upstream date cannot age a rebuilt bottle", func(a *domain.Assessment) { a.Nodes[0].Evidence[3].Publication = domain.UpstreamPublication }, domain.Hold},
		{"unsupported publication event", func(a *domain.Assessment) { a.Nodes[0].Evidence[3].Publication = 99 }, domain.Hold},
		{"unknown advisory applicability", func(a *domain.Assessment) { a.Nodes[1].Evidence[4].Applicability = domain.Unknown }, domain.Hold},
		{"known dependency vulnerability", func(a *domain.Assessment) { a.Nodes[1].Evidence[4].Applicability = domain.Affected }, domain.Deny},
		{"stale clean advisory response", func(a *domain.Assessment) { a.Nodes[1].Evidence[4].ExpiresAt = a.Now }, domain.Hold},
		{"missing provenance", func(a *domain.Assessment) {
			a.Nodes[0].Evidence = append(a.Nodes[0].Evidence[:2], a.Nodes[0].Evidence[3:]...)
		}, domain.Hold},
		{"duplicate provenance", func(a *domain.Assessment) { a.Nodes[0].Evidence = append(a.Nodes[0].Evidence, a.Nodes[0].Evidence[2]) }, domain.Hold},
		{"duplicate conceals failed signature", func(a *domain.Assessment) {
			e := a.Nodes[0].Evidence[2]
			e.Status = domain.Failed
			a.Nodes[0].Evidence = append(a.Nodes[0].Evidence, e)
		}, domain.Deny},
		{"replaced bytes", func(a *domain.Assessment) {
			a.Nodes[1].Evidence[1].Subject.SHA256 = domain.Digest(strings.Repeat("b", 64))
		}, domain.Hold},
		{"same version rebottle", func(a *domain.Assessment) { a.Nodes[0].Evidence[3].Subject.Rebuild++ }, domain.Hold},
		{"wrong platform", func(a *domain.Assessment) { a.Nodes[0].Evidence[2].Subject.Arch = "amd64" }, domain.Hold},
		{"different macOS bottle", func(a *domain.Assessment) { a.Nodes[0].Evidence[2].Subject.BottleTag = "arm64_sonoma" }, domain.Hold},
		{"missing dependency", func(a *domain.Assessment) { a.Nodes = a.Nodes[:1] }, domain.Hold},
		{"cycle", func(a *domain.Assessment) { a.Nodes[1].Dependencies = a.Targets }, domain.Hold},
		{"extra unrequested node", func(a *domain.Assessment) { a.Nodes[0].Dependencies = nil }, domain.Hold},
		{"duplicate package versions", func(a *domain.Assessment) { a.Nodes = append(a.Nodes, a.Nodes[0]) }, domain.Hold},
		{"zero policy", func(a *domain.Assessment) { a.Policy = domain.Policy{} }, domain.Hold},
		{"zero evidence", func(a *domain.Assessment) { a.Nodes[0].Evidence[0] = domain.Evidence{} }, domain.Hold},
		{"unknown status", func(a *domain.Assessment) { a.Nodes[0].Evidence[2].Status = 99 }, domain.Hold},
		{"missing provider attribution", func(a *domain.Assessment) { a.Nodes[0].Evidence[2].Source = "" }, domain.Hold},
		{"explicit bounded age waiver", waiveYoung, domain.Allow},
		{"emergency invalid signature", func(a *domain.Assessment) { waiveYoung(a); a.Nodes[0].Evidence[2].Status = domain.Failed }, domain.Deny},
		{"emergency integrity unavailable", func(a *domain.Assessment) { waiveYoung(a); a.Nodes[0].Evidence[1].Status = domain.Unavailable }, domain.Hold},
		{"emergency vulnerability", func(a *domain.Assessment) { waiveYoung(a); a.Nodes[1].Evidence[4].Applicability = domain.Affected }, domain.Deny},
		{"emergency unknown age", func(a *domain.Assessment) { waiveYoung(a); a.Nodes[0].Evidence[3].Status = domain.Unavailable }, domain.Allow},
		{"dependency not waived", func(a *domain.Assessment) { waiveYoung(a); a.Exception.Waivers = a.Exception.Waivers[:1] }, domain.Hold},
		{"expired waiver", func(a *domain.Assessment) { waiveYoung(a); a.Exception.ExpiresAt = a.Now }, domain.Hold},
		{"replayed attempt", func(a *domain.Assessment) { waiveYoung(a); a.AttemptAlreadyStarted = true }, domain.Hold},
		{"policy changed", func(a *domain.Assessment) { waiveYoung(a); a.Binding.Policy = domain.Digest(strings.Repeat("b", 64)) }, domain.Hold},
		{"dependency graph changed", func(a *domain.Assessment) { waiveYoung(a); a.Binding.Graph = domain.Digest(strings.Repeat("b", 64)) }, domain.Hold},
		{"attempt changed", func(a *domain.Assessment) { waiveYoung(a); a.Binding.Attempt = domain.Digest(strings.Repeat("b", 64)) }, domain.Hold},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := eligibleAssessment()
			tc.edit(&a)
			d := domain.Evaluate(a)
			if d.Outcome != tc.want || (d.Outcome != domain.Allow && len(d.Reasons) == 0) {
				t.Fatalf("want %v, got %+v", tc.want, d)
			}
		})
	}
}

func TestAgeBoundaryAndWaiverAccounting(t *testing.T) {
	a := eligibleAssessment()
	a.Nodes[0].Evidence[3].PublishedAt = a.Now - a.Policy.MinimumAgeSeconds()
	if domain.Evaluate(a).Outcome != domain.Allow {
		t.Fatal("exact minimum age must pass")
	}
	a.Nodes[0].Evidence[3].PublishedAt++
	if domain.Evaluate(a).Outcome != domain.Hold {
		t.Fatal("one second young must hold")
	}
	waiveYoung(&a)
	if got := domain.Evaluate(a); got.Outcome != domain.Allow || len(got.Waived) != 2 {
		t.Fatalf("missing waiver accounting: %+v", got)
	}
	if domain.Evaluate(domain.Assessment{}).Outcome != domain.Hold {
		t.Fatal("zero assessment allowed")
	}
}
