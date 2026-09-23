package domain

// Execution outcomes are in-process diagnostics, never durable authorizations
// or saved plans. An interrupted process remains unknown until a fresh run.
type AttemptOutcome string

const (
	AttemptSucceeded AttemptOutcome = "succeeded"
	AttemptFailed    AttemptOutcome = "failed"
	AttemptPartial   AttemptOutcome = "partial"
	AttemptUnknown   AttemptOutcome = "unknown"
)
