package tests

import (
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/adapters/attestation"
)

func installedBottleVerifier(t *testing.T) attestation.PublicGH {
	t.Helper()
	gh, err := attestation.InstalledGH()
	if err != nil {
		t.Fatal(err)
	}
	return gh
}
