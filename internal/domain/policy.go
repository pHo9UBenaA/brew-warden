// Package domain contains deterministic decisions over explicit evidence.
package domain

import "errors"

const DefaultMinimumAgeSeconds int64 = 168 * 60 * 60

// Policy cannot disable integrity, provenance, trust or vulnerability checks.
// Its zero value is invalid; use NewPolicy or DefaultPolicy.
type Policy struct {
	minimumAgeSeconds int64
	valid             bool
}

func NewPolicy(minimumAgeSeconds int64) (Policy, error) {
	if minimumAgeSeconds < 0 || minimumAgeSeconds > 9223372036 {
		return Policy{}, errors.New("minimum age must fit a nonnegative duration in whole seconds")
	}
	return Policy{minimumAgeSeconds: minimumAgeSeconds, valid: true}, nil
}

func DefaultPolicy() Policy {
	return Policy{minimumAgeSeconds: DefaultMinimumAgeSeconds, valid: true}
}

func (p Policy) MinimumAgeSeconds() int64 { return p.minimumAgeSeconds }
func (p Policy) Valid() bool              { return p.valid }
