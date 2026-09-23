package tests

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/application"
	"github.com/pHo9UBenaA/brew-warden/internal/cli"
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

type servicePlanner struct {
	p         ports.Prepared
	s         ports.ExecutionSession
	calls     int
	err       error
	overrides []ports.AgeOverride
	request   ports.Request
}

func (p *servicePlanner) Prepare(_ context.Context, r ports.Request, _ domain.Policy, o []ports.AgeOverride, _ int64) (ports.Prepared, ports.ExecutionSession, error) {
	p.calls++
	p.overrides = o
	p.request = r
	return p.p, p.s, p.err
}

func TestPlanPresentationFailurePreventsLaunch(t *testing.T) {
	p := preparedExecution()
	session := &executionSession{prepared: p}
	planner := &servicePlanner{p: p, s: session}
	s := application.Service{Planner: planner, Clock: &executionClock{p.Assessment.Now}, Present: func(ports.Prepared) error { return io.ErrClosedPipe }}
	if _, err := s.Run(context.Background(), ports.Request{Operation: "install", Targets: []string{"jq"}}, domain.DefaultPolicy(), nil); err == nil || session.ran || !session.closed {
		t.Fatal(err, session)
	}
}

func TestFreshRetryRequiresNewBoundPlan(t *testing.T) {
	p := preparedExecution()
	first := &executionSession{prepared: p, result: ports.ExecutionResult{ExitKnown: true, ExitCode: 2, AfterState: p.BeforeState}}
	planner := &servicePlanner{p: p, s: first}
	s := application.Service{Planner: planner, Clock: &executionClock{p.Assessment.Now}}
	request := ports.Request{Operation: "install", Targets: []string{"jq"}}
	if _, err := s.Run(context.Background(), request, domain.DefaultPolicy(), nil); err == nil || !first.ran {
		t.Fatal("failed operation did not run", err)
	}
	p.Assessment.Binding.Attempt = domain.Digest(strings.Repeat("f", 64))
	second := &executionSession{prepared: p, result: ports.ExecutionResult{ExitKnown: true, AfterState: domain.Digest(strings.Repeat("c", 64)), MatchesPlan: true}}
	planner.p, planner.s = p, second
	if result, err := s.Run(context.Background(), request, domain.DefaultPolicy(), nil); err != nil || result.Outcome != domain.AttemptSucceeded || planner.calls != 2 || !second.ran {
		t.Fatal("fresh retry did not use new plan", result, err)
	}
}

func TestRuntimeCLIRejectsChildOptionsAndRemovedCommands(t *testing.T) {
	for _, args := range [][]string{{"brew", "install", "jq", "--age-exception", "jq=urgent"}, {"brew", "--help"}, {"brew", "install", "--cask", "jq"}, {"--age-exception", "jq=\x1b[31m", "brew", "install", "jq"}, {"--age-exception", "jq=urgent", "--age-exception", "jq=again", "brew", "install", "jq"}, {"history"}, {"status"}, {"reconcile"}} {
		planner := &servicePlanner{}
		s := application.Service{Planner: planner}
		var out bytes.Buffer
		if cli.RunWithRuntime(context.Background(), args, &out, &out, nil, &s) == 0 || planner.calls != 0 {
			t.Fatal("unsupported invocation reached planner", args, out.String())
		}
	}
	p := preparedExecution()
	session := &executionSession{prepared: p, result: ports.ExecutionResult{ExitKnown: true, ExitCode: 7, AfterState: p.BeforeState}}
	planner := &servicePlanner{p: p, s: session}
	s := application.Service{Planner: planner, Clock: &executionClock{p.Assessment.Now}}
	var out bytes.Buffer
	code := cli.RunWithRuntime(context.Background(), []string{"--age-exception", "jq=\u7dca\u6025\u4fee\u6b63", "brew", "install", "jq"}, &out, &out, nil, &s)
	if code != 7 || !session.ran || len(planner.overrides) != 1 || planner.overrides[0].Reason != "\u7dca\u6025\u4fee\u6b63" || !strings.Contains(out.String(), "Checking requested formulae and dependencies:") {
		t.Fatal(code, out.String(), planner)
	}
}
func TestAgeReasonRejectsControlsAndInvisibleText(t *testing.T) {
	for _, reason := range []string{"", "   ", "\x1b[31m", "a\nb", "a\u202eb", "a\u200bb", string([]byte{255}), strings.Repeat("x", 513)} {
		if domain.ValidAgeReason(reason) {
			t.Fatalf("accepted %q", reason)
		}
	}
	if !domain.ValidAgeReason("\u7dca\u6025\u306e\u4fee\u6b63\u3092\u9069\u7528") {
		t.Fatal("rejected printable reason")
	}
}
