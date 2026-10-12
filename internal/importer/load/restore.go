package load

// Purpose: ordered restore through the Docker funnel.
// Inputs: verified bundle and target image. Outputs: tables, users, objects, metadata.
// Constraints: password only EnvPass; no restore before emptiness and storage checks.
import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer/verify"
	"github.com/nself-org/cli/internal/portable"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Options struct {
	SkipStorage bool
	// Progress, when set, records whether the target was written and the last finished step.
	Progress *Progress
}

// Progress tells the caller whether Apply changed the target. Wrote is set immediately before
// the first statement that can change it (the compat or pre-data one-shot), so any error with
// Wrote false left the target untouched.
type Progress struct {
	Wrote bool
	Step  string
}

func (p *Progress) begin() {
	if p != nil {
		p.Wrote = true
	}
}
func (p *Progress) done(step string) {
	if p != nil {
		p.Step = step
	}
}

var runOneShot = docker.RunOneShot

func Image(ctx context.Context, cfg *config.Config) (string, error) {
	image := compose.ResolveImage("postgres", compose.ResolvePostgresImage(cfg.Postgres))
	digests, err := docker.ImageRepoDigests(ctx, image)
	if err == nil && len(digests) > 0 {
		return digests[0], nil
	}
	ref, ok := compose.LockedRef("postgres")
	if strings.Contains(image, "pgvector/") {
		ref, ok = compose.LockedRef("pgvector")
	}
	if !ok || image != ref.String() {
		return "", errs.New("E521", "target image must be present locally or match the image lock")
	}
	_, err = docker.EnsureImage(ctx, docker.LockedImage{Name: ref.Name, Repository: ref.Repository, Version: ref.Version, IndexDigest: ref.IndexDigest, Mirror: ref.Mirror})
	if err != nil {
		return "", errs.New("E521", "target image unavailable")
	}
	return image, nil
}
func Apply(ctx context.Context, cfg *config.Config, r *portable.Reader, opts Options) ([]portable.Exemption, error) {
	db, err := Connect(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close(ctx) }()
	store, err := Store(cfg)
	if err != nil {
		return nil, err
	}
	m := r.Manifest()
	users, tables, err := preflight(ctx, cfg, db, r, store, opts)
	if err != nil {
		return nil, err
	}
	image, err := Image(ctx, cfg)
	if err != nil {
		return nil, err
	}
	work, err := os.MkdirTemp(filepath.Dir(r.Dir()), ".load-")
	if err != nil {
		return nil, errs.New("E521", "restore staging failed")
	}
	defer func() { _ = os.RemoveAll(work) }()
	if err = copyMember(r, "db/data.dump", filepath.Join(work, "data.dump")); err != nil {
		return nil, errs.New("E521", "restore member invalid")
	}
	spec := docker.RunSpec{Image: image, Network: cfg.ProjectName + "_network", User: strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid()), EnvPass: map[string]string{"PGPASSWORD": cfg.Postgres.Password}, Mounts: []docker.Mount{{Source: work, Destination: "/import", ReadOnly: true}}}
	// Skip CREATE SCHEMA for namespaces the started target already owns.
	spec.Args = []string{"pg_restore", "--list", "/import/data.dump"}
	toc, _, err := runOneShot(ctx, spec)
	if err != nil {
		return nil, errs.New("E521", "restore archive listing failed")
	}
	if err = validateArchive(toc, m); err != nil {
		return nil, errs.New("E521", "archive does not match the bundle: "+err.Error())
	}
	if err = TypeFunctionCollisions(ctx, db, toc, m.DB.Schemas); err != nil {
		return nil, err
	}
	// Every read-only check is done; compat.sql is the first write when the bundle has one.
	if verify.HasMember(r, "db/compat.sql") {
		if err = copyMember(r, "db/compat.sql", filepath.Join(work, "compat.sql")); err != nil {
			return nil, errs.New("E521", "compat member invalid")
		}
		opts.Progress.begin()
		spec.Args = append([]string{"psql", "-v", "ON_ERROR_STOP=1"}, connectionArgs(cfg)...)
		spec.Args = append(spec.Args, "-f", "/import/compat.sql")
		if _, _, err = runOneShot(ctx, spec); err != nil {
			return nil, errs.New("E521", "compat restore failed")
		}
		opts.Progress.done("compat")
	}
	// Read after compat.sql: it may create schemas that the dump must then not create again.
	filtered, err := filterSchemas(ctx, db, toc)
	if err != nil {
		return nil, err
	}
	if os.WriteFile(filepath.Join(work, "restore.list"), []byte(filtered), 0o600) != nil {
		return nil, errs.New("E521", "restore list write failed")
	}
	restore := func(section string) error {
		opts.Progress.begin()
		spec.Args = append([]string{"pg_restore", "--exit-on-error", "--single-transaction", "--no-owner", "--no-privileges", "--section=" + section, "--use-list=/import/restore.list"}, connectionArgs(cfg)...)
		spec.Args = append(spec.Args, "/import/data.dump")
		if _, _, e := runOneShot(ctx, spec); e != nil {
			return errs.New("E521", "database restore failed in "+section)
		}
		opts.Progress.done(section)
		return nil
	}
	if err = restore("pre-data"); err != nil {
		return nil, err
	}
	if err = restore("data"); err != nil {
		return nil, err
	}
	if err = ApplyUsers(ctx, db, users); err != nil {
		return nil, errs.New("E521", err.Error())
	}
	opts.Progress.done("users")
	exemptions, err := ApplyStorage(ctx, r, store, opts.SkipStorage)
	if err != nil {
		return nil, err
	}
	opts.Progress.done("storage")
	if err = restore("post-data"); err != nil {
		return nil, err
	}
	// Late check: see the preflight above; this plan is the one applied.
	plan, err := database.BuildAdditivePlan(ctx, cfg, tables)
	if err != nil {
		return nil, errs.New("E521", "additive metadata planning failed")
	}
	if err = database.ApplyAdditivePlan(ctx, cfg, plan); err != nil {
		return nil, errs.New("E521", "additive metadata apply failed")
	}
	opts.Progress.done("metadata")
	return exemptions, nil
}
func connectionArgs(cfg *config.Config) []string {
	return []string{"-h", "postgres", "-U", cfg.Postgres.User, "-d", cfg.Postgres.DB}
}
func copyMember(r *portable.Reader, name, path string) error {
	src, err := r.Open(name)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	dst, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	closeErr := dst.Close()
	if err == nil {
		err = closeErr
	}
	return err
}
func filterSchemas(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, toc string) (string, error) {
	lines := strings.Split(toc, "\n")
	for i, line := range lines {
		f := strings.Fields(line)
		if len(f) >= 7 && f[3] == "SCHEMA" && f[4] == "-" {
			name := f[5]
			if _, e := database.SanitizeIdentifier(name); e != nil {
				return "", errors.New("unsupported archive schema")
			}
			var exists bool
			if db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname=$1)`, name).Scan(&exists) != nil {
				return "", errors.New("target schema read failed")
			}
			if exists {
				lines[i] = ";" + line
			}
		}
	}
	return strings.Join(lines, "\n"), nil
}
