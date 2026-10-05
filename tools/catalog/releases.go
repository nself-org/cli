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

var sha256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)

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
		for p, sum := range rel.PlatformChecksums {
			if !sha256Re.MatchString(sum) {
				probs = append(probs, s+": platform_checksums."+p+" must be 64 lowercase hex characters")
			}
		}
	}
	return probs
}
