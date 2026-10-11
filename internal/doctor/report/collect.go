package report

// collect.go: gathers the support-bundle files from a project and scrubs them.
//
// Purpose: `nself doctor report` (EPIC D21): versions, env key NAMES, generated
// compose, generated nginx confs, the doctor run, per-service log tails, and a
// manifest, every byte scrubbed before it is added.
// Inputs: Options (project dir, log lines) and Sources (injectable seams for
//         docker, the doctor child run, versions, clock).
// Outputs: a Bundle (ordered files + redaction counts + notes).
// Constraints: env VALUES are used only as scrub inputs; .env* files, secret
//         stores, ssl/ key material and ~/.nself licence data are never read
//         into the bundle. Nothing is uploaded.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Sources are the seams tests replace.
type Sources struct {
	Versions func(ctx context.Context) map[string]string
	Doctor   func(ctx context.Context) (string, error)
	// Logs returns the log tail of each service, keyed by service name.
	Logs func(ctx context.Context, tail int) (map[string]string, error)
	// LogNotes returns the notes the Logs call collected (for example a
	// container whose log read timed out); optional, called right after Logs.
	LogNotes func() []string
	Now      func() time.Time
}

// DefaultMaxLogBytes caps one service log (the tail is kept).
const DefaultMaxLogBytes = 4 << 20

// Options configure one collection.
type Options struct {
	ProjectDir string
	// NginxDir is the served nginx directory (internal/nginxtopo); empty skips nginx.
	NginxDir string
	Notes    []string // caller notes carried into manifest.json
	LogLines int
	// ComposeFragments are the compose files of the stack (.nself/compose-files.txt
	// resolved): read as scrub inputs only, never bundled.
	ComposeFragments []string
	// Environ is the process environment seam; nil means os.Environ.
	Environ func() []string
	// MaxLogBytes caps each service log; 0 means DefaultMaxLogBytes.
	MaxLogBytes int
}

// File is one entry of the bundle. Dir entries have no data.
type File struct {
	Name string
	Data []byte
	Dir  bool
}

// Bundle is the scrubbed result.
type Bundle struct {
	Files      []File
	Redactions map[string]int
	Total      int
	Notes      []string
}

// Collect gathers, scrubs and orders the bundle files; manifest.json is last.
func Collect(ctx context.Context, opt Options, src Sources) (*Bundle, error) {
	if src.Now == nil {
		src.Now = time.Now
	}
	environ := os.Environ
	if opt.Environ != nil {
		environ = opt.Environ
	}
	in := newInputSet(opt.ProjectDir)
	loadEnvInputs(opt.ProjectDir, in)
	loadProcessEnv(environ(), in)
	loadLicenseKeys(in)
	loadCustomEnvFiles(in)
	compose, cnote := readCompose(opt.ProjectDir, in)
	loadFragments(opt.ComposeFragments, in)
	notes := append(in.notes, opt.Notes...)
	if cnote != "" {
		notes = append(notes, cnote)
	}
	if sn := in.shortNote(); sn != "" {
		notes = append(notes, sn)
	}
	scr := NewScrubber(in.values)
	b := &Bundle{}
	addRaw := func(name, text string) { // text is already scrubbed
		b.Files = append(b.Files, File{Name: name, Data: []byte(text)})
	}
	add := func(name, text string) { addRaw(name, scr.Scrub(text)) }

	vers := map[string]string{}
	if src.Versions != nil {
		vers = src.Versions(ctx)
	}
	vb, err := json.MarshalIndent(vers, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding versions: %w", err)
	}
	add("versions.json", string(vb)+"\n")
	add("config-keys.txt", strings.Join(in.sortedKeys(), "\n")+"\n")
	if compose != "" {
		add("docker-compose.yml", compose)
	}
	var nginxFiles []File
	if opt.NginxDir != "" {
		var nnotes []string
		nginxFiles, nnotes = readNginx(opt.NginxDir)
		notes = append(notes, nnotes...)
	}
	usedNginx := map[string]bool{}
	for _, nf := range nginxFiles {
		rel := strings.TrimPrefix(nf.Name, "nginx/")
		name := uniqueName(usedNginx, func(suffix string) string { return scr.entryPath("nginx/", rel+suffix) })
		add(name, string(nf.Data))
	}
	if src.Doctor != nil {
		notes = append(notes, collectDoctor(ctx, src, add)...)
	}
	b.Files = append(b.Files, File{Name: "logs/", Dir: true})
	notes = append(notes, collectLogs(ctx, opt, src, addRaw, scr)...)
	return finish(b, scr, notes, src.Now())
}

// collectDoctor adds doctor.txt; a timeout keeps the partial output and says so.
func collectDoctor(ctx context.Context, src Sources, add func(string, string)) []string {
	out, derr := src.Doctor(ctx)
	var notes []string
	var te *TimeoutError
	switch {
	case errors.As(derr, &te):
		out += fmt.Sprintf("\n[doctor timed out after %s: the output above is partial]\n", te.After)
		notes = append(notes, fmt.Sprintf("doctor timed out after %s: doctor.txt holds the partial output", te.After))
	case derr != nil && out == "":
		out = "doctor did not produce output: " + derr.Error() + "\n"
	case derr != nil:
		notes = append(notes, "doctor ended with an error: "+derr.Error())
	}
	add("doctor.txt", out)
	return notes
}

// finish writes manifest.json (counts are final: scrubbing it adds none).
func finish(b *Bundle, scr *Scrubber, notes []string, now time.Time) (*Bundle, error) {
	for i := range notes {
		notes[i] = scr.Scrub(notes[i])
	}
	b.Notes = notes
	b.Redactions, b.Total = scr.Counts(), scr.Total()
	type entry struct {
		Name  string `json:"name"`
		Bytes int    `json:"bytes"`
	}
	list := make([]entry, 0, len(b.Files)+1)
	for _, f := range b.Files {
		list = append(list, entry{f.Name, len(f.Data)})
	}
	list = append(list, entry{Name: "manifest.json"}) // own size set below (N3)
	man := struct {
		Generated  string         `json:"generated"`
		Files      []entry        `json:"files"`
		Redactions map[string]int `json:"redactions"`
		Total      int            `json:"redactions_total"`
		Notes      []string       `json:"notes"`
	}{now.UTC().Format(time.RFC3339), list, b.Redactions, b.Total, notes}
	if man.Notes == nil {
		man.Notes = []string{}
	}
	// The manifest lists its own size: re-encode until the number is stable
	// (it converges in two or three rounds; the digit count only grows).
	var data []byte
	for i := 0; i < 5; i++ {
		var err error
		if data, err = json.MarshalIndent(man, "", "  "); err != nil {
			return nil, fmt.Errorf("encoding manifest: %w", err)
		}
		data = append(data, '\n')
		if list[len(list)-1].Bytes == len(data) {
			break
		}
		list[len(list)-1].Bytes = len(data)
	}
	b.Files = append(b.Files, File{Name: "manifest.json", Data: data})
	return b, nil
}
