package report

// collect_logs.go: the logs/ entries of the bundle and the entry names.
//
// Purpose: add logs/<service>.log, scrubbed BEFORE the byte cap so a cut can
// never leave the tail of a secret without its head (S1), and make every
// entry name (logs, nginx) pass the scrubber and then a path-safe filter
// (lead ruling R6 f).
// Inputs: the log texts of the Sources seam, the Scrubber.
// Outputs: bundle entries (already scrubbed), manifest notes.
// Constraints: names have no separators other than the fixed "logs/" or
//         "nginx/" prefix structure, no dot-dot parts, and are unique.

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// collectLogs adds logs/<service>.log files and returns notes. addRaw stores
// text that is already scrubbed.
func collectLogs(ctx context.Context, opt Options, src Sources, addRaw func(string, string), scr *Scrubber) []string {
	if src.Logs == nil {
		return []string{"no running stack: log collection is not available"}
	}
	logs, err := src.Logs(ctx, opt.LogLines)
	var notes []string
	if src.LogNotes != nil {
		notes = append(notes, src.LogNotes()...)
	}
	if err != nil {
		return append(notes, "no running stack: "+scr.Scrub(err.Error()))
	}
	if len(logs) == 0 {
		return append(notes, "no running stack: no containers found, logs/ is empty")
	}
	limit := opt.MaxLogBytes
	if limit <= 0 {
		limit = DefaultMaxLogBytes
	}
	names := make([]string, 0, len(logs))
	for n := range logs {
		names = append(names, n)
	}
	sort.Strings(names)
	used := map[string]bool{}
	for _, n := range names {
		f := uniqueName(used, func(suffix string) string { return scr.entryName("logs/", n+suffix, ".log") })
		clean := scr.Scrub(logs[n]) // scrub first, cap second (S1)
		text, cut := capTail(clean, limit)
		if cut {
			notes = append(notes, fmt.Sprintf("%s truncated to its last %d bytes (was %d bytes)", f, limit, len(clean)))
		}
		addRaw(f, text)
	}
	return notes
}

// uniqueName returns the first unused name of build("") then build("-2"), ...
func uniqueName(used map[string]bool, build func(suffix string) string) string {
	f := build("")
	for i := 2; used[f]; i++ {
		f = build(fmt.Sprintf("-%d", i))
	}
	used[f] = true
	return f
}

// capTail keeps the last limit bytes of text, starting at a line boundary when
// one is near, and reports whether it cut anything. text is already scrubbed.
func capTail(text string, limit int) (string, bool) {
	if len(text) <= limit {
		return text, false
	}
	tail := text[len(text)-limit:]
	if i := strings.IndexByte(tail, '\n'); i >= 0 && i < len(tail)-1 {
		tail = tail[i+1:]
	}
	return "[truncated: only the end of this log is kept]\n" + tail, true
}

// entryName is prefix + the scrubbed, path-safe form of name + suffix; name is
// one path segment (a service name), so a "/" in it is replaced.
func (s *Scrubber) entryName(prefix, name, suffix string) string {
	return prefix + safeSegment(s.Scrub(name)) + suffix
}

// entryPath is like entryName for a relative path (nginx/<rel>): each "/"-part
// is scrubbed and sanitized on its own and the separators stay.
func (s *Scrubber) entryPath(prefix, rel string) string {
	parts := strings.Split(rel, "/")
	for i, p := range parts {
		parts[i] = safeSegment(s.Scrub(p))
	}
	return prefix + strings.Join(parts, "/")
}

// LogName maps a service name to logs/<name>.log with no path separators or
// dot-dot components.
func LogName(service string) string {
	return "logs/" + safeSegment(service) + ".log"
}

// safeSegment keeps letters, digits, "-", "_" and "."; everything else becomes
// "_". The result has no "..", no leading or trailing ".", and is never empty.
func safeSegment(s string) string {
	var sb strings.Builder
	for _, r := range s {
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
	return n
}
