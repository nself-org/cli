package reconcile

// handedit.go — did a human change a file nself generated? (EPIC P7-LIVE D5,
// Ticket P7-LIVE-04).
//
// Purpose: answer the one question the hand-edit policy needs: is this path
// recorded in the generated-state record with a hash that no longer matches
// the file's bytes? Plan marks such artifacts hand_edited (via
// Request.HandEdited, which Apply supplies from the record); Apply prints
// their unified diff and leaves the refusal, the --force override and the
// prompt to reconcile.Confirm.
// Inputs: the record (LoadGeneratedState) and a plan display path.
// Outputs: GeneratedState.HandEdited, the pure check over given bytes, and
// HandEditedFn, the Request callback that resolves a display path to the disk
// and reads the file.
// Constraints: a path with no record is never hand-edited (an unmarked,
// unrecorded file is never a write target either — the plan only lists what
// the pipeline renders); the file's exact bytes are hashed, so any change a
// formatter or editor makes counts.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginxtopo"
)

// HandEdited reports whether the file at path (a plan display path) was
// changed by a human since nself last wrote it: the record holds the sha256 of
// the bytes it wrote and current hashes differently. A nil record, a missing
// entry or a file that cannot be read is never hand-edited — detection fails
// open here so an unreadable file cannot block a build; the overwrite itself
// is what Confirm guards.
func (s *GeneratedState) HandEdited(path string, current []byte) bool {
	if s == nil || s.Files == nil {
		return false
	}
	want, ok := s.Files[path]
	if !ok {
		return false
	}
	sum := sha256.Sum256(current)
	return hex.EncodeToString(sum[:]) != want
}

// HandEditedFn returns the Request.HandEdited callback over this record: it
// resolves each display path to its location on disk and reads the file's
// current bytes. A nil record (no state yet) answers false for everything:
// the first run on an existing project records and never blocks.
func (s *GeneratedState) HandEditedFn(projectDir string) func(path string) bool {
	return s.handEditedFnWith(projectDir, nil, "")
}

// handEditedFnWith is HandEditedFn with the env files a pending change will
// leave (P7-LIVE-05): the fronting stack is resolved from them, so a change of
// NGINX_FRONTED_BY is judged against the stack the build will write to.
// pluginDir is the plugin directory "@plugins/" paths resolve under ("" means
// DefaultPluginDir).
func (s *GeneratedState) handEditedFnWith(projectDir string, env map[string][]byte, pluginDir string) func(path string) bool {
	var fronting string
	var frontingOK, frontingDone bool
	return func(path string) bool {
		disk, ok := displayDiskPath(projectDir, env, pluginDir, path, &fronting, &frontingOK, &frontingDone)
		if !ok {
			return false
		}
		current, err := os.ReadFile(disk)
		if err != nil {
			// A deleted generated file holds no bytes to lose: the build
			// recreates it (D5: hand-edited means current sha != recorded).
			// Any other read failure (permission, a directory, a dangling
			// symlink in the file's place) cannot prove the file is untouched,
			// so a recorded path counts as hand-edited: fail closed.
			// A dangling symlink also reads as ErrNotExist, but the link
			// itself is the user's: Lstat tells the two apart.
			if s == nil {
				return false
			}
			_, recorded := s.Files[path]
			if errors.Is(err, fs.ErrNotExist) {
				if _, lerr := os.Lstat(disk); lerr != nil {
					return false
				}
			}
			return recorded
		}
		return s.HandEdited(path, current)
	}
}

// displayPath is the plan display path of a canonical build key: the key
// itself for project-relative and "@fronting/" keys, "@plugins/<rel>" or
// "@abs/<path>" for absolute ones (overlay.display; a bare overlay maps
// paths, it needs no render). pluginDir is the build's plugin directory.
func displayPath(pluginDir, key string) string {
	return overlay{plugins: pluginDir}.display(key)
}

