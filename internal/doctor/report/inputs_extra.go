package report

// inputs_extra.go: the scrub inputs beyond the env cascade (lead ruling R3).
//
// Purpose: the process environment (the doctor child inherits it and shell or
// CI exported secrets drive the build), compose manifest fragments listed in
// .nself/compose-files.txt (plugin fragments, user override), the stored
// licence key, and the env files named by CS_N_ENV_FILE and compose env_file
// (inputs_envfile.go).
// Inputs: project dir, fragment paths, an environ seam.
// Outputs: values added to an inputSet; notes. Never bundle contents.
// Constraints: scrub inputs only. CS_N_ENV_FILE paths stay inside the project
//         dir (no absolute paths, no .., symlinks resolved and re-checked);
//         files are regular and at most 1 MiB.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/license"
)

// loadProcessEnv learns every secret-named variable of the environment. PWD and
// OLDPWD are the shell's directory variables (they match the PWD secret word),
// not secrets: learning them would redact the working directory everywhere.
func loadProcessEnv(environ []string, in *inputSet) {
	in.process = true
	defer func() { in.process = false }()
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok && k != "" && k != "PWD" && k != "OLDPWD" {
			in.recordEnv(k, v)
			in.learn(k, v)
		}
	}
}

// loadLicenseKeys learns the licence keys the CLI would use: the stored key
// (license.GetKey, ~/.nself/license/key), NSELF_PLUGIN_LICENSE_KEY and
// NSELF_LICENSE_KEY_1..10. Scrub inputs only; nothing here reaches the bundle.
func loadLicenseKeys(in *inputSet) {
	in.process = true
	defer func() { in.process = false }()
	for _, k := range license.CollectLicenseKeys() {
		in.learn("NSELF_LICENSE_KEY", k)
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
		if !learnComposeYAML(raw, filepath.Dir(f), in) {
			in.notes = append(in.notes, fmt.Sprintf("compose fragment %s is not valid YAML: environment values were not extracted", filepath.Base(f)))
		}
	}
}
