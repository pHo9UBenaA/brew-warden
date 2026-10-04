package homebrew

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

// publicSession owns one bound execution and its operation lock. The lock
// coordinates only BrewWarden operations, not independent Homebrew clients.
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
	Name           string             `json:"name" required:"true"`
	Pinned         bool               `json:"pinned" required:"true"`
	Outdated       bool               `json:"outdated" required:"true"`
	KegOnly        bool               `json:"keg_only" required:"true"`
	ActiveVersion  *string            `json:"active_version,omitempty"`
	LinkIncomplete bool               `json:"link_incomplete,omitempty"`
	Installed      []installedVersion `json:"installed" required:"true"`
}

type installedVersion struct {
	Version       string                `json:"version" required:"true"`
	OnRequest     bool                  `json:"installed_on_request" required:"true"`
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
	if err := matchExecutionMetadata(metadata, names, nodes); err != nil {
		return nil, "", err
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
		// Public info represents linked_keg as null for keg-only formulae.
		// Observe the opt link, and for normal formulae also require Homebrew's
		// linked-keg record. A partially poured but unlinked keg is not a
		// successfully installed formula, even if its opt link exists.
		formula.ActiveVersion, formula.LinkIncomplete, err = installedLink("/opt/homebrew", formula.Name, formula.KegOnly)
		if err != nil {
			return nil, "", err
		}
		candidate := nodes[slices.Index(names, formula.Name)].Artifact
		if err := formula.bindInstalledReceipts(candidate); err != nil {
			return nil, "", err
		}
	}
	digest, err := w.saveInstalledState(document.Formulae)
	if err != nil {
		return nil, "", err
	}
	return document.Formulae, digest, nil
}

// Store the exact observation once; an existing digest path must still contain
// the same bytes. Saving a snapshot does not establish installation success.
func (w workspace) saveInstalledState(formulae []installedFormula) (domain.Digest, error) {
	raw, err := json.Marshal(formulae)
	if err != nil {
		return "", err
	}
	digest := digestBytes(raw)
	destination := filepath.Join(w.root, "states", string(digest)+".json")
	if old, err := readRegular(destination, maxManifest); err == nil {
		if digestBytes(old) != digest {
			return "", errors.New("saved installed observation changed")
		}
	} else if err := writeRecord(destination, raw); err != nil {
		return "", err
	}
	return digest, nil
}

// parseInfo has already established that metadata contains exactly the requested
// names. Compare each identity and dependency set with the verified plan.
func matchExecutionMetadata(metadata []formulaMetadata, names []string, nodes []domain.Node) error {
	for _, formula := range metadata {
		node := nodes[slices.Index(names, formula.Name)]
		if formula.artifact() != node.Artifact {
			return errors.New("execution metadata differs from verified candidate")
		}
		dependencies := make([]string, 0, len(node.Dependencies))
		for _, dependency := range node.Dependencies {
			dependencies = append(dependencies, dependency.Name)
		}
		slices.Sort(dependencies)
		slices.Sort(formula.Dependencies)
		if !slices.Equal(dependencies, formula.Dependencies) {
			return errors.New("execution dependency graph changed")
		}
	}
	return nil
}

// Bind every observed version, not just the active keg: a plan also depends on
// inactive receipts remaining unchanged until execution.
func (formula *installedFormula) bindInstalledReceipts(candidate domain.Artifact) error {
	for i := range formula.Installed {
		version := &formula.Installed[i]
		digest, err := installedReceiptDigest(candidate, version.Version)
		if err != nil {
			return err
		}
		version.ReceiptSHA256 = digest
	}
	return nil
}

