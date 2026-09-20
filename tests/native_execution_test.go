package tests

import (
	"bytes"
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

	"brewwarden/internal/adapters/attestation"
	"brewwarden/internal/adapters/githubrelease"
	"brewwarden/internal/adapters/homebrew"
	"brewwarden/internal/adapters/localstate"
	"brewwarden/internal/adapters/osv"
	"brewwarden/internal/application"
	"brewwarden/internal/domain"
	"brewwarden/internal/ports"
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
	raw, err := os.ReadFile(filepath.Join(source, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
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
	collector := homebrew.Collector{Runtime: homebrew.Runtime{Root: source, ManifestSHA256: domain.Digest(hex.EncodeToString(sum[:]))}, Directory: directory, Publication: githubrelease.New(), Vulnerabilities: osv.New(), Verifier: func(path string, digest domain.Digest) ports.ProvenanceVerifier {
		return attestation.Verifier{Path: path, SHA256: digest}
	}}
	collection, err := collector.Collect(context.Background(), ports.Request{Operation: operation, Targets: []string{"jq"}}, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	fault := os.Getenv("BREWWARDEN_VM_FAULT")
	switch fault {
	case "", "age", "age-exception", "changed-input", "exception-changed-input", "affected-dependent":
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
	if fault == "affected-dependent" {
		rack := "/opt/homebrew/Cellar/brewwarden-acceptance-dependent"
		if err := os.Mkdir(rack, 0755); err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(rack)
		keg := filepath.Join(rack, "0.0.1")
		if err := os.Mkdir(keg, 0755); err != nil {
			t.Fatal(err)
		}
		receipt := []byte(`{"runtime_dependencies":[{"full_name":"jq","version":"1.8.1","revision":0}]}`)
		if err := os.WriteFile(filepath.Join(keg, "INSTALL_RECEIPT.json"), receipt, 0644); err != nil {
			t.Fatal(err)
		}
	}
	var nativeErrors bytes.Buffer
	completeBefore, err := kegSnapshot("/opt/homebrew/Cellar")
	if err != nil {
		t.Fatal(err)
	}
	prepared, session, err := collection.Prepare(context.Background(), policy, waivers, time.Now().Unix(), homebrew.Streams{In: os.Stdin, Out: os.Stdout, Err: io.MultiWriter(os.Stderr, &nativeErrors)})
	if fault == "affected-dependent" {
		if err == nil {
			_ = session.Close()
			t.Fatal("unverified affected dependent accepted")
		}
		if !strings.Contains(nativeErrors.String(), "affected installed dependent requires a verified plan") {
			t.Fatal("wrong dependent failure", err, nativeErrors.String())
		}
		after, stateErr := kegSnapshot("/opt/homebrew/Cellar")
		if stateErr != nil || after != completeBefore {
			t.Fatal("held dependent changed prefix", stateErr)
		}
		t.Log("verified affected-dependent refusal")
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	contender := exec.Command("/opt/homebrew/bin/brew", "ruby", "-e", `require "lock_file"; FormulaLock.new("jq").lock; puts "UNEXPECTED_LOCK"`)
	contender.Env = []string{"HOME=" + directory, "PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ANALYTICS=1", "HOMEBREW_NO_INSTALL_FROM_API=1", "HOMEBREW_DEVELOPER=1", "HOMEBREW_NO_BOOTSNAP=1"}
	output, lockErr := contender.CombinedOutput()
	if lockErr == nil || !strings.Contains(string(output), "already locked") {
		t.Fatalf("native lock not retained: %v %s", lockErr, output)
	}
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
	result, err := application.Execute(context.Background(), prepared, session, journal, nativeClock{})
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
