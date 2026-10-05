package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
)

// Cover is the statement tally of one package.
type Cover struct {
	Covered int // statements in blocks executed at least once
	Total   int // all statements
}

// Percent is covered/total as a percentage; 0 when there are no statements.
func (c Cover) Percent() float64 {
	if c.Total == 0 {
		return 0
	}
	return 100 * float64(c.Covered) / float64(c.Total)
}

// block is one profile line: a source range with a statement count and a hit count.
type block struct {
	stmts int
	count int64
}

// ReadProfile parses the Go coverage profile at file and returns the tally per
// package, keyed by module-relative directory ("." for the module root). It
// fails closed: an unreadable file, a missing or unknown `mode:` header, any
// malformed line, a file outside module, a block repeated with a different
// statement count, or a profile with no statements at all is an error.
func ReadProfile(file, module string) (map[string]*Cover, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("profile: %w", err)
	}
	defer func() { _ = f.Close() }()
	return parseProfile(f, module)
}

// parseProfile is ReadProfile over a reader.
func parseProfile(r io.Reader, module string) (map[string]*Cover, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	if !sc.Scan() {
		return nil, fmt.Errorf("profile: empty (no mode header)")
	}
	switch strings.TrimSpace(sc.Text()) {
	case "mode: set", "mode: count", "mode: atomic":
	default:
		return nil, fmt.Errorf("profile: line 1: want a `mode: set|count|atomic` header, got %q", sc.Text())
	}

	// A block repeated across lines (merged test binaries) counts once; the
	// highest hit count wins, so a block is covered if any run covered it.
	blocks := map[string]block{}
	var order []string
	for n := 2; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		key, b, err := parseBlock(line)
		if err != nil {
			return nil, fmt.Errorf("profile: line %d: %w", n, err)
		}
		if prev, seen := blocks[key]; seen {
			if prev.stmts != b.stmts {
				return nil, fmt.Errorf("profile: line %d: block %s repeated with %d statements, was %d", n, key, b.stmts, prev.stmts)
			}
			if prev.count > b.count {
				b.count = prev.count
			}
		} else {
			order = append(order, key)
		}
		blocks[key] = b
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("profile: %w", err)
	}

	out := map[string]*Cover{}
	for _, key := range order {
		b := blocks[key]
		if b.stmts == 0 {
			continue
		}
		pkg, err := packageOf(key[:strings.LastIndex(key, ":")], module)
		if err != nil {
			return nil, err
		}
		c := out[pkg]
		if c == nil {
			c = &Cover{}
			out[pkg] = c
		}
		c.Total += b.stmts
		if b.count > 0 {
			c.Covered += b.stmts
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("profile: no statements in any block")
	}
	return out, nil
}

// parseBlock parses `<file>:<a>.<b>,<c>.<d> <numstmt> <count>`.
func parseBlock(line string) (string, block, error) {
	fields := strings.Fields(line)
	if len(fields) != 3 {
		return "", block{}, fmt.Errorf("want `<file>:<range> <numstmt> <count>`, got %q", line)
	}
	pos := fields[0]
	i := strings.LastIndex(pos, ":")
	if i <= 0 || !strings.Contains(pos[i:], ",") {
		return "", block{}, fmt.Errorf("bad position %q", pos)
	}
	stmts, err := strconv.Atoi(fields[1])
	if err != nil || stmts < 0 {
		return "", block{}, fmt.Errorf("bad statement count %q", fields[1])
	}
	count, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil || count < 0 {
		return "", block{}, fmt.Errorf("bad hit count %q", fields[2])
	}
	return pos, block{stmts: stmts, count: count}, nil
}

// packageOf maps a profile file path (an import path plus file name) to its
// module-relative package directory.
func packageOf(file, module string) (string, error) {
	dir := path.Dir(file)
	switch {
	case dir == module:
		return ".", nil
	case strings.HasPrefix(dir, module+"/"):
		return strings.TrimPrefix(dir, module+"/"), nil
	}
	return "", fmt.Errorf("profile: file %s is outside module %s", file, module)
}
