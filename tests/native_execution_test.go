package tests

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
	"github.com/pHo9UBenaA/brew-warden/internal/adapters/localstate"
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
	case "", "age", "age-exception", "changed-input", "exception-changed-input", "recovery", "link-conflict":
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
	journal := localstate.Journal{Path: filepath.Join(directory, "attempts")}
	if fault == "recovery" {
		if err := journal.StartAttempt(domain.AttemptStart{Binding: prepared.Assessment.Binding, BeforeState: prepared.BeforeState, StartedAt: time.Now().Unix()}); err != nil {
			t.Fatal(err)
		}
		engine := homebrew.Engine{Collector: &collector}
		service := application.Service{Journal: journal, Recovery: engine, Clock: nativeClock{}}
		if err := service.Reconcile(context.Background(), prepared.Assessment.Binding.Attempt); err == nil {
			t.Fatal("reconciled while original session still held its operation lock")
		}
		if err := session.Close(); err != nil {
			t.Fatal(err)
		}
		if err := service.Reconcile(context.Background(), prepared.Assessment.Binding.Attempt); err != nil {
			t.Fatal(err)
		}
		records, err := journal.Attempts()
		if err != nil || len(records) != 1 || records[0].Unresolved() || records[0].Finish.Outcome != domain.AttemptReconciled || records[0].Finish.ExitKnown {
			t.Fatal(records, err)
		}
		after, err := kegSnapshot("/opt/homebrew/Cellar")
		if err != nil || after != completeBefore {
			t.Fatal("recovery changed packages", err)
		}
		t.Log("BrewWarden active-lock refusal and stopped-session reconciliation verified")
		return
	}
	result, err := application.Execute(context.Background(), prepared, session, journal, nativeClock{})
	if fault == "link-conflict" {
		if err == nil || result.Outcome != domain.AttemptPartial || !result.ExitKnown || result.ExitCode == 0 {
			t.Fatal("partial native failure was misclassified", result, err)
		}
		raw, readErr := os.ReadFile("/opt/homebrew/bin/jq")
		if readErr != nil || string(raw) != "owned acceptance conflict\n" {
			t.Fatal("existing shared file replaced", readErr)
		}
		records, readErr := journal.Attempts()
		if readErr != nil || len(records) != 1 || records[0].Finish.Outcome != domain.AttemptPartial || !records[0].Finish.AfterState.Valid() {
			t.Fatal(records, readErr)
		}
		if _, err := os.Stat("/opt/homebrew/Cellar/oniguruma/6.9.10"); err != nil {
			t.Fatal("did not exercise partial dependency installation", err)
		}
		t.Log("partial installation retained, existing link conflict preserved, actual state recorded")
		return
	}
	if fault == "changed-input" || fault == "exception-changed-input" || fault == "age" {
		if err == nil {
			t.Fatal("held request executed", fault)
		}
		attempts, readErr := journal.Attempts()
		if readErr != nil || len(attempts) != 0 {
			t.Fatal("held request started", attempts, readErr)
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
	attempts, err := journal.Attempts()
	if err != nil || len(attempts) != 1 || attempts[0].Unresolved() {
		t.Fatal(attempts, err)
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
