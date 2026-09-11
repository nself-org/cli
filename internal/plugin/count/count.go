// Package count is the CLI's single source for "how many plugins are
// there". It embeds a vendored copy of the public plugins repo's generated
// plugins/counts.json artifact so `nself plugin count` answers offline, with
// no registry checkout and no network call.
//
// Purpose: parse and expose the locked counts.json schema (v1) so exactly
// one number — Artifact.Advertised — is ever shown to a user as "the plugin
// count", replacing three prior implementations (this CLI's
// scripts/plugin-counts.sh, the web build, and ad hoc doc counts) that each
// encoded a different counting rule and disagreed even when all three were
// freshly regenerated.
//
// Inputs: the embedded counts.json bytes (see the sync TODO below); no
// runtime inputs.
//
// Outputs: a parsed Artifact plus the raw embedded bytes verbatim, for
// callers that need the exact published JSON (e.g. `--json` output).
//
// Constraints: this package must never compute a count itself — it only
// parses and re-serves the artifact produced by
// plugins/scripts/plugin-counts.sh in the nself-org/plugins repo. Schema
// source of truth: that repo's COUNTS-SCHEMA.md (schema_version 1).
//
// TODO(sync): counts.json here is a manually vendored snapshot, not yet
// wired to auto-update. Once nself-org/plugins publishes
// plugins/counts.json on its main branch, add a CI job in this repo
// (.github/workflows/) that fetches it from that repo's origin/main on a
// schedule (or on the plugins repo's release webhook) and opens a PR
// updating internal/plugin/count/counts.json — the same generated-artifact
// pattern already used for internal/deprecation/registry.yaml. Until that
// job exists, refresh this file by hand from
// https://github.com/nself-org/plugins/blob/main/plugins/counts.json and
// note the source commit in the commit message.
package count

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// countsJSON is the vendored, locked-schema counts artifact. See the
// package doc TODO(sync) for how this gets refreshed.
//
//go:embed counts.json
var countsJSON []byte

// RepoSource identifies the registry file a count was computed from.
type RepoSource struct {
	Repo string `json:"repo"`
	SHA  string `json:"sha"`
}

// Sources names the two registries the artifact was computed from.
type Sources struct {
	Free RepoSource `json:"free"`
	Pro  RepoSource `json:"pro"`
}

// RegistryCount is the entries/installable/nonInstallable breakdown for one
// registry (free or pro).
//
// entries is the raw registry slug count. installable excludes any slug
// whose manifest declares installable: false. Per the locked schema,
// entries is never a user-facing number — only installable and, after
// de-duplication across registries, advertised are.
type RegistryCount struct {
	Entries        int      `json:"entries"`
	Installable    int      `json:"installable"`
	NonInstallable []string `json:"nonInstallable"`
}

// Overlap describes slugs present in both registries.
//
// sharedSlugs is every slug in both. dualRegistry is the subset explicitly
// marked as two genuinely different plugins (counted twice); duplicates is
// the remainder (counted once, subtracted from the naive sum).
type Overlap struct {
	SharedSlugs  []string `json:"sharedSlugs"`
	DualRegistry []string `json:"dualRegistry"`
	Duplicates   []string `json:"duplicates"`
}

// Totals is the combined free+pro view before and after de-duplication.
type Totals struct {
	Entries     int `json:"entries"`
	Installable int `json:"installable"`
}

// Artifact is the parsed shape of plugins/counts.json (schema_version 1).
//
// Advertised is THE number for any user-facing surface — website, CLI,
// docs, marketing. Never publish Totals.Entries or either registry's
// Entries field as a headline count.
type Artifact struct {
	Generated     string        `json:"_generated"`
	SchemaVersion int           `json:"schema_version"`
	GeneratedAt   string        `json:"generated_at"`
	Sources       Sources       `json:"sources"`
	Free          RegistryCount `json:"free"`
	Pro           RegistryCount `json:"pro"`
	Overlap       Overlap       `json:"overlap"`
	Totals        Totals        `json:"totals"`
	Advertised    int           `json:"advertised"`
}

const supportedSchemaVersion = 1

// Load parses the embedded counts.json artifact.
//
// Returns an error if the embedded bytes are missing, malformed, or declare
// an unsupported schema_version — this package refuses to guess at an
// unknown schema rather than silently mis-render a count.
func Load() (*Artifact, error) {
	return parse(countsJSON)
}

// RawJSON returns the embedded artifact bytes verbatim, exactly as
// published by plugins/scripts/plugin-counts.sh. Used by `--json` output so
// consumers get the canonical artifact, not a Go-reserialized copy that
// could silently drop or reorder fields.
func RawJSON() []byte {
	out := make([]byte, len(countsJSON))
	copy(out, countsJSON)
	return out
}

func parse(b []byte) (*Artifact, error) {
	if len(b) == 0 {
		return nil, fmt.Errorf("plugin count: empty embedded artifact")
	}
	var a Artifact
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, fmt.Errorf("plugin count: parse embedded counts.json: %w", err)
	}
	if a.SchemaVersion != supportedSchemaVersion {
		return nil, fmt.Errorf("plugin count: unsupported schema_version %d (want %d)", a.SchemaVersion, supportedSchemaVersion)
	}
	return &a, nil
}
