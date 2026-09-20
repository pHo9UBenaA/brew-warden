package domain

// AttemptStart consumes one attempt and, when present, one age exception.
// BeforeState and Binding refer to immutable, validated records, not user labels.
type AttemptStart struct {
	Binding     Binding
	BeforeState Digest
	Exception   Digest
	StartedAt   int64
}

func (s AttemptStart) Valid() bool {
	return s.Binding.Valid() && s.BeforeState.Valid() && s.StartedAt > 0 && (s.Exception == "" || s.Exception.Valid())
}

type AttemptOutcome string

const (
	AttemptNotStarted AttemptOutcome = "not_started"
	AttemptSucceeded  AttemptOutcome = "succeeded"
	AttemptFailed     AttemptOutcome = "failed"
	AttemptPartial    AttemptOutcome = "partial"
	AttemptUnknown    AttemptOutcome = "unknown"
	AttemptReconciled AttemptOutcome = "reconciled"
)

// Exit status and observed state are separate facts. Unknown requires recovery;
// reconciled means state was inspected, never that the lost process succeeded.
type AttemptFinish struct {
	Attempt    Digest
	FinishedAt int64
	Outcome    AttemptOutcome
	ExitKnown  bool
	ExitCode   int
	AfterState Digest
}

func (f AttemptFinish) Valid() bool {
	if !f.Attempt.Valid() || f.FinishedAt <= 0 || f.ExitCode < 0 || f.ExitCode > 255 || (!f.ExitKnown && f.ExitCode != 0) {
		return false
	}
	switch f.Outcome {
	case AttemptSucceeded:
		return f.ExitKnown && f.ExitCode == 0 && f.AfterState.Valid()
	case AttemptFailed, AttemptPartial:
		return f.ExitKnown && f.ExitCode != 0 && f.AfterState.Valid()
	case AttemptUnknown:
		return f.AfterState == "" || f.AfterState.Valid()
	case AttemptReconciled, AttemptNotStarted:
		return !f.ExitKnown && f.AfterState.Valid()
	default:
		return false
	}
}

type Attempt struct {
	Start  AttemptStart
	Finish AttemptFinish
}

func (a Attempt) Unresolved() bool {
	return a.Finish.Outcome == "" || a.Finish.Outcome == AttemptUnknown
}

func (a Attempt) Valid() bool {
	if !a.Start.Valid() {
		return false
	}
	if a.Finish == (AttemptFinish{}) {
		return true
	}
	return a.Finish.Valid() && a.Finish.Attempt == a.Start.Binding.Attempt && a.Finish.FinishedAt >= a.Start.StartedAt
}
