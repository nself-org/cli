package supabase

// Purpose: snapshot a Supabase project into a portable bundle.
// Inputs: source database URL, optional storage API URL and service key. Outputs: a v1 bundle
// (dump without policies, users, compat shims, translated permissions, objects) and the
// policies that could not be translated. Constraints: reuses the Postgres snapshot extractor;
// no credential in any error.
import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/importer/load"
	"github.com/nself-org/cli/internal/importer/postgres"
	"github.com/nself-org/cli/internal/portable"
)

// run carries what the snapshot hooks learn for the later steps.
type run struct {
	skipStorage  bool
	api          *api
	catalog      map[string]int64 // objects per bucket in storage.objects
	total        int64
	names        map[string]map[string]int64 // bucket -> key -> size (-1 when unknown)
	untranslated []importer.Policy
}

var projectRef = regexp.MustCompile(`^[a-z0-9]{1,40}$`)

// apiFor builds the storage client from --api-url or --project-ref; nil when neither is set.
func apiFor(in importer.Input) (*api, error) {
	if in.SkipStorage {
		return nil, nil
	}
	raw := in.Values["api-url"]
	if raw == "" && in.Values["project-ref"] != "" {
		if !projectRef.MatchString(in.Values["project-ref"]) {
			return nil, errs.New("E517", "storage api url invalid")
		}
		raw = "https://" + in.Values["project-ref"] + ".supabase.co"
	}
	if raw == "" {
		return nil, nil
	}
	key, err := in.Secret("service-key-env")
	if err != nil {
		return nil, err
	}
	return newAPI(raw, key)
}

func (Source) Extract(ctx context.Context, in importer.Input, stage string) (importer.Extracted, error) {
	client, err := apiFor(in)
	if err != nil {
		return importer.Extracted{}, err
	}
	db, err := dial(ctx, in)
	if err != nil {
		return importer.Extracted{}, err
	}
	d, err := detect(ctx, db, in)
	if err != nil {
		_ = db.Close(ctx)
		return importer.Extracted{}, err
	}
	// Fail before the dump when storage objects exist but no storage API was configured.
	if n, qerr := storageObjects(ctx, db); qerr != nil || (n > 0 && client == nil && !in.SkipStorage) {
		_ = db.Close(ctx)
		if qerr != nil {
			return importer.Extracted{}, qerr
		}
		return importer.Extracted{}, errs.New("E520", "source has storage objects; --api-url (or --project-ref) and --service-key-env are required")
	}
	r := &run{api: client, skipStorage: in.SkipStorage}
	ext, err := postgres.Snapshot(ctx, db, d, in.Target, stage, postgres.Options{Source: "supabase", InTx: r.inTx, Finish: r.finish})
	ext.UntranslatedPolicies = r.untranslated
	return ext, err
}

// inTx runs inside the snapshot: users, policies, shims and the storage catalog counts all
// describe the same instant as the dump.
func (r *run) inTx(ctx context.Context, tx pgx.Tx, w *portable.Writer, m *portable.Manifest, work string) error {
	if err := exportUsers(ctx, tx, w, m); err != nil {
		return err
	}
	if err := r.policies(ctx, tx, w, m); err != nil {
		return err
	}
	if err := r.compat(w, m, work); err != nil {
		return err
	}
	return r.catalogCounts(ctx, tx, m)
}

func (r *run) policies(ctx context.Context, tx pgx.Tx, w *portable.Writer, m *portable.Manifest) error {
	tables, hasRLS, err := readTables(ctx, tx, m)
	if err != nil {
		return err
	}
	res, err := Plan(tables)
	if err != nil {
		return err
	}
	foreignPolicies, foreignExemptions, err := readForeign(ctx, tx, m)
	if err != nil {
		return err
	}
	m.Exemptions = append(append(m.Exemptions, res.Exemptions...), foreignExemptions...)
	r.untranslated = append(res.Untranslated, foreignPolicies...)
	if res.Doc != nil {
		if _, err = w.WriteFile("hasura/metadata.json", bytes.NewReader(res.Doc)); err != nil {
			return err
		}
		m.Hasura.Tables, m.Hasura.Permissions = res.Tables, res.Permissions
	}
	if hasRLS {
		// Policies and the ENABLE ROW LEVEL SECURITY switch name Supabase roles and functions.
		_, err = w.WriteFile(load.RestoreSkipMember, strings.NewReader("POLICY\nROW SECURITY\n"))
	}
	return err
}

func (r *run) compat(w *portable.Writer, m *portable.Manifest, work string) error {
	f, err := os.Open(filepath.Join(work, "schema.sql"))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, 256<<20))
	if err != nil {
		return err
	}
	if err = CheckForeignTriggers(string(b)); err != nil {
		return err
	}
	c := BuildCompat(string(b))
	m.Exemptions = append(m.Exemptions, c.Exemptions...)
	if c.SQL == "" {
		return nil
	}
	m.Compat = append(m.Compat, c.Shims...)
	_, err = w.WriteFile("db/compat.sql", strings.NewReader(c.SQL))
	return err
}
