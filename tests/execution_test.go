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
func (s *executionSession) Close() error { s.closed = true; return s.closeErr }

func preparedExecution() ports.Prepared {
	a := eligibleAssessment()
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
			p := preparedExecution()
			s := &executionSession{prepared: p, result: tc.actual}
			result, err := application.Execute(context.Background(), p, s, &executionClock{p.Assessment.Now})
			if result.Outcome != tc.want || !s.ran || !s.closed || (err == nil) != (tc.want == domain.AttemptSucceeded) {
				t.Fatal(result, err, s)
			}
			s.ran = false // A new test session cannot reuse a saved binding.
			s.prepared.Assessment.Binding.Attempt = domain.Digest(strings.Repeat("d", 64))
			if _, err := application.Execute(context.Background(), p, s, &executionClock{p.Assessment.Now}); err == nil || s.ran {
				t.Fatal("changed attempt reused an old plan", err)
			}
		})
	}
}

func TestExecutionCannotReportSuccessWhenOwnedProcessStateIsUnresolved(t *testing.T) {
	p := preparedExecution()
	s := &executionSession{prepared: p, result: ports.ExecutionResult{ExitKnown: true, AfterState: domain.Digest(strings.Repeat("c", 64)), MatchesPlan: true}, closeErr: errors.New("owned process still active")}
	result, err := application.Execute(context.Background(), p, s, &executionClock{p.Assessment.Now})
	if err == nil || result.Outcome != domain.AttemptUnknown || !s.ran || !s.closed {
		t.Fatal("success inferred despite unresolved process", result, err)
	}
}

func TestFreshExecutionFailureGatesNeverLaunch(t *testing.T) {
	for _, reason := range []string{"plan changed", "state changed", "expired", "emergency signature", "stale vulnerability", "cancel during revalidation", "clock advances"} {
		t.Run(reason, func(t *testing.T) {
			p := preparedExecution()
			clock := &executionClock{p.Assessment.Now}
			s := &executionSession{prepared: preparedExecution()}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch reason {
			case "plan changed":
				s.prepared.Assessment.Binding.Plan = domain.Digest(strings.Repeat("d", 64))
			case "state changed":
				s.prepared.BeforeState = domain.Digest(strings.Repeat("d", 64))
			case "expired":
				clock.now += 121
			case "emergency signature":
				waiveYoung(&s.prepared.Assessment)
				s.prepared.Assessment.Nodes[0].Evidence[2].Status = domain.Failed
				s.prepared.ExceptionID = domain.Digest(strings.Repeat("e", 64))
				p.ExceptionID = s.prepared.ExceptionID
			case "stale vulnerability":
				s.prepared.Assessment.Nodes[1].Evidence[4].ExpiresAt = clock.now
			case "cancel during revalidation":
				s.afterValidate = cancel
			case "clock advances":
				s.afterValidate = func() { clock.now += 121 }
			}
			if _, err := application.Execute(ctx, p, s, clock); err == nil || s.ran || !s.closed {
				t.Fatal("failure gate launched Homebrew", err, s)
			}
		})
	}
}
