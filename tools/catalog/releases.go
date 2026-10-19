package main

// releases.go: read and validate releases.json.
//
// Purpose: release data (version, sha256, signature, tarball URL) is written by
// the release pipeline from published bytes; the generator only reads it.
// Inputs: the path of releases.json. Outputs: model.Releases.
// Constraints: unknown keys are rejected so a pipeline typo cannot be dropped
// silently; every problem is reported, not just the first.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/nself-org/cli/tools/catalog/model"
)

var (
	sha256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)
	binaryRe = regexp.MustCompile(model.BinaryPattern)
)

// loadReleases reads and validates the file at path.
func loadReleases(path string) (*model.Releases, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("releases: %w", err)
	}
	var r model.Releases
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("releases %s: %w", path, err)
	}
	if r.SchemaVersion != model.ReleasesVersion {
		return nil, fmt.Errorf("releases %s: schema_version %d is not supported (want %d)", path, r.SchemaVersion, model.ReleasesVersion)
	}
	if probs := validateReleases(&r); len(probs) > 0 {
		return nil, fmt.Errorf("releases %s:\n  %s", path, strings.Join(probs, "\n  "))
	}
	return &r, nil
}

// validateReleases lists every problem in r, sorted by slug.
func validateReleases(r *model.Releases) []string {
	slugs := make([]string, 0, len(r.Releases))
	for s := range r.Releases {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)
	var probs []string
	for _, s := range slugs {
		rel := r.Releases[s]
		if rel.Version == "" {
			probs = append(probs, s+": version is empty")
		}
		if !sha256Re.MatchString(rel.SHA256) {
			probs = append(probs, s+": sha256 must be 64 lowercase hex characters")
		}
		if sig := rel.ReleaseSignature; sig != nil {
			if sig.KeyID == "" || sig.Sig == "" || sig.Alg != model.SigAlg {
				probs = append(probs, s+": release_signature needs key_id, sig and alg "+model.SigAlg)
			}
		}
		for _, b := range rel.Binaries {
			if !binaryRe.MatchString(b) {
				probs = append(probs, s+": binaries entry "+b+" must match "+model.BinaryPattern)
			}
		}
		for p, sum := range rel.PlatformChecksums {
			if !sha256Re.MatchString(sum) {
				probs = append(probs, s+": platform_checksums."+p+" must be 64 lowercase hex characters")
			}
		}
		probs = append(probs, platformSignatureProblems(s, rel)...)
	}
	return probs
}

// platformSignatureProblems lists every problem in rel.PlatformSignatures
// (contract:plugins.release-signature v1.1): each key is a known platform with a
// platform_checksums entry, each value is a complete ed25519 signature, and the
// source release_signature is set because it is the anchor.
func platformSignatureProblems(slug string, rel model.Release) []string {
	if len(rel.PlatformSignatures) == 0 {
		return nil
	}
	var probs []string
	if rel.ReleaseSignature == nil {
		probs = append(probs, slug+": platform_signatures needs a release_signature (the source signature is the anchor)")
	}
	known := map[string]bool{}
	for _, p := range model.Platforms {
		known[p] = true
	}
	names := make([]string, 0, len(rel.PlatformSignatures))
	for p := range rel.PlatformSignatures {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, p := range names {
		pre := slug + ": platform_signatures." + p
		if !known[p] {
			probs = append(probs, pre+" is not a platform (want one of "+strings.Join(model.Platforms, ", ")+")")
		}
		if _, ok := rel.PlatformChecksums[p]; !ok {
			probs = append(probs, pre+" has no platform_checksums."+p)
		}
		sig := rel.PlatformSignatures[p]
		if sig == nil || sig.KeyID == "" || sig.Sig == "" || sig.Alg != model.SigAlg {
			probs = append(probs, pre+" needs key_id, sig and alg "+model.SigAlg)
		}
	}
	return probs
}
