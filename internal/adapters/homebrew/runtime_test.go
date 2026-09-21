package homebrew

import (
	"context"
	"encoding/json"
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func runtimeFixture(t *testing.T) (Runtime, string) {
	t.Helper()
	root := t.TempDir()
	data := []byte("pinned verifier fixture")
	if err := os.WriteFile(filepath.Join(root, "verifier"), data, 0755); err != nil {
		t.Fatal(err)
	}
	m := runtimeManifest{Schema: 1, BrewRevision: brewRevision, Files: []runtimeEntry{{Path: "verifier", Mode: 0755, SHA256: digestBytes(data)}}}
	raw, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	return Runtime{root, digestBytes(raw)}, filepath.Join(t.TempDir(), "runtime")
}
func TestRuntimeInventoryAndSubstitution(t *testing.T) {
	r, dst := runtimeFixture(t)
	if _, err := r.materialize(dst); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []string{"changed input", "extra input", "manifest changed", "symlink input"} {
		t.Run(tc, func(t *testing.T) {
			r, dst := runtimeFixture(t)
			switch tc {
			case "changed input":
				if err := os.WriteFile(filepath.Join(r.Root, "verifier"), []byte("changed"), 0755); err != nil {
					t.Fatal(err)
				}
			case "extra input":
				if err := os.WriteFile(filepath.Join(r.Root, "injected.rb"), []byte("unexpected"), 0600); err != nil {
					t.Fatal(err)
				}
			case "manifest changed":
				r.ManifestSHA256 = domain.Digest(strings.Repeat("a", 64))
			case "symlink input":
				if err := os.Remove(filepath.Join(r.Root, "verifier")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("manifest.json", filepath.Join(r.Root, "verifier")); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := r.materialize(dst); err == nil {
				t.Fatal("unsafe runtime accepted")
			}
		})
	}
}
func TestRuntimeUnsafeManifest(t *testing.T) {
	for _, entry := range []runtimeEntry{
		{Path: "../outside", Mode: 0755, SHA256: domain.Digest(strings.Repeat("a", 64))},
		{Path: "bad", Mode: 0755, Link: "../../outside"},
		{Path: "bad", Mode: 0755, Link: "/etc/passwd"},
		{Path: "bad", Mode: 0777, SHA256: domain.Digest(strings.Repeat("a", 64))},
	} {
		r, dst := runtimeFixture(t)
		raw, _ := json.Marshal(runtimeManifest{Schema: 1, BrewRevision: brewRevision, Files: []runtimeEntry{entry}})
		if err := os.WriteFile(filepath.Join(r.Root, "manifest.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		r.ManifestSHA256 = digestBytes(raw)
		if _, err := r.materialize(dst); err == nil {
			t.Fatal("unsafe manifest accepted", entry)
		}
	}
}
func TestLiveNativeMetadata(t *testing.T) {
	source := os.Getenv("BREWWARDEN_LIVE_RUNTIME")
	if source == "" {
		t.Skip("requires explicitly built native runtime")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("native runtime requires Apple Silicon macOS")
	}
	raw, err := os.ReadFile(filepath.Join(source, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := workspace{root: root}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	if _, err := (Runtime{source, digestBytes(raw)}).materialize(filepath.Join(root, "runtime")); err != nil {
		t.Fatal(err)
	}
	metadata, err := os.ReadFile("../../../.cache/formula.jws.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeNew(filepath.Join(root, "formula.jws.json"), metadata, 0600); err != nil {
		t.Fatal(err)
	}
	profile, err := w.sandbox("read", false, false, []string{filepath.Join(root, "runtime/brew/Library")})
	if err != nil {
		t.Fatal(err)
	}
	result, err := w.native(context.Background(), "metadata", profile, "metadata.rb", "jq")
	if err != nil {
		log, _ := os.ReadFile(filepath.Join(root, "metadata.stderr"))
		t.Fatalf("%v: %s", err, log)
	}
	recipes, candidates, err := parseMetadata(result, []string{"jq"})
	if err != nil {
		t.Fatal(err)
	}
	if len(recipes) != 6 || len(candidates) != 2 || candidates[0].Name != "jq" || candidates[1].Name != "oniguruma" {
		t.Fatal("unexpected closure", recipes, candidates)
	}
	t.Log("authenticated complete metadata closure", digestBytes(result))
	inputs, err := filepath.Abs("../../../.cache/dependency-probe-inputs")
	if err != nil {
		t.Fatal(err)
	}
	for _, recipe := range recipes {
		data, err := os.ReadFile(filepath.Join(inputs, recipe.Name+".rb"))
		if err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(root, "runtime/brew/Library/Taps/homebrew/homebrew-core", recipe.RecipePath)
		if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			t.Fatal(err)
		}
		if err := writeNew(destination, data, 0600); err != nil {
			t.Fatal(err)
		}
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
	// Native verification is not a credential-free substitute for the bundled
	// verifier: exercise the actual CLI with the same empty private home.
	if _, err := w.invoke(context.Background(), "cli-verify", profile, "verify", "--json", "--bottle-tag=arm64_tahoe", "jq"); err == nil {
		t.Fatal("credential-free native verification unexpectedly succeeded")
	}
	verifyLog, err := os.ReadFile(filepath.Join(root, "cli-verify.stderr"))
	if err != nil || !strings.Contains(string(verifyLog), "missing credentials") {
		t.Fatalf("unexpected native verification failure: %v: %s", err, verifyLog)
	}
	lockProbe := `require "lock_file"
require "open3"
root = Pathname(ARGV.fetch(0)).realpath
raise "private test prefix required" unless HOMEBREW_PREFIX.realpath.to_s.start_with?(root.to_s + "/")
lock = FormulaLock.new("jq")
lock.lock
begin
  out, err, status = Open3.capture3(HOMEBREW_BREW_FILE.to_s, "install", "--formula", "--force-bottle", "jq")
  raise "ordinary CLI bypassed retained lock" if status.success?
  raise "unexpected CLI failure: #{out} #{err}" unless err.include?("already locked")
  raise "CLI installed despite retained lock" if (HOMEBREW_CELLAR/"jq").exist?
  puts "ordinary CLI cannot inherit a wrapper-held formula lock"
ensure
  lock.unlock
end
`
	if err := writeNew(filepath.Join(root, "cli-lock-probe.rb"), []byte(lockProbe), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := w.native(context.Background(), "cli-lock-probe", profile, "cli-lock-probe.rb"); err != nil {
		log, _ := os.ReadFile(filepath.Join(root, "cli-lock-probe.stderr"))
		t.Fatalf("%v: %s", err, log)
	}
	t.Log("native CLI requires credentials and cannot inherit retained formula locks")
	doc := nativeInputs{Schema: 1, Recipes: recipes, Candidates: candidates, Targets: []string{"jq"}, Operation: "install"}
	data, _ := json.Marshal(doc)
	if err := writeNew(filepath.Join(root, "native-inputs.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	inspected, err := w.native(context.Background(), "inspect", profile, "inspect.rb")
	if err != nil {
		log, _ := os.ReadFile(filepath.Join(root, "inspect.stderr"))
		t.Fatalf("%v: %s", err, log)
	}
	var inspectedDoc inspectionDocument
	if err := decodeStrict(inspected, &inspectedDoc); err != nil {
		t.Fatal(err)
	}
	if inspectedDoc.Schema != 1 || len(inspectedDoc.Candidates) != 2 || !inspectedDoc.Candidates[0].UnmodifiedSource || !inspectedDoc.Candidates[1].UnmodifiedSource {
		t.Fatal("source or closure inspection failed", inspectedDoc)
	}
	t.Log("inspected authenticated current and embedded recipes with exact OCI closure", digestBytes(inspected))
	contract := `require_relative "source"
require_relative "candidate"
root = Pathname(ARGV.fetch(0))
text = (root/"runtime/brew/Library/Taps/homebrew/homebrew-core/Formula/j/jq.rb").read
raise "baseline unrecognized" unless BrewWardenSource.reviewed?(text)
raise "source coordinates affected identity" unless BrewWardenSource.reviewed?("\n# Additional metadata comment\n" + text)
raise "new version changed build identity" unless BrewWardenSource.reviewed?(text.gsub("1.8.2", "1.8.3"))
raise "build transformation accepted" if BrewWardenSource.reviewed?(text.sub('system "make", "install"', 'system "patch", "install"'))
raise "shell expression accepted" if BrewWardenSource.reviewed?(text.sub('"--disable-maintainer-mode"', '"--prefix=$(touch source.c)"'))
raise "ambiguous method accepted" if BrewWardenSource.reviewed?(text + "\nclass Other; def install; end; end\n")
raise "invalid syntax accepted" if BrewWardenSource.reviewed?(text + "\ndef")
candidate = BrewWardenCandidate.new(root)
formula = candidate.formulae.fetch("jq")
candidate.validate_install_behavior(formula)
formula = Class.new(formula.class).allocate
formula.class.post_install_steps { mkdir_p "brewwarden-must-not-run", base: :var }
begin
  candidate.validate_install_behavior(formula)
  raise "declarative post-install accepted"
rescue RuntimeError => error
  raise unless error.message == "unsupported post-install or service"
end
formula.class.instance_variable_set(:@post_install_steps_defined, false)
formula.class.service { run ["/usr/bin/false"] }
begin
  candidate.validate_install_behavior(formula)
  raise "service accepted"
rescue RuntimeError => error
  raise unless error.message == "unsupported post-install or service"
end
formula.class.instance_variable_set(:@service_block, nil)
formula.class.link_overwrite "bin/unplanned"
begin
  candidate.validate_install_behavior(formula)
  raise "shared link replacement accepted"
rescue RuntimeError => error
  raise unless error.message == "unsupported shared link replacement"
end
require "formula_installer"
require_relative "installer_guard"
$brewwarden_session = Struct.new(:actions, :candidate).new([{name: "jq", operation: "install"}], candidate)
FormulaInstaller.prepend(BrewWardenInstallerGuard)
installer = FormulaInstaller.new(candidate.formulae.fetch("jq"), force_bottle: true)
begin
  installer.build
  raise "source build guard bypassed"
rescue RuntimeError => error
  raise unless error.message == "native source fallback prohibited"
end
begin
  FormulaInstaller.new(candidate.formulae.fetch("oniguruma"), force_bottle: true)
  raise "unplanned dependency installer accepted"
rescue RuntimeError => error
  raise unless error.message == "unplanned native installer"
end
puts '{"sourceContract":"passed","installationHooks":"rejected"}'
`
	if err := writeNew(filepath.Join(root, "source-contract.rb"), []byte(contract), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := w.native(context.Background(), "source-contract", profile, "source-contract.rb"); err != nil {
		log, _ := os.ReadFile(filepath.Join(root, "source-contract.stderr"))
		t.Fatalf("%v: %s", err, log)
	}

}
