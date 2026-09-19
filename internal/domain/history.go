package domain

// Refusal records a request stopped before execution, never an installation.
// EventID distinguishes repeated identical requests at the same timestamp.
type Refusal struct {
	EventID           Digest
	OccurredAt        int64
	Operation         string
	Targets           []string
	ReasonCode        string
	MinimumAgeSeconds int64
}

func (r Refusal) Valid() bool {
	if !r.EventID.Valid() || r.OccurredAt <= 0 || r.ReasonCode != "execution_binding_unverified" || !ValidRequest(r.Operation, r.Targets) {
		return false
	}
	if _, err := NewPolicy(r.MinimumAgeSeconds); err != nil {
		return false
	}
	return true
}

func ValidRequest(operation string, targets []string) bool {
	if (operation != "install" && operation != "upgrade") || len(targets) > 128 || (operation == "install" && len(targets) == 0) {
		return false
	}
	for _, target := range targets {
		if !formulaName.MatchString(target) {
			return false
		}
	}
	return true
}

type HistoryEntry struct {
	ID      Digest
	Refusal Refusal
}
