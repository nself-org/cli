package reconcile

// overlay.go — reads project files as the planned build would leave them
// (EPIC P7-LIVE D7): planned bytes first, a planned removal as absent, then a
// pending env file (P7-LIVE-05: Request.EnvOverlay, the line `config set` is
// about to write), else the file on disk. Split from containers.go for the
// file-size cap.

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/joho/godotenv"
	nbuild "github.com/nself-org/cli/internal/build"
)

// overlay reads project files as the planned build would leave them: planned
// bytes first, a planned removal as absent, else the file on disk.
type overlay struct {
	dir      string
	fronting string
	files    map[string]nbuild.PlannedFile
	removed  map[string]bool
	env      map[string][]byte // pending env files by project-relative name
}

// newOverlay indexes a planned build.
func newOverlay(dir, fronting string, pb *nbuild.PlannedBuild) overlay {
	ov := overlay{dir: dir, fronting: fronting, files: pb.Files, removed: map[string]bool{}}
	for _, k := range pb.Removed {
		ov.removed[k] = true
	}
	return ov
}

// display is the plan path of a canonical key: project-relative and
// "@fronting/" keys as they are, an absolute key under the plugin directory as
// "@plugins/<rel>", any other absolute key as "@abs/<path>".
func (o overlay) display(key string) string {
	if !filepath.IsAbs(key) {
		return key
	}
	if rel, err := filepath.Rel(nbuild.DefaultPluginDir(), key); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return PluginsPrefix + filepath.ToSlash(rel)
	}
	return AbsPrefix + strings.TrimPrefix(filepath.ToSlash(key), "/")
}

// diskPath resolves a canonical key to its location on disk.
func (o overlay) diskPath(key string) string {
	switch {
	case strings.HasPrefix(key, FrontingPrefix):
		return filepath.Join(o.fronting, filepath.FromSlash(strings.TrimPrefix(key, FrontingPrefix)))
	case filepath.IsAbs(key):
		return key
	}
	return filepath.Join(o.dir, filepath.FromSlash(key))
}

// read returns the bytes of a canonical key as planned. ok is false when the
// file would not exist; an error is a read failure other than not-exist.
func (o overlay) read(key string) ([]byte, bool, error) {
	if f, ok := o.files[key]; ok {
		return f.Data, true, nil
	}
	if o.removed[key] {
		return nil, false, nil
	}
	if data, ok := o.env[key]; ok {
		return data, true, nil
	}
	data, err := os.ReadFile(o.diskPath(key))
	switch {
	case err == nil:
		return data, true, nil
	case os.IsNotExist(err):
		return nil, false, nil
	}
	return nil, false, err
}

// readAbs is read for an absolute path: it is looked up under the key the
// build uses (project-relative inside the project, absolute outside).
func (o overlay) readAbs(path string) ([]byte, bool, error) {
	if rel, err := filepath.Rel(o.dir, path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return o.read(filepath.ToSlash(rel))
	}
	return o.read(filepath.ToSlash(path))
}

// pendingEnvPerm is the mode `config set` leaves on every env file it writes.
const pendingEnvPerm fs.FileMode = 0o600

