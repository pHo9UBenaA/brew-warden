package ports

import "github.com/pHo9UBenaA/brew-warden/internal/domain"

type History interface {
	RecordRefusal(operation string, targets []string, policy domain.Policy) (domain.Digest, error)
	History() ([]domain.HistoryEntry, error)
}
