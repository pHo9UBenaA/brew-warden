package domain

// AttemptOutcome is an in-process diagnostic, never a durable authorization
// or saved plan. An interrupted process remains unknown until a fresh run.
type AttemptOutcome string

const (
	AttemptSucceeded AttemptOutcome = "succeeded"
	AttemptFailed    AttemptOutcome = "failed"
	AttemptPartial   AttemptOutcome = "partial"
	AttemptUnknown   AttemptOutcome = "unknown"
)
