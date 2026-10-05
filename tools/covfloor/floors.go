package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Entry is one floors-file line: `<package> <integer floor>  # comment`.
type Entry struct {
	Package string
	Floor   int
	Raw     string   // the line exactly as read or generated
	Lead    []string // comment lines directly above it, kept with it when sorting
}

// Floors is a parsed floors file.
type Floors struct {
	Header  []string // comment and blank lines before the first entry
	Entries []Entry
	Trailer []string // comment lines after the last entry
}

// ReadFloors parses the floors file. A missing file, a line that is not
// `<package> <0..100>` optionally followed by `# comment`, or a duplicate
// package is an error: a gate with unreadable data must not pass.
func ReadFloors(file string) (*Floors, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("floors: %w", err)
	}
	defer func() { _ = f.Close() }()

	fl := &Floors{}
	seen := map[string]int{}
	var pending []string
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		raw := sc.Text()
		text := strings.TrimSpace(raw)
		if text == "" || strings.HasPrefix(text, "#") {
			pending = append(pending, raw)
			continue
		}
		e, err := parseEntry(raw)
		if err != nil {
			return nil, fmt.Errorf("floors: line %d: %w", n, err)
		}
		if first, dup := seen[e.Package]; dup {
			return nil, fmt.Errorf("floors: line %d: package %s already has a floor on line %d", n, e.Package, first)
		}
		seen[e.Package] = n
		if len(fl.Entries) == 0 {
			fl.Header, pending = pending, nil
		} else {
			e.Lead, pending = pending, nil
		}
		fl.Entries = append(fl.Entries, e)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("floors: %w", err)
	}
	if len(fl.Entries) == 0 {
		return nil, fmt.Errorf("floors: %s has no package lines", file)
	}
	fl.Trailer = pending
	return fl, nil
}

func parseEntry(raw string) (Entry, error) {
	body := raw
	if i := strings.Index(body, "#"); i >= 0 {
		body = body[:i]
	}
	fields := strings.Fields(body)
	if len(fields) != 2 {
		return Entry{}, fmt.Errorf("want `<package> <floor>`, got %q", raw)
	}
	floor, err := strconv.Atoi(fields[1])
	if err != nil || floor < 0 || floor > 100 {
		return Entry{}, fmt.Errorf("floor %q is not an integer from 0 to 100", fields[1])
	}
	return Entry{Package: fields[0], Floor: floor, Raw: raw}, nil
}

// Map returns the floor of every package.
func (f *Floors) Map() map[string]int {
	m := make(map[string]int, len(f.Entries))
	for _, e := range f.Entries {
		m[e.Package] = e.Floor
	}
	return m
}

// newFloor is the floor `-write` gives a package it has not seen:
// max(0, floor(measured) - 2).
func newFloor(pct float64) int {
	n := int(pct) - 2
	if n < 0 {
		return 0
	}
	return n
}

// Append adds one line per package in missing (which maps package to measured
// percent), sorted bytewise with the existing lines. Existing lines are kept
// byte for byte; nothing is ever lowered, raised or removed.
func (f *Floors) Append(missing map[string]float64, date string) {
	for pkg, pct := range missing {
		fl := newFloor(pct)
		f.Entries = append(f.Entries, Entry{
			Package: pkg,
			Floor:   fl,
			Raw:     fmt.Sprintf("%s %d  # measured %.1f %s", pkg, fl, pct, date),
		})
	}
	sort.SliceStable(f.Entries, func(i, j int) bool { return f.Entries[i].Package < f.Entries[j].Package })
}

// Write stores the file, replacing it atomically through a temp file.
func (f *Floors) Write(file string) error {
	var b strings.Builder
	for _, l := range f.Header {
		b.WriteString(l + "\n")
	}
	for _, e := range f.Entries {
		for _, l := range e.Lead {
			b.WriteString(l + "\n")
		}
		b.WriteString(e.Raw + "\n")
	}
	for _, l := range f.Trailer {
		b.WriteString(l + "\n")
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("floors: %w", err)
	}
	if err := os.Rename(tmp, file); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("floors: %w", err)
	}
	return nil
}
