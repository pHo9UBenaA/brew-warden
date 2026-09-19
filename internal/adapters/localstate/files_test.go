package localstate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigurationFileBoundary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	f := Files{ConfigPath: path}
	if p, err := f.LoadConfig(""); err != nil || !p.Valid() {
		t.Fatal("missing optional config must use defaults", err)
	}
	if _, err := f.LoadConfig(path); err == nil {
		t.Fatal("explicit missing config ignored")
	}
	if err := os.WriteFile(path, []byte(`{"schemaVersion":1,"age":{"minimumHours":24}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if p, err := f.LoadConfig(""); err != nil || p.MinimumAgeSeconds() != 86400 {
		t.Fatal("config not loaded", err)
	}
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	if _, err := f.LoadConfig(""); err == nil {
		t.Fatal("writable config accepted")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := f.LoadConfig(link); err == nil {
		t.Fatal("symlink config accepted")
	}
	if _, err := f.LoadConfig(dir); err == nil {
		t.Fatal("directory config accepted")
	}
}
