package docker

// imagetools.go — read an image's index (manifest list) without pulling it.
//
// Purpose: the image-lock generator (tools/imagelock) and `nself update images`
// need the index digest and the per-platform manifest digests of a reference.
// All docker CLI calls stay in this package (the funnel).
// Inputs: an image reference; for ParseIndex, the raw index bytes.
// Outputs: the raw index JSON (ImageIndexRaw), a parsed digest plus platform
// map (ParseIndex), or the raw `docker manifest inspect` output.
// Constraints: no Go registry client (ADR 0030): the docker CLI does the
// network work. Only linux/amd64, linux/arm64 and linux/arm/v7 are recorded;
// attestation manifests (platform unknown) are skipped. A reference whose
// manifest is a single image rather than an index is an error: it has no
// digest-addressable index to pin per platform.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
)

// IndexInfo is the parsed form of an image index.
type IndexInfo struct {
	// MediaType is the index media type as published.
	MediaType string
	// Digest is "sha256:<hex>" of the raw index bytes (the index digest).
	Digest string
	// Platforms maps "linux/amd64", "linux/arm64", "linux/arm/v7" to the
	// per-platform manifest digest.
	Platforms map[string]string
}

// ErrNotIndex means the reference resolves to a single manifest, not an index.
var ErrNotIndex = errors.New("reference is not an image index")

// ImageIndexRaw returns the raw index JSON of ref via
// `docker buildx imagetools inspect --raw`.
func ImageIndexRaw(ctx context.Context, ref string) ([]byte, error) {
	return runCapture(ctx, "buildx", "imagetools", "inspect", "--raw", ref)
}

// ManifestInspect runs `docker manifest inspect [--verbose] ref` and returns
// its stdout.
func ManifestInspect(ctx context.Context, ref string, verbose bool) ([]byte, error) {
	args := []string{"manifest", "inspect"}
	if verbose {
		args = append(args, "--verbose")
	}
	return runCapture(ctx, append(args, ref)...)
}

func runCapture(ctx context.Context, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, "docker", args...)
	var so, se bytes.Buffer
	c.Stdout, c.Stderr = &so, &se
	if err := c.Run(); err != nil {
		return nil, fmt.Errorf("docker %s: %w: %s", args[0], err, lastLine(se.String()))
	}
	return so.Bytes(), nil
}

// ParseIndex computes the index digest of raw and extracts the per-platform
// digests. A trailing newline added by the CLI is not part of the digest.
func ParseIndex(raw []byte) (IndexInfo, error) {
	raw = bytes.TrimSuffix(raw, []byte("\n"))
	var doc struct {
		MediaType string `json:"mediaType"`
		Manifests []struct {
			Digest   string `json:"digest"`
			Platform struct {
				OS, Architecture, Variant string
			} `json:"platform"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return IndexInfo{}, fmt.Errorf("parse image index: %w", err)
	}
	if len(doc.Manifests) == 0 {
		return IndexInfo{}, ErrNotIndex
	}
	sum := sha256.Sum256(raw)
	info := IndexInfo{MediaType: doc.MediaType, Digest: "sha256:" + hex.EncodeToString(sum[:]), Platforms: map[string]string{}}
	for _, m := range doc.Manifests {
		p := m.Platform
		var key string
		switch {
		case p.OS == "linux" && p.Architecture == "amd64":
			key = "linux/amd64"
		case p.OS == "linux" && p.Architecture == "arm64":
			key = "linux/arm64"
		case p.OS == "linux" && p.Architecture == "arm" && p.Variant == "v7":
			key = "linux/arm/v7"
		default:
			continue
		}
		info.Platforms[key] = m.Digest
	}
	return info, nil
}
