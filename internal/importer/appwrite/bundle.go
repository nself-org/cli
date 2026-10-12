package appwrite

// Purpose: assemble the portable bundle: scratch-built db members plus users, storage and Hasura
// metadata. Inputs: mapped collections. Outputs: a finished bundle at the stage path.
// Constraints: scratch.Build owns the db section and finishes its own bundle, so its members are
// copied into the real bundle; the key never reaches the bundle; permissions go to exemptions.
import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/importer/scratch"
	"github.com/nself-org/cli/internal/portable"
)

func (c *client) assemble(ctx context.Context, in importer.Input, cols []*collection, work, stage string) error {
	copies, counts, exemptions := []scratch.CopyFile{}, []portable.SourceCount{}, []portable.Exemption{}
	for i, col := range cols {
		names := make([]string, len(col.Columns))
		for j, cl := range col.Columns {
			names[j] = cl.Name
		}
		got, err := c.copyDocuments(ctx, col.DB, col, filepath.Join(work, fmt.Sprintf("docs-%d.csv", i)))
		if err != nil {
			return err
		}
		id := col.Schema + "." + col.Table
		copies = append(copies, scratch.CopyFile{Schema: col.Schema, Table: col.Table, Path: got.Path, Columns: names})
		counts, exemptions = addCount(counts, exemptions, "table", id, "documents", got.Total, got.Rows)
		// Exemption ids carry a #suffix so they never silence a tracking or relationship check.
		if len(col.Permissions) > 0 || got.DocPerms > 0 {
			exemptions = append(exemptions, portable.Exemption{Kind: "metadata", ID: id + "#permissions", Reason: fmt.Sprintf("Appwrite permissions not mapped to Hasura permissions (collection: %d, documents with $permissions: %d)", len(col.Permissions), got.DocPerms)})
		}
		keys := make([]string, 0, len(got.Missing))
		for k := range got.Missing {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			exemptions = append(exemptions, portable.Exemption{Kind: "metadata", ID: id + "#missing:" + k, Reason: fmt.Sprintf("%d documents had no value for attribute %s in the API response; their column is NULL (some Appwrite versions do not return relationship values unless asked)", got.Missing[k], k)})
		}
		for _, n := range col.notes {
			exemptions = append(exemptions, portable.Exemption{Kind: "metadata", ID: id + "#" + n.ID, Reason: n.Reason})
		}
	}
	sql, err := ddl(cols)
	if err != nil {
		return err
	}
	base := filepath.Join(work, "base")
	m, err := build(ctx, in.Target, sql, copies, base)
	if err != nil {
		return err
	}
	reader, err := portable.Open(base)
	if err != nil {
		return errs.New("E520", "bundle creation failed")
	}
	w, err := portable.NewWriter(stage)
	if err != nil {
		return errs.New("E520", "bundle creation failed")
	}
	for _, name := range []string{"db/data.dump", "db/schema.sql"} {
		f, e := reader.Open(name)
		if e != nil {
			return errs.New("E520", "dump member write failed")
		}
		_, e = w.WriteFile(name, f)
		_ = f.Close()
		if e != nil {
			return errs.New("E520", "dump member write failed")
		}
	}
	m.Producer.Source, m.Files = "appwrite", nil
	us, err := c.exportUsers(ctx)
	if err != nil {
		return err
	}
	if us.Auth.Users > 0 {
		if _, err = w.WriteFile("auth/users.jsonl", bytes.NewReader(us.JSONL)); err != nil {
			return errs.New("E520", "auth snapshot extraction failed")
		}
	}
	m.Auth = us.Auth
	counts, exemptions = addCount(counts, exemptions, "users", "", "users", us.Total, int64(us.Auth.Users))
	st, err := c.exportStorage(ctx, w)
	if err != nil {
		return err
	}
	m.Storage = st.Storage
	exemptions = append(exemptions, st.Notes...)
	if us.MFA > 0 {
		exemptions = append(exemptions, portable.Exemption{Kind: "metadata", ID: "users#mfa", Reason: fmt.Sprintf("%d users have MFA enabled in Appwrite; MFA does not transfer and they must enrol again", us.MFA)})
	}
	var sum, read int64
	capped := false
	for i, t := range st.Totals {
		sum += t.Total
		read += int64(st.Storage.Buckets[i].Objects)
		capped = capped || t.Total >= totalCap
		if t.Total > 0 {
			counts, exemptions = addCount(counts, exemptions, "objects", t.Bucket, "files", t.Total, int64(st.Storage.Buckets[i].Objects))
		}
	}
	if len(st.Totals) > 0 {
		// The all-buckets count is only "not independent" when some bucket hit the list cap.
		if capped {
			counts, exemptions = addCount(counts, exemptions, "objects", "", "files", sum, read)
		} else {
			counts = append(counts, portable.SourceCount{Kind: "objects", Count: sum, Method: "files list total"})
		}
	}
	meta, rels, err := hasuraMetadata(cols)
	if err != nil {
		return errs.New("E520", "bundle manifest write failed")
	}
	if _, err = w.WriteFile("hasura/metadata.json", io.Reader(bytes.NewReader(meta))); err != nil {
		return errs.New("E520", "bundle manifest write failed")
	}
	m.Hasura.Tables, m.Hasura.Relationships = len(cols), rels
	m.SourceCounts = counts
	m.Exemptions = append(m.Exemptions, exemptions...)
	if _, err = w.Finish(m); err != nil {
		return errs.New("E520", "bundle manifest write failed")
	}
	return os.RemoveAll(base)
}

// totalCap is the most Appwrite reports in a list `total` (APP_LIMIT_COUNT). At the cap the total
// is not independent of the rows read, so the count falls back to the rows and the report says so.
const totalCap = 5000

// addCount appends the source count for one list. total is the first page's `total`, read the
// number of items actually read; a lost page still shows as read < total.
func addCount(counts []portable.SourceCount, ex []portable.Exemption, kind, id, what string, total, read int64) ([]portable.SourceCount, []portable.Exemption) {
	method := what + " list total"
	if total >= totalCap && read >= total {
		total = read
		method = what + " read (Appwrite caps list totals at 5000)"
		ex = append(ex, portable.Exemption{Kind: "metadata", ID: kind + ":" + id + "#count", Reason: "the " + what + " count is not independent: Appwrite caps list totals at 5000, so the count of items read is used"})
	}
	return append(counts, portable.SourceCount{Kind: kind, ID: id, Count: total, Method: method}), ex
}
