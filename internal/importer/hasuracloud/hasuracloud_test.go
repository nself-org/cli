package hasuracloud

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/nself-org/cli/internal/importer"
)

func TestRegisteredWithEnvNameFlags(t *testing.T) {
	var found importer.Source
	for _, s := range importer.Sources() {
		if s.Name() == "hasura-cloud" {
			found = s
		}
	}
	if found == nil {
		t.Fatal("hasura-cloud not registered")
	}
	want := map[string]bool{"db-url-env": true, "hasura-url": false, "admin-secret-env": true}
	if len(found.Flags()) != len(want) {
		t.Fatalf("flags %+v", found.Flags())
	}
	for _, f := range found.Flags() {
		if env, ok := want[f.Name]; !ok || env != f.EnvVarName {
			t.Errorf("flag %+v", f)
		}
	}
	if got := pgInput(importer.Input{Values: map[string]string{"db-url-env": "SRC"}}); got.Values["from-env"] != "SRC" {
		t.Fatalf("pgInput %+v", got)
	}
}

type fakeRow struct {
	v   any
	err error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	switch d := dest[0].(type) {
	case *bool:
		*d = r.v.(bool)
	case *int64:
		*d = r.v.(int64)
	}
	return nil
}

type fakeDB map[string]any // SQL fragment -> value; a missing fragment is a failed lookup

func (f fakeDB) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	for frag, v := range f {
		if strings.Contains(sql, frag) {
			return fakeRow{v: v}
		}
	}
	return fakeRow{err: errors.New("unexpected query")}
}

func TestOwnDatabaseRefusesAuthAndStorageData(t *testing.T) {
	ctx := context.Background()
	clean := fakeDB{"table_schema = 'auth'": int64(0), "to_regclass('storage.files')": false}
	if err := ownDatabase(ctx, clean); err != nil {
		t.Fatalf("plain database refused: %v", err)
	}
	emptyStorage := fakeDB{"table_schema = 'auth'": int64(0), "to_regclass('storage.files')": true, "FROM storage.files": false}
	if err := ownDatabase(ctx, emptyStorage); err != nil {
		t.Fatalf("empty storage.files refused: %v", err)
	}
	cases := map[string]struct {
		db   fakeDB
		want string
	}{
		"custom auth schema": {fakeDB{"table_schema = 'auth'": int64(2)}, "auth schema"},
		"storage rows":       {fakeDB{"table_schema = 'auth'": int64(0), "to_regclass('storage.files')": true, "FROM storage.files": true}, "hasura-storage files"},
		"lookup failure":     {fakeDB{}, "lookup failed"},
	}
	for name, c := range cases {
		err := ownDatabase(ctx, c.db)
		if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), "E517") {
			t.Errorf("%s: %v", name, err)
		}
	}
}
