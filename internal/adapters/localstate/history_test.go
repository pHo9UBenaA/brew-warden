package localstate

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"brewwarden/internal/domain"
)

func TestHistoryIsImmutableAndNonOverwriting(t *testing.T) {
	f := Files{StatePath: filepath.Join(t.TempDir(), "state")}
	if records, err := f.History(); err != nil || len(records) != 0 {
		t.Fatal(records, err)
	}
	if _, err := os.Stat(f.StatePath); !os.IsNotExist(err) {
		t.Fatal("history read created state")
	}
	first, err := f.RecordRefusal("install", []string{"wget"}, domain.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.RecordRefusal("install", []string{"wget"}, domain.DefaultPolicy())
	if err != nil || first == second {
		t.Fatal("repeated request overwrote history", err)
	}
	records, err := f.History()
	if err != nil || len(records) != 2 {
		t.Fatal(records, err)
	}
	for _, r := range records {
		if !r.Refusal.Valid() || r.Refusal.ReasonCode != "execution_binding_unverified" {
			t.Fatal(r)
		}
	}
	path := filepath.Join(f.StatePath, string(first)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "wget", "evil", 1))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.History(); err == nil {
		t.Fatal("tampered history accepted")
	}
	if _, err := f.RecordRefusal("upgrade", nil, domain.DefaultPolicy()); err == nil {
		t.Fatal("continued corrupt journal")
	}
}

func TestHistoryWriterLockAndIncompleteFile(t *testing.T) {
	f := Files{StatePath: filepath.Join(t.TempDir(), "state")}
	root, err := f.openHistory(true)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	lock, err := openPrivate(root, ".writer.lock", os.O_CREATE|os.O_RDWR)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	if _, err := f.RecordRefusal("upgrade", nil, domain.DefaultPolicy()); err == nil {
		t.Fatal("concurrent writer accepted")
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.StatePath, ".pending-interrupted"), []byte("{incomplete"), 0o600); err != nil {
		t.Fatal(err)
	}
	if records, err := f.History(); err != nil || len(records) != 0 {
		t.Fatal("uncommitted refusal treated as completed", records, err)
	}
	if _, err := f.RecordRefusal("upgrade", nil, domain.DefaultPolicy()); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryRejectsUnsafePathsAndSchemas(t *testing.T) {
	for _, mode := range []string{"symlink-directory", "public-directory", "symlink-record", "public-record", "missing-field", "duplicate-field"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			f := Files{StatePath: filepath.Join(dir, "state")}
			id, err := f.RecordRefusal("upgrade", nil, domain.DefaultPolicy())
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(f.StatePath, string(id)+".json")
			switch mode {
			case "symlink-directory":
				moved := filepath.Join(dir, "other")
				if err := os.Rename(f.StatePath, moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(moved, f.StatePath); err != nil {
					t.Fatal(err)
				}
			case "public-directory":
				if err := os.Chmod(f.StatePath, 0o755); err != nil {
					t.Fatal(err)
				}
			case "symlink-record":
				outside := filepath.Join(dir, "outside")
				if err := os.Rename(path, outside); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, path); err != nil {
					t.Fatal(err)
				}
			case "public-record":
				if err := os.Chmod(path, 0o644); err != nil {
					t.Fatal(err)
				}
			default:
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "missing-field" {
					data = []byte(strings.Replace(string(data), `,"minimumAgeSeconds":604800`, "", 1))
				} else {
					data = []byte(strings.Replace(string(data), `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1))
				}
				sum := sha256.Sum256(data)
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(f.StatePath, hex.EncodeToString(sum[:])+".json"), data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.History(); err == nil {
				t.Fatal("unsafe history accepted")
			}
		})
	}
}

func TestImmutableWriteNeverReplaces(t *testing.T) {
	f := Files{StatePath: filepath.Join(t.TempDir(), "state")}
	root, err := f.openHistory(true)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	data := []byte("test bytes\n")
	id, err := writeImmutable(root, data, "one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeImmutable(root, data, "two"); err == nil {
		t.Fatal("existing record replaced")
	}
	if read, err := os.ReadFile(filepath.Join(f.StatePath, string(id)+".json")); err != nil || string(read) != string(data) {
		t.Fatal("existing bytes changed", err)
	}
}
