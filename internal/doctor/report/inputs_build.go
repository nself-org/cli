package report

// inputs_build.go: build-written and extra env files read as scrub inputs
// (lead ruling R6 a).
//
// Purpose: the secrets a stack really runs with can live in files the env
// cascade does not name: .nself/compose.env (the interpolation file `nself
// build` writes and every `docker compose` call reads), .env.computed, and any
// .env.<name> a stack was started with (ENV=demo). Their values are scrub
// inputs.
// Inputs: the project dir and the cascade file names already read.
// Outputs: values added to the input set; one note per unreadable or oversized
//         file.
// Constraints: scrub inputs ONLY: the key names of these files are never
//         listed in config-keys.txt (that file lists the env cascade). Over
//         reading a scrub input is safe, so every regular .env.* file counts.
//         Size cap 1 MiB per file.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// computedEnvFile is the .env.computed file `nself build` writes (Step 10 in
// internal/build/orchestrator_build_finish.go; no exported name there).
const computedEnvFile = ".env.computed"

// readEnvInput learns one dotenv file; name is its label in notes. A missing or
// non-regular path is skipped silently.
func (in *inputSet) readEnvInput(path, name string, listKeys bool) {
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		return
	}
	if st.Size() > maxFileBytes {
		in.notes = append(in.notes, fmt.Sprintf("%s skipped as a scrub input: larger than 1 MiB", name))
		return
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- project env file read as a scrub input only
	if err != nil {
		in.notes = append(in.notes, fmt.Sprintf("%s unreadable: values in it cannot be scrubbed", name))
		return
	}
	in.learnEnvText(string(raw), listKeys)
}

// loadBuildInputs reads the env files `nself build` writes for compose
// interpolation (composeFiles: build.ComposeEnvFiles, passed in by the command;
// internal/doctor may not import internal/build), .env.computed and every other
// .env.* regular file of the project dir. Cascade files already read are
// skipped.
func loadBuildInputs(projectDir string, composeFiles []string, in *inputSet) {
	for _, p := range composeFiles {
		in.readEnvInput(p, filepath.Base(p), false)
	}
	extra := map[string]bool{computedEnvFile: true}
	if ents, err := os.ReadDir(projectDir); err == nil {
		for _, e := range ents {
			if n := e.Name(); strings.HasPrefix(n, ".env.") && len(n) > len(".env.") {
				extra[n] = true
			}
		}
	}
	names := make([]string, 0, len(extra))
	for n := range extra {
		if !in.cascade[n] {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		in.readEnvInput(filepath.Join(projectDir, n), n, false)
	}
}
