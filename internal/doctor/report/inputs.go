package report

// inputs.go: reads the project files the bundle is built from.
//
// Purpose: env key names + secret values (scrub inputs only), the generated
// compose file and its environment values, and the generated nginx confs.
// Inputs: the project directory.
// Outputs: SecretValue lists, key names, file texts, notes.
// Constraints: env files are read only to learn names and values to scrub; a
//         value never reaches the bundle. nginx: regular *.conf files only,
//         no symlinks, no ssl/ or key/cert directories, nothing with a PEM
//         header. Size caps keep a huge file from filling the bundle.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/joho/godotenv"
	"github.com/nself-org/cli/internal/config"
	"gopkg.in/yaml.v3"
)

const maxFileBytes = 1 << 20

var skipNginxDirs = map[string]bool{
	"ssl": true, "certs": true, "certificates": true, "keys": true, "private": true, "secrets": true,
}

// loadEnvInputs reads every env cascade file (the resolved environment's
// cascade plus every other known layer) and returns the secret values, the
// sorted key names, and notes.
func loadEnvInputs(projectDir string) (values []SecretValue, keys []string, notes []string) {
	env, _ := config.ResolveEnv(projectDir)
	names := map[string]bool{}
	for _, f := range config.EnvCascade(projectDir, env, config.LegacyOrderActive()) {
		names[f.Name] = true
	}
	for _, n := range config.AllCascadeFilenames {
		names[n] = true
	}
	seenKey, seenVal := map[string]bool{}, map[string]bool{}
	addPair := func(k, v string) {
		seenKey[k] = true
		for _, sv := range ExpandSecretValues(k, v) {
			if id := k + "\x00" + sv.Variants[0]; !seenVal[id] {
				seenVal[id] = true
				values = append(values, sv)
			}
		}
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)
	for _, n := range sorted {
		path := filepath.Join(projectDir, n)
		st, err := os.Stat(path)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		if st.Size() > maxFileBytes {
			notes = append(notes, fmt.Sprintf("%s skipped as a scrub input: larger than 1 MiB", n))
			continue
		}
		raw, err := os.ReadFile(path) // #nosec G304 -- fixed cascade names under the project dir
		if err != nil {
			notes = append(notes, fmt.Sprintf("%s unreadable: values in it cannot be scrubbed", n))
			continue
		}
		if m, perr := godotenv.Unmarshal(string(raw)); perr == nil {
			for k, v := range m {
				addPair(k, v)
			}
		}
		for _, line := range strings.Split(string(raw), "\n") { // raw spelling, even if godotenv refused the file
			line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
			if k, v, ok := strings.Cut(line, "="); ok && !strings.HasPrefix(k, "#") && strings.TrimSpace(k) != "" {
				addPair(strings.TrimSpace(k), v)
			}
		}
	}
	for k := range seenKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return values, keys, notes
}

// readCompose returns the generated docker-compose.yml text and the secret
// values found in environment/args/labels maps and lists anywhere in it.
func readCompose(projectDir string) (text string, values []SecretValue, note string) {
	path := filepath.Join(projectDir, "docker-compose.yml")
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() {
		return "", nil, "docker-compose.yml not found: run nself build"
	}
	if st.Size() > 4*maxFileBytes {
		return "", nil, "docker-compose.yml skipped: larger than 4 MiB"
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- fixed name under the project dir
	if err != nil {
		return "", nil, "docker-compose.yml unreadable"
	}
	var doc any
	if yaml.Unmarshal(raw, &doc) != nil {
		note = "docker-compose.yml is not valid YAML: environment values were not extracted"
	} else {
		walkEnv(doc, func(k, v string) {
			values = append(values, ExpandSecretValues(k, v)...)
		})
	}
	return string(raw), values, note
}

// walkEnv calls fn for every KEY/VALUE under a map key named environment, args
// or labels, in map form or "KEY=VALUE" list form, at any depth.
func walkEnv(n any, fn func(k, v string)) {
	switch t := n.(type) {
	case map[string]any:
		for k, v := range t {
			if k == "environment" || k == "args" || k == "labels" {
				emitPairs(v, fn)
			}
			walkEnv(v, fn)
		}
	case []any:
		for _, v := range t {
			walkEnv(v, fn)
		}
	}
}

func emitPairs(v any, fn func(k, v string)) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if val != nil {
				fn(k, fmt.Sprint(val))
			}
		}
	case []any:
		for _, item := range t {
			if s, ok := item.(string); ok {
				if k, val, found := strings.Cut(s, "="); found {
					fn(k, val)
				}
			}
		}
	}
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
