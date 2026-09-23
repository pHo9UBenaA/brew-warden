package homebrew

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLiveNativeMetadata(t *testing.T) {
	if os.Getenv("BREWWARDEN_LIVE_RUNTIME") != "1" {
		t.Skip("requires explicit disposable Apple Silicon VM")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("native runtime requires Apple Silicon macOS")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := workspace{root: root}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	if _, err := (Runtime{}).materialize(filepath.Join(root, "runtime")); err != nil {
		t.Fatal(err)
	}
	metadata, err := os.ReadFile("../../../.cache/packages.arm64_tahoe.jws.json")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := w.sandbox("read", false, false, []string{filepath.Join(root, "runtime/brew/Library")})
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(root, metadataCachePath)
	if err := os.MkdirAll(filepath.Dir(cache), 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeNew(cache, metadata, 0600); err != nil {
		t.Fatal(err)
	}
	result, err := w.metadata(context.Background(), profile, []string{"jq"})
	if err != nil {
		log, _ := os.ReadFile(filepath.Join(root, "metadata.stderr"))
		t.Fatalf("%v: %s", err, log)
	}
	recipes, candidates, err := parseMetadata(result, []string{"jq"})
	if err != nil {
		t.Fatal(err)
	}
	if len(recipes) != 2 || len(candidates) != 2 || candidates[0].Name != "jq" || candidates[1].Name != "oniguruma" {
		t.Fatal("unexpected closure", recipes, candidates)
	}
	t.Log("authenticated complete metadata closure", digestBytes(result))

	// A warmed derived cache must not bypass signature verification on a later
	// info invocation. Corrupt only the signature, retaining valid JSON.
	var signed struct {
		Payload    string `json:"payload"`
		Signatures []struct {
			Protected string          `json:"protected"`
			Header    json.RawMessage `json:"header"`
			Signature string          `json:"signature"`
		} `json:"signatures"`
	}
	if err := json.Unmarshal(metadata, &signed); err != nil || len(signed.Signatures) == 0 {
		t.Fatal("missing signed fixture")
	}
	sig := signed.Signatures[0].Signature
	if len(sig) == 0 {
		t.Fatal("empty signature")
	}
	replacement := "A"
	if strings.HasPrefix(sig, replacement) {
		replacement = "B"
	}
	signed.Signatures[0].Signature = replacement + sig[1:]
	corrupt, err := json.Marshal(signed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cache, corrupt, 0600); err != nil {
		t.Fatal(err)
	}
	_, err = w.invoke(context.Background(), "invalid-signature", filepath.Join(root, "metadata.sb"), "info", "--json=v2", "--formula", "homebrew/core/jq")
	if err == nil {
		t.Fatal("tampered metadata accepted with a warmed derived cache")
	}
	// Homebrew deletes invalid signed input before printing its signature error.
	// The product's immutable-input sandbox blocks that deletion. Repeat with
	// cache writes allowed (network and host writes still denied) to establish
	// the precise rejection cause, rather than accept any process failure.
	_, err = w.invoke(context.Background(), "signature-diagnostic", profile, "info", "--json=v2", "--formula", "homebrew/core/jq")
	log, _ := os.ReadFile(filepath.Join(root, "signature-diagnostic.stderr"))
	if err == nil || !strings.Contains(strings.ToLower(string(log)), "signature") {
		t.Fatalf("tampered metadata was not rejected for its signature: %v: %s", err, log)
	}
	if err := os.WriteFile(cache, metadata, 0600); err != nil {
		t.Fatal(err)
	}
	cached, err := filepath.Abs("../../../.cache/vm-evidence/acceptance-03/brewwarden-probe.3mwiEaQB/cache/downloads")
	if err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(cached)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "cache/downloads"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		info, err := file.Info()
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(cached, file.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := writeNew(filepath.Join(root, "cache/downloads", file.Name()), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	downloaded, err := w.fetchBottles(context.Background(), profile, candidates)
	if err != nil {
		log, _ := os.ReadFile(filepath.Join(root, "fetch.stderr"))
		t.Fatalf("public CLI fetch failed: %v: %s", err, log)
	}
	if err := w.copyDownloads(downloaded, candidates); err != nil {
		t.Fatal(err)
	}
	t.Log("public CLI fetched complete bottle closure from frozen offline cache")
	// Homebrew's native verification is not equivalent to the installed gh
	// attestation check; exercise the skipped-credential path explicitly.
	if _, err := w.invoke(context.Background(), "cli-verify", profile, "verify", "--json", "--bottle-tag=arm64_tahoe", "jq"); err == nil {
		t.Fatal("credential-free native verification unexpectedly succeeded")
	}
	verifyLog, err := os.ReadFile(filepath.Join(root, "cli-verify.stderr"))
	if err != nil || !strings.Contains(string(verifyLog), "missing credentials") {
		t.Fatalf("unexpected native verification failure: %v: %s", err, verifyLog)
	}
	if err := w.checkBottleMetadata(candidates); err != nil {
		t.Fatal(err)
	}
}
