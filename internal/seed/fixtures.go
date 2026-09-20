package seed

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FixtureManifest describes a fixture set with checksums for deterministic verification.
type FixtureManifest struct {
	Name      string        // fixture name (e.g. "demo", "load-test")
	Files     []FixtureFile // SQL files in this fixture
	TotalHash string        // SHA256 of all file checksums concatenated
}

// FixtureFile describes a single SQL file within a fixture.
type FixtureFile struct {
	Name     string
	Path     string
	Checksum string // SHA256 of file contents
	Size     int64
}

// FixtureRunResult describes the result of running a fixture.
type FixtureRunResult struct {
	FixtureName string
	FilesRun    int
	Duration    time.Duration
	Errors      []error
}

// SQLRunner is the function called for each SQL file when running a fixture.
type SQLRunner func(filePath string) error

// LoadFixtureManifest reads all SQL files in a fixture directory and computes
// checksums for verification purposes.
func LoadFixtureManifest(projectDir, fixtureName string) (FixtureManifest, error) {
	dir := FixtureDir(projectDir, fixtureName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return FixtureManifest{}, fmt.Errorf("fixture %q not found at %s", fixtureName, dir)
		}
		return FixtureManifest{}, fmt.Errorf("reading fixture dir %s: %w", dir, err)
	}

	manifest := FixtureManifest{Name: fixtureName}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}

		path := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil {
			return FixtureManifest{}, fmt.Errorf("stat fixture file %s: %w", path, err)
		}

		checksum, err := sha256File(path)
		if err != nil {
			return FixtureManifest{}, err
		}

		manifest.Files = append(manifest.Files, FixtureFile{
			Name:     e.Name(),
			Path:     path,
			Checksum: checksum,
			Size:     info.Size(),
		})
	}

	// Compute TotalHash from all individual checksums joined with newlines.
	var sb strings.Builder
	for i, f := range manifest.Files {
		sb.WriteString(f.Checksum)
		if i < len(manifest.Files)-1 {
			sb.WriteString("\n")
		}
	}
	h := sha256.Sum256([]byte(sb.String()))
	manifest.TotalHash = hex.EncodeToString(h[:])

	return manifest, nil
}

// VerifyFixture checks that a fixture's files match their expected checksums.
// If expectedManifest is nil, it computes fresh checksums (always passes).
// Returns a list of files that have changed since expectedManifest was recorded.
func VerifyFixture(projectDir, fixtureName string, expectedManifest *FixtureManifest) (changed []string, err error) {
	current, err := LoadFixtureManifest(projectDir, fixtureName)
	if err != nil {
		return nil, err
	}

	if expectedManifest == nil {
		return nil, nil
	}

	// Build lookup: name → checksum for current files.
	currentByName := make(map[string]string, len(current.Files))
	for _, f := range current.Files {
		currentByName[f.Name] = f.Checksum
	}

	// Compare each expected file against current.
	for _, expected := range expectedManifest.Files {
		cur, exists := currentByName[expected.Name]
		if !exists || cur != expected.Checksum {
			changed = append(changed, expected.Name)
		}
	}

	return changed, nil
}

// WriteFixtureManifest writes a fixture manifest to .nself/seed-manifests/{fixture}.json.
func WriteFixtureManifest(projectDir string, manifest FixtureManifest) error {
	dir := filepath.Join(projectDir, ".nself", "seed-manifests")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating seed-manifests dir: %w", err)
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling fixture manifest: %w", err)
	}

	path := filepath.Join(dir, manifest.Name+".json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing fixture manifest %s: %w", path, err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("chmod fixture manifest %s: %w", path, err)
	}

	return nil
}

// LoadStoredManifest reads a previously stored manifest from disk.
// Returns nil, nil if no stored manifest exists.
func LoadStoredManifest(projectDir, fixtureName string) (*FixtureManifest, error) {
	path := filepath.Join(projectDir, ".nself", "seed-manifests", fixtureName+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// KEEP. No stored manifest means this fixture has never been
			// seeded, which is exactly what the caller needs to know to decide
			// to seed it. ENOENT only — an unreadable manifest errors rather
			// than triggering a re-seed over data that is already there.
			return nil, nil
		}
		return nil, fmt.Errorf("reading stored manifest %s: %w", path, err)
	}

	var m FixtureManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing stored manifest %s: %w", path, err)
	}

	return &m, nil
}

// RunFixture executes all SQL files in a fixture in dependency order.
// It calls the provided runner function for each file.
func RunFixture(projectDir, fixtureName string, runner SQLRunner) (FixtureRunResult, error) {
	start := time.Now()
	result := FixtureRunResult{FixtureName: fixtureName}

	// Collect seeds that belong to this fixture.
	all, err := ListSeeds(projectDir)
	if err != nil {
		return result, fmt.Errorf("listing seeds for fixture %q: %w", fixtureName, err)
	}

	prefix := "fixture:" + fixtureName
	var fixtureSeeds []SeedFile
	for _, s := range all {
		if s.Env == prefix {
			fixtureSeeds = append(fixtureSeeds, s)
		}
	}

	if len(fixtureSeeds) == 0 {
		return result, fmt.Errorf("fixture %q has no seed files", fixtureName)
	}

	// Resolve dependency order.
	ordered, err := ResolveDependencyOrder(fixtureSeeds)
	if err != nil {
		return result, fmt.Errorf("resolving dependency order for fixture %q: %w", fixtureName, err)
	}

	for _, s := range ordered {
		if err := runner(s.Path); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("seed %s: %w", s.Name, err))
		} else {
			result.FilesRun++
		}
	}

	result.Duration = time.Since(start)

	if len(result.Errors) > 0 {
		return result, result.Errors[0]
	}
	return result, nil
}

// sha256File computes the hex-encoded SHA256 checksum of a file's contents.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening file for checksum %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("computing checksum for %s: %w", path, err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
