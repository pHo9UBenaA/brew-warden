package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Reuse immutable facts from another retained VM test when API quota is scarce.
// Product code still validates their subject/digest, rechecks signatures and
// performs fresh advisory queries. The default fixture starts without a cache.
func seedVMEvidenceCache(t *testing.T, destination string) {
	t.Helper()
	source := os.Getenv("BREWWARDEN_VM_EVIDENCE_CACHE")
	if source == "" {
		return
	}
	for _, kind := range []string{"attestations", "registrations"} {
		entries, err := os.ReadDir(filepath.Join(source, kind))
		if err != nil {
			t.Fatal(err)
		}
		directory := filepath.Join(destination, kind)
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if len(name) != 69 || !strings.HasSuffix(name, ".json") || strings.Trim(name[:64], "0123456789abcdef") != "" {
				t.Fatal("invalid evidence fixture name")
			}
			info, err := entry.Info()
			if err != nil || !info.Mode().IsRegular() || info.Size() > 8*1024*1024 {
				t.Fatal("invalid evidence fixture", err)
			}
			raw, err := os.ReadFile(filepath.Join(source, kind, name))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, name), raw, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}