// pendingEnv renders Request.EnvOverlay against the disk: for each env file
// (project-relative name) the bytes it will hold once every pending line is
// written, by the same rewrite `config set` uses; changed reports whether any
// file's bytes differ from the disk (or the file does not exist yet). It reads the disk each time,
// so a plan recomputed after the line was written sees the same bytes
// (EnvFileWithLine is idempotent) and an edit made meanwhile is not hidden.
func pendingEnv(dir string, in map[string]map[string]string) (map[string][]byte, bool, error) {
	if len(in) == 0 {
		return nil, false, nil
	}
	out := make(map[string][]byte, len(in))
	changed := false
	for name, kv := range in {
		if !validEnvName(name) {
			return nil, false, fmt.Errorf("env overlay: %q is not an env file at the project root", name)
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		exists := err == nil
		if err != nil && !os.IsNotExist(err) {
			return nil, false, fmt.Errorf("reading %s for the plan: %w", name, err)
		}
		was := string(data)
		keys := make([]string, 0, len(kv))
		for k := range kv {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		present := exists
		for _, k := range keys {
			if data, _, err = EnvFileWithLine(data, present, k, kv[k]); err != nil {
				return nil, false, fmt.Errorf("scanning %s: %w", name, err)
			}
			present = true
		}
		changed = changed || !exists || string(data) != was
		out[name] = data
	}
	return out, changed, nil
}

// validEnvName accepts a root-level ".env" or ".env.<suffix>" name.
func validEnvName(name string) bool {
	return (name == ".env" || strings.HasPrefix(name, ".env.")) && !strings.ContainsAny(name, `/\`)
}

// pendingHandEdited judges an overlaid env file on its pending bytes, as the
// apply will find it after the line is written; other paths go to next. A nil
// next (no hand-edit detection asked for) stays nil.
// A fronting path is resolved against the fronting stack the pending env
// names (a change of NGINX_FRONTED_BY moves it).
func pendingHandEdited(next func(string) bool, state *GeneratedState, env map[string][]byte, projectDir string) func(string) bool {
	if len(env) == 0 || next == nil {
		return next
	}
	fronting := state.handEditedFnWith(projectDir, env)
	return func(path string) bool {
		if data, ok := env[path]; ok {
			return state.HandEdited(path, data)
		}
		if strings.HasPrefix(path, FrontingPrefix) {
			return fronting(path)
		}
		return next(path)
	}
}

// EnvFileWithLine returns old with KEY=VALUE set: every line whose key is key
// is replaced, else the line is appended; comments and blank lines are kept,
// lines end in "\n". exists false means no file yet (the result is the line
// alone). found reports a replacement. This is the one rewrite `config set`
// writes and the planner previews (P7-LIVE-05). A value the dotenv parser
// cannot read back unchanged returns an error without the value (see
// quoteEnvLineValue).
func EnvFileWithLine(old []byte, exists bool, key, value string) (out []byte, found bool, err error) {
	enc, err := quoteEnvLineValue(key, value)
	if err != nil {
		return nil, false, err
	}
	newLine := key + "=" + enc
	if !exists {
		return []byte(newLine + "\n"), false, nil
	}
	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(old))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			lines = append(lines, line)
			continue
		}
		if eq := strings.IndexByte(trimmed, '='); eq > 0 && strings.TrimSpace(trimmed[:eq]) == key {
			lines = append(lines, newLine)
			found = true
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, false, err
	}
	if !found {
		lines = append(lines, newLine)
	}
	return []byte(strings.Join(lines, "\n") + "\n"), found, nil
}

// quoteEnvLineValue returns the text written after "KEY=" so godotenv v1.5.1
// (the loader's parser) reads value back unchanged.
//
// Inputs: the key (for the error text) and the value. Outputs: the encoded
// value, or an error that never holds the value. Candidates in order, the
// first that round-trips through godotenv.UnmarshalBytes wins:
//  1. raw, for a non-empty value with no space, tab, #, quote, backslash, $,
//     backtick, newline or CR;
//  2. double-quoted with \ -> \\, " -> \", $ -> \$, newline -> \n, CR -> \r
//     (godotenv expands \n, \\X and $VAR inside double quotes, so a literal
//     backslash or $ must be escaped, or `a\nb` loads as a newline and `$HOME`
//     as the variable);
//  3. raw unquoted, for a value that holds only backslashes as special
//     characters: godotenv ends a quoted value at the first quote not preceded
//     by a backslash, so a value ending in a backslash cannot be terminated
//     inside quotes, and an unquoted value gets no escape processing;
//  4. single-quoted (no processing), for a value with no ' or line break that
//     (2) cannot hold, such as one ending in a double quote (godotenv trims
//     every trailing quote character off the quoted text).
//
// No candidate round-tripping is an error (a value such as `a b\`).
func quoteEnvLineValue(key, v string) (string, error) {
	var cands []string
	switch {
	case v == "":
		cands = append(cands, `""`)
	case !strings.ContainsAny(v, " \t#\"'\\$`\n\r"):
		cands = append(cands, v)
	}
	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "\n", `\n`, "\r", `\r`)
	cands = append(cands, `"`+esc.Replace(v)+`"`)
	if v != "" && !strings.ContainsAny(v, " \t#\"'$`\n\r") {
		cands = append(cands, v)
	}
	if !strings.ContainsAny(v, "'\n\r") {
		cands = append(cands, "'"+v+"'")
	}
	for _, c := range cands {
		if m, err := godotenv.UnmarshalBytes([]byte(key + "=" + c)); err == nil && m[key] == v {
			return c, nil
		}
	}
	if strings.HasSuffix(v, `\`) {
		return "", fmt.Errorf("value for %s ends in a backslash and holds characters that need quoting; a dotenv file cannot store it", key)
	}
	return "", fmt.Errorf("value for %s cannot be stored in a dotenv file", key)
}

// rollbackOnFailure wraps opt.BeforeWrite to note that it ran and returns the
// deferred step that calls opt.Rollback when the apply then fails (*err set).
func rollbackOnFailure(opt *ApplyOptions, err *error) func() {
	if opt.Rollback == nil {
		return func() {}
	}
	wrote := false
	before := opt.BeforeWrite
	opt.BeforeWrite = func() error {
		wrote = true
		if before != nil {
			return before()
		}
		return nil
	}
	return func() {
		// A panic after the write rolls back too, then keeps panicking so the
		// caller's recovery still reports it. recover works here: this closure
		// is the deferred function itself.
		if r := recover(); r != nil {
			if wrote {
				_ = opt.Rollback()
			}
			panic(r)
		}
		if *err != nil && wrote {
			if rerr := opt.Rollback(); rerr != nil {
				*err = errors.Join(*err, rerr)
			}
		}
	}
}
