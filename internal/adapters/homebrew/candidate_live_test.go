package homebrew

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLiveHistoricalBottleDependencies(t *testing.T) {
	source := os.Getenv("BREWWARDEN_LIVE_RUNTIME")
	if source == "" {
		t.Skip("requires explicitly built native runtime")
	}
	manifest, err := os.ReadFile(filepath.Join(source, "manifest.json"))
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
	if _, err := (Runtime{source, digestBytes(manifest)}).materialize(filepath.Join(root, "runtime")); err != nil {
		t.Fatal(err)
	}
	profile, err := w.sandbox("dependencies", false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	script := `require_relative "candidate"
subject = BrewWardenCandidate.allocate
subject.instance_variable_set(:@items, {"ripgrep" => {"dependencies" => ["pcre2"]}, "pcre2" => {"dependencies" => ["support"]}, "support" => {"dependencies" => []}})
subject.instance_variable_set(:@formulae, {"pcre2" => Struct.new(:pkg_version).new(PkgVersion.parse("10.48")), "support" => Struct.new(:pkg_version).new(PkgVersion.parse("1.2"))})
historical = [{"full_name" => "pcre2", "version" => "10.47", "revision" => 1}, {"full_name" => "support", "version" => "1.1", "revision" => 0}]
subject.validate_runtime_dependencies("ripgrep", historical)
{
  "missing transitive dependency" => historical.take(1),
  "extra dependency" => historical + [{"full_name" => "unexpected", "version" => "1", "revision" => 0}],
  "duplicate dependency" => historical + historical.take(1),
  "newer required version" => [historical[0].merge("version" => "10.49")] + historical.drop(1),
  "missing revision" => [historical[0].reject { |key, _| key == "revision" }] + historical.drop(1),
}.each do |name, dependencies|
  refused = false
  begin
    subject.validate_runtime_dependencies("ripgrep", dependencies)
  rescue RuntimeError, KeyError
    refused = true
  end
  raise "accepted #{name}" unless refused
end
puts "historical versions and complete runtime closure checked"
`
	if err := writeNew(filepath.Join(root, "dependency-test.rb"), []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := w.invoke(context.Background(), "dependency-test", profile, "ruby", filepath.Join(root, "dependency-test.rb")); err != nil {
		diagnostic, _ := os.ReadFile(filepath.Join(root, "dependency-test.stderr"))
		t.Fatalf("%v: %s", err, diagnostic)
	}
}
