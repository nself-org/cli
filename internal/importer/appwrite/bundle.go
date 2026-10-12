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
		counts = append(counts, portable.SourceCount{Kind: "table", ID: id, Count: got.Total, Method: "documents list total"})
		if len(col.Permissions) > 0 || got.DocPerms > 0 {
			exemptions = append(exemptions, portable.Exemption{Kind: "metadata", ID: id, Reason: fmt.Sprintf("Appwrite permissions not mapped to Hasura permissions (collection: %d, documents with $permissions: %d)", len(col.Permissions), got.DocPerms)})
		}
		for _, note := range col.notes {
			exemptions = append(exemptions, portable.Exemption{Kind: "metadata", ID: id, Reason: note})
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
	jsonl, auth, total, err := c.exportUsers(ctx)
	if err != nil {
		return err
	}
	if auth.Users > 0 {
		if _, err = w.WriteFile("auth/users.jsonl", bytes.NewReader(jsonl)); err != nil {
			return errs.New("E520", "auth snapshot extraction failed")
		}
	}
	m.Auth = auth
	counts = append(counts, portable.SourceCount{Kind: "users", Count: total, Method: "users list total"})
	st, err := c.exportStorage(ctx, w)
	if err != nil {
		return err
	}
	m.Storage = st.Storage
	meta, rels, err := hasuraMetadata(cols)
	if err != nil {
		return errs.New("E520", "bundle manifest write failed")
	}
	if _, err = w.WriteFile("hasura/metadata.json", io.Reader(bytes.NewReader(meta))); err != nil {
		return errs.New("E520", "bundle manifest write failed")
	}
	m.Hasura.Tables, m.Hasura.Relationships = len(cols), rels
	m.SourceCounts = append(counts, st.Counts...)
	m.Exemptions = append(m.Exemptions, exemptions...)
	if _, err = w.Finish(m); err != nil {
		return errs.New("E520", "bundle manifest write failed")
	}
	return os.RemoveAll(base)
}
