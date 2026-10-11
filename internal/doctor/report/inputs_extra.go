package report

// inputs_extra.go: the scrub inputs beyond the env cascade (lead ruling R3).
//
// Purpose: the process environment (the doctor child inherits it and shell or
// CI exported secrets drive the build), compose manifest fragments listed in
// .nself/compose-files.txt (plugin fragments, user override), and the
// CS_N_ENV_FILE custom-service env files.
// Inputs: project dir, fragment paths, an environ seam.
// Outputs: values added to an inputSet; notes. Never bundle contents.
// Constraints: scrub inputs only. CS_N_ENV_FILE paths stay inside the project
//         dir (no absolute paths, no .., symlinks resolved and re-checked);
//         files are regular and at most 1 MiB.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var csEnvFileKeyRe = regexp.MustCompile(`^CS_[0-9]+_ENV_FILE$`)

// loadProcessEnv learns every secret-named variable of the environment.
func loadProcessEnv(environ []string, in *inputSet) {
	in.process = true
	defer func() { in.process = false }()
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok && k != "" {
			in.learn(k, v)
		}
	}
}

// loadFragments learns the environment values of the listed compose files.
func loadFragments(files []string, in *inputSet) {
	for _, f := range files {
		st, err := os.Stat(f)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		if st.Size() > 4*maxFileBytes {
			in.notes = append(in.notes, fmt.Sprintf("compose fragment %s skipped as a scrub input: larger than 4 MiB", filepath.Base(f)))
			continue
		}
		raw, err := os.ReadFile(f) // #nosec G304 -- listed in .nself/compose-files.txt
		if err != nil {
			in.notes = append(in.notes, fmt.Sprintf("compose fragment %s unreadable: values in it cannot be scrubbed", filepath.Base(f)))
			continue
		}
		if !learnComposeYAML(raw, in) {
			in.notes = append(in.notes, fmt.Sprintf("compose fragment %s is not valid YAML: environment values were not extracted", filepath.Base(f)))
		}
	}
}

// loadCustomEnvFiles learns the values of every CS_N_ENV_FILE target, found in
// the cascade and in the process environment.
func loadCustomEnvFiles(projectDir string, environ []string, in *inputSet) {
	refs := map[string]string{}
	for k, v := range in.refs {
		if csEnvFileKeyRe.MatchString(k) {
			refs[k] = v
		}
	}
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok && csEnvFileKeyRe.MatchString(k) {
			refs[k] = v
		}
	}
	for _, v := range refs {
		for _, rel := range strings.Split(TrimQuotes(v), ",") {
			rel = strings.TrimSpace(rel)
			if rel == "" {
				continue
			}
			path, ok := confinedFile(projectDir, rel)
			if !ok {
				in.notes = append(in.notes, fmt.Sprintf("CS_N_ENV_FILE %q skipped as a scrub input: missing, not a regular file, or outside the project directory", rel))
				continue
			}
			if raw, err := os.ReadFile(path); err == nil { // #nosec G304 -- confined to the project dir by confinedFile
				in.learnEnvText(string(raw), false)
				continue
			}
			in.notes = append(in.notes, fmt.Sprintf("CS_N_ENV_FILE %q unreadable: values in it cannot be scrubbed", rel))
		}
	}
}

// confinedFile resolves rel under projectDir and returns the real path when it
// is a regular file of at most maxFileBytes that stays inside the project.
func confinedFile(projectDir, rel string) (string, bool) {
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) {
		return "", false
	}
	for _, part := range strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return "", false
		}
	}
	root, err := filepath.EvalSymlinks(projectDir)
	if err != nil {
		return "", false
	}
	real, err := filepath.EvalSymlinks(filepath.Join(projectDir, rel))
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
