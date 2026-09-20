package tests

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/adapters/localstate"
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

type serviceRecovery struct {
	state domain.Digest
	err   error
	calls int
}

func (r *serviceRecovery) Snapshot(context.Context, domain.Binding) (domain.Digest, error) {
	r.calls++
	return r.state, r.err
}
func TestServiceRequiresReconciliationBeforePlanning(t *testing.T) {
	p := preparedExecution()
	j := localstate.Journal{Path: filepath.Join(t.TempDir(), "attempts")}
	if err := j.StartAttempt(domain.AttemptStart{Binding: p.Assessment.Binding, BeforeState: p.BeforeState, StartedAt: p.Assessment.Now}); err != nil {
		t.Fatal(err)
	}
	planner := &servicePlanner{p: p}
	recovery := &serviceRecovery{err: errors.New("native locks held")}
	s := application.Service{Planner: planner, Journal: j, Recovery: recovery, Clock: &executionClock{p.Assessment.Now}}
	if _, err := s.Run(context.Background(), ports.Request{Operation: "install", Targets: []string{"jq"}}, domain.DefaultPolicy(), nil); err == nil || planner.calls != 0 {
		t.Fatal("planned while unresolved", err)
	}
	if err := s.Reconcile(context.Background(), p.Assessment.Binding.Attempt); err == nil {
		t.Fatal("reconciled active native process")
	}
	records, _ := j.Attempts()
	if !records[0].Unresolved() {
		t.Fatal("failed recovery changed attempt")
	}
	recovery.err = nil
	recovery.state = domain.Digest(strings.Repeat("e", 64))
	if err := s.Reconcile(context.Background(), p.Assessment.Binding.Attempt); err != nil {
		t.Fatal(err)
	}
	records, _ = j.Attempts()
	if records[0].Finish.Outcome != domain.AttemptReconciled || records[0].Finish.ExitKnown || records[0].Finish.AfterState != recovery.state {
		t.Fatal(records)
	}
	if err := s.Reconcile(context.Background(), p.Assessment.Binding.Attempt); err == nil {
		t.Fatal("duplicate reconciliation")
	}
}
func TestPlanPresentationFailurePreventsDurableStart(t *testing.T) {
	p := preparedExecution()
	session := &executionSession{prepared: p}
	planner := &servicePlanner{p: p, s: session}
	j := localstate.Journal{Path: filepath.Join(t.TempDir(), "attempts")}
	s := application.Service{Planner: planner, Journal: j, Clock: &executionClock{p.Assessment.Now}, Present: func(ports.Prepared) error { return io.ErrClosedPipe }}
	if _, err := s.Run(context.Background(), ports.Request{Operation: "install", Targets: []string{"jq"}}, domain.DefaultPolicy(), nil); err == nil || session.ran || !session.closed {
		t.Fatal(err, session)
	}
	records, _ := j.Attempts()
	if len(records) != 0 {
		t.Fatal(records)
	}
}
func TestRuntimeCLIRejectsChildWrapperOptionsAndPreservesExit(t *testing.T) {
	for _, args := range [][]string{{"brew", "install", "jq", "--age-exception", "jq=urgent"}, {"brew", "--help"}, {"brew", "install", "--cask", "jq"}, {"--age-exception", "jq=\x1b[31m", "brew", "install", "jq"}, {"--age-exception", "jq=urgent", "--age-exception", "jq=again", "brew", "install", "jq"}} {
		planner := &servicePlanner{}
		s := application.Service{Planner: planner}
		var out bytes.Buffer
		if cli.RunWithRuntime(context.Background(), args, &out, &out, nil, nil, &s) == 0 || planner.calls != 0 {
			t.Fatal(args, out.String())
		}
	}
	p := preparedExecution()
	session := &executionSession{prepared: p, result: ports.ExecutionResult{ExitKnown: true, ExitCode: 7, AfterState: p.BeforeState}}
	planner := &servicePlanner{p: p, s: session}
	s := application.Service{Planner: planner, Journal: localstate.Journal{Path: filepath.Join(t.TempDir(), "attempts")}, Clock: &executionClock{p.Assessment.Now}}
	var out bytes.Buffer
	code := cli.RunWithRuntime(context.Background(), []string{"--age-exception", "jq=\u7dca\u6025\u4fee\u6b63", "brew", "install", "jq"}, &out, &out, nil, nil, &s)
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
