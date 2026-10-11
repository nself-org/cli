package report

// nginx.go: reads the generated nginx confs into the bundle.
//
// Purpose: include the served nginx configuration for routing support.
// Inputs: the served nginx directory (nginxtopo).
// Outputs: bundle entries nginx/<relative path>, notes.
// Constraints: regular *.conf files only (no symlinks: a symlinked root yields
//         nothing), no ssl/ or key/cert directories, nothing with a PEM header,
//         1 MiB per file.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var skipNginxDirs = map[string]bool{
	"ssl": true, "certs": true, "certificates": true, "keys": true, "private": true, "secrets": true,
}

// readNginx returns <root>/**/*.conf regular files as bundle entries named
// nginx/<relative path>. root is the served nginx directory (nginxtopo).
func readNginx(root string) (files []File, notes []string) {
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != root && skipNginxDirs[strings.ToLower(d.Name())] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || !strings.HasSuffix(strings.ToLower(d.Name()), ".conf") { // symlinks, keys, certs
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		name := "nginx/" + filepath.ToSlash(rel)
		if st, serr := d.Info(); serr != nil || st.Size() > maxFileBytes {
			notes = append(notes, name+" skipped: larger than 1 MiB")
			return nil
		}
		data, rerr := os.ReadFile(p) // #nosec G304 -- walked under the project nginx dir
		if rerr != nil {
			return nil
		}
		if strings.Contains(string(data), "-----BEGIN") {
			notes = append(notes, name+" skipped: holds PEM material")
			return nil
		}
		files = append(files, File{Name: name, Data: data})
		return nil
	})
	return files, notes
}
