package localstate

import (
	"errors"
	"io"

	"brewwarden/internal/domain"
)

type configDocument struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Age           *ageConfig          `json:"age"`
	Trust         *trustConfig        `json:"trust"`
	Verification  *verificationConfig `json:"verification"`
	Emergency     *emergencyConfig    `json:"emergency"`
}
type ageConfig struct {
	MinimumHours *int64 `json:"minimumHours"`
}
type trustConfig struct {
	AllowedTaps []string `json:"allowedTaps"`
}
type verificationConfig struct {
	RequireChecksum          *bool `json:"requireChecksum"`
	RequireBottleAttestation *bool `json:"requireBottleAttestation"`
}
type emergencyConfig struct {
	Mode          string   `json:"mode"`
	WaivableRules []string `json:"waivableRules"`
}

func ParseConfig(r io.Reader) (domain.Policy, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxDocumentBytes+1))
	if err != nil {
		return domain.Policy{}, errors.New("cannot read configuration")
	}
	var doc configDocument
	if err := decodeStrict(data, &doc); err != nil {
		return domain.Policy{}, err
	}
	if doc.SchemaVersion != 1 {
		return domain.Policy{}, errors.New("configuration schemaVersion must be 1")
	}
	if doc.Trust != nil && (len(doc.Trust.AllowedTaps) != 1 || doc.Trust.AllowedTaps[0] != "homebrew/core") {
		return domain.Policy{}, errors.New("only homebrew/core is currently supported")
	}
	if doc.Verification != nil {
		v := doc.Verification
		if (v.RequireChecksum != nil && !*v.RequireChecksum) || (v.RequireBottleAttestation != nil && !*v.RequireBottleAttestation) {
			return domain.Policy{}, errors.New("required verification cannot be disabled")
		}
	}
	if doc.Emergency != nil && (doc.Emergency.Mode != "suggest" || len(doc.Emergency.WaivableRules) != 1 || doc.Emergency.WaivableRules[0] != "age") {
		return domain.Policy{}, errors.New("emergency configuration permits only suggest mode and age waivers")
	}
	seconds := domain.DefaultMinimumAgeSeconds
	if doc.Age != nil && doc.Age.MinimumHours != nil {
		hours := *doc.Age.MinimumHours
		if hours < 0 || hours > 9223372036/3600 {
			return domain.Policy{}, errors.New("minimumHours is outside the supported duration range")
		}
		seconds = hours * 3600
	}
	return domain.NewPolicy(seconds)
}
