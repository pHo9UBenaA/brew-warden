package tests

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/application"
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

type executionClock struct{ now int64 }

func (c *executionClock) Now() int64 { return c.now }

type executionSession struct {
	prepared      ports.Prepared
	result        ports.ExecutionResult
	runErr        error
	validationErr error
	closeErr      error
	ran, closed   bool
	afterValidate func()
}

func (s *executionSession) Revalidate(context.Context) (ports.Prepared, error) {
	if s.ran {
		return ports.Prepared{}, errors.New("consumed session")
	}
	if s.afterValidate != nil {
		s.afterValidate()
	}
	return s.prepared, s.validationErr
}

func (s *executionSession) Run(_ context.Context, _ domain.Binding) (ports.ExecutionResult, error) {
	s.ran = true
	return s.result, s.runErr
}

func (s *executionSession) Close() error {
	s.closed = true
	return s.closeErr
}

func preparedExecution(t testing.TB) ports.Prepared {
	t.Helper()
	a := eligibleAssessment(t)
	return ports.Prepared{Assessment: a, BeforeState: domain.Digest(strings.Repeat("b", 64)), ExpiresAt: a.Now + 120}
}

func TestFreshExecutionReportsActualExitAndState(t *testing.T) {
	for _, tc := range []struct {
		name   string
		actual ports.ExecutionResult
		want   domain.AttemptOutcome
	}{
		{"success", ports.ExecutionResult{ExitKnown: true, AfterState: domain.Digest(strings.Repeat("c", 64)), MatchesPlan: true}, domain.AttemptSucceeded},
		{"partial failure", ports.ExecutionResult{ExitKnown: true, ExitCode: 2, AfterState: domain.Digest(strings.Repeat("c", 64))}, domain.AttemptPartial},
		{"unchanged failure", ports.ExecutionResult{ExitKnown: true, ExitCode: 3, AfterState: domain.Digest(strings.Repeat("b", 64))}, domain.AttemptFailed},
		{"zero exit wrong state", ports.ExecutionResult{ExitKnown: true, AfterState: domain.Digest(strings.Repeat("c", 64))}, domain.AttemptUnknown},
		{"lost child", ports.ExecutionResult{}, domain.AttemptUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := preparedExecution(t)
			s := &executionSession{prepared: p, result: tc.actual}
			result, err := application.Execute(context.Background(), p, s, &executionClock{p.Assessment.Now})
			if result.Outcome != tc.want || !s.ran || !s.closed || (err == nil) != (tc.want == domain.AttemptSucceeded) {
				t.Fatalf("want outcome=%s, launch, session release and error iff not succeeded: result=%+v ran=%t closed=%t error=%v", tc.want, result, s.ran, s.closed, err)
			}
		})
	}
}

func TestExecutionRejectsChangedAttemptWithoutLaunch(t *testing.T) {
	original := preparedExecution(t)
	fresh := preparedExecution(t)
	fresh.Assessment.Binding.Attempt = domain.Digest(strings.Repeat("d", 64))
	session := &executionSession{prepared: fresh}
	_, err := application.Execute(context.Background(), original, session, &executionClock{original.Assessment.Now})
	if err == nil || session.ran || !session.closed {
		t.Fatalf("changed attempt must refuse launch and release session: ran=%t closed=%t error=%v", session.ran, session.closed, err)
	}
}

func TestExecutionCannotReportSuccessWhenOwnedProcessStateIsUnresolved(t *testing.T) {
	p := preparedExecution(t)
	s := &executionSession{
		prepared: p,
		result:   ports.ExecutionResult{ExitKnown: true, AfterState: domain.Digest(strings.Repeat("c", 64)), MatchesPlan: true},
		closeErr: errors.New("owned process still active"),
	}
	result, err := application.Execute(context.Background(), p, s, &executionClock{p.Assessment.Now})
	if err == nil || result.Outcome != domain.AttemptUnknown || !s.ran || !s.closed {
		t.Fatalf("unresolved process must report unknown with launch and release: result=%+v ran=%t closed=%t error=%v", result, s.ran, s.closed, err)
	}
}

func TestFreshExecutionFailureGatesNeverLaunch(t *testing.T) {
	type attempt struct {
		original ports.Prepared
		session  *executionSession
		clock    *executionClock
		cancel   context.CancelFunc
	}
	for _, test := range []struct {
		name      string
		change    func(*attempt)
		wantError string
	}{
		{"plan changed", func(a *attempt) {
			a.session.prepared.Assessment.Binding.Plan = domain.Digest(strings.Repeat("d", 64))
		}, "execution plan changed or expired"},
		{"state changed", func(a *attempt) {
			a.session.prepared.BeforeState = domain.Digest(strings.Repeat("d", 64))
		}, "execution plan changed or expired"},
		{"expired", func(a *attempt) { a.clock.now += 121 }, "execution plan changed or expired"},
		{"emergency signature", func(a *attempt) {
			waiveYoung(&a.session.prepared.Assessment)
			evidenceFor(&a.session.prepared.Assessment.Nodes[0], domain.Provenance).Status = domain.Failed
			a.session.prepared.ExceptionID = domain.Digest(strings.Repeat("e", 64))
			a.original.ExceptionID = a.session.prepared.ExceptionID
		}, "execution held by policy"},
		{"stale vulnerability", func(a *attempt) {
			evidenceFor(&a.session.prepared.Assessment.Nodes[1], domain.Vulnerabilities).ExpiresAt = a.clock.now
		}, "execution held by policy"},
		{"cancel during revalidation", func(a *attempt) { a.session.afterValidate = a.cancel }, "context canceled"},
		{"clock advances", func(a *attempt) {
			a.session.afterValidate = func() { a.clock.now += 121 }
		}, "execution plan changed or expired"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fixture := &attempt{
				original: preparedExecution(t),
				session:  &executionSession{prepared: preparedExecution(t)},
				cancel:   cancel,
			}
			fixture.clock = &executionClock{fixture.original.Assessment.Now}
			test.change(fixture)
			_, err := application.Execute(ctx, fixture.original, fixture.session, fixture.clock)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("want refusal containing %q, got %v", test.wantError, err)
			}
			if fixture.session.ran || !fixture.session.closed {
				t.Fatalf("failure gate must refuse launch and release session: ran=%t closed=%t", fixture.session.ran, fixture.session.closed)
			}
		})
	}
}
