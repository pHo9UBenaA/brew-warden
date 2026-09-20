package homebrew

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"brewwarden/internal/domain"
	"brewwarden/internal/ports"
)

// Collector owns acquisition in a fresh private workspace. Evidence eligibility
// does not expose a brew subprocess or authorize installation.
type Collector struct {
	Runtime         Runtime
	Directory       string
	Publication     ports.EvidenceCollector
	Vulnerabilities ports.EvidenceCollector
	Verifier        func(string, domain.Digest) ports.ProvenanceVerifier
	client          *http.Client
}
type Collection struct {
	root          string
	inputs        nativeInputs
	nodes         []domain.Node
	observedAt    int64
	runtimeDigest domain.Digest
}
type downloadEntry struct {
	Name string `json:"name" required:"true"`
	Path string `json:"path" required:"true"`
}
type downloadDocument struct {
	Schema    int             `json:"schema" required:"true"`
	Downloads []downloadEntry `json:"downloads" required:"true"`
}

func (c *Collector) Collect(ctx context.Context, request ports.Request, now int64) (*Collection, error) {
	if c == nil || ctx == nil || c.Publication == nil || c.Vulnerabilities == nil || c.Verifier == nil || now <= 0 || now > 1<<62 || runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" || !domain.ValidRequest(request.Operation, request.Targets) || len(request.Targets) == 0 {
		return nil, errors.New("unsupported native collection request")
	}
	if !filepath.IsAbs(c.Directory) {
		return nil, errors.New("collection directory must be absolute")
	}
	info, err := os.Lstat(c.Directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("collection directory must be private")
	}
	parent, err := filepath.EvalSymlinks(c.Directory)
	if err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp(parent, "collection-")
	if err != nil {
		return nil, err
	}
	result := &Collection{root: root, observedAt: now, runtimeDigest: c.Runtime.ManifestSHA256}
	if err := c.collect(ctx, result, request); err != nil {
		// Preserve incomplete inputs for diagnosis. They cannot become a session.
		return nil, fmt.Errorf("candidate collection stopped: %w", err)
	}
	return result, nil
}
func (c *Collector) collect(ctx context.Context, result *Collection, request ports.Request) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	w := workspace{result.root}
	if err := w.initialize(); err != nil {
		return err
	}
	verifierDigest, err := c.Runtime.materialize(filepath.Join(w.root, "runtime"))
	if err != nil {
		return err
	}
	verifier := c.Verifier(filepath.Join(w.root, "runtime/verifier"), verifierDigest)
	if verifier == nil {
		return errors.New("provenance verifier unavailable")
	}
	client := c.client
	if client == nil {
		client = publicClient()
	}
	metadata, err := download(ctx, client, "https://formulae.brew.sh/api/formula.jws.json", 80*1024*1024)
	if err != nil {
		return err
	}
	if err := writeNew(filepath.Join(w.root, "formula.jws.json"), metadata, 0600); err != nil {
		return err
	}
	profile, err := w.sandbox("collect", true, false, []string{filepath.Join(w.root, "runtime/brew/Library")})
	if err != nil {
		return err
	}
	parsed, err := w.native(ctx, "metadata", profile, "metadata.rb", request.Targets...)
	if err != nil {
		return err
	}
	recipes, candidates, err := parseMetadata(parsed, request.Targets)
	if err != nil {
		return err
	}
	for _, recipe := range recipes {
		address := "https://raw.githubusercontent.com/Homebrew/homebrew-core/" + recipe.TapCommit + "/" + recipe.RecipePath
		data, err := download(ctx, client, address, 1024*1024)
		if err != nil {
			return err
		}
		if digestBytes(data) != recipe.RecipeSHA256 {
			return errors.New("authenticated recipe checksum mismatch")
		}
		dest := filepath.Join(w.root, "runtime/brew/Library/Taps/homebrew/homebrew-core", recipe.RecipePath)
		if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return err
		}
		if err := writeNew(dest, data, 0600); err != nil {
			return err
		}
	}
	result.inputs = nativeInputs{Schema: 1, Recipes: recipes, Candidates: candidates, Targets: request.Targets, Operation: request.Operation}
	inputBytes, err := json.Marshal(result.inputs)
	if err != nil {
		return err
	}
	if err := writeNew(filepath.Join(w.root, "native-inputs.json"), inputBytes, 0600); err != nil {
		return err
	}
	downloaded, err := w.native(ctx, "fetch", profile, "fetch.rb")
	if err != nil {
		return err
	}
	if err := w.copyDownloads(downloaded, candidates); err != nil {
		return err
	}
	metadataClaim, _ := json.Marshal(struct{ SignedMetadata, NativeResult, Runtime domain.Digest }{digestBytes(metadata), digestBytes(parsed), result.runtimeDigest})
	for _, f := range candidates {
		node := domain.Node{Artifact: f.artifact(), Dependencies: []domain.Artifact{}}
		for _, name := range f.Dependencies {
			for _, dependency := range candidates {
				if name == dependency.Name {
					node.Dependencies = append(node.Dependencies, dependency.artifact())
				}
			}
		}
		for _, claim := range []domain.Claim{domain.Metadata, domain.Checksum} {
			raw := metadataClaim
			if claim == domain.Checksum {
				raw, _ = json.Marshal(struct{ Bottle, Recipe domain.Digest }{f.BottleSHA256, f.RecipeSHA256})
			}
			evidence, err := domain.NewEvidence(domain.Evidence{Claim: claim, Subject: f.artifact(), Status: domain.Verified, Provider: domain.Homebrew, Source: "Homebrew signed formula API", ProviderVersion: brewRevision, RawSHA256: digestBytes(raw), ObservedAt: result.observedAt, ExpiresAt: result.observedAt + 3600})
			if err != nil {
				return err
			}
			if err := w.observation(evidence, raw); err != nil {
				return err
			}
			node.Evidence = append(node.Evidence, evidence)
		}
		address := "https://api.github.com/repos/Homebrew/homebrew-core/attestations/sha256:" + string(f.BottleSHA256)
		response, err := download(ctx, client, address, maxManifest)
		if err != nil {
			return err
		}
		bundle, err := bundles(response)
		if err != nil {
			return err
		}
		file := filepath.Join(w.root, "inputs", f.Name+".bundle.jsonl")
		if err := writeNew(file, bundle, 0600); err != nil {
			return err
		}
		if err := w.rawObservation(response); err != nil {
			return err
		}
		home := filepath.Join(w.root, "home", f.Name)
		if err := os.Mkdir(home, 0700); err != nil {
			return err
		}
		e, raw, err := verifier.Verify(ctx, f.artifact(), filepath.Join(w.root, "inputs", nativeBottleName(f)), file, home, result.observedAt)
		if err != nil {
			return errors.New("candidate provenance verification failed")
		}
		if err := checkClaim(e, f.artifact(), domain.Provenance, result.observedAt); err != nil {
			return err
		}
		if e.Status != domain.Verified {
			return errors.New("candidate provenance is not verified")
		}
		if err := w.observation(e, raw); err != nil {
			return err
		}
		node.Evidence = append(node.Evidence, e)
		result.nodes = append(result.nodes, node)
	}
	readProfile, err := w.sandbox("inspect", false, false, []string{filepath.Join(w.root, "runtime/brew/Library"), filepath.Join(w.root, "inputs")})
	if err != nil {
		return err
	}
	inspected, err := w.native(ctx, "inspect", readProfile, "inspect.rb")
	if err != nil {
		return err
	}
	var doc inspectionDocument
	if err := decodeStrict(inspected, &doc); err != nil || doc.Schema != 1 || len(doc.Candidates) != len(candidates) {
		return errors.New("incomplete native candidate inspection")
	}
	for i, f := range candidates {
		inspected := doc.Candidates[i]
		if inspected.Name != f.Name || !inspected.EmbeddedRecipeSHA256.Valid() {
			return errors.New("native candidate inspection mismatch")
		}
		source := ports.SourceCandidate{Artifact: f.artifact(), SourceURL: f.SourceURL, SourceSHA256: f.SourceSHA256, RecipeSHA256: f.RecipeSHA256, UnmodifiedSource: inspected.UnmodifiedSource}
		for _, provider := range []struct {
			claim     domain.Claim
			collector ports.EvidenceCollector
		}{{domain.Publication, c.Publication}, {domain.Vulnerabilities, c.Vulnerabilities}} {
			e, raw, err := provider.collector.Collect(ctx, source, result.observedAt)
			if err != nil {
				// Unavailable publication may be eligible for an explicit age exception;
				// unavailable vulnerability evidence always holds the complete operation.
				raw, _ = json.Marshal(struct {
					Schema int
					Claim  domain.Claim
					Status string
				}{1, provider.claim, "unavailable"})
				e = domain.Evidence{Claim: provider.claim, Subject: source.Artifact, Status: domain.Unavailable, Provider: domain.Supplement, Source: "candidate collector", ProviderVersion: "1", RawSHA256: digestBytes(raw), ObservedAt: result.observedAt, ExpiresAt: result.observedAt + 3600}
			}
			if err := checkClaim(e, source.Artifact, provider.claim, result.observedAt); err != nil {
				return err
			}
			if err := w.observation(e, raw); err != nil {
				return err
			}
			result.nodes[i].Evidence = append(result.nodes[i].Evidence, e)
		}
	}
	return nil
}
func checkClaim(e domain.Evidence, a domain.Artifact, claim domain.Claim, now int64) error {
	if _, err := domain.NewEvidence(e); err != nil {
		return err
	}
	if e.Subject != a || e.Claim != claim || e.ObservedAt != now || e.ExpiresAt > now+3600 {
		return errors.New("collector evidence binding mismatch")
	}
	return nil
}
func (w workspace) copyDownloads(raw []byte, candidates []formulaMetadata) error {
	var doc downloadDocument
	if err := decodeStrict(raw, &doc); err != nil || doc.Schema != 1 || len(doc.Downloads) != len(candidates) {
		return errors.New("incomplete native downloads")
	}
	for i, item := range doc.Downloads {
		if item.Name != candidates[i].Name || !filepath.IsAbs(item.Path) || !strings.HasPrefix(item.Path, filepath.Join(w.root, "cache")+string(filepath.Separator)) {
			return errors.New("native download outside workspace")
		}
		actual, err := filepath.EvalSymlinks(item.Path)
		if err != nil || actual != item.Path {
			return errors.New("native download path changed")
		}
		data, err := readRegular(actual, 128*1024*1024)
		if err != nil {
			return err
		}
		if digestBytes(data) != candidates[i].BottleSHA256 {
			return errors.New("candidate bottle checksum mismatch")
		}
		if err := writeNew(filepath.Join(w.root, "inputs", nativeBottleName(candidates[i])), data, 0600); err != nil {
			return err
		}
	}
	return nil
}
func (w workspace) observation(e domain.Evidence, raw []byte) error {
	if digestBytes(raw) != e.RawSHA256 {
		return errors.New("evidence observation digest mismatch")
	}
	return w.rawObservation(raw)
}
func (w workspace) rawObservation(raw []byte) error {
	if len(raw) == 0 || len(raw) > 16*1024*1024 {
		return errors.New("observation exceeds limit")
	}
	file := filepath.Join(w.root, "observations", string(digestBytes(raw))+".json")
	if _, err := os.Lstat(file); err == nil {
		existing, err := readRegular(file, 16*1024*1024)
		if err != nil || digestBytes(existing) != digestBytes(raw) {
			return errors.New("observation changed")
		}
		return nil
	}
	return writeNew(file, raw, 0600)
}

// Evidence returns a detached diagnostic view, never an execution permit.
func (c *Collection) Evidence() []domain.Node {
	if c == nil {
		return nil
	}
	result := make([]domain.Node, len(c.nodes))
	for i, node := range c.nodes {
		result[i] = node
		result[i].Dependencies = append([]domain.Artifact{}, node.Dependencies...)
		result[i].Evidence = append([]domain.Evidence{}, node.Evidence...)
	}
	return result
}
