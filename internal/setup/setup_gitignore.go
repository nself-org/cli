package setup

// setup_gitignore.go — .gitignore scaffolding for a new project.
//
// Purpose: ensure a freshly initialized project's .gitignore covers the standard nSelf-generated paths, used by Initialize in setup.go, split out for file size.
// Inputs: the project directory to scaffold.
// Outputs: a written or updated .gitignore file.
// Constraints: split from setup.go (CLI-R12 Batch E). P7-CI-58 (ADR 0024 §6):
// authored .nself/ files are committed, state is ignored, so new projects get
// `.nself/*` + `!.nself/ci.yaml`. An existing `.nself/` line is left alone and
// nothing .nself-related is appended: git cannot re-include a file under an
// excluded directory, so appending would look fixed while staying ignored.

import (
	"os"
	"path/filepath"
	"strings"
)

// gitignoreEntries are appended to .gitignore if not already present.
var gitignoreEntries = []string{
	".env",
	".env.local",
	".env.*.local",
	".env.secrets",
	".env.ai",
	".volumes/",
	"logs/",
	"*.log",
	"node_modules/",
	".DS_Store",
	".nself/*",
	"!.nself/ci.yaml", // after the wildcard: a negation only re-includes what an earlier line excluded
}

// nselfDirLines are the spellings of an existing whole-directory ignore that
// a `!.nself/ci.yaml` negation cannot undo.
var nselfDirLines = map[string]bool{".nself/": true, "/.nself/": true, ".nself": true, "/.nself": true}

// ensureGitignore creates or appends to .gitignore with required entries.
func ensureGitignore(workDir string) error {
	giPath := filepath.Join(workDir, ".gitignore")
	existing := ""
	if data, err := os.ReadFile(giPath); err == nil {
		existing = string(data)
	}

	lines := map[string]bool{}
	for _, l := range strings.Split(existing, "\n") {
		lines[strings.TrimSpace(l)] = true
	}
	dirIgnored := false
	for l := range nselfDirLines {
		dirIgnored = dirIgnored || lines[l]
	}
	var toAdd []string
	for _, entry := range gitignoreEntries {
		if strings.HasPrefix(entry, ".nself/") || strings.HasPrefix(entry, "!.nself/") {
			// Line-exact: ".nself/*" must not count as present because the
			// text ".nself/" is; and an existing directory ignore wins.
			if !dirIgnored && !lines[entry] {
				toAdd = append(toAdd, entry)
			}
			continue
		}
		if !strings.Contains(existing, entry) {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) == 0 {
		return nil // Nothing to add.
	}

	f, err := os.OpenFile(giPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	// Add a header comment if the file is new or we're appending.
	header := "\n# nSelf\n"
	if existing == "" {
		header = "# nSelf\n"
	}
	if _, err := f.WriteString(header); err != nil {
		return err
	}
	for _, entry := range toAdd {
		if _, err := f.WriteString(entry + "\n"); err != nil {
			return err
		}
	}
	return nil
}

// writeFile writes content to path with the given permissions.
// Parent directories are created as needed.
func writeFile(path, content string, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), perm)
}
