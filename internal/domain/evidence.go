package domain

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Digest string

func (d Digest) Valid() bool {
	return len(d) == 64 && strings.Trim(string(d), "0123456789abcdef") == ""
}

var formulaName = regexp.MustCompile(`^[a-z0-9][a-z0-9+_.@-]{0,127}$`)
var versionText = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9+_.:-]{0,127}$`)

// Artifact identifies bytes, including same-version bottle rebuilds and platform.
// Exported values are revalidated when evaluated; serialization is not trust.
type Artifact struct {
	Tap       string
	Name      string
	Version   string
	Revision  int
	Rebuild   int
	OS        string
	Arch      string
	BottleTag string
	SHA256    Digest
}

func (a Artifact) Valid() bool {
	return a.Tap == "homebrew/core" && formulaName.MatchString(a.Name) &&
		versionText.MatchString(a.Version) && a.Revision >= 0 && a.Rebuild >= 0 &&
		a.OS == "macos" && (a.Arch == "arm64" || a.Arch == "amd64") &&
		formulaName.MatchString(a.BottleTag) && a.SHA256.Valid()
}

type Claim uint8

const (
	Metadata Claim = iota + 1
	Checksum
	Provenance
	Publication
	Vulnerabilities
)

type Verification uint8

const (
	Unassessed Verification = iota
	Verified
	Failed
	Unavailable
	Unsupported
)

type Provider uint8

const (
	Homebrew Provider = iota + 1
	Supplement
)

type Applicability uint8

const (
	Unknown Applicability = iota
	NoKnownApplicableFindings
	Affected
)

type PublicationEvent uint8

const (
	UnknownPublication PublicationEvent = iota
	UpstreamPublication
	DistributionPublication
)

// Evidence is an adapter claim, not a verifier. The adapter must establish the
// claim for these exact bytes and must not convert a successful process exit or
// an empty/incomplete advisory response into Verified.
type Evidence struct {
	Claim           Claim
	Subject         Artifact
	Status          Verification
	Provider        Provider
	Source          string
	ProviderVersion string
	RawSHA256       Digest
	ObservedAt      int64
	ExpiresAt       int64
	PublishedAt     int64
	Publication     PublicationEvent
	Applicability   Applicability
}

func NewEvidence(e Evidence) (Evidence, error) {
	if !e.valid() {
		return Evidence{}, errors.New("invalid evidence identity, attribution or validity interval")
	}
	return e, nil
}

func (e Evidence) valid() bool {
	return e.Subject.Valid() && e.Claim >= Metadata && e.Claim <= Vulnerabilities &&
		e.Status >= Verified && e.Status <= Unsupported &&
		(e.Provider == Homebrew || e.Provider == Supplement) &&
		validLabel(e.Source) && validLabel(e.ProviderVersion) && e.RawSHA256.Valid() &&
		e.ObservedAt > 0 && e.ExpiresAt > e.ObservedAt
}

func validLabel(s string) bool {
	if len(s) == 0 || len(s) > 256 {
		return false
	}
	for _, c := range s {
		if c < 32 || c > 126 {
			return false
		}
	}
	return true
}

// ValidAgeReason permits human explanations without terminal control characters.
func ValidAgeReason(s string) bool {
	if len(s) > 512 || !utf8.ValidString(s) || strings.TrimSpace(s) == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}
