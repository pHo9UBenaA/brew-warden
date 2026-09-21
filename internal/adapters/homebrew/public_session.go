package homebrew

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

// It coordinates only BrewWarden operations, not independent Homebrew clients.
type publicSession struct {
	w        workspace
	prepared ports.Prepared
	plan     executionPlan
	streams  Streams
	profile  string
	lock     *os.File
	consumed bool
}
type installedFormula struct {
	Name      string             `json:"name" required:"true"`
	Pinned    bool               `json:"pinned" required:"true"`
	Outdated  bool               `json:"outdated" required:"true"`
	KegOnly   bool               `json:"keg_only" required:"true"`
	LinkedKeg *string            `json:"active_version,omitempty"`
	Installed []installedVersion `json:"installed" required:"true"`
}
type installedVersion struct {
	Version       string                `json:"version" required:"true"`
	Options       []string              `json:"used_options" required:"true"`
	Poured        bool                  `json:"poured_from_bottle" required:"true"`
	Built         bool                  `json:"built_as_bottle" required:"true"`
	Time          int64                 `json:"time" required:"true"`
	Dependencies  []installedDependency `json:"runtime_dependencies" required:"true"`
	ReceiptSHA256 domain.Digest         `json:"receipt_sha256,omitempty"`
}
type installedDependency struct {
	Name     string `json:"full_name" required:"true"`
	Version  string `json:"version" required:"true"`
	Revision int    `json:"revision" required:"true"`
}

