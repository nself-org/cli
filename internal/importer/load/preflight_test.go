package load

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/portable"
)

type fakeRow struct {
	val bool
	err error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*(dest[0].(*bool)) = r.val
	return nil
}

// fakeDB answers each QueryRow in order and records the SQL it saw.
type fakeDB struct {
	rows []fakeRow
	sqls []string
}

func (f *fakeDB) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	f.sqls = append(f.sqls, sql)
	r := f.rows[0]
	if len(f.rows) > 1 {
		f.rows = f.rows[1:]
	}
	return r
}

func codeOf(err error) string {
	var cli *errs.CLIError
	if errors.As(err, &cli) {
		return cli.Code
	}
	return ""
}

func TestAuthPreflight(t *testing.T) {
	ctx := context.Background()
	if err := AuthPreflight(ctx, &fakeDB{rows: []fakeRow{{val: true}}}, 0); err != nil {
		t.Fatalf("a bundle without users must not look at auth: %v", err)
	}
	if err := AuthPreflight(ctx, &fakeDB{rows: []fakeRow{{val: true}, {val: false}}}, 3); err != nil {
		t.Fatalf("empty auth.users must pass: %v", err)
	}
	err := AuthPreflight(ctx, &fakeDB{rows: []fakeRow{{val: true}, {val: true}}}, 3)
	if codeOf(err) != "E519" || !strings.Contains(err.Error(), "target auth.users is not empty") {
		t.Fatalf("want E519 for a used auth.users: %v", err)
	}
	if codeOf(AuthPreflight(ctx, &fakeDB{rows: []fakeRow{{val: false}}}, 3)) != "E521" {
		t.Fatal("a target without auth.users must be refused before any write")
	}
}

func TestTryLock(t *testing.T) {
	ctx := context.Background()
	if err := TryLock(ctx, &fakeDB{rows: []fakeRow{{val: true}}}); err != nil {
		t.Fatal(err)
	}
	err := TryLock(ctx, &fakeDB{rows: []fakeRow{{val: false}}})
	if codeOf(err) != "E519" || !strings.Contains(err.Error(), "another import is running") {
		t.Fatalf("want E519 when the lock is held: %v", err)
	}
	if codeOf(TryLock(ctx, &fakeDB{rows: []fakeRow{{err: errors.New("boom")}}})) != "E521" {
		t.Fatal("lock query failure must be E521")
	}
}

func TestTypeFunctionCollisions(t *testing.T) {
	toc := `; header
10; 1255 16400 FUNCTION public uuidv7(timestamp with time zone) owner
11; 1247 16401 TYPE other mood owner
12; 1259 16402 TABLE public notes owner
`
	ctx := context.Background()
	hit := &fakeDB{rows: []fakeRow{{val: true}}}
	err := TypeFunctionCollisions(ctx, hit, toc, []string{"public"})
	if codeOf(err) != "E519" || !strings.Contains(err.Error(), "function public.uuidv7") {
		t.Fatalf("want E519 naming the function: %v", err)
	}
	// Only declared schemas are looked at: "other" is not declared, so the type is not queried.
	clean := &fakeDB{rows: []fakeRow{{val: false}}}
	if err = TypeFunctionCollisions(ctx, clean, toc, []string{"public"}); err != nil {
		t.Fatal(err)
	}
	if len(clean.sqls) != 1 {
		t.Fatalf("queried %d objects, want 1 (the function in a declared schema)", len(clean.sqls))
	}
	typ := &fakeDB{rows: []fakeRow{{val: true}}}
	err = TypeFunctionCollisions(ctx, typ, toc, []string{"other"})
	if codeOf(err) != "E519" || !strings.Contains(err.Error(), "type other.mood") {
		t.Fatalf("want E519 naming the type: %v", err)
	}
}

func TestValidateArchiveRefusals(t *testing.T) {
	m := portable.Manifest{DB: portable.DB{Schemas: []string{"public"}, Tables: []portable.Table{{Schema: "public", Name: "notes"}}}}
	ok := "1; 1259 1 TABLE public notes o\n2; 0 1 TABLE DATA public notes o\n"
	if err := validateArchive(ok, m); err != nil {
		t.Fatalf("declared table refused: %v", err)
	}
	for name, toc := range map[string]string{
		"partition attach":  ok + "3; 0 0 TABLE ATTACH public events_2024 o\n",
		"undeclared child":  ok + "3; 1259 2 TABLE public events_2024 o\n",
		"short table line":  ok + "3; 1259 2 TABLE public\n",
		"too short a line":  ok + "garbage\n",
		"short data record": ok + "3; 0 1 TABLE DATA public\n",
	} {
		err := validateArchive(toc, m)
		if err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
	err := validateArchive(ok+"3; 0 0 TABLE ATTACH public events_2024 o\n", m)
	if !strings.Contains(err.Error(), "partitioned tables are not supported") {
		t.Fatalf("attach refusal must say why: %v", err)
	}
}

func TestProgressNilSafeAndRecords(t *testing.T) {
	var nilp *Progress
	nilp.begin()
	nilp.done("x")
	p := &Progress{}
	if p.Wrote {
		t.Fatal("fresh progress must report no write")
	}
	p.begin()
	p.done("pre-data")
	if !p.Wrote || p.Step != "pre-data" {
		t.Fatalf("%+v", p)
	}
}

func TestSkipPlatformOwned(t *testing.T) {
	toc := "; header\n349; 1255 16569 FUNCTION public uuidv7() postgres\n243; 1259 16846 TABLE public notes postgres\n"
	ctx := context.Background()
	have := &fakeDB{rows: []fakeRow{{val: true}}}
	got, err := SkipPlatformOwned(ctx, have, toc, "nself")
	if err != nil || !strings.Contains(got, ";349; 1255 16569 FUNCTION public uuidv7() postgres") || !strings.Contains(got, "\n243; 1259 16846 TABLE public notes postgres") {
		t.Fatalf("an owned platform function must be commented out, nothing else: %q %v", got, err)
	}
	if err = TypeFunctionCollisions(ctx, &fakeDB{rows: []fakeRow{{val: true}}}, got, []string{"public"}); err != nil {
		t.Fatalf("the commented line must not collide: %v", err)
	}
	missing := &fakeDB{rows: []fakeRow{{val: false}}}
	if got, err = SkipPlatformOwned(ctx, missing, toc, "nself"); err != nil || got != toc {
		t.Fatalf("a target without the function must keep the line: %q %v", got, err)
	}
	foreign := &fakeDB{rows: []fakeRow{{val: true}}}
	if got, err = SkipPlatformOwned(ctx, foreign, toc, "postgres"); err != nil || got != toc || len(foreign.sqls) != 0 {
		t.Fatalf("a bundle from another source must be left alone: %q %v", got, err)
	}
	if _, err = SkipPlatformOwned(ctx, &fakeDB{rows: []fakeRow{{err: errors.New("boom")}}}, toc, "nself"); codeOf(err) != "E521" {
		t.Fatalf("lookup failure must be E521: %v", err)
	}
}
