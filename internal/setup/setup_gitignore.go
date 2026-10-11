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
// D-0163: `/ssl/` (anchored, so src/ssl/ stays tracked) is ignored so
// ssl/certificates/*/privkey.pem never lands in git. Lines are matched like
// git does (only a trailing CR and trailing spaces are dropped, so a leading
// space or tab is part of the pattern), whole-line, so `ssl/.acme/` does not
// count. A negation that can reach ssl/ (see negationMayHitSSL) after the last
// ssl-dir line re-includes keys, so the entry is appended again.

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
	"/ssl/", // certificate private keys (D-0163)
	".nself/*",
	"!.nself/ci.yaml", // after the wildcard: a negation only re-includes what an earlier line excluded
}

// nselfDirLines are the spellings of an existing whole-directory ignore that
// a `!.nself/ci.yaml` negation cannot undo.
var nselfDirLines = map[string]bool{".nself/": true, "/.nself/": true, ".nself": true, "/.nself": true}

// sslDirLines are existing spellings that already ignore everything under
// ssl/. Only whole normalised lines count: `ssl/.acme/` contains "ssl/" but
// leaves the certificate keys committable. `ssl/*` and `ssl/**` are not
// listed: they leave the dir itself unexcluded, so a nested ssl/.gitignore
// `!privkey.pem` could re-include keys. They count only when no ssl
// negation follows (see sslIgnored).
var sslDirLines = map[string]bool{"ssl/": true, "/ssl/": true, "ssl": true, "/ssl": true}

// ensureGitignore creates or appends to .gitignore with required entries.
func ensureGitignore(workDir string) error {
	giPath := filepath.Join(workDir, ".gitignore")
	existing := ""
	if data, err := os.ReadFile(giPath); err == nil {
		existing = string(data)
	}

	lines := map[string]bool{}
	var ordered []string
	for _, l := range strings.Split(existing, "\n") {
		n := gitignoreLine(l)
		lines[n] = true
		ordered = append(ordered, n)
	}
	dirIgnored := anyLine(lines, nselfDirLines)
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
		if entry == "/ssl/" {
			if !sslIgnored(ordered) {
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

// gitignoreLine normalises a line the way git reads a pattern: only a
// trailing CR and trailing spaces are dropped; leading whitespace and tabs
// stay part of the pattern.
func gitignoreLine(l string) string {
	if i := strings.IndexByte(l, 0); i >= 0 {
		l = l[:i] // git truncates a pattern at the first NUL
	}
	return strings.TrimRight(strings.TrimSuffix(l, "\r"), " ")
}

// negationMayHitSSL reports whether a normalised "!" line could re-include
// something under ssl/. Only a pattern anchored to the .gitignore dir (a "/"
// before its last char) whose first segment is a plain name other than ssl
// (git may match case-insensitively) is harmless, e.g. !.nself/ci.yaml.
func negationMayHitSSL(l string) bool {
	p := strings.TrimPrefix(strings.TrimPrefix(l, "!"), "/")
	i := strings.Index(p, "/")
	if i < 0 || i >= len(p)-1 {
		return true
	}
	seg := p[:i]
	return strings.ContainsAny(seg, "*?[\\") || strings.ToLower(seg) == "ssl"
}

// sslIgnored reports whether the last ssl-dir line comes after the last
// negation line that may reach ssl/, so no negation re-includes the keys.
func sslIgnored(lines []string) bool {
	lastDir, lastNeg := -1, -1
	for i, l := range lines {
		if sslDirLines[l] {
			lastDir = i
		} else if strings.HasPrefix(l, "!") && negationMayHitSSL(l) {
			lastNeg = i
		}
	}
	return lastDir >= 0 && lastDir > lastNeg
}

// anyLine reports whether any of the spellings is a whole line in lines.
func anyLine(lines, spellings map[string]bool) bool {
	for l := range spellings {
		if lines[l] {
			return true
		}
	}
	return false
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