// installedReceiptDigest binds a recorded official-core installation to its
// receipt bytes. It does not authenticate the installed payload itself.
func installedReceiptDigest(candidate domain.Artifact, version string) (domain.Digest, error) {
	candidate.Version = version
	if !candidate.Valid() {
		return "", errors.New("invalid installed version path")
	}
	keg := filepath.Join("/opt/homebrew/Cellar", candidate.Name, version)
	resolved, err := filepath.EvalSymlinks(keg)
	if err != nil || resolved != keg {
		return "", errors.New("installed keg escapes expected prefix")
	}
	raw, err := readRegular(filepath.Join(keg, "INSTALL_RECEIPT.json"), 1024*1024)
	if err != nil {
		return "", err
	}
	var receipt struct {
		Arch   string `json:"arch" required:"true"`
		Source struct {
			Tap  string `json:"tap" required:"true"`
			Spec string `json:"spec" required:"true"`
		} `json:"source" required:"true"`
	}
	if err := decodeSchema(raw, &receipt, true); err != nil {
		return "", errors.New("installed receipt is not official core stable")
	}
	officialCoreStable := receipt.Arch == "arm64" && receipt.Source.Tap == "homebrew/core" && receipt.Source.Spec == "stable"
	if !officialCoreStable {
		return "", errors.New("installed receipt is not official core stable")
	}
	return digestBytes(raw), nil
}

// Homebrew 7.0.4's Keg#linked? uses var/homebrew/linked, while Keg#optlinked?
// uses opt. Neither link authenticates the installed payload; both establish
// whether Homebrew actually completed its own shared-prefix link step.
func installedLink(prefix, name string, kegOnly bool) (*string, bool, error) {
	if !domain.ValidRequest("install", []string{name}) {
		return nil, false, errors.New("invalid installed link identity")
	}
	observe := func(file string) (*string, error) {
		info, err := os.Lstat(file)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			return nil, errors.New("installed Homebrew link is unsafe")
		}
		resolved, err := filepath.EvalSymlinks(file)
		if err != nil || filepath.Dir(resolved) != filepath.Join(prefix, "Cellar", name) {
			return nil, errors.New("installed Homebrew link escapes candidate rack")
		}
		keg, err := os.Stat(resolved)
		if err != nil || !keg.IsDir() {
			return nil, errors.New("installed Homebrew link has no keg")
		}
		version := filepath.Base(resolved)
		return &version, nil
	}
	active, err := observe(filepath.Join(prefix, "opt", name))
	if err != nil {
		return nil, false, err
	}
	linked, err := observe(filepath.Join(prefix, "var", "homebrew", "linked", name))
	if err != nil {
		return nil, false, err
	}
	if linked != nil && (active == nil || *linked != *active) {
		return nil, false, errors.New("installed Homebrew linked-keg record differs from opt link")
	}
	// Observe the actual partial state after a failed pour. Preparation and
	// post-execution success checks refuse it, but the changed state remains
	// available to classify a nonzero exit as partial rather than unknown.
	return active, !kegOnly && active != nil && linked == nil, nil
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
		if state.LinkIncomplete {
			return nil, errors.New("installed Homebrew link step is incomplete; repair with brew link before retrying")
		}
		operation, err := installedAction(state, node.Artifact, request)
		if err != nil {
			return nil, err
		}
		result = append(result, plannedAction{Name: state.Name, Operation: operation})
	}
	return result, nil
}

func installedAction(state installedFormula, candidate domain.Artifact, request collectionInputs) (string, error) {
	if len(state.Installed) == 0 {
		if request.Operation == "upgrade" && slices.Contains(request.Targets, state.Name) {
			return "", errors.New("upgrade target is not installed")
		}
		return "install", nil
	}
	candidateKegVersion := kegVersion(candidate.Version, candidate.Revision)
	if state.ActiveVersion == nil || *state.ActiveVersion == "" {
		return "", errors.New("unlinked installed candidate requires manual Homebrew repair")
	}
	operation := ""
	for _, version := range state.Installed {
		if version.Version != *state.ActiveVersion {
			continue
		}
		if len(version.Options) != 0 || !version.Poured || !version.Built {
			return "", errors.New("installed candidate is not a supported bottle")
		}
		// Public receipt flags describe installation state, not payload integrity.
		if version.Version == candidateKegVersion {
			operation = "keep"
			if request.Operation == "install" && slices.Contains(request.Targets, state.Name) && !version.OnRequest {
				// Public install promotes the receipt without reinstalling payload.
				operation = "install"
			}
		} else if state.Outdated {
			operation = "upgrade"
		} else {
			return "", errors.New("candidate downgrade or unmatched installed version")
		}
	}
	if operation == "" {
		return "", errors.New("active installed version is absent from public info")
	}
	return operation, nil
}

