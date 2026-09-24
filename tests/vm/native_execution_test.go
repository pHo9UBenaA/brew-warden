//go:build vmacceptance

package vm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/adapters/homebrew"
	"github.com/pHo9UBenaA/brew-warden/internal/application"
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

type nativeClock struct{}

func (nativeClock) Now() int64 { return time.Now().Unix() }
func kegSnapshot(root string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s %d\n", relative, info.Mode())
		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			_, err = io.Copy(h, file)
			closeErr := file.Close()
			if err != nil {
				return err
			}
			return closeErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			fmt.Fprintln(h, link)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func TestLiveNativeExecution(t *testing.T) {
	source := os.Getenv("BREWWARDEN_VM_RUNTIME")
	if source == "" {
		t.Skip("requires explicitly provisioned disposable macOS VM")
	}
	model, err := exec.Command("/usr/sbin/sysctl", "-n", "hw.model").Output()
	if err != nil || !strings.HasPrefix(string(model), "VirtualMac") {
		t.Fatal("native execution tests require a disposable macOS VM")
	}
	operation := os.Getenv("BREWWARDEN_VM_OPERATION")
	if operation == "" {
		operation = "install"
	}
	if operation != "install" && operation != "upgrade" {
		t.Fatal("unsupported VM operation")
	}
	directory, err := os.MkdirTemp(filepath.Dir(source), "live-execution-")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("retained execution", directory)
	dep := "/opt/homebrew/Cellar/oniguruma/6.9.10"
	before, err := kegSnapshot(dep)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	collector := homebrew.Collector{Runtime: homebrew.Runtime{}, Directory: directory, BottleVerifier: installedBottleVerifier(t)}
	collection, err := collector.Collect(context.Background(), ports.Request{Operation: operation, Targets: []string{"jq"}}, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	fault := os.Getenv("BREWWARDEN_VM_FAULT")
	switch fault {
	case "", "age", "age-exception", "changed-input", "exception-changed-input", "link-conflict":
	default:
		t.Fatal("unsupported VM fault")
	}
	policy := domain.DefaultPolicy()
	waivers := []domain.AgeWaiver{}
	if fault == "age" || fault == "age-exception" || fault == "exception-changed-input" {
		policy, err = domain.NewPolicy(9223372036)
		if err != nil {
			t.Fatal(err)
		}
		if fault == "age-exception" || fault == "exception-changed-input" {
			for _, node := range collection.Evidence() {
				waivers = append(waivers, domain.AgeWaiver{Artifact: node.Artifact, Reason: "Explicit VM acceptance exception"})
			}
		}
	}
	if fault == "link-conflict" {
		if _, err := os.Lstat("/opt/homebrew/Cellar/jq"); !os.IsNotExist(err) {
			t.Fatal("link-conflict test requires absent jq")
		}
		if _, err := os.Lstat("/opt/homebrew/Cellar/oniguruma"); !os.IsNotExist(err) {
			t.Fatal("link-conflict test requires absent dependency")
		}
		file, err := os.OpenFile("/opt/homebrew/bin/jq", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.WriteString("owned acceptance conflict\n"); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		defer os.Remove("/opt/homebrew/bin/jq")
	}
	completeBefore, err := kegSnapshot("/opt/homebrew/Cellar")
	if err != nil {
		t.Fatal(err)
	}
	prepared, session, err := collection.Prepare(context.Background(), policy, waivers, time.Now().Unix(), homebrew.Streams{In: os.Stdin, Out: os.Stdout, Err: os.Stderr})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if fault == "changed-input" || fault == "exception-changed-input" {
		files, err := filepath.Glob(filepath.Join(directory, "collection-*", "inputs", "jq--*.tar.gz"))
		if err != nil || len(files) != 1 {
			t.Fatal("missing candidate fixture", files, err)
		}
		file, err := os.OpenFile(files[0], os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte("changed")); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	result, err := application.Execute(context.Background(), prepared, session, nativeClock{})
	if fault == "link-conflict" {
		if err == nil || result.Outcome != domain.AttemptPartial || !result.ExitKnown || result.ExitCode == 0 {
			t.Fatal("partial native failure was misclassified", result, err)
		}
		raw, readErr := os.ReadFile("/opt/homebrew/bin/jq")
		if readErr != nil || string(raw) != "owned acceptance conflict\n" {
			t.Fatal("existing shared file replaced", readErr)
		}
		if _, err := os.Stat("/opt/homebrew/Cellar/oniguruma/6.9.10"); err != nil {
			t.Fatal("did not exercise partial dependency installation", err)
		}
		partial, err := kegSnapshot("/opt/homebrew/Cellar")
		if err != nil {
			t.Fatal(err)
		}
		// A new invocation must not call an opt-linked partial pour a
		// successful installation. The user can repair it with Homebrew while
		// BrewWarden is idle, then retry with fresh verification.
		engine := homebrew.Engine{Collector: &collector}
		fresh, next, err := engine.Prepare(context.Background(), ports.Request{Operation: "install", Targets: []string{"jq"}}, domain.DefaultPolicy(), nil, time.Now().Unix())
		if err == nil {
			_ = next.Close()
			t.Fatal("partial link was reported as a fresh successful install", fresh)
		}
		if !strings.Contains(err.Error(), "link step is incomplete") {
			t.Fatal("partial link held for the wrong reason", err)
		}
		if after, err := kegSnapshot("/opt/homebrew/Cellar"); err != nil || after != partial {
			t.Fatal("held fresh retry mutated the partial prefix", err)
		}
		t.Log("partial installation retained; incomplete link prevents false success on fresh retry")
		return
	}
	if fault == "changed-input" || fault == "exception-changed-input" || fault == "age" {
		if err == nil {
			t.Fatal("held request executed", fault)
		}
		after, stateErr := kegSnapshot("/opt/homebrew/Cellar")
		if stateErr != nil || after != completeBefore {
			t.Fatal("held request changed prefix", stateErr)
		}
		t.Log("verified mutation refusal", fault)
		return
	}
	if err != nil {
		t.Fatalf("execution failed: %+v %v", result, err)
	}
	if result.Outcome != domain.AttemptSucceeded || !result.ExitKnown || result.ExitCode != 0 {
		t.Fatal(result)
	}
	if before != "" {
		after, err := kegSnapshot(dep)
		if err != nil || before != after {
			t.Fatal("existing dependency changed", before, after, err)
		}
	}
	installed, err := exec.Command("/opt/homebrew/bin/jq", "--version").Output()
	if err != nil || strings.TrimSpace(string(installed)) != "jq-"+collection.Evidence()[0].Artifact.Version {
		t.Fatal("installed executable mismatch", string(installed), err)
	}
	t.Log("verified native execution", operation, prepared.Assessment.Binding.Plan, result.Outcome)
}
