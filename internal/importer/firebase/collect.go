package firebase

// Purpose: walk every Firestore collection and sub-collection once, streaming each document to a
// per-table JSON-lines spool while tracking the types seen per top-level field.
// Inputs: a Firestore reader and a spool directory. Outputs: tables with the document count from
// the listing and the independent count from the COUNT aggregation.
// Constraints: document memory is bounded to one query page; table and collection metadata is
// retained for independent counts. The spool lives in the private extract directory.
import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// record is one spooled document. Scalars holds the text of each promotable top-level value.
type record struct {
	ID         string            `json:"id"`
	ParentPath string            `json:"pp,omitempty"`
	ParentID   string            `json:"pi,omitempty"`
	Data       map[string]any    `json:"data"`
	Scalars    map[string]string `json:"sc,omitempty"`
}

type tableAcc struct {
	pattern string // "users" or "users/*/orders"
	parent  *tableAcc
	name    string // set by assignNames
	listed  int64  // documents the listing returned
	counted int64  // sum of COUNT aggregations over every parent
	kinds   map[string]map[kind]bool
	present map[string]int  // documents that hold each top-level field
	unsafe  map[string]bool // fields with a text value psql \copy would cut short
	spool   string
	file    *os.File
	w       *bufio.Writer
	enc     *json.Encoder
}

type collector struct {
	fs     *firestore
	dir    string
	tables map[string]*tableAcc
}

func newCollector(fs *firestore, dir string) *collector {
	return &collector{fs: fs, dir: dir, tables: map[string]*tableAcc{}}
}

func (c *collector) table(pattern string, parent *tableAcc) (*tableAcc, error) {
	if t, ok := c.tables[pattern]; ok {
		return t, nil
	}
	spool := filepath.Join(c.dir, "spool-"+strconv.Itoa(len(c.tables))+".jsonl")
	f, err := os.OpenFile(spool, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, errors.New("firestore spool unavailable")
	}
	w := bufio.NewWriterSize(f, 1<<20)
	t := &tableAcc{pattern: pattern, parent: parent, kinds: map[string]map[kind]bool{}, present: map[string]int{}, unsafe: map[string]bool{}, spool: spool, file: f, w: w, enc: json.NewEncoder(w)}
	c.tables[pattern] = t
	return t, nil
}

// close flushes every spool; it must run before the spools are read.
func (c *collector) close() error {
	var first error
	for _, t := range c.tables {
		if err := t.w.Flush(); err != nil && first == nil {
			first = errors.New("firestore spool write failed")
		}
		if err := t.file.Close(); err != nil && first == nil {
			first = errors.New("firestore spool write failed")
		}
	}
	return first
}

// walk reads all descendant documents at one snapshot and checks database completeness.
func (c *collector) walk(ctx context.Context, _ string, _ *tableAcc) error {
	total, err := c.fs.count(ctx, "", "")
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	var listed int64
	err = c.fs.queryAll(ctx, func(path string, d document) error {
		t, parentPath, err := c.documentTable(ctx, path, seen)
		if err != nil {
			return err
		}
		listed++
		return c.add(t, parentPath, d)
	})
	if err == nil && listed != total {
		return errors.New("firestore database count differs from query; discovery incomplete")
	}
	return err
}

// documentTable reconstructs ancestor tables even when their parent documents were deleted.
func (c *collector) documentTable(ctx context.Context, path string, seen map[string]bool) (*tableAcc, string, error) {
	parts := strings.Split(path, "/")
	var parent *tableAcc
	pattern := ""
	for i := 0; i < len(parts); i += 2 {
		if pattern != "" {
			pattern += "/*/"
		}
		pattern += parts[i]
		t, err := c.table(pattern, parent)
		if err != nil {
			return nil, "", err
		}
		collection := strings.Join(parts[:i+1], "/")
		if !seen[collection] {
			n, err := c.fs.count(ctx, strings.Join(parts[:i], "/"), parts[i])
			if err != nil {
				return nil, "", err
			}
			t.counted += n
			seen[collection] = true
		}
		parent = t
	}
	return parent, strings.Join(parts[:len(parts)-2], "/"), nil
}

// add decodes one document and appends it to the table spool.
func (c *collector) add(t *tableAcc, parentPath string, d document) error {
	data, kinds, err := decodeFields(d.Fields, 0)
	if err != nil {
		return err
	}
	if hasDotLine(d.ID) || hasDotLine(parentPath) {
		return errors.New("document id holds a line that psql would read as end of data")
	}
	r := record{ID: d.ID, ParentPath: parentPath, Data: data, Scalars: map[string]string{}}
	if parentPath != "" {
		r.ParentID = parentPath[lastSlash(parentPath)+1:]
	}
	for name, k := range kinds {
		if t.kinds[name] == nil {
			t.kinds[name] = map[kind]bool{}
		}
		t.kinds[name][k] = true
		t.present[name]++
		if promotable(k) {
			text := scalarText(data[name])
			if hasDotLine(text) {
				t.unsafe[name] = true
				continue
			}
			r.Scalars[name] = text
		}
	}
	if err = t.enc.Encode(r); err != nil {
		return errors.New("firestore spool write failed")
	}
	t.listed++
	return nil
}

// hasDotLine reports whether s has a line that is exactly `\.`; psql's client-side \copy reads
// that as the end of the data even inside a quoted CSV field, so such a value is never promoted.
func hasDotLine(s string) bool {
	if !strings.Contains(s, "\\.") {
		return false
	}
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSuffix(line, "\r") == "\\." {
			return true
		}
	}
	return false
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

// promotable kinds can become a typed column.
func promotable(k kind) bool {
	return k == kString || k == kInteger || k == kDouble || k == kBoolean || k == kTimestamp
}

// sorted returns the tables in pattern order, which puts every parent before its children.
func (c *collector) sorted() []*tableAcc {
	out := make([]*tableAcc, 0, len(c.tables))
	for _, t := range c.tables {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].pattern < out[j].pattern })
	return out
}
