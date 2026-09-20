package homebrew

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Exercise the maintained parser in the actual product runtime. This does not
// execute recipes or permit writes to the host Homebrew prefix.
func TestLiveNativeBuildInspection(t *testing.T) {
	source := os.Getenv("BREWWARDEN_LIVE_RUNTIME")
	if source == "" {
		t.Skip("requires explicitly built native runtime")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("requires Apple Silicon macOS")
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
	if err := writeNew(filepath.Join(root, "source-contract.rb"), []byte(sourceBuildContract), 0600); err != nil {
		t.Fatal(err)
	}
	profile, err := w.sandbox("source-contract", false, false, []string{filepath.Join(root, "runtime")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.native(context.Background(), "source-contract", profile, "source-contract.rb"); err != nil {
		log, _ := os.ReadFile(filepath.Join(root, "source-contract.stderr"))
		t.Fatalf("%v: %s", err, log)
	}
}

const sourceBuildContract = `require_relative "source"
def recipe(body)
  "class ArbitraryName < Formula\n  def install\n#{body}\n  end\nend\n"
end
accepted = [
 'system "./configure", *std_configure_args; system "make", "install"',
 'system "autoreconf", "--force", "--install", "--verbose"; system "./configure", "--prefix=#{prefix}"; system "make"; system "make", "install"',
 'system "autoreconf", "--install" if build.head?; system "./configure", *std_configure_args; system "make", "install"',
 'args = %W[-DCARES_STATIC=ON -DCARES_SHARED=ON -DCMAKE_INSTALL_RPATH=#{rpath}]; system "cmake", "-S", ".", "-B", "build", *args, *std_cmake_args; system "cmake", "--build", "build"; system "cmake", "--install", "build"',
 'args = ["-DFOO=ON", "-DBAR:BOOL=OFF"]; system "cmake", "-S", ".", "-B", "build", *std_cmake_args, *args; system "cmake", "--install", "build"',
]
accepted.each do |body|
 raise "ordinary build rejected: #{body}" unless BrewWardenSource.reviewed?(recipe(body))
end
rejected = [
 '', 'args = ["unused"]',
 'system "patch", "-p1"',
 'system "sh", "-c", "patch source.c"',
 'system "make", "--eval=source.c:; touch source.c"',
 'system "cmake", "-P", "patch.cmake"',
 'system "cmake", "-E", "copy", "patched.c", "source.c"',
 'system "cmake", "-S", ".", "-B", "build", "-DCMAKE_PROJECT_INCLUDE=patch.cmake"',
 'system "./configure", "--prefix=$(touch source.c)"',
 'inreplace "source.c", "unsafe", "fixed"; system "make"',
 'File.write("source.c", "replacement"); system "make"',
 'system "make", "#{patch_source}"',
 'args = patch_source; system "make", *args',
 'system "make", *unknown_args',
 'system "make", *["all", ["nested"]]',
 'system "make", *std_cmake_args',
 'system "make"; rescue; system "patch"',
 'system "patch" unless build.head?',
 'system "patch" if build.stable?',
 'system "patch" if custom.head?',
 'build = "head"; system "patch" if build.head?; system "make"',
 'args = ["all"]; args = ["install"]; system "make", *args',
 'send(:system, "patch"); system "make"',
 'system "make" do; patch_source; end',
 'def hidden; end; system "make"',
]
rejected.each do |body|
 raise "unsupported build accepted: #{body}" if BrewWardenSource.reviewed?(recipe(body))
end
raise "oversized arguments accepted" if BrewWardenSource.reviewed?(recipe('system "make", *[' + ([ '"all"' ] * 257).join(',') + ']'))
raise "oversized string accepted" if BrewWardenSource.reviewed?(recipe('system "./configure", "--prefix=' + ('a' * 4096) + '"'))
raise "oversized body accepted" if BrewWardenSource.reviewed?(recipe(([ 'system "make"' ] * 129).join(';')))
raise "duplicate definition accepted" if BrewWardenSource.reviewed?(recipe('system "make"') + recipe('system "make"'))
raise "invalid syntax accepted" if BrewWardenSource.reviewed?(recipe('system "make"') + "def")

puts "#{accepted.length} supported builds, #{rejected.length + 5} rejected cases, native parser passed"
`
