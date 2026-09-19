package ports

import "brewwarden/internal/domain"

// ConfigSource loads an optional user-owned policy. An empty location selects
// the user's default configuration, never implicit current-directory policy.
type ConfigSource interface {
	LoadConfig(location string) (domain.Policy, error)
}
