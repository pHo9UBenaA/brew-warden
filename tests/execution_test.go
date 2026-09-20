package tests

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"brewwarden/internal/adapters/localstate"
	"brewwarden/internal/application"
	"brewwarden/internal/domain"
	"brewwarden/internal/ports"
)

type executionClock struct{ now int64 }

func (c *executionClock) Now() int64 { return c.now }

type executionSession struct {
	prepared      ports.Prepared
	result        ports.ExecutionResult
	runErr        error
	validationErr error
	ran, closed   bool
	beforeRun     func()
}

func (s *executionSession) Revalidate(context.Context) (ports.Prepared, error) {
	return s.prepared, s.validationErr
}
func (s *executionSession) Run(_ context.Context, _ domain.Binding) (ports.ExecutionResult, error) {
	s.ran = true
	if s.beforeRun != nil {
		s.beforeRun()
	}
	return s.result, s.runErr
}
func (s *executionSession) Close() error { s.closed = true; return nil }

type attemptStore interface{ ports.Attempts }
type interceptedJournal struct {
	attemptStore
	startErr, finishErr error
	afterStart          func()
}

func (j interceptedJournal) StartAttempt(s domain.AttemptStart) error {
	if j.startErr != nil {
		return j.startErr
	}
	if err := j.attemptStore.StartAttempt(s); err != nil {
		return err
	}
	if j.afterStart != nil {
		j.afterStart()
	}
	return nil
}
func (j interceptedJournal) FinishAttempt(f domain.AttemptFinish) error {
	if j.finishErr != nil {
		return j.finishErr
	}
	return j.attemptStore.FinishAttempt(f)
}

func preparedExecution() ports.Prepared {
	a := eligibleAssessment()
	return ports.Prepared{Assessment: a, BeforeState: domain.Digest(strings.Repeat("b", 64)), ExpiresAt: a.Now + 120}
}

func TestExecutionRequiresDurableStartAndRecordsActualOutcome(t *testing.T) {
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
			journal := localstate.Journal{Path: filepath.Join(t.TempDir(), "attempts")}
			s := &executionSession{prepared: p, result: tc.actual}
			s.beforeRun = func() {
				prior, err := journal.Attempts()
				if err != nil || len(prior) != 1 || prior[0].Start.Binding != p.Assessment.Binding || !prior[0].Unresolved() {
					t.Fatal("process launched without durable start", prior, err)
				}
			}
			result, err := application.Execute(context.Background(), p, s, journal, &executionClock{p.Assessment.Now})
			if result.Outcome != tc.want || !s.ran || !s.closed || (err == nil) != (tc.want == domain.AttemptSucceeded) {
				t.Fatal(result, err, s)
			}
			stored, err := journal.Attempts()
			if err != nil || len(stored) != 1 || stored[0].Finish.Outcome != tc.want || stored[0].Finish.ExitKnown != tc.actual.ExitKnown || stored[0].Finish.ExitCode != tc.actual.ExitCode {
				t.Fatal(stored, err)
			}
			s.ran = false
			if _, err := application.Execute(context.Background(), p, s, journal, &executionClock{p.Assessment.Now}); err == nil || s.ran {
				t.Fatal("attempt replayed", err)
			}
		})
	}
}

func TestExecutionFailureGatesNeverLaunch(t *testing.T) {
	for _, reason := range []string{"plan changed", "state changed", "expired", "emergency signature", "stale vulnerability", "start write failed", "cancel during start", "evidence expires during start", "clock reverses"} {
		t.Run(reason, func(t *testing.T) {
			p := preparedExecution()
			clock := &executionClock{p.Assessment.Now}
			s := &executionSession{prepared: preparedExecution()}
			store := localstate.Journal{Path: filepath.Join(t.TempDir(), "attempts")}
			journal := interceptedJournal{attemptStore: store}
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
			case "start write failed":
				journal.startErr = errors.New("disk full")
			case "cancel during start":
				journal.afterStart = cancel
			case "evidence expires during start":
				journal.afterStart = func() { clock.now += 61 }
			case "clock reverses":
				journal.afterStart = func() { clock.now-- }
			}
			if _, err := application.Execute(ctx, p, s, journal, clock); err == nil || s.ran || !s.closed {
				t.Fatal("failure gate launched", err, s)
			}
			attempts, err := store.Attempts()
			if err != nil {
				t.Fatal(err)
			}
			if journal.afterStart != nil {
				if len(attempts) != 1 || attempts[0].Finish.Outcome != domain.AttemptNotStarted || attempts[0].Finish.ExitKnown {
					t.Fatal("prelaunch cancellation invented child exit", attempts)
				}
			} else if len(attempts) != 0 {
				t.Fatal("preflight failure consumed attempt", attempts)
			}
		})
	}
}

func TestOutcomeWriteFailureRequiresReconciliation(t *testing.T) {
	p := preparedExecution()
	store := localstate.Journal{Path: filepath.Join(t.TempDir(), "attempts")}
	s := &executionSession{prepared: p, result: ports.ExecutionResult{ExitKnown: true, MatchesPlan: true, AfterState: p.BeforeState}}
	journal := interceptedJournal{attemptStore: store, finishErr: errors.New("sync failure")}
	result, err := application.Execute(context.Background(), p, s, journal, &executionClock{p.Assessment.Now})
	if err == nil || result.Outcome != domain.AttemptUnknown || !s.ran {
		t.Fatal(result, err)
	}
	prior, err := store.Attempts()
	if err != nil || len(prior) != 1 || !prior[0].Unresolved() {
		t.Fatal(prior, err)
	}
	p.Assessment.Binding.Attempt = domain.Digest(strings.Repeat("f", 64))
	s.prepared = p
	s.ran = false
	if _, err := application.Execute(context.Background(), p, s, store, &executionClock{p.Assessment.Now}); err == nil || s.ran {
		t.Fatal("unresolved state bypassed with a new attempt")
	}
}