// displayDiskPath resolves a plan display path to its location on disk:
// project files under the project, "@fronting/<rel>" under the fronting
// stack's root, "@plugins/<rel>" under pluginDir (DefaultPluginDir when "")
// and "@abs/<path>" as itself. A fronting path resolves only while the
// fronting stack is laid out the way the build resolves it (NGINX_FRONTED_BY
// naming the parent directory); its resolution is computed once and cached in
// the three closure variables HandEditedFn owns.
func displayDiskPath(projectDir string, env map[string][]byte, pluginDir, display string, fronting *string, frontingOK, frontingDone *bool) (string, bool) {
	rel := filepath.FromSlash(strings.TrimPrefix(display, FrontingPrefix))
	switch {
	case strings.HasPrefix(display, FrontingPrefix):
		if !*frontingDone {
			*fronting, *frontingOK = frontingRoot(projectDir, env)
			*frontingDone = true
		}
		if !*frontingOK {
			return "", false
		}
		return filepath.Join(*fronting, rel), true
	case strings.HasPrefix(display, PluginsPrefix):
		return filepath.Join(pluginDirOr(pluginDir),
			filepath.FromSlash(strings.TrimPrefix(display, PluginsPrefix))), true
	case strings.HasPrefix(display, AbsPrefix):
		return string(filepath.Separator) + filepath.FromSlash(strings.TrimPrefix(display, AbsPrefix)), true
	}
	return filepath.Join(projectDir, filepath.FromSlash(display)), true
}

// frontingRoot resolves the fronting stack's root the way the build does:
// NGINX_FRONTED_BY names the stack and its root is the parent directory when
// that directory carries the name (nginxtopo.ResolveFrontingDir). pending,
// when set, stands in for the named env files (P7-LIVE-05).
func frontingRoot(projectDir string, pending map[string][]byte) (string, bool) {
	return nginxtopo.ResolveFrontingDir(projectDir, strings.TrimSpace(cascadeValue(projectDir, pending, "NGINX_FRONTED_BY")))
}

// cascadeValue reads one key from the env cascade the way config.Load leaves
// it, without loading the whole config: the cascade layer is chosen by ENV as
// config.ResolveEnv chooses it, and a file value wins over the process
// environment (godotenv.Overload). pending, when set, stands in for the named
// env files. Empty when no source sets the key.
func cascadeValue(projectDir string, pending map[string][]byte, key string) string {
	value := os.Getenv(key)
	env, _ := config.ResolveEnv(projectDir)
	if data, ok := pending[".env"]; ok && os.Getenv("ENV") == "" {
		env = "dev"
		if m, err := godotenv.UnmarshalBytes(data); err == nil && m["ENV"] != "" {
			env = m["ENV"]
		}
	}
	for _, name := range config.EnvCascadeOrder(env, config.LegacyOrderActive()) {
		m, err := godotenv.Read(filepath.Join(projectDir, name))
		if data, ok := pending[name]; ok {
			m, err = godotenv.UnmarshalBytes(data)
		}
		if err != nil {
			continue
		}
		if v, ok := m[key]; ok {
			value = v
		}
	}
	return value
}

// diskEnv is the project's ENV as config.Load resolves it from the files on
// disk now ("dev" when nothing sets it): the env a pending change starts from.
func diskEnv(projectDir string) string {
	if v := cascadeValue(projectDir, nil, "ENV"); strings.TrimSpace(v) != "" {
		return v
	}
	return "dev"
}

// warnHandEdits prints the unified diff of every hand-edited artifact the plan
// would overwrite or remove — the operator sees exactly what would be lost
// before anything is refused or written — and, in v1.4 mode, the one-line
// warning that stands in for the v1.5 refusal: warn with the diff, then
// proceed as build always did.
func warnHandEdits(req Request, p *Plan, c *computed, v15 bool) error {
	hand := HandEditedPaths(*p)
	if len(hand) == 0 || req.Stderr == nil {
		return nil
	}
	listed := make(map[string]bool, len(hand))
	for _, h := range hand {
		listed[h] = true
	}
	arts := make([]Artifact, 0, len(hand))
	for _, a := range p.Artifacts {
		if listed[a.Path] {
			arts = append(arts, a)
		}
	}
	if err := writeDiffs(req.Stderr, arts, c.before, c.after); err != nil {
		return err
	}
	if !v15 {
		// v1.4 keeps today's proceed (the refusal is v1.5, ADR 0021, gated in
		// Confirm); it warns with the diff instead of refusing.
		_, _ = fmt.Fprintf(req.Stderr,
			"nself: hand-edited generated files would be overwritten: %s; v1.5 refuses without --force (NSELF_V15=1 shows it now)\n",
			strings.Join(hand, ", "))
	}
	return nil
}

// firstRunNotice is the one-time warning of a project's first recorded apply.
const firstRunNotice = "nself: no generated-state record was found; the files this build wrote are recorded in .nself/state/generated.json, and hand-edits are detected against them from now on\n"
