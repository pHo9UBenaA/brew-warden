package ports

import "brewwarden/internal/domain"

type History interface {
	RecordRefusal(operation string, targets []string, policy domain.Policy) (domain.Digest, error)
	History() ([]domain.HistoryEntry, error)
}
