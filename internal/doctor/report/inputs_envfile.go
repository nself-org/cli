package report

// inputs_envfile.go: env files read as scrub inputs (lead rulings R3, R5 a/b).
//
// Purpose: the files a stack feeds its containers from are scrub inputs:
// CS_N_ENV_FILE custom-service env files, and env_file entries (string, list of
// strings, list of {path: ...}) of the generated compose file and every
// manifest fragment.
// Inputs: the input set (project dir, every CS_N_ENV_FILE value seen), compose
//         documents.
// Outputs: values added to the input set; one manifest note per miss.
// Constraints: scrub inputs only. A file must resolve inside the project dir
//         (symlinks resolved and re-checked), be regular, and be at most 1 MiB.
//         CS_N_ENV_FILE paths are project-relative with no absolute path or
//         "..". Compose env_file paths are relative to the document's own
//         directory, then to the project dir; an absolute one is accepted only
//         when it lands inside the project.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var csEnvFileKeyRe = regexp.MustCompile(`^CS_[0-9]+_ENV_FILE$`)

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

// loadCustomEnvFiles learns the target of every CS_N_ENV_FILE value seen in any
// cascade layer or the process environment. The build keeps only one winner
// per key; which one depends on cascade order and on the process env, so every
// candidate is read (over-reading a scrub input is safe).
func loadCustomEnvFiles(in *inputSet) {
	keys := make([]string, 0, len(in.refs))
	for k := range in.refs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range in.refs[k] {
			for _, rel := range strings.Split(TrimQuotes(v), ",") {
				if rel = strings.TrimSpace(rel); rel != "" {
					in.learnEnvFile("CS_N_ENV_FILE", in.projectDir, rel, true)
				}
			}
		}
	}
}

// learnComposeEnvFile learns one env_file entry of the compose document in dir.
func (in *inputSet) learnComposeEnvFile(dir, rel string) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return
	}
	in.learnEnvFile("compose env_file", dir, rel, false)
}

// learnEnvFile resolves rel (strict: CS_N_ENV_FILE rules) and learns the file.
// A non-strict miss under base is retried under the project dir.
func (in *inputSet) learnEnvFile(label, base, rel string, strict bool) {
	path, ok := confinedFile(in.projectDir, base, rel, strict)
	if !ok && !strict && base != in.projectDir {
		path, ok = confinedFile(in.projectDir, in.projectDir, rel, strict)
	}
	if !ok {
		in.notes = append(in.notes, fmt.Sprintf("%s %q skipped as a scrub input: missing, not a regular file, or outside the project directory", label, rel))
		return
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- confined to the project dir by confinedFile
	if err != nil {
		in.notes = append(in.notes, fmt.Sprintf("%s %q unreadable: values in it cannot be scrubbed", label, rel))
		return
	}
	in.learnEnvText(string(raw), false)
}

// walkEnvFiles calls fn for every env_file entry at any depth: a string, a list
// of strings, or a list of {path: ...} maps.
func walkEnvFiles(n any, fn func(path string)) {
	switch t := n.(type) {
	case map[string]any:
		for k, v := range t {
			if k == "env_file" {
				emitEnvFiles(v, fn)
			}
			walkEnvFiles(v, fn)
		}
	case []any:
		for _, v := range t {
			walkEnvFiles(v, fn)
		}
	}
}

func emitEnvFiles(v any, fn func(path string)) {
	switch t := v.(type) {
	case string:
		fn(t)
	case map[string]any:
		if p, ok := t["path"].(string); ok {
			fn(p)
		}
	case []any:
		for _, item := range t {
			emitEnvFiles(item, fn)
		}
	}
}

// confinedFile resolves rel under base and returns the real path when it is a
// regular file of at most maxFileBytes that stays inside projectDir. strict
// rejects absolute paths and ".." parts before resolving.
func confinedFile(projectDir, base, rel string, strict bool) (string, bool) {
	abs := filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`)
	if strict {
		if abs {
			return "", false
		}
		for _, part := range strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == '\\' }) {
			if part == ".." {
				return "", false
			}
		}
	}
	root, err := filepath.EvalSymlinks(projectDir)
	if err != nil {
		return "", false
	}
	cand := filepath.Join(base, rel)
	if abs {
		cand = filepath.Clean(rel)
	}
	real, err := filepath.EvalSymlinks(cand)
	if err != nil {
		return "", false
	}
	if r, rerr := filepath.Rel(root, real); rerr != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", false
	}
	st, err := os.Stat(real)
	if err != nil || !st.Mode().IsRegular() || st.Size() > maxFileBytes {
		return "", false
	}
	return real, true
}
