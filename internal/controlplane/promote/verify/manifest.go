// Package verify checks a signed release manifest and its evidence before a
// promotion changes any target (contract:ci.release-manifest, ADR 0024 §9).
package verify

// Purpose: decode the fields promote needs from a release-manifest payload.
// Inputs:  the verified DSSE payload bytes.
// Outputs: a Manifest, or E492 when the payload is not a v1 manifest.
// Constraints: the contract is experimental until P8 (X19): another schema
//              major is refused, unknown additive fields are accepted and
//              ignored. Only fields promote reads are decoded.

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// PayloadType is the DSSE payloadType of a v1 release manifest.
const PayloadType = "application/vnd.nself.ci.release-manifest.v1+json"

const (
	schemaPrefix = "ci.release-manifest/v"
	supportedMaj = 1
	// EvidenceFile is the only evidence document name the manifest may pin.
	EvidenceFile = "evidence.json"
)

var (
	revisionRE = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
	sha256RE   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	digestRE   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// Manifest is the subset of the release manifest promote reads.
type Manifest struct {
	Schema    string      `json:"schema"`
	Revision  string      `json:"revision"`
	Evidence  FileRef     `json:"evidence"`
	Artifacts []Artifact  `json:"artifacts"`
	Signing   SigningInfo `json:"signing"`
}

// FileRef names a file of the release directory and its digest.
type FileRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Artifact is one released file; Image is set for container images.
type Artifact struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Image  *Image `json:"image"`
}

// Image is a container image artifact.
type Image struct {
	IndexDigest string          `json:"index_digest"`
	Manifests   []ImageManifest `json:"manifests"`
}

// ImageManifest is one per-platform image of an index.
type ImageManifest struct {
	Platform string `json:"platform"`
	Digest   string `json:"digest"`
}

// SigningInfo names the purpose and key id the manifest claims.
type SigningInfo struct {
	Purpose string `json:"purpose"`
	KeyID   string `json:"key_id"`
}

// decodeManifest parses payload and enforces the major version and shape.
func decodeManifest(payload []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(payload, &m); err != nil {
		return nil, errs.Wrap("E492", "release manifest payload is not valid JSON", err)
	}
	major, ok := schemaMajor(m.Schema)
	if !ok {
		return nil, errs.Newf("E492", "release manifest schema %q is not %sN", clip(m.Schema), schemaPrefix)
	}
	if major != supportedMaj {
		return nil, errs.Newf("E492", "release manifest schema major %d is not supported (this nself reads v%d)", major, supportedMaj)
	}
	if !revisionRE.MatchString(m.Revision) {
		return nil, errs.New("E492", "release manifest revision is not a full lowercase commit hash")
	}
	if m.Evidence.Path != EvidenceFile || !sha256RE.MatchString(m.Evidence.SHA256) {
		return nil, errs.Newf("E492", "release manifest must pin %s with a sha256", EvidenceFile)
	}
	if err := checkArtifacts(m.Artifacts); err != nil {
		return nil, err
	}
	return &m, nil
}

// checkArtifacts refuses artifact entries whose digests or platforms are not in
// the v1 shape, so a malformed entry cannot hide beside a valid one. Additive
// fields stay accepted.
func checkArtifacts(list []Artifact) error {
	for i, a := range list {
		if !sha256RE.MatchString(a.SHA256) {
			return errs.Newf("E492", "release manifest artifact %d has no valid sha256", i)
		}
		if a.Image == nil {
			continue
		}
		if !digestRE.MatchString(a.Image.IndexDigest) {
			return errs.Newf("E492", "release manifest artifact %d has no valid image index digest", i)
		}
		for j, im := range a.Image.Manifests {
			if im.Platform == "" || !digestRE.MatchString(im.Digest) {
				return errs.Newf("E492", "release manifest artifact %d image manifest %d needs a platform and a sha256 digest", i, j)
			}
		}
	}
	return nil
}

func schemaMajor(schema string) (int, bool) {
	rest, ok := strings.CutPrefix(schema, schemaPrefix)
	if !ok || rest == "" {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n < 0 || strconv.Itoa(n) != rest {
		return 0, false
	}
	return n, true
}

func clip(s string) string {
	if len(s) > 40 {
		return s[:40] + "..."
	}
	return s
}
