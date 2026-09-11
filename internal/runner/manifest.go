// Package runner implements `nself runner provision` and `nself runner
// verify` (G-012): building and auditing self-hosted GitHub Actions CI
// runner hosts.
//
// Purpose: CI runner hosts were hand-built, so required system dependencies
//
//	were discovered only when a job failed mid-run, and two hosts
//	advertising the identical GitHub Actions labels (self-hosted,Linux,X64)
//	could silently drift apart — one had `gh`/`zip`/`unzip`, the other
//	didn't, and the same commit passed or failed depending on which host
//	claimed the job.
//
// Inputs:  a declarative Manifest (manifest.yaml, embedded below) plus an
//
//	Executor bound to one host (local or SSH).
//
// Outputs: for provision, packages/user/sudoers/systemd units installed on
//
//	that host; for verify, a per-host CheckResult set that composeable
//	callers turn into a cross-host parity matrix (report.go).
//
// Constraints: this package never talks to a host directly — every side
//
//	effect goes through the Executor interface (exec.go), so unit tests
//	exercise the exact command strings without touching real hardware.
package runner

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed manifest.yaml
var manifestYAML []byte

// Dependency is one required system package, checked either by looking for
// a binary on PATH (Binary != "") or by asking the package manager whether
// AptPackage is installed (Binary == "", e.g. a runtime library with no
// CLI entry point).
type Dependency struct {
	Name       string `yaml:"name"`
	AptPackage string `yaml:"apt_package"`
	Binary     string `yaml:"binary"`
	Reason     string `yaml:"reason"`
}

// HasBinary reports whether this dependency is checked via `command -v`
// rather than the package manager.
func (d Dependency) HasBinary() bool { return d.Binary != "" }

// Manifest is the parsed form of manifest.yaml: the full declarative
// dependency set plus the fixed layout conventions (runner user, work dir,
// where to look for a cached Chromium) that provision and verify both need
// to agree on.
type Manifest struct {
	SchemaVersion      int          `yaml:"schema_version"`
	RunnerUser         string       `yaml:"runner_user"`
	WorkDir            string       `yaml:"work_dir"`
	Dependencies       []Dependency `yaml:"dependencies"`
	ChromiumCacheGlobs []string     `yaml:"chromium_cache_globs"`
}

// LoadEmbeddedManifest parses the manifest compiled into the binary.
// Returns an error if the embedded YAML is malformed, which would indicate
// a build-time bug (a hand edit that broke the schema), not a runtime
// condition — callers should treat a non-nil error as fatal.
func LoadEmbeddedManifest() (*Manifest, error) {
	return ParseManifest(manifestYAML)
}

// ParseManifest decodes manifest YAML bytes. Exported so tests (and any
// future NSELF_RUNNER_MANIFEST override, mirroring the deprecation
// registry's pattern) can parse an alternate manifest without touching the
// embedded default.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("runner: parse manifest: %w", err)
	}
	if m.SchemaVersion != 1 {
		return nil, fmt.Errorf("runner: unsupported manifest schema_version %d (want 1)", m.SchemaVersion)
	}
	if len(m.Dependencies) == 0 {
		return nil, fmt.Errorf("runner: manifest declares zero dependencies")
	}
	return &m, nil
}

// AptPackages returns the apt package names for every dependency, in
// manifest order, for a single `apt-get install -y <names...>` invocation.
func (m *Manifest) AptPackages() []string {
	pkgs := make([]string, len(m.Dependencies))
	for i, d := range m.Dependencies {
		pkgs[i] = d.AptPackage
	}
	return pkgs
}