func publicBrewCommand(ctx context.Context, w workspace, profile string, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, "/usr/bin/sandbox-exec", append([]string{"-f", profile, "/opt/homebrew/bin/brew"}, args...)...)
	command.Env = w.environment()
	command.Env = append(command.Env, "HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1", "HOMEBREW_NO_PATH_SHADOW_CHECK=1", "HOMEBREW_NO_ASK=1")
	command.Dir = w.root
	command.WaitDelay = 2 * time.Second
	return command
}
func (w workspace) publicState(ctx context.Context, profile string, nodes []domain.Node) ([]installedFormula, domain.Digest, error) {
	names := make([]string, 0, len(nodes))
	args := []string{"info", "--json=v2", "--formula"}
	for _, node := range nodes {
		names = append(names, node.Artifact.Name)
		args = append(args, "homebrew/core/"+node.Artifact.Name)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	command := publicBrewCommand(ctx, w, profile, args...)
	output, diagnostics := &processOutput{}, &processOutput{}
	command.Stdout, command.Stderr = output, diagnostics
	if err := command.Run(); err != nil || output.overflow || diagnostics.overflow {
		return nil, "", errors.New("public Homebrew state inspection failed")
	}
	metadata, err := parseInfo(output.Bytes(), names)
	if err != nil {
		return nil, "", err
	}
	for _, f := range metadata {
		i := slices.Index(names, f.Name)
		if f.artifact() != nodes[i].Artifact {
			return nil, "", errors.New("execution metadata differs from verified candidate")
		}
		dependencies := make([]string, 0, len(nodes[i].Dependencies))
		for _, dep := range nodes[i].Dependencies {
			dependencies = append(dependencies, dep.Name)
		}
		slices.Sort(dependencies)
		slices.Sort(f.Dependencies)
		if !slices.Equal(dependencies, f.Dependencies) {
			return nil, "", errors.New("execution dependency graph changed")
		}
	}
	var document struct {
		Formulae []installedFormula `json:"formulae" required:"true"`
	}
	if err := decodeSchema(output.Bytes(), &document, true); err != nil {
		return nil, "", err
	}
	slices.SortFunc(document.Formulae, func(a, b installedFormula) int { return strings.Compare(a.Name, b.Name) })
	for i := range document.Formulae {
		formula := &document.Formulae[i]
		// Public info represents an absent linked_keg as null. Observe the actual
		// opt link instead; it also identifies active keg-only installations.
		formula.LinkedKeg = nil
		opt := filepath.Join("/opt/homebrew/opt", formula.Name)
		if _, err := os.Lstat(opt); err == nil {
			resolved, err := filepath.EvalSymlinks(opt)
			if err != nil || filepath.Dir(resolved) != filepath.Join("/opt/homebrew/Cellar", formula.Name) {
				return nil, "", errors.New("active keg escapes candidate rack")
			}
			active := filepath.Base(resolved)
			formula.LinkedKeg = &active
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, "", err
		}
		for j := range formula.Installed {
			version := &formula.Installed[j]
			identity := nodes[slices.Index(names, formula.Name)].Artifact
			identity.Version = version.Version
			if !identity.Valid() {
				return nil, "", errors.New("invalid installed version path")
			}
			keg := filepath.Join("/opt/homebrew/Cellar", formula.Name, version.Version)
			resolved, err := filepath.EvalSymlinks(keg)
			if err != nil || resolved != keg {
				return nil, "", errors.New("installed keg escapes expected prefix")
			}
			raw, err := readRegular(filepath.Join(keg, "INSTALL_RECEIPT.json"), 1024*1024)
			if err != nil {
				return nil, "", err
			}
			var receipt struct {
				Arch   string `json:"arch" required:"true"`
				Source struct {
					Tap  string `json:"tap" required:"true"`
					Spec string `json:"spec" required:"true"`
				} `json:"source" required:"true"`
			}
			if err := decodeSchema(raw, &receipt, true); err != nil || receipt.Arch != "arm64" || receipt.Source.Tap != "homebrew/core" || receipt.Source.Spec != "stable" {
				return nil, "", errors.New("installed receipt is not official core stable")
			}
			version.ReceiptSHA256 = digestBytes(raw)
		}
	}
	raw, err := json.Marshal(document.Formulae)
	if err != nil {
		return nil, "", err
	}
	digest := digestBytes(raw)
	destination := filepath.Join(w.root, "states", string(digest)+".json")
	if old, err := readRegular(destination, maxManifest); err == nil {
		if digestBytes(old) != digest {
			return nil, "", errors.New("saved installed observation changed")
		}
	} else if err := writeRecord(destination, raw); err != nil {
		return nil, "", err
	}
	return document.Formulae, digest, nil
}
func publicActions(states []installedFormula, nodes []domain.Node, request collectionInputs) ([]plannedAction, error) {
	if len(states) != len(nodes) {
		return nil, errors.New("incomplete installed state")
	}
	result := make([]plannedAction, 0, len(nodes))
	for i, node := range nodes {
		state := states[i]
		if state.Name != node.Artifact.Name || state.Pinned || len(state.Installed) > 32 {
			return nil, errors.New("pinned or ambiguous installed candidate")
		}
		action := "install"
		if len(state.Installed) == 0 {
			if request.Operation == "upgrade" && slices.Contains(request.Targets, state.Name) {
				return nil, errors.New("upgrade target is not installed")
			}
		} else {
			wanted := node.Artifact.Version
			if node.Artifact.Revision > 0 {
				wanted += "_" + strconv.Itoa(node.Artifact.Revision)
			}
			selected := ""
			if state.LinkedKeg != nil {
				selected = *state.LinkedKeg
			}
			if selected == "" {
				return nil, errors.New("unlinked installed candidate requires reconciliation")
			}
			found := false
			for _, version := range state.Installed {
				if version.Version != selected {
					continue
				}
				found = true
				if len(version.Options) != 0 || !version.Poured || !version.Built {
					return nil, errors.New("installed candidate is not a supported bottle")
				}
				// These public receipt flags describe recorded installation state.
				// They do not establish the digest of an already installed payload.
				if version.Version == wanted {
					action = "keep"
				} else if state.Outdated {
					action = "upgrade"
				} else {
					return nil, errors.New("candidate downgrade or unmatched installed version")
				}
			}
			if !found {
				return nil, errors.New("active installed version is absent from public info")
			}
		}
		result = append(result, plannedAction{Name: state.Name, Operation: action})
	}
	return result, nil
}
func acquireOperationLock(directory string) (*os.File, error) {
	file, err := os.OpenFile(filepath.Join(directory, "execution.lock"), os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		file.Close()
		return nil, errors.New("execution lock must be a private regular file")
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, errors.New("another BrewWarden execution is active")
	}
	return file, nil
}

// Compare the actual prefix's executable source with the reviewed inspection
// runtime. Extra installed formulae/taps are not an executable source inventory.
func (w workspace) checkPublicRuntime() error {
	source := filepath.Join(w.root, "runtime/brew")
	for _, directory := range []string{"bin", "Library"} {
		err := filepath.WalkDir(filepath.Join(source, directory), func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}
			if relative == "Library/Taps" {
				return filepath.SkipDir
			}
			if entry.IsDir() {
				return nil
			}
			destination := filepath.Join("/opt/homebrew", relative)
			if entry.Type()&os.ModeSymlink != 0 {
				a, err := os.Readlink(path)
				if err != nil {
					return err
				}
				b, err := os.Readlink(destination)
				if err != nil || a != b {
					return errors.New("installed Homebrew runtime link differs from supported build")
				}
				return nil
			}
			a, err := readRegular(path, 128*1024*1024)
			if err != nil {
				return err
			}
			b, err := readRegular(destination, 128*1024*1024)
			if err != nil || digestBytes(a) != digestBytes(b) {
				return errors.New("installed Homebrew differs from supported build")
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
func (c *Collection) Prepare(ctx context.Context, policy domain.Policy, waivers []domain.AgeWaiver, now int64, streams Streams) (ports.Prepared, ports.ExecutionSession, error) {
	if c == nil || ctx == nil || !policy.Valid() || now < c.observedAt || now >= c.observedAt+3600 || len(c.nodes) == 0 {
		return ports.Prepared{}, nil, errors.New("collection unavailable or expired")
	}
	s := &publicSession{w: workspace{c.root}, streams: streams}
	fail := func(err error) (ports.Prepared, ports.ExecutionSession, error) {
		s.Close()
		return ports.Prepared{}, nil, err
	}
	files, err := s.w.freezeInputs()
	if err != nil || !slices.Equal(files, c.frozen) {
		return fail(errors.New("collected inputs changed before planning"))
	}
	if err := s.w.checkPublicRuntime(); err != nil {
		return fail(err)
	}
	s.lock, err = acquireOperationLock(filepath.Dir(c.root))
	if err != nil {
		return fail(err)
	}
	immutable := []string{filepath.Join(s.w.root, "runtime"), filepath.Join(s.w.root, "cache/api"), filepath.Join(s.w.root, "cache/downloads"), filepath.Join(s.w.root, "plan.json")}
	for _, file := range files {
		immutable = append(immutable, filepath.Join(s.w.root, file.Path))
	}
	s.profile, err = s.w.sandbox("public-execution", false, true, immutable)
	if err != nil {
		return fail(err)
	}
	states, before, err := s.w.publicState(ctx, s.profile, c.nodes)
	if err != nil {
		return fail(err)
	}
	actions, err := publicActions(states, c.nodes, c.inputs)
	if err != nil {
		return fail(err)
	}
	osCommand := exec.CommandContext(ctx, "/usr/bin/sw_vers", "-productVersion")
	osVersion, err := osCommand.Output()
	if err != nil {
		return fail(err)
	}
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return fail(err)
	}
	s.plan = executionPlan{Schema: 3, MinimumAge: policy.MinimumAgeSeconds(), Nodes: c.Evidence(), Actions: actions, BeforeState: before, Environment: executionEnvironment{c.runtimeDigest, strings.TrimSpace(string(osVersion)), "/opt/homebrew"}, Inputs: files, Attempt: domain.Digest(hex.EncodeToString(nonce)), IssuedAt: now, ExpiresAt: min(now+600, c.observedAt+3600), Waivers: append([]domain.AgeWaiver{}, waivers...), Targets: []domain.Artifact{}}
	for _, name := range c.inputs.Targets {
		for _, node := range c.nodes {
			if name == node.Artifact.Name {
				s.plan.Targets = append(s.plan.Targets, node.Artifact)
			}
		}
	}
	raw, err := json.Marshal(s.plan)
	if err != nil {
		return fail(err)
	}
	if err := writeRecord(filepath.Join(c.root, "plan.json"), raw); err != nil {
		return fail(err)
	}
	s.prepared, err = s.plan.prepared(digestBytes(raw))
	if err != nil {
		return fail(err)
	}
	return s.prepared, s, nil
}
func (s *publicSession) Revalidate(ctx context.Context) (ports.Prepared, error) {
	if s.consumed {
		return ports.Prepared{}, errors.New("session already consumed")
	}
	fresh, err := s.w.readPrepared(s.prepared.Assessment.Binding.Plan)
	if err != nil {
		return ports.Prepared{}, err
	}
	_, state, err := s.w.publicState(ctx, s.profile, s.plan.Nodes)
	if err != nil || state != fresh.BeforeState {
		return ports.Prepared{}, errors.New("installed state changed; retry verification")
	}
	return fresh, nil
}
func (s *publicSession) Run(ctx context.Context, binding domain.Binding) (ports.ExecutionResult, error) {
	if s.consumed || binding != s.prepared.Assessment.Binding {
		return ports.ExecutionResult{}, errors.New("session binding mismatch or replay")
	}
	fresh, err := s.Revalidate(ctx)
	s.consumed = true
	if err != nil {
		return ports.ExecutionResult{}, err
	}
	fresh.Assessment.Now = time.Now().Unix()
	if fresh.Assessment.Now < s.plan.IssuedAt || fresh.Assessment.Now >= fresh.ExpiresAt || domain.Evaluate(fresh.Assessment).Outcome != domain.Allow {
		return ports.ExecutionResult{}, errors.New("execution policy or plan expired")
	}
	s.prepared = fresh
	s.plan.Nodes, s.plan.Targets = fresh.Assessment.Nodes, fresh.Assessment.Targets
	remaining := map[string]plannedAction{}
	for _, action := range s.plan.Actions {
		if action.Operation != "keep" {
			remaining[action.Name] = action
		}
	}
	sequence := 0
	for len(remaining) != 0 {
		var selected plannedAction
		for _, node := range s.plan.Nodes {
			action, ok := remaining[node.Artifact.Name]
			if !ok {
				continue
			}
			ready := true
			for _, dependency := range node.Dependencies {
				if _, pending := remaining[dependency.Name]; pending {
					ready = false
				}
			}
			if ready {
				selected = action
				break
			}
		}
		if selected.Name == "" {
			return ports.ExecutionResult{}, errors.New("cyclic execution plan")
		}
		fresh.Assessment.Now = time.Now().Unix()
		if fresh.Assessment.Now < s.plan.IssuedAt || fresh.Assessment.Now >= s.plan.ExpiresAt || domain.Evaluate(fresh.Assessment).Outcome != domain.Allow {
			return ports.ExecutionResult{}, errors.New("execution plan expired")
		}
		args := []string{selected.Operation, "--formula", "--force-bottle"}
		if selected.Operation == "install" && !slices.ContainsFunc(s.plan.Targets, func(a domain.Artifact) bool { return a.Name == selected.Name }) {
			args = append(args, "--as-dependency")
		}
		args = append(args, "homebrew/core/"+selected.Name)
		result, err := s.runCommand(ctx, sequence, args)
		if err != nil || !result.ExitKnown || result.ExitCode != 0 {
			return result, err
		}
		sequence++
		delete(remaining, selected.Name)
	}
	states, after, err := s.w.publicState(ctx, s.profile, s.plan.Nodes)
	if err != nil {
		return ports.ExecutionResult{ExitKnown: true, ExitCode: 0}, err
	}
	actions, err := publicActions(states, s.plan.Nodes, collectionInputs{Operation: "install"})
	matches := err == nil && slices.IndexFunc(actions, func(a plannedAction) bool { return a.Operation != "keep" }) < 0
	return ports.ExecutionResult{ExitKnown: true, ExitCode: 0, AfterState: after, MatchesPlan: matches}, err
}
func (s *publicSession) runCommand(ctx context.Context, sequence int, args []string) (ports.ExecutionResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	gate, release, err := os.Pipe()
	if err != nil {
		return ports.ExecutionResult{}, err
	}
	defer gate.Close()
	defer release.Close()
	base := publicBrewCommand(ctx, s.w, s.profile, args...)
	// Hold the child before exec until its process session is durably recorded.
	// All command arguments remain separate; the shell program is a fixed literal.
	command := exec.CommandContext(ctx, "/bin/sh", append([]string{"-c", `read -r ready <&3 || exit 125; exec 3<&-; exec "$@"`, "brewwarden-exec"}, base.Args...)...)
	command.Env, command.Dir = base.Env, base.Dir
	command.ExtraFiles = []*os.File{gate, s.lock}
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	command.Cancel = func() error { return syscall.Kill(-command.Process.Pid, syscall.SIGINT) }
	command.WaitDelay = 2 * time.Second
	command.Stdin, command.Stdout, command.Stderr = s.streams.In, s.streams.Out, s.streams.Err
	if err := command.Start(); err != nil {
		return ports.ExecutionResult{}, err
	}
	raw, _ := json.Marshal(processRecord{PID: command.Process.Pid, Session: command.Process.Pid})
	if err := writeRecord(filepath.Join(s.w.root, fmt.Sprintf("process-%d.json", sequence)), raw); err != nil {
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		_ = command.Wait()
		return ports.ExecutionResult{}, err
	}
	if _, err := io.WriteString(release, "start\n"); err != nil {
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		_ = command.Wait()
		return ports.ExecutionResult{}, err
	}
	_ = release.Close()
	_ = gate.Close()
	waitErr := command.Wait()
	if command.ProcessState == nil {
		return ports.ExecutionResult{}, errors.New("public command exit state unavailable")
	}
	code := command.ProcessState.ExitCode()
	if status, ok := command.ProcessState.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		code = 128 + int(status.Signal())
	}
	result := ports.ExecutionResult{ExitKnown: code >= 0 && code <= 255, ExitCode: code}
	if active, err := processSessionActive(command.Process.Pid); err != nil || active {
		return result, errors.New("owned subprocesses are still active or unavailable; reconcile before retrying")
	}
	if waitErr != nil {
		observe, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_, result.AfterState, _ = s.w.publicState(observe, s.profile, s.plan.Nodes)
		return result, waitErr
	}
	return result, nil
}
func (s *publicSession) Close() error {
	s.consumed = true
	if s.lock != nil {
		err := s.lock.Close()
		s.lock = nil
		return err
	}
	return nil
}
