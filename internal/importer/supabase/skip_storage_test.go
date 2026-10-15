package supabase

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/portable"
)

// catalogTx adapts the in-memory read-only catalog connection to the snapshot contract.
// Inputs: fixture connection. Outputs: query rows. No transaction writes are supported.
type catalogTx struct {
	pgx.Tx
	db *pgx.Conn
}

func (tx catalogTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return tx.db.Query(ctx, sql, args...)
}

func (tx catalogTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return tx.db.QueryRow(ctx, sql, args...)
}

func TestSkipStorageExemptsSnapshotWithoutDownloading(t *testing.T) {
	for _, bucket := range []string{"user_uploads", "UserUploads", "avatars"} {
		t.Run(bucket, func(t *testing.T) {
			objects := []portable.Object{{Bucket: bucket, Key: "a.png"}, {Bucket: bucket, Key: "dir/b.png"}}
			db := detectionDB(t, true, objects)
			var seen []string
			srv := storageServer(t, &seen)
			a, err := newAPI(srv.URL, fixtureKey)
			if err != nil {
				t.Fatal(err)
			}
			r := &run{api: a, skipStorage: true}
			m := portable.Manifest{}
			if err := r.catalogCounts(context.Background(), catalogTx{db: db}, &m); err != nil {
				t.Fatal(err)
			}
			if err := r.finish(context.Background(), nil, &m); err != nil {
				t.Fatal(err)
			}
			want := []portable.Exemption{
				{Kind: "object", ID: bucket + "/a.png", Reason: "--skip-storage"},
				{Kind: "object", ID: bucket + "/dir/b.png", Reason: "--skip-storage"},
			}
			if !reflect.DeepEqual(m.Exemptions, want) {
				t.Fatalf("exemptions = %+v, want %+v", m.Exemptions, want)
			}
			if len(seen) != 0 || len(m.Storage.Objects) != 0 || len(m.Storage.Buckets) != 0 || len(m.SourceCounts) != 0 {
				t.Fatalf("skipped storage produced requests, members or counts: requests=%v manifest=%+v", seen, m)
			}
		})
	}
}

func TestSkipStorageNeedsNoAPICredentials(t *testing.T) {
	in := importer.Input{SkipStorage: true, Values: map[string]string{"api-url": "https://unused.example"}, Env: func(string) string {
		t.Fatal("skipped storage must not read a service key")
		return ""
	}}
	if a, err := apiFor(in); err != nil || a != nil {
		t.Fatalf("skipped storage constructed API: %v %v", a, err)
	}
}

func TestSkipStorageDetectionNotes(t *testing.T) {
	db := detectionDB(t, true, []portable.Object{{Bucket: "user_uploads", Key: "a.png"}})
	d, err := detect(context.Background(), db, importer.Input{SkipStorage: true})
	if err != nil {
		t.Fatal(err)
	}
	if notes := strings.Join(d.Notes, " "); !strings.Contains(notes, "--skip-storage") || strings.Contains(notes, "API is required") {
		t.Fatalf("skip detection notes: %v", d.Notes)
	}
}

func TestSkipStorageEmptyCatalog(t *testing.T) {
	for _, has := range []bool{true, false} {
		db := detectionDB(t, has, nil)
		r := &run{skipStorage: true}
		m := portable.Manifest{}
		if err := r.catalogCounts(context.Background(), catalogTx{db: db}, &m); err != nil {
			t.Fatal(err)
		}
		if err := r.finish(context.Background(), nil, &m); err != nil || len(m.Exemptions) != 0 {
			t.Fatalf("empty catalog: %+v %v", m, err)
		}
	}
}