func acquireOperationLock(directory string) (*os.File, error) {
	file, err := os.OpenFile(filepath.Join(directory, "execution.lock"), os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
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
func (w workspace) checkPublicRuntime(ctx context.Context, revision string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if reviewedBrewRevisions[revision] == "" {
		return errors.New("unrecognized Homebrew execution source")
	}
	actual, err := reviewedBrewSource(ctx, "/opt/homebrew")
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if err == nil && actual != revision {
		return errors.New("installed Homebrew release changed before execution")
	}
	if err != nil && revision != brewRevision {
		return errors.New("installed Homebrew release is no longer reviewed")
	}
	source := filepath.Join(w.root, "runtime/brew")
	for _, directory := range []string{"bin", "Library"} {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := filepath.WalkDir(filepath.Join(source, directory), func(path string, entry os.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
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
				copiedLink, err := os.Readlink(path)
				if err != nil {
					return err
				}
				installedLink, err := os.Readlink(destination)
				if err != nil || copiedLink != installedLink {
					return errors.New("installed Homebrew runtime link differs from supported build")
				}
				return nil
			}
			copiedBytes, err := readRegular(path, 128*1024*1024)
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			installedBytes, err := readRegular(destination, 128*1024*1024)
			if err != nil || digestBytes(copiedBytes) != digestBytes(installedBytes) {
				return errors.New("installed Homebrew differs from supported build")
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (c *Collection) Prepare(ctx context.Context, policy domain.Policy, waivers []domain.AgeWaiver, now int64, streams Streams) (ports.Prepared, ports.ExecutionSession, error) {
	if c == nil || ctx == nil || !policy.Valid() || now < c.observedAt || now >= c.observedAt+3600 || len(c.nodes) == 0 {
		return ports.Prepared{}, nil, errors.New("collection unavailable or expired")
	}
	s := &publicSession{w: workspace{c.root}, streams: streams}
	fail := func(err error) (ports.Prepared, ports.ExecutionSession, error) {
		if s.lock == nil {
			_ = removeCollection(filepath.Dir(c.root), filepath.Base(c.root))
		} else if closeErr := s.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
		return ports.Prepared{}, nil, err
	}
	files, err := s.w.freezeInputs()
	if err != nil || !slices.Equal(files, c.frozen) {
		return fail(errors.New("collected inputs changed before planning"))
	}
	if err := s.w.checkPublicRuntime(ctx, c.runtimeRevision); err != nil {
		return fail(err)
	}
	s.lock, err = acquireOperationLock(filepath.Dir(c.root))
	if err != nil {
		return fail(err)
	}
	if err := clearStoppedInFlight(filepath.Dir(c.root)); err != nil {
		return fail(err)
	}
	immutable := []string{
		filepath.Join(s.w.root, "public-execution.sb"),
		filepath.Join(s.w.root, "runtime"),
		filepath.Join(s.w.root, "cache/api"),
		filepath.Join(s.w.root, "cache/downloads"),
		filepath.Join(s.w.root, "plan.json"),
	}
	// Installer descendants must not rewrite confinement or frozen inputs.
	for _, file := range files {
		immutable = append(immutable, filepath.Join(s.w.root, file.Path))
	}
	s.profile, err = s.w.sandbox("public-execution", sandboxPermissions{AllowPrefixWrites: true}, immutable)
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
	s.plan = executionPlan{
		Schema: 3, MinimumAgeSeconds: policy.MinimumAgeSeconds(),
		Nodes: c.Evidence(), Actions: actions, BeforeState: before,
		Environment: executionEnvironment{
			Runtime: c.runtimeDigest, BrewRevision: c.runtimeRevision,
			OSVersion: strings.TrimSpace(string(osVersion)), Prefix: "/opt/homebrew",
		},
		Inputs: files, Attempt: domain.Digest(hex.EncodeToString(nonce)),
		IssuedAt: now, ExpiresAt: min(now+600, c.observedAt+3600),
		Waivers: append([]domain.AgeWaiver{}, waivers...), Targets: []domain.Artifact{},
	}
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
	requestedInstalls := []string{}
	for _, action := range s.plan.Actions {
		if action.Operation == "install" && slices.ContainsFunc(s.plan.Targets, func(a domain.Artifact) bool { return a.Name == action.Name }) {
			requestedInstalls = append(requestedInstalls, action.Name)
		}
		if action.Operation != "keep" {
			remaining[action.Name] = action
		}
	}
	for len(remaining) != 0 {
		selected := nextReadyAction(s.plan.Nodes, remaining)
		if selected.Name == "" {
			return ports.ExecutionResult{}, errors.New("cyclic execution plan")
		}
		fresh.Assessment.Now = time.Now().Unix()
		if fresh.Assessment.Now < s.plan.IssuedAt || fresh.Assessment.Now >= s.plan.ExpiresAt || domain.Evaluate(fresh.Assessment).Outcome != domain.Allow {
			return ports.ExecutionResult{}, errors.New("execution plan expired")
		}
		if err := s.w.checkPublicRuntime(ctx, s.plan.Environment.BrewRevision); err != nil {
			return ports.ExecutionResult{}, err
		}
		args := []string{selected.Operation, "--formula", "--force-bottle"}
		if selected.Operation == "install" && !slices.ContainsFunc(s.plan.Targets, func(a domain.Artifact) bool { return a.Name == selected.Name }) {
			args = append(args, "--as-dependency")
		}
		args = append(args, "homebrew/core/"+selected.Name)
		result, err := s.runCommand(ctx, args)
		if err != nil || !result.ExitKnown || result.ExitCode != 0 {
			return result, err
		}
		delete(remaining, selected.Name)
	}
	states, after, err := s.w.publicState(ctx, s.profile, s.plan.Nodes)
	if err != nil {
		return ports.ExecutionResult{ExitKnown: true, ExitCode: 0}, err
	}
	actions, err := publicActions(states, s.plan.Nodes, collectionInputs{Operation: "install", Targets: requestedInstalls})
	matches := err == nil && !slices.ContainsFunc(actions, func(action plannedAction) bool { return action.Operation != "keep" })
	return ports.ExecutionResult{ExitKnown: true, ExitCode: 0, AfterState: after, MatchesPlan: matches}, err
}

// Select the first pending action with no dependency action still pending.
// Node order remains deterministic; an empty result means no progress is possible.
func nextReadyAction(nodes []domain.Node, remaining map[string]plannedAction) plannedAction {
	for _, node := range nodes {
		action, pending := remaining[node.Artifact.Name]
		if !pending {
			continue
		}
		blocked := slices.ContainsFunc(node.Dependencies, func(dependency domain.Artifact) bool {
			_, pending := remaining[dependency.Name]
			return pending
		})
		if !blocked {
			return action
		}
	}
	return plannedAction{}
}

func (s *publicSession) runCommand(ctx context.Context, args []string) (ports.ExecutionResult, error) {
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
	startupGate := `read -r ready <&3 || exit 125
exec 3<&-
exec "$@"`
	command := exec.CommandContext(ctx, "/bin/sh", append([]string{"-c", startupGate, "brewwarden-exec"}, base.Args...)...)
	command.Env, command.Dir = base.Env, base.Dir
	command.ExtraFiles = []*os.File{gate, s.lock}
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	command.Cancel = func() error { return syscall.Kill(-command.Process.Pid, syscall.SIGINT) }
	command.WaitDelay = 2 * time.Second
	command.Stdin, command.Stdout, command.Stderr = s.streams.In, s.streams.Out, s.streams.Err
	if err := command.Start(); err != nil {
		return ports.ExecutionResult{}, err
	}
	owned := inFlight{
		Schema: 1, Plan: s.prepared.Assessment.Binding.Plan,
		Attempt: s.prepared.Assessment.Binding.Attempt,
		PID:     command.Process.Pid, Session: command.Process.Pid,
		Collection: filepath.Base(s.w.root),
	}
	if err := saveInFlight(filepath.Dir(s.w.root), owned); err != nil {
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
	if err := finishInFlight(filepath.Dir(s.w.root), owned); err != nil {
		return result, errors.New("owned subprocesses are still active or unavailable; retry after they stop")
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
	if s.lock == nil {
		return nil
	}
	// A continued child keeps its workspace until a later fresh invocation
	// establishes that the whole session stopped. No saved plan is replayed.
	err := clearStoppedInFlight(filepath.Dir(s.w.root))
	if err == nil {
		err = removeCollection(filepath.Dir(s.w.root), filepath.Base(s.w.root))
	}
	closeErr := s.lock.Close()
	s.lock = nil
	return errors.Join(err, closeErr)
}
