package report

// inputs.go: reads the project files the bundle is built from.
//
// Purpose: env key names + secret values (scrub inputs only) from every env
// cascade layer, and the generated compose file with its environment values.
// Inputs: the project directory.
// Outputs: an inputSet (values, key names, short-secret names), file texts, notes.
// Constraints: env files are read only to learn names and values to scrub; a
//         value never reaches the bundle. Size caps keep a huge file from
//         filling memory. More inputs: inputs_extra.go.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/joho/godotenv"
	"github.com/nself-org/cli/internal/config"
	"gopkg.in/yaml.v3"
)

const maxFileBytes = 1 << 20

// inputSet accumulates the scrub inputs of one collection.
type inputSet struct {
	values  []SecretValue
	keys    map[string]bool     // cascade key names listed in config-keys.txt
	refs    map[string][]string // CS_N_ENV_FILE key -> every value seen (cascade layers, process env)
	seen    map[string]bool
	short   map[string]bool // secret-named keys whose value is under MinSecretLen
	notes   []string
	process bool // learning from the process environment (shorts not counted)

	projectDir string // confinement root for env files read as inputs
}

func newInputSet(projectDir string) *inputSet {
	return &inputSet{keys: map[string]bool{}, refs: map[string][]string{}, seen: map[string]bool{}, short: map[string]bool{}, projectDir: projectDir}
}

// learn adds the scrub inputs of one key=value pair.
func (in *inputSet) learn(k, v string) {
	if csEnvFileKeyRe.MatchString(k) {
		in.refs[k] = appendUnique(in.refs[k], v)
	}
	if !in.process && IsKeyName(k) && IsSecretName(k) {
		if tv := TrimQuotes(v); tv != "" && len(tv) < MinSecretLen && !refValueRe.MatchString(tv) && !isWordValue(tv) {
			in.short[k] = true
		}
	}
	for _, sv := range ExpandSecretValues(k, v) {
		if id := sv.Key + "\x00" + sv.Variants[0]; !in.seen[id] {
			in.seen[id] = true
			in.values = append(in.values, sv)
		}
	}
}

// learnEnvText learns a dotenv text; listKeys also records its key names.
func (in *inputSet) learnEnvText(raw string, listKeys bool) {
	add := func(k, v string) {
		in.learn(k, v)
		if listKeys && IsKeyName(k) {
			in.keys[k] = true
		}
	}
	if m, err := godotenv.Unmarshal(raw); err == nil {
		for k, v := range m {
			add(k, v)
		}
	}
	for _, p := range scanDotenv(raw) { // raw spelling, even if godotenv refused the file
		add(p.Key, p.Value)
	}
}

// sortedKeys returns the cascade key names, sorted.
func (in *inputSet) sortedKeys() []string {
	out := make([]string, 0, len(in.keys))
	for k := range in.keys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// shortNote describes the secret-named values too short to be literal inputs.
func (in *inputSet) shortNote() string {
	if len(in.short) == 0 {
		return ""
	}
	names := make([]string, 0, len(in.short))
	for k := range in.short {
		names = append(names, k)
	}
	sort.Strings(names)
	list := names
	more := ""
	if len(list) > 20 {
		list, more = list[:20], fmt.Sprintf(" and %d more", len(names)-20)
	}
	return fmt.Sprintf("%d secret-named values under %d characters were not literal-scrubbed (names: %s%s); check them by hand before sharing",
		len(names), MinSecretLen, strings.Join(list, ", "), more)
}

// loadEnvInputs reads every env cascade file (the resolved environment's
// cascade plus every other known layer).
func loadEnvInputs(projectDir string, in *inputSet) {
	env, _ := config.ResolveEnv(projectDir)
	names := map[string]bool{}
	for _, f := range config.EnvCascade(projectDir, env, config.LegacyOrderActive()) {
		names[f.Name] = true
	}
	for _, n := range config.AllCascadeFilenames {
		names[n] = true
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
			in.notes = append(in.notes, fmt.Sprintf("%s skipped as a scrub input: larger than 1 MiB", n))
			continue
		}
		raw, err := os.ReadFile(path) // #nosec G304 -- fixed cascade names under the project dir
		if err != nil {
			in.notes = append(in.notes, fmt.Sprintf("%s unreadable: values in it cannot be scrubbed", n))
			continue
		}
		in.learnEnvText(string(raw), true)
	}
}

// readCompose returns the generated docker-compose.yml text and learns the
// secret values found in environment/args/labels maps and lists anywhere in it,
// plus the files its env_file entries name.
func readCompose(projectDir string, in *inputSet) (text, note string) {
	path := filepath.Join(projectDir, "docker-compose.yml")
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() {
		return "", "docker-compose.yml not found: run nself build"
	}
	if st.Size() > 4*maxFileBytes {
		return "", "docker-compose.yml skipped: larger than 4 MiB"
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- fixed name under the project dir
	if err != nil {
		return "", "docker-compose.yml unreadable"
	}
	if !learnComposeYAML(raw, projectDir, in) {
		note = "docker-compose.yml is not valid YAML: environment values were not extracted"
	}
	return string(raw), note
}

// learnComposeYAML learns the environment values of a compose document and the
// files of its env_file entries (resolved against dir, the document's own
// directory; see learnComposeEnvFile).
func learnComposeYAML(raw []byte, dir string, in *inputSet) bool {
	var doc any
	if yaml.Unmarshal(raw, &doc) != nil {
		return false
	}
	walkEnv(doc, in.learn)
	walkEnvFiles(doc, func(p string) { in.learnComposeEnvFile(dir, p) })
	return true
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
