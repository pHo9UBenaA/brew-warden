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

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

// Collector owns acquisition in a fresh private workspace. Evidence eligibility
// does not expose a brew subprocess or authorize installation.
type Collector struct {
	Runtime        Runtime
	Directory      string
	BottleVerifier ports.BottleVerifier
	LegacyState    string
	client         *http.Client
}
type Collection struct {
	root          string
	inputs        collectionInputs
	nodes         []domain.Node
	observedAt    int64
	runtimeDigest domain.Digest
	frozen        []frozenInput
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
	if c == nil || ctx == nil || c.BottleVerifier == nil || now <= 0 || now > 1<<62 || runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" || !domain.ValidRequest(request.Operation, request.Targets) || len(request.Targets) == 0 {
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
	result := &Collection{root: root, observedAt: now}
	if err := c.collect(ctx, result, request); err != nil {
		_ = os.RemoveAll(root) // No mutation can start before collection succeeds.
		return nil, fmt.Errorf("candidate collection stopped: %w", err)
	}
	result.frozen, err = (workspace{result.root}).freezeInputs()
	if err != nil {
		_ = os.RemoveAll(root)
		return nil, err
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
	var err error
	result.runtimeDigest, err = c.Runtime.materialize(filepath.Join(w.root, "runtime"))
	if err != nil {
		return err
	}
	client := c.client
	if client == nil {
		client = publicClient()
	}
	profile, err := w.sandbox("collect", true, false, []string{filepath.Join(w.root, "runtime/brew/Library")})
	if err != nil {
		return err
	}
	parsed, err := w.metadata(ctx, profile, request.Targets)
	if err != nil {
		return err
	}
	metadata, err := readRegular(filepath.Join(w.root, metadataCachePath), 80*1024*1024)
	if err != nil {
		return err
	}
	_, candidates, err := parseMetadata(parsed, request.Targets)
	if err != nil {
		return err
	}
	result.inputs = collectionInputs{Schema: 2, Candidates: candidates, Targets: request.Targets, Operation: request.Operation}
	inputBytes, err := json.Marshal(result.inputs)
	if err != nil {
		return err
	}
	if err := writeNew(filepath.Join(w.root, "inputs.json"), inputBytes, 0600); err != nil {
		return err
	}
	downloaded, err := w.fetchBottles(ctx, profile, candidates)
	if err != nil {
		return err
	}
	if err := w.copyDownloads(downloaded, candidates); err != nil {
		return err
	}
	metadataClaim, _ := json.Marshal(struct{ SignedMetadata, PublicResult, Runtime domain.Digest }{digestBytes(metadata), digestBytes(parsed), result.runtimeDigest})
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
				raw, _ = json.Marshal(struct{ Bottle domain.Digest }{f.BottleSHA256})
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
		provenance, age, raw, err := c.BottleVerifier.VerifyEvidence(ctx, f.artifact(), filepath.Join(w.root, "inputs", bottleName(f)), result.observedAt)
		if err != nil {
			return fmt.Errorf("attestation for %s unavailable: %w", f.Name, err)
		}
		for _, e := range []domain.Evidence{provenance, age} {
			if err := checkClaim(e, f.artifact(), e.Claim, result.observedAt); err != nil || e.Status != domain.Verified ||
				(e.Claim != domain.Provenance && e.Claim != domain.Publication) || e.RawSHA256 != digestBytes(raw) {
				return errors.New("candidate attestation evidence is incomplete or unbound")
			}
			if err := w.observation(e, raw); err != nil {
				return err
			}
			node.Evidence = append(node.Evidence, e)
		}
		result.nodes = append(result.nodes, node)
	}
	if err := w.checkBottleMetadata(candidates); err != nil {
		return err
	}
	advisoryEvidence, err := w.collectPublicAdvisories(ctx, client, candidates, result.observedAt)
	if err != nil {
		return err
	}
	for i, f := range candidates {
		if err := checkClaim(advisoryEvidence[i], f.artifact(), domain.Vulnerabilities, result.observedAt); err != nil {
			return err
		}
		result.nodes[i].Evidence = append(result.nodes[i].Evidence, advisoryEvidence[i])
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
		if err := validateBottleArchive(data, candidates[i]); err != nil {
			return err
		}
		if err := writeNew(filepath.Join(w.root, "inputs", bottleName(candidates[i])), data, 0600); err != nil {
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
	return writeRecord(file, raw)
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
