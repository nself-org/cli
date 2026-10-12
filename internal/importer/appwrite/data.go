package appwrite

// Purpose: stream a collection's documents into a COPY-ready CSV file.
// Inputs: the paginated documents list. Outputs: a CSV file, its row count and the server `total`.
// Constraints: strings are always quoted so an empty string stays distinct from NULL; relationship
// values reduce to document ids; document and collection permissions are counted, never mapped.
import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// copied is the result of writing one collection.
type copied struct {
	Path        string
	Rows, Total int64
	DocPerms    int64
}

func (c *client) copyDocuments(ctx context.Context, db string, col *collection, path string) (copied, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return copied{}, errs.New("E520", "snapshot staging unavailable")
	}
	w := bufio.NewWriter(f)
	out := copied{Path: path}
	var werr error
	total, err := c.each(ctx, fmt.Sprintf("/databases/%s/collections/%s/documents", url.PathEscape(db), url.PathEscape(col.ID)), "documents", func(raw json.RawMessage) error {
		doc := map[string]any{}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if dec.Decode(&doc) != nil {
			return errs.New("E520", "source tables unavailable")
		}
		if perms, ok := doc["$permissions"].([]any); ok && len(perms) > 0 {
			out.DocPerms++
		}
		cells := make([]string, len(col.Columns))
		for i, cl := range col.Columns {
			cells[i] = cell(cl, doc[cl.Key])
		}
		if _, werr = w.WriteString(strings.Join(cells, ",") + "\n"); werr != nil {
			return errs.New("E520", "snapshot staging unavailable")
		}
		out.Rows++
		return nil
	})
	if err == nil {
		err = werr
	}
	if e := w.Flush(); err == nil && e != nil {
		err = errs.New("E520", "snapshot staging unavailable")
	}
	if e := f.Close(); err == nil && e != nil {
		err = errs.New("E520", "snapshot staging unavailable")
	}
	out.Total = total
	return out, err
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// cell renders one CSV field; nil values become the unquoted empty field, which COPY reads as NULL.
func cell(c column, v any) string {
	if v == nil {
		return ""
	}
	switch c.Kind {
	case "rel":
		if id := refID(v); id != "" {
			return quote(id)
		}
		return ""
	case "reljson":
		list, ok := v.([]any)
		if !ok {
			return ""
		}
		ids := make([]any, 0, len(list))
		for _, item := range list {
			ids = append(ids, refID(item))
		}
		b, _ := json.Marshal(ids)
		return quote(string(b))
	case "json":
		b, _ := json.Marshal(v)
		return quote(string(b))
	}
	switch x := v.(type) {
	case string:
		return quote(x)
	case json.Number:
		return quote(x.String())
	case bool:
		return fmt.Sprintf("%t", x)
	}
	b, _ := json.Marshal(v)
	return quote(string(b))
}

// refID reduces a relationship value (an id or an expanded document) to the document id.
func refID(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case map[string]any:
		id, _ := x["$id"].(string)
		return id
	}
	return ""
}
