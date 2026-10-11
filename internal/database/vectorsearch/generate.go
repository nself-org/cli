package vectorsearch

// generate.go is the whole `db generate vector-search` flow behind injectable
// seams: inspect the column, check the table is tracked on disk, write the
// migration, and with Track send one targeted tracking call and mirror the
// entry to disk when the on-disk metadata shows no drift. Nothing is written
// before the inspection passes.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

// Deps are the seams Run uses; the command wires the real ones.
type Deps struct {
	// Exec and PG read the column type from the Postgres container.
	Exec Exec
	PG   PG
	// TableTracked reports on-disk tracking: present is false when the project
	// has no on-disk metadata at all.
	TableTracked func(projectDir, schema, table string) (present, tracked bool, err error)
	// Drift lists the on-disk metadata keys that differ from live Hasura.
	Drift func(ctx context.Context, projectDir string) ([]string, error)
	// Track sends one bulk call: pg_track_function plus a permission per role.
	Track func(ctx context.Context, schema, name, table string, roles []string) error
	// Now stamps a new migration file name.
	Now func() time.Time
}

// Options are the caller's choices.
type Options struct {
	Spec         Spec
	ProjectDir   string
	MigrationDir string // default <ProjectDir>/migrations
	Track        bool
}

// Result is what Run did; it is also the --json envelope data.
type Result struct {
	Function     string   `json:"function"`
	Table        string   `json:"table"`
	Column       string   `json:"column"`
	Dim          int      `json:"dim"`
	Distance     string   `json:"distance"`
	Roles        []string `json:"roles"`
	Migration    string   `json:"migration"`
	MigrationNew bool     `json:"migration_written"`
	Tracked      bool     `json:"tracked"`
	DiskMetadata string   `json:"disk_metadata"` // updated | unchanged | skipped | none
	Note         string   `json:"note"`
}

// Run executes the flow. Errors carry E513 or E514 where the contract says so.
func Run(ctx context.Context, d Deps, o Options) (Result, error) {
	s := o.Spec
	if err := s.Validate(); err != nil {
		return Result{}, err
	}
	dim, err := InspectColumn(ctx, d.Exec, d.PG, s)
	if err != nil {
		return Result{}, err
	}
	s.Dim = dim
	present, tracked, err := d.TableTracked(o.ProjectDir, s.Schema, s.Table)
	if err != nil {
		return Result{}, err
	}
	if !present {
		return Result{}, errs.Newf("E513", "no on-disk Hasura metadata in %s/hasura; run nself db hasura metadata export first", o.ProjectDir)
	}
	if !tracked {
		return Result{}, errs.Newf("E513", "table %s.%s is not tracked in the on-disk Hasura metadata", s.Schema, s.Table)
	}
	sqlText, err := Render(s)
	if err != nil {
		return Result{}, err
	}
	res := Result{Function: s.Schema + "." + s.FunctionName(), Table: s.Schema + "." + s.Table, Column: s.Column,
		Dim: dim, Distance: string(s.Distance), Roles: s.Roles, DiskMetadata: "none"}
	dir := o.MigrationDir
	if dir == "" {
		dir = filepath.Join(o.ProjectDir, "migrations")
	}
	if res.Migration, res.MigrationNew, err = writeMigration(dir, s, sqlText, d.Now()); err != nil {
		return res, err
	}
	if !o.Track {
		return res, nil
	}
	return track(ctx, d, o, s, res)
}

// track sends the live call, then mirrors it to disk only on a clean diff.
func track(ctx context.Context, d Deps, o Options, s Spec, res Result) (Result, error) {
	drift, err := d.Drift(ctx, o.ProjectDir)
	if err != nil {
		return res, fmt.Errorf("compare on-disk metadata with live Hasura: %w", err)
	}
	if err := d.Track(ctx, s.Schema, s.FunctionName(), s.Table, s.Roles); err != nil {
		return res, err
	}
	res.Tracked = true
	if len(drift) > 0 {
		res.DiskMetadata = "skipped"
		res.Note = "on-disk metadata differs from live Hasura (" + strings.Join(drift, ", ") +
			"); it was left unchanged. Run nself db hasura metadata export to refresh it."
		return res, nil
	}
	file, changed, err := MirrorFunction(o.ProjectDir, s.Schema, s.FunctionName(), s.Roles)
	switch {
	case errors.Is(err, ErrUnsupportedLayout):
		res.DiskMetadata = "skipped"
		res.Note = "on-disk metadata was left unchanged: " + err.Error()
	case err != nil:
		return res, err
	case changed:
		res.DiskMetadata = "updated"
		res.Note = "on-disk metadata updated: " + file
	default:
		res.DiskMetadata = "unchanged"
		res.Note = "on-disk metadata already lists the function"
	}
	return res, nil
}

// writeMigration writes migrations/<ts>_vector_search_<table>_<column>.sql. If
// the newest earlier file for the same table and column already holds exactly
// this SQL, it is reused, so `generate` followed by `generate --track` leaves
// one file. It returns the path and whether a file was written.
func writeMigration(dir string, s Spec, sqlText string, now time.Time) (string, bool, error) {
	suffix := "_vector_search_" + s.Table + "_" + s.Column + ".sql"
	existing, _ := filepath.Glob(filepath.Join(dir, "*"+suffix))
	sort.Strings(existing)
	if n := len(existing); n > 0 {
		if old, err := os.ReadFile(existing[n-1]); err == nil && string(old) == sqlText {
			return existing[n-1], false, nil
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, fmt.Errorf("create %s: %w", dir, err)
	}
	path := filepath.Join(dir, now.UTC().Format("20060102150405")+suffix)
	if err := os.WriteFile(path, []byte(sqlText), 0o644); err != nil {
		return "", false, fmt.Errorf("write %s: %w", path, err)
	}
	return path, true, nil
}
