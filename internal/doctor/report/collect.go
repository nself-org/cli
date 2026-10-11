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
	"fmt"
	"sort"
	"strings"
	"time"
)

// Sources are the seams tests replace.
type Sources struct {
	Versions func(ctx context.Context) map[string]string
	Doctor   func(ctx context.Context) (string, error)
	// Logs returns the log tail of each service, keyed by service name.
	Logs func(ctx context.Context, tail int) (map[string]string, error)
	Now  func() time.Time
}

// Options configure one collection.
type Options struct {
	ProjectDir string
	// NginxDir is the served nginx directory (internal/nginxtopo); empty skips nginx.
	NginxDir string
	Notes    []string // caller notes carried into manifest.json
	LogLines int
}

// File is one entry of the bundle.
type File struct {
	Name string
	Data []byte
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
	values, keys, notes := loadEnvInputs(opt.ProjectDir)
	notes = append(notes, opt.Notes...)
	compose, composeVals, cnote := readCompose(opt.ProjectDir)
	values = append(values, composeVals...)
	if cnote != "" {
		notes = append(notes, cnote)
	}
	scr := NewScrubber(values)
	b := &Bundle{}
	add := func(name, text string) {
		b.Files = append(b.Files, File{Name: name, Data: []byte(scr.Scrub(text))})
	}

	vers := map[string]string{}
	if src.Versions != nil {
		vers = src.Versions(ctx)
	}
	vb, err := json.MarshalIndent(vers, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding versions: %w", err)
	}
	add("versions.json", string(vb)+"\n")
	add("config-keys.txt", strings.Join(keys, "\n")+"\n")
	if compose != "" {
		add("docker-compose.yml", compose)
	}
	var nginxFiles []File
	if opt.NginxDir != "" {
		var nnotes []string
		nginxFiles, nnotes = readNginx(opt.NginxDir)
		notes = append(notes, nnotes...)
	}
	for _, nf := range nginxFiles {
		add(nf.Name, string(nf.Data))
	}
	if src.Doctor != nil {
		out, derr := src.Doctor(ctx)
		if derr != nil && out == "" {
			out = "doctor did not produce output: " + derr.Error() + "\n"
		}
		add("doctor.txt", out)
	}
	notes = append(notes, collectLogs(ctx, opt.LogLines, src, add, scr)...)
	return finish(b, scr, notes, src.Now()), nil
}

// collectLogs adds logs/<service>.log files and returns notes.
func collectLogs(ctx context.Context, tail int, src Sources, add func(string, string), scr *Scrubber) []string {
	if src.Logs == nil {
		return []string{"no running stack: log collection is not available"}
	}
	logs, err := src.Logs(ctx, tail)
	if err != nil {
		return []string{"no running stack: " + scr.Scrub(err.Error())}
	}
	if len(logs) == 0 {
		return []string{"no running stack: no containers found, logs/ is empty"}
	}
	names := make([]string, 0, len(logs))
	for n := range logs {
		names = append(names, n)
	}
	sort.Strings(names)
	used := map[string]bool{}
	for _, n := range names {
		f := LogName(n)
		for i := 2; used[f]; i++ {
			f = LogName(fmt.Sprintf("%s-%d", n, i))
		}
		used[f] = true
		add(f, logs[n])
	}
	return nil
}

// finish writes manifest.json (counts are final: scrubbing it adds none).
func finish(b *Bundle, scr *Scrubber, notes []string, now time.Time) *Bundle {
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
	list = append(list, entry{Name: "manifest.json"})
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
	data, _ := json.MarshalIndent(man, "", "  ")
	b.Files = append(b.Files, File{Name: "manifest.json", Data: append(data, '\n')})
	return b
}

// LogName maps a service name to logs/<name>.log with no path separators or
// dot-dot components.
func LogName(service string) string {
	var sb strings.Builder
	for _, r := range service {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			sb.WriteRune(r)
		default:
			sb.WriteByte('_')
		}
	}
	n := strings.ReplaceAll(sb.String(), "..", "_")
	n = strings.Trim(n, ".")
	if n == "" {
		n = "unnamed"
	}
	return "logs/" + n + ".log"
}
