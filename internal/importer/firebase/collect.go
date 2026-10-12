package firebase

// Purpose: walk every Firestore collection and sub-collection once, streaming each document to a
// per-table JSON-lines spool while tracking the types seen per top-level field.
// Inputs: a Firestore reader and a spool directory. Outputs: tables with the document count from
// the listing and the independent count from the COUNT aggregation.
// Constraints: memory is O(one document); the spool lives in the private extract directory.
import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	t := &tableAcc{pattern: pattern, parent: parent, kinds: map[string]map[kind]bool{}, spool: spool, file: f, w: w, enc: json.NewEncoder(w)}
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

// walk reads every collection under parent ("" = root); parentTable is nil at the root.
func (c *collector) walk(ctx context.Context, parent string, parentTable *tableAcc) error {
	ids, err := c.fs.collectionIDs(ctx, parent)
	if err != nil {
		return err
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err = checkText(id); err != nil {
			return err
		}
		pattern := id
		path := id
		if parent != "" {
			pattern, path = parentTable.pattern+"/*/"+id, parent+"/"+id
		}
		t, err := c.table(pattern, parentTable)
		if err != nil {
			return err
		}
		n, err := c.fs.count(ctx, parent, id)
		if err != nil {
			return err
		}
		t.counted += n
		_, err = c.fs.listDocs(ctx, path, func(d document) error {
			if !d.Missing {
				if err := c.add(t, parent, d); err != nil {
					return err
				}
			}
			return c.walk(ctx, path+"/"+d.ID, t)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// add decodes one document and appends it to the table spool.
func (c *collector) add(t *tableAcc, parentPath string, d document) error {
	data, kinds, err := decodeFields(d.Fields, 0)
	if err != nil {
		return err
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
		if promotable(k) {
			r.Scalars[name] = scalarText(data[name])
		}
	}
	if err = t.enc.Encode(r); err != nil {
		return errors.New("firestore spool write failed")
	}
	t.listed++
	return nil
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
